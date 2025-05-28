package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// VersionMiddleware обрабатывает версионность API
func VersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Логируем запрос
		log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		// Проверяем, что путь начинается с /api/
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			writeErrorResponse(w, "API path must start with /api/", http.StatusNotFound)
			return
		}

		// Проверяем версию в пути
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathParts) < 2 {
			writeErrorResponse(w, "API version is required. Use /api/v1/...", http.StatusBadRequest)
			return
		}

		version := pathParts[1]
		if !isValidVersion(version) {
			writeErrorResponse(w, "Unsupported API version. Supported versions: v1", http.StatusBadRequest)
			return
		}

		// Добавляем версию в контекст запроса через header
		w.Header().Set("API-Version", version)

		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware добавляет CORS заголовки
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, API-Version")
		w.Header().Set("Access-Control-Expose-Headers", "API-Version")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware логирует запросы
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Создаем wrapper для захвата статус кода
		lrw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(lrw, r)

		duration := time.Since(start)
		log.Printf("API Request: %s %s - Status: %d - Duration: %v",
			r.Method, r.URL.Path, lrw.statusCode, duration)
	})
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

// isValidVersion проверяет, является ли версия поддерживаемой
func isValidVersion(version string) bool {
	supportedVersions := []string{"v1"}
	for _, v := range supportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// ErrorResponse для middleware ошибок
type ErrorResponse struct {
	Error     string    `json:"error"`
	Code      int       `json:"code"`
	Timestamp time.Time `json:"timestamp"`
	Path      string    `json:"path"`
}

// writeErrorResponse отправляет ответ с ошибкой
func writeErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Error:     message,
		Code:      statusCode,
		Timestamp: time.Now().UTC(),
		Path:      "",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding error response: %v", err)
	}
}
