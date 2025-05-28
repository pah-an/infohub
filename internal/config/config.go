package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pah-an/infohub/internal/auth"
	"github.com/pah-an/infohub/internal/cache"
	"github.com/pah-an/infohub/internal/domain"
	"github.com/pah-an/infohub/internal/logger"
)

// Config представляет конфигурацию приложения
type Config struct {
	Server       ServerConfig     `yaml:"server"`
	Sources      []domain.Source  `yaml:"sources"`
	Interval     time.Duration    `yaml:"interval"`
	Cache        CacheConfig      `yaml:"cache"`
	Redis        cache.Config     `yaml:"redis"`
	Auth         auth.Config      `yaml:"auth"`
	RateLimiting RateLimitConfig  `yaml:"rate_limiting"`
	Logging      logger.Config    `yaml:"logging"`
	Monitoring   MonitoringConfig `yaml:"monitoring"`
	Health       HealthConfig     `yaml:"health"`
	CORS         CORSConfig       `yaml:"cors"`
	Security     SecurityConfig   `yaml:"security"`
	Profiling    ProfilingConfig  `yaml:"profiling"`
}

// ServerConfig содержит настройки HTTP сервера
type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         string        `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// CacheConfig содержит настройки кэширования
type CacheConfig struct {
	FilePath string `yaml:"file_path"`
}

// RateLimitConfig содержит настройки rate limiting
type RateLimitConfig struct {
	Enabled           bool    `yaml:"enabled"`
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	BurstSize         int     `yaml:"burst_size"`
}

// MonitoringConfig содержит настройки мониторинга
type MonitoringConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Namespace string `yaml:"namespace"`
	Subsystem string `yaml:"subsystem"`
}

// HealthConfig содержит настройки health checks
type HealthConfig struct {
	Timeout time.Duration `yaml:"timeout"`
	Checks  HealthChecks  `yaml:"checks"`
}

// HealthChecks содержит флаги для различных проверок
type HealthChecks struct {
	Redis           bool `yaml:"redis"`
	ExternalSources bool `yaml:"external_sources"`
	DiskSpace       bool `yaml:"disk_space"`
	Memory          bool `yaml:"memory"`
}

// CORSConfig содержит настройки CORS
type CORSConfig struct {
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
}

// SecurityConfig содержит настройки безопасности
type SecurityConfig struct {
	EnableSecurityHeaders bool   `yaml:"enable_security_headers"`
	ContentSecurityPolicy string `yaml:"content_security_policy"`
}

// ProfilingConfig содержит настройки профилирования
type ProfilingConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    string `yaml:"port"`
}

// LoadConfig загружает конфигурацию из YAML файла
func LoadConfig(path string) (*Config, error) {
	// Пытаемся загрузить из переменной окружения
	if configPath := os.Getenv("CONFIG_PATH"); configPath != "" {
		path = configPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Устанавливаем значения по умолчанию
	setDefaults(&config)

	return &config, nil
}

// setDefaults устанавливает значения по умолчанию
func setDefaults(config *Config) {
	// Server defaults
	if config.Server.Port == "" {
		config.Server.Port = "8080"
	}
	if config.Server.Host == "" {
		config.Server.Host = "localhost"
	}
	if config.Server.ReadTimeout == 0 {
		config.Server.ReadTimeout = 15 * time.Second
	}
	if config.Server.WriteTimeout == 0 {
		config.Server.WriteTimeout = 15 * time.Second
	}
	if config.Server.IdleTimeout == 0 {
		config.Server.IdleTimeout = 60 * time.Second
	}

	// General defaults
	if config.Interval == 0 {
		config.Interval = 30 * time.Second
	}

	// Cache defaults
	if config.Cache.FilePath == "" {
		config.Cache.FilePath = "news_cache.json"
	}

	// Redis defaults
	if config.Redis.Address == "" {
		config.Redis.Address = "localhost:6379"
	}
	if config.Redis.TTL == 0 {
		config.Redis.TTL = 5 * time.Minute
	}
	if config.Redis.Prefix == "" {
		config.Redis.Prefix = "infohub:"
	}

	// Auth defaults
	if config.Auth.JWTTTL == 0 {
		config.Auth.JWTTTL = 24 * time.Hour
	}
	if config.Auth.PublicPaths == nil {
		config.Auth.PublicPaths = []string{
			"/api/v1/healthz",
			"/swagger/",
			"/docs/",
			"/metrics",
		}
	}

	// Rate limiting defaults
	if config.RateLimiting.RequestsPerSecond == 0 {
		config.RateLimiting.RequestsPerSecond = 10
	}
	if config.RateLimiting.BurstSize == 0 {
		config.RateLimiting.BurstSize = 20
	}

	// Logging defaults
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}
	if config.Logging.Format == "" {
		config.Logging.Format = "text"
	}
	if config.Logging.Output == "" {
		config.Logging.Output = "stdout"
	}

	// Monitoring defaults
	if config.Monitoring.Namespace == "" {
		config.Monitoring.Namespace = "infohub"
	}
	if config.Monitoring.Subsystem == "" {
		config.Monitoring.Subsystem = "api"
	}

	// Health defaults
	if config.Health.Timeout == 0 {
		config.Health.Timeout = 5 * time.Second
	}

	// CORS defaults
	if config.CORS.AllowedOrigins == nil {
		config.CORS.AllowedOrigins = []string{"*"}
	}
	if config.CORS.AllowedMethods == nil {
		config.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	}
	if config.CORS.AllowedHeaders == nil {
		config.CORS.AllowedHeaders = []string{
			"Content-Type",
			"Authorization",
			"X-API-Key",
			"X-Request-ID",
		}
	}

	// Security defaults
	if config.Security.ContentSecurityPolicy == "" {
		config.Security.ContentSecurityPolicy = "default-src 'self'"
	}

	// Profiling defaults
	if config.Profiling.Host == "" {
		config.Profiling.Host = "localhost"
	}
	if config.Profiling.Port == "" {
		config.Profiling.Port = "6060"
	}
}

// GetLogLevel возвращает уровень логирования из переменной окружения или конфигурации
func (c *Config) GetLogLevel() string {
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		return level
	}
	return c.Logging.Level
}

// GetRedisAddress возвращает адрес Redis из переменной окружения или конфигурации
func (c *Config) GetRedisAddress() string {
	if addr := os.Getenv("REDIS_ADDRESS"); addr != "" {
		return addr
	}
	return c.Redis.Address
}
