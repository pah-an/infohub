package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/pah-an/infohub/internal/auth"
	"github.com/pah-an/infohub/internal/config"
	"github.com/pah-an/infohub/internal/domain"
	"github.com/pah-an/infohub/internal/health"
	"github.com/pah-an/infohub/internal/logger"
	"github.com/pah-an/infohub/internal/metrics"
	"github.com/pah-an/infohub/internal/middleware"
	v1 "github.com/pah-an/infohub/internal/server/v1"
)

// Config содержит конфигурацию сервера
type Config struct {
	Host          string
	Port          string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	NewsProvider  NewsProvider
	Logger        *logger.Logger
	Metrics       *metrics.Metrics
	AuthManager   *auth.Manager
	HealthManager *health.Manager
	RateLimiting  config.RateLimitConfig
	CORS          config.CORSConfig
	Security      config.SecurityConfig
}

// InfoHubServer представляет HTTP сервер
type InfoHubServer struct {
	httpServer    *http.Server
	logger        *logger.Logger
	metrics       *metrics.Metrics
	healthManager *health.Manager
}

// NewsProvider определяет интерфейс для получения новостей
type NewsProvider interface {
	GetLatestNews(limit int) domain.NewsList
}

// NewInfoHubServer создает новый HTTP сервер
func NewInfoHubServer(cfg Config) *InfoHubServer {
	server := &InfoHubServer{
		logger:        cfg.Logger,
		metrics:       cfg.Metrics,
		healthManager: cfg.HealthManager,
	}

	router := mux.NewRouter()

	// Применяем базовые middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.Security)

	if cfg.Logger != nil {
		router.Use(middleware.Logging(cfg.Logger))
	}

	if cfg.Metrics != nil {
		router.Use(middleware.Metrics(cfg.Metrics))
	}

	router.Use(middleware.CORS(
		cfg.CORS.AllowedOrigins,
		cfg.CORS.AllowedMethods,
		cfg.CORS.AllowedHeaders,
	))

	if cfg.RateLimiting.Enabled {
		rateLimiter := middleware.NewRateLimiter(
			cfg.RateLimiting.RequestsPerSecond,
			cfg.RateLimiting.BurstSize,
		)
		router.Use(rateLimiter.Middleware)
	}

	router.Use(middleware.Recovery(cfg.Logger))
	router.Use(middleware.Timeout(30 * time.Second))

	v1Handlers := v1.NewHandlers(cfg.NewsProvider)

	// API v1 routes с аутентификацией
	apiV1 := router.PathPrefix("/api/v1").Subrouter()

	if cfg.AuthManager != nil {
		// Публичные endpoints (без аутентификации)
		apiV1.HandleFunc("/healthz", v1Handlers.GetHealth).Methods("GET")

		// Приватные endpoints (с аутентификацией)
		protectedV1 := apiV1.PathPrefix("").Subrouter()
		protectedV1.Use(middleware.Auth(cfg.AuthManager, cfg.Logger))
		protectedV1.HandleFunc("/news", v1Handlers.GetNews).Methods("GET")

		// Admin endpoints
		adminV1 := apiV1.PathPrefix("/admin").Subrouter()
		adminV1.Use(cfg.AuthManager.RequireAdmin())
		adminV1.HandleFunc("/stats", v1Handlers.GetAdminStats).Methods("GET")
		adminV1.HandleFunc("/sources", v1Handlers.GetAdminSources).Methods("GET")
		adminV1.HandleFunc("/cache/clear", v1Handlers.ClearAdminCache).Methods("POST")
	} else {
		// Без аутентификации (development mode)
		apiV1.HandleFunc("/news", v1Handlers.GetNews).Methods("GET")
		apiV1.HandleFunc("/healthz", v1Handlers.GetHealth).Methods("GET")
	}

	// Health checks (детальный endpoint)
	if cfg.HealthManager != nil {
		router.HandleFunc("/health", cfg.HealthManager.HTTPHandler()).Methods("GET")
		router.HandleFunc("/health/live", server.handleLiveness).Methods("GET")
		router.HandleFunc("/health/ready", server.handleReadiness).Methods("GET")
	}

	// Metrics endpoint
	if cfg.Metrics != nil {
		router.Handle("/metrics", metrics.Handler()).Methods("GET")
	}

	// Swagger документация
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
	router.HandleFunc("/docs/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusMovedPermanently)
	})

	// API информация
	router.HandleFunc("/api", server.handleAPIInfo).Methods("GET")
	router.HandleFunc("/api/", server.handleAPIInfo).Methods("GET")

	// Authentication endpoints
	if cfg.AuthManager != nil {
		authRouter := router.PathPrefix("/auth").Subrouter()
		authRouter.HandleFunc("/login", v1Handlers.PostLogin(cfg.AuthManager)).Methods("POST")
		authRouter.HandleFunc("/validate", v1Handlers.GetValidateToken(cfg.AuthManager)).Methods("GET")
	}

	// Admin panel (static files)
	router.PathPrefix("/admin/").Handler(
		http.StripPrefix("/admin/",
			http.HandlerFunc(server.handleAdminPanel),
		),
	).Methods("GET")

	// Обратная совместимость (redirect старых endpoints)
	router.HandleFunc("/news", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/news", http.StatusMovedPermanently)
	})
	router.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/v1/healthz", http.StatusMovedPermanently)
	})

	// 404 handler
	router.NotFoundHandler = http.HandlerFunc(server.handleNotFound)

	server.httpServer = &http.Server{
		Addr:         cfg.Host + ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	return server
}

