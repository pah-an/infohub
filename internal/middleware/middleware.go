package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/pah-an/infohub/internal/auth"
	"github.com/pah-an/infohub/internal/logger"
	"github.com/pah-an/infohub/internal/metrics"
)

// RateLimiter реализует rate limiting
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mutex    sync.RWMutex
	rate     rate.Limit
	burst    int
}

// NewRateLimiter создает новый rate limiter
func NewRateLimiter(rps float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(rps),
		burst:    burst,
	}
}

// getLimiter получает лимитер для IP адреса
func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// Allow проверяет, разрешен ли запрос
func (rl *RateLimiter) Allow(ip string) bool {
	return rl.getLimiter(ip).Allow()
}

// Middleware для rate limiting
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)

		if !rl.Allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)

			response := map[string]interface{}{
				"error":   "Rate limit exceeded",
				"code":    429,
				"message": "Too many requests. Please try again later.",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP извлекает IP адрес клиента
func getClientIP(r *http.Request) string {
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		return strings.TrimSpace(ips[0])
	}

	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		return xRealIP
	}

	ip := r.RemoteAddr
	if lastColon := strings.LastIndex(ip, ":"); lastColon != -1 {
		ip = ip[:lastColon]
	}

	return ip
}

// RequestID middleware добавляет уникальный ID к каждому запросу
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		ctx := context.WithValue(r.Context(), "request_id", requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID генерирует уникальный ID запроса
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Security middleware добавляет заголовки безопасности
func Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		csp := "default-src 'self'; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com; " +
			"font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com; " +
			"script-src 'self' 'unsafe-inline' https://cdnjs.cloudflare.com; " +
			"img-src 'self' data: https:; " +
			"connect-src 'self'"
		// Для Swagger endpoints используем более мягкую CSP
		if strings.HasPrefix(r.URL.Path, "/swagger/") || r.URL.Path == "/docs/" {
			swaggerCSP := "default-src 'self'; " +
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdnjs.cloudflare.com https://unpkg.com; " +
				"font-src 'self' https://fonts.gstatic.com https://cdnjs.cloudflare.com; " +
				"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdnjs.cloudflare.com https://unpkg.com; " +
				"img-src 'self' data: https:; " +
				"connect-src 'self'"
			w.Header().Set("Content-Security-Policy", swaggerCSP)
		} else {
			w.Header().Set("Content-Security-Policy", csp)
		}

		next.ServeHTTP(w, r)
	})
}

// Recovery middleware обрабатывает панику
func Recovery(logger *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.WithFields(map[string]interface{}{
						"panic":      err,
						"request_id": r.Context().Value("request_id"),
						"method":     r.Method,
						"path":       r.URL.Path,
						"ip":         getClientIP(r),
					}).Error("Panic recovered")

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					response := map[string]interface{}{
						"error":   "Internal server error",
						"code":    500,
						"message": "An unexpected error occurred",
					}
					json.NewEncoder(w).Encode(response)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// Timeout middleware устанавливает таймаут для запросов
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// Logging middleware логирует запросы
func Logging(logger *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}
			next.ServeHTTP(lrw, r)
			duration := time.Since(start)
			logger.WithFields(map[string]interface{}{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     lrw.statusCode,
				"duration":   duration.String(),
				"ip":         getClientIP(r),
				"user_agent": r.UserAgent(),
				"request_id": r.Context().Value("request_id"),
			}).Info("HTTP request processed")
		})
	}
}

// Metrics middleware записывает метрики
func Metrics(metrics *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			endpoint := r.URL.Path
			metrics.HTTPRequestsInFlight.WithLabelValues(r.Method, endpoint).Inc()
			defer metrics.HTTPRequestsInFlight.WithLabelValues(r.Method, endpoint).Dec()
			lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: 200}

			next.ServeHTTP(lrw, r)

			duration := time.Since(start)
			metrics.RecordHTTPRequest(r.Method, endpoint, lrw.statusCode, duration)
		})
	}
}

// Auth middleware для аутентификации
func Auth(authManager *auth.Manager, logger *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := authManager.AuthenticateRequest(r)
			if err != nil {
				logger.WithFields(map[string]interface{}{
					"error":      err.Error(),
					"method":     r.Method,
					"path":       r.URL.Path,
					"ip":         getClientIP(r),
					"request_id": r.Context().Value("request_id"),
				}).Warn("Authentication failed")

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)

				response := map[string]interface{}{
					"error":   "Unauthorized",
					"code":    401,
					"message": "Authentication required",
				}
				json.NewEncoder(w).Encode(response)
				return
			}

			r.Header.Set("X-User-ID", user.ID)
			r.Header.Set("X-User-Admin", strconv.FormatBool(user.IsAdmin))

			logger.WithFields(map[string]interface{}{
				"user_id":    user.ID,
				"is_admin":   user.IsAdmin,
				"request_id": r.Context().Value("request_id"),
			}).Debug("User authenticated")

			next.ServeHTTP(w, r)
		})
	}
}

// CORS middleware для Cross-Origin Resource Sharing
func CORS(allowedOrigins []string, allowedMethods []string, allowedHeaders []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// loggingResponseWriter для захвата статус кода
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}
