package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	_ "github.com/pah-an/infohub/docs"
	"github.com/pah-an/infohub/internal/aggregator"
	"github.com/pah-an/infohub/internal/auth"
	"github.com/pah-an/infohub/internal/cache"
	"github.com/pah-an/infohub/internal/collector"
	"github.com/pah-an/infohub/internal/config"
	"github.com/pah-an/infohub/internal/domain"
	"github.com/pah-an/infohub/internal/health"
	"github.com/pah-an/infohub/internal/logger"
	"github.com/pah-an/infohub/internal/metrics"
	"github.com/pah-an/infohub/internal/server"
	"github.com/pah-an/infohub/internal/storage"
)

// Package main InfoHub API
//
// Сервис агрегации новостей из различных источников
//
// Schemes: http, https
// Host: localhost:8080
// BasePath: /api/v1
// Version: 1.0.0
// Contact: InfoHub Support<pah-an@yandex.ru>
//
// Security:
// - ApiKeyAuth: []
// - BearerAuth: []
//
// SecurityDefinitions:
// ApiKeyAuth:
//
//	type: apiKey
//	in: header
//	name: X-API-Key
//
// BearerAuth:
//
//	type: http
//	scheme: bearer
//	bearerFormat: JWT
//
// swagger:meta

var (
	version   = "1.0.0"
	gitCommit = "dev"
	buildTime = "unknown"
)