// Start запускает HTTP сервер
func (s *InfoHubServer) Start() error {
	s.logger.WithField("address", s.httpServer.Addr).Info("Starting HTTP server")
	s.logger.Info("Available endpoints:")
	s.logger.Info("  GET /api/v1/news         - Get latest news")
	s.logger.Info("  GET /api/v1/healthz      - Simple health check")
	s.logger.Info("  GET /health              - Detailed health check")
	s.logger.Info("  GET /health/live         - Liveness probe")
	s.logger.Info("  GET /health/ready        - Readiness probe")
	s.logger.Info("  GET /metrics             - Prometheus metrics")
	s.logger.Info("  GET /swagger/            - API documentation")
	s.logger.Info("  GET /admin/              - Admin panel")
	s.logger.Info("  POST /auth/login         - Authentication")

	return s.httpServer.ListenAndServe()
}

// Shutdown корректно завершает работу сервера
func (s *InfoHubServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server...")
	return s.httpServer.Shutdown(ctx)
}

// handleAPIInfo предоставляет информацию об API
func (s *InfoHubServer) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"service": "InfoHub API",
		"version": "1.0.0",
		"versions": map[string]interface{}{
			"v1": map[string]interface{}{
				"status": "active",
				"endpoints": []string{
					"/api/v1/news",
					"/api/v1/healthz",
					"/api/v1/admin/stats",
					"/api/v1/admin/sources",
					"/api/v1/admin/cache/clear",
				},
			},
		},
		"documentation": "/swagger/",
		"monitoring": map[string]string{
			"health":  "/health",
			"metrics": "/metrics",
		},
		"admin_panel": "/admin/",
		"timestamp":   time.Now().UTC(),
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleLiveness обрабатывает liveness probe для Kubernetes
func (s *InfoHubServer) handleLiveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "alive",
		"timestamp": time.Now().UTC(),
	}
	json.NewEncoder(w).Encode(response)
}

// handleReadiness обрабатывает readiness probe для Kubernetes
func (s *InfoHubServer) handleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "ready",
		"timestamp": time.Now().UTC(),
	}
	json.NewEncoder(w).Encode(response)
}

// handleAdminPanel обслуживает админ панель
func (s *InfoHubServer) handleAdminPanel(w http.ResponseWriter, r *http.Request) {
	// Простая админ панель (в реальном проекте это должны быть статические файлы)
	html := `
<!DOCTYPE html>
<html>
<head>
    <title>InfoHub Admin Panel</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .header { background: #2c3e50; color: white; padding: 20px; margin: -40px -40px 20px -40px; }
        .card { border: 1px solid #ddd; padding: 20px; margin: 20px 0; border-radius: 5px; }
        .endpoint { background: #f8f9fa; padding: 10px; margin: 5px 0; border-radius: 3px; font-family: monospace; }
    </style>
</head>
<body>
    <div class="header">
        <h1>InfoHub Admin Panel</h1>
        <p>News Aggregation Service</p>
    </div>
    
    <div class="card">
        <h2>API Endpoints</h2>
        <div class="endpoint">GET /api/v1/news - Get latest news</div>
        <div class="endpoint">GET /api/v1/admin/stats - System statistics</div>
        <div class="endpoint">GET /api/v1/admin/sources - Source information</div>
        <div class="endpoint">POST /api/v1/admin/cache/clear - Clear cache</div>
    </div>
    
    <div class="card">
        <h2>Monitoring</h2>
        <div class="endpoint">GET /health - Detailed health check</div>
        <div class="endpoint">GET /metrics - Prometheus metrics</div>
        <div class="endpoint">GET /swagger/ - API documentation</div>
    </div>
    
    <div class="card">
        <h2>Quick Actions</h2>
        <button onclick="clearCache()">Clear Cache</button>
        <button onclick="viewStats()">View Statistics</button>
        <button onclick="viewHealth()">Check Health</button>
    </div>
    
    <script>
        function clearCache() {
            fetch('/api/v1/admin/cache/clear', {method: 'POST'})
                .then(r => r.json())
                .then(d => alert(d.message));
        }
        
        function viewStats() {
            window.open('/api/v1/admin/stats', '_blank');
        }
        
        function viewHealth() {
            window.open('/health', '_blank');
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleNotFound обрабатывает 404 ошибки
func (s *InfoHubServer) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)

	response := map[string]interface{}{
		"error":   "Not Found",
		"code":    404,
		"message": "The requested endpoint does not exist",
		"path":    r.URL.Path,
		"suggestions": []string{
			"/api/v1/news",
			"/api/v1/healthz",
			"/swagger/",
			"/health",
		},
	}
	json.NewEncoder(w).Encode(response)
}