func main() {
	fmt.Printf("Starting InfoHub API v%s (commit: %s, built: %s)\n",
		version, gitCommit, buildTime)
	fmt.Printf("Go version: %s\n", runtime.Version())

	// Загружаем конфигурацию
	cfg, err := config.LoadConfig("configs/config.yaml")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Создаем логгер
	cfg.Logging.Level = cfg.GetLogLevel()
	appLogger := logger.New(cfg.Logging)

	appLogger.WithFields(map[string]interface{}{
		"version":    version,
		"git_commit": gitCommit,
		"build_time": buildTime,
		"go_version": runtime.Version(),
		"sources":    len(cfg.Sources),
		"interval":   cfg.Interval,
	}).Info("Starting InfoHub API")

	// Создаем контекст для грейсфул завершения
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Инициализируем метрики
	var appMetrics *metrics.Metrics
	if cfg.Monitoring.Enabled {
		appMetrics = metrics.New(cfg.Monitoring.Namespace, cfg.Monitoring.Subsystem)
		if err = appMetrics.Register(); err != nil {
			appLogger.WithError(err).Warn("Failed to register metrics, continuing without metrics")
			appMetrics = nil
		} else {
			appMetrics.SetApplicationInfo(version, runtime.Version(), gitCommit)
			appLogger.Info("Metrics system initialized")
		}
	}

	// Инициализируем кэш
	var cacheSystem cache.Cache
	if cfg.Redis.Enabled {
		cfg.Redis.Address = cfg.GetRedisAddress()
		redisCache, err := cache.NewRedisCache(cfg.Redis)
		if err != nil {
			appLogger.WithError(err).Warn("Failed to connect to Redis, using memory cache")
			cacheSystem = cache.NewMemoryCache(cfg.Redis.TTL, 10*time.Minute)
		} else {
			cacheSystem = redisCache
			appLogger.WithField("address", cfg.Redis.Address).Info("Connected to Redis cache")
		}
	} else {
		cacheSystem = cache.NewMemoryCache(5*time.Minute, 10*time.Minute)
		appLogger.Info("Using in-memory cache")
	}

	// Создаем health check manager и регистрируем
	healthManager := health.NewManager(cfg.Health.Timeout)

	if cfg.Health.Checks.Redis && cfg.Redis.Enabled {
		if redisCache, ok := cacheSystem.(*cache.RedisCache); ok {
			healthManager.RegisterCheck("redis", health.RedisCheck(redisCache.Ping))
		}
	}

	if cfg.Health.Checks.DiskSpace {
		healthManager.RegisterCheck("disk_space", health.DiskSpaceCheck("/", 90.0))
	}

	if cfg.Health.Checks.Memory {
		healthManager.RegisterCheck("memory", health.MemoryCheck(90.0))
	}

	// Создаем менеджер аутентификации
	var authManager *auth.Manager
	if cfg.Auth.Enabled {
		authManager, err = auth.NewManager(cfg.Auth)
		if err != nil {
			appLogger.WithError(err).Fatal("Failed to initialize authentication")
		}
		appLogger.Info("Authentication system enabled")
	} else {
		authManager, _ = auth.NewManager(auth.Config{Enabled: false})
		appLogger.Info("Authentication disabled (development mode)")
	}

	newsChannel := make(chan domain.NewsList, 100)
	errorChannel := make(chan error, 100)

	// Создаем хранилище с кэшированием
	fileStorage := storage.NewFileCache(cfg.Cache.FilePath)
	cachedStorage := cache.NewCachedNewsRepository(cacheSystem, fileStorage, 5*time.Minute)

	// Создаем агрегатор и загружаем кэшированные новости при старте
	agg := aggregator.New(cachedStorage)
	if err = agg.LoadFromRepository(); err != nil {
		appLogger.WithError(err).Warn("Failed to load cached news")
	} else {
		appLogger.Info("Loaded cached news from storage")
	}

	// Создаем коллектор и регистрируем health checks
	coll := collector.New(cfg.Sources, cfg.Interval)

	if cfg.Health.Checks.ExternalSources {
		for _, source := range cfg.Sources {
			sourceName := source.Name
			sourceURL := source.URL
			healthManager.RegisterCheck(
				fmt.Sprintf("source_%s", sourceName),
				health.ExternalServiceCheck(sourceName, sourceURL, 10*time.Second),
			)
		}
	}

	srv := server.NewInfoHubServer(server.Config{
		Host:          cfg.Server.Host,
		Port:          cfg.Server.Port,
		ReadTimeout:   cfg.Server.ReadTimeout,
		WriteTimeout:  cfg.Server.WriteTimeout,
		IdleTimeout:   cfg.Server.IdleTimeout,
		NewsProvider:  agg,
		Logger:        appLogger,
		Metrics:       appMetrics,
		AuthManager:   authManager,
		HealthManager: healthManager,
		RateLimiting:  cfg.RateLimiting,
		CORS:          cfg.CORS,
		Security:      cfg.Security,
	})

	// Запускаем профилирование, если включено
	if cfg.Profiling.Enabled {
		go func() {
			pprofAddr := fmt.Sprintf("%s:%s", cfg.Profiling.Host, cfg.Profiling.Port)
			appLogger.WithField("address", pprofAddr).Info("Starting pprof server")
			if err = http.ListenAndServe(pprofAddr, nil); err != nil {
				appLogger.WithError(err).Error("Pprof server failed")
			}
		}()
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		appLogger.WithComponent("aggregator").Info("Starting news aggregator")
		agg.Start(ctx, newsChannel, errorChannel)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		appLogger.WithComponent("collector").Info("Starting news collector")

		if appMetrics != nil {
			appMetrics.SetSourceStatus("active", len(cfg.Sources))
		}

		coll.Start(ctx, newsChannel, errorChannel)
	}()

	if appMetrics != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case err = <-errorChannel:
					appLogger.WithError(err).Error("Collector error")
					// Здесь можно добавить логику для определения источника и типа ошибки
					appMetrics.RecordNewsCollectionError("unknown", "unknown")
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err = srv.Start(); err != nil && !errors.Is(http.ErrServerClosed, err) {
			appLogger.WithError(err).Error("HTTP server error")
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	appLogger.Info("Received shutdown signal, starting graceful shutdown...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err = srv.Shutdown(shutdownCtx); err != nil {
		appLogger.WithError(err).Error("HTTP server shutdown error")
	}

	// Сохраняем последние новости в кэш
	appLogger.Info("Saving news to cache...")
	if err = agg.SaveToRepository(); err != nil {
		appLogger.WithError(err).Error("Error saving news cache")
	} else {
		appLogger.Info("News saved to cache successfully")
	}

	if err = cacheSystem.Close(); err != nil {
		appLogger.WithError(err).Error("Error closing cache")
	}

	// Закрываем каналы
	close(newsChannel)
	close(errorChannel)

	// Ожидаем завершения всех горутин
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		appLogger.Info("All goroutines finished gracefully")
	case <-time.After(15 * time.Second):
		appLogger.Warn("Timeout waiting for goroutines to finish")
	}

	appLogger.Info("InfoHub API stopped successfully")
}
