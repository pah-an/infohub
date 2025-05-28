package v1

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/pah-an/infohub/internal/auth"
	"github.com/pah-an/infohub/internal/domain"
)

// NewsProvider определяет интерфейс для получения новостей
type NewsProvider interface {
	GetLatestNews(limit int) domain.NewsList
}

// Handlers содержит все обработчики для API v1
type Handlers struct {
	newsProvider NewsProvider
}

// NewHandlers создает новый экземпляр обработчиков
func NewHandlers(newsProvider NewsProvider) *Handlers {
	return &Handlers{
		newsProvider: newsProvider,
	}
}

// NewsResponse представляет ответ с новостями
type NewsResponse struct {
	Count   int             `json:"count" example:"10"`
	News    domain.NewsList `json:"news"`
	Version string          `json:"version" example:"v1"`
}

// HealthResponse представляет ответ healthcheck
type HealthResponse struct {
	Status    string    `json:"status" example:"ok"`
	Timestamp time.Time `json:"timestamp"`
	Service   string    `json:"service" example:"InfoHub"`
	Version   string    `json:"version" example:"v1"`
}

// ErrorResponse представляет ответ с ошибкой
type ErrorResponse struct {
	Error   string `json:"error" example:"Invalid request"`
	Code    int    `json:"code" example:"400"`
	Version string `json:"version" example:"v1"`
}

// GetNews
// @Summary      Получить список новостей
// @Description  Возвращает последние новости, агрегированные из всех источников
// @Tags         news
// @Accept       json
// @Produce      json
// @Param        limit    query     int  false  "Количество новостей (по умолчанию 100)"  minimum(1)  maximum(1000)
// @Success      200      {object}  NewsResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /news [get]
func (h *Handlers) GetNews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем лимит из query параметра, по умолчанию 100
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		} else {
			h.writeErrorResponse(w, "Invalid limit parameter. Must be between 1 and 1000", http.StatusBadRequest)
			return
		}
	}

	news := h.newsProvider.GetLatestNews(limit)

	response := NewsResponse{
		Count:   len(news),
		News:    news,
		Version: "v1",
	}

	h.writeJSONResponse(w, response, http.StatusOK)
}

// GetHealth
// @Summary      Проверка состояния сервиса
// @Description  Возвращает статус работы сервиса
// @Tags         system
// @Accept       json
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /healthz [get]
func (h *Handlers) GetHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Service:   "InfoHub",
		Version:   "v1",
	}

	h.writeJSONResponse(w, response, http.StatusOK)
}

// writeJSONResponse отправляет JSON ответ
func (h *Handlers) writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse отправляет ответ с ошибкой
func (h *Handlers) writeErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	response := ErrorResponse{
		Error:   message,
		Code:    statusCode,
		Version: "v1",
	}

	h.writeJSONResponse(w, response, statusCode)
}

// AdminStatsResponse представляет ответ со статистикой
type AdminStatsResponse struct {
	TotalRequests int     `json:"total_requests" example:"1000"`
	TotalNews     int     `json:"total_news" example:"5000"`
	SourcesActive int     `json:"sources_active" example:"2"`
	CacheHitRatio float64 `json:"cache_hit_ratio" example:"0.85"`
	Uptime        string  `json:"uptime" example:"24h30m"`
	MemoryUsage   string  `json:"memory_usage" example:"128MB"`
	Goroutines    int     `json:"goroutines" example:"50"`
}

// AdminSourceResponse представляет информацию об источнике
type AdminSourceResponse struct {
	Name        string    `json:"name" example:"Tech News"`
	URL         string    `json:"url" example:"https://tech-news-api.herokuapp.com/api/news"`
	Status      string    `json:"status" example:"healthy"`
	LastCheck   time.Time `json:"last_check"`
	NewsCount   int       `json:"news_count" example:"2500"`
	AvgResponse string    `json:"avg_response" example:"1.2s"`
}

// AdminClearCacheResponse представляет ответ на очистку кэша
type AdminClearCacheResponse struct {
	Success   bool      `json:"success" example:"true"`
	Message   string    `json:"message" example:"Cache cleared successfully"`
	Timestamp time.Time `json:"timestamp"`
}

// LoginRequest представляет запрос на авторизацию
type LoginRequest struct {
	APIKey string `json:"api_key" example:"your-api-key"`
}

// LoginResponse представляет ответ на авторизацию
type LoginResponse struct {
	Token     string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	UserID    string `json:"user_id" example:"user123"`
	IsAdmin   bool   `json:"is_admin" example:"false"`
	ExpiresIn string `json:"expires_in" example:"24h"`
}

// ValidateTokenResponse представляет ответ на проверку токена
type ValidateTokenResponse struct {
	Valid   bool     `json:"valid" example:"true"`
	UserID  string   `json:"user_id" example:"user123"`
	IsAdmin bool     `json:"is_admin" example:"false"`
	Scopes  []string `json:"scopes" example:"read,write"`
}

// GetAdminStats
// @Summary      Получить статистику системы
// @Description  Возвращает статистику работы системы (только для администраторов)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  AdminStatsResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/stats [get]
func (h *Handlers) GetAdminStats(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	response := AdminStatsResponse{
		TotalRequests: 1000, // TODO: получить реальные метрики
		TotalNews:     len(h.newsProvider.GetLatestNews(10000)),
		SourcesActive: 2,
		CacheHitRatio: 0.85,
		Uptime:        "24h30m", // TODO: реальный uptime
		MemoryUsage:   formatBytes(m.Alloc),
		Goroutines:    runtime.NumGoroutine(),
	}

	h.writeJSONResponse(w, response, http.StatusOK)
}

// GetAdminSources
// @Summary      Получить информацию об источниках
// @Description  Возвращает информацию о всех источниках новостей (только для администраторов)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   AdminSourceResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/sources [get]
func (h *Handlers) GetAdminSources(w http.ResponseWriter, r *http.Request) {
	// TODO: получить реальную информацию об источниках
	sources := []AdminSourceResponse{
		{
			Name:        "Tech News",
			URL:         "https://tech-news-api.herokuapp.com/api/news",
			Status:      "healthy",
			LastCheck:   time.Now().Add(-30 * time.Second),
			NewsCount:   2500,
			AvgResponse: "1.2s",
		},
		{
			Name:        "Local Mock API",
			URL:         "http://localhost:3001/api/news",
			Status:      "healthy",
			LastCheck:   time.Now().Add(-30 * time.Second),
			NewsCount:   2500,
			AvgResponse: "0.1s",
		},
	}

	h.writeJSONResponse(w, sources, http.StatusOK)
}

// ClearAdminCache
// @Summary      Очистить кэш
// @Description  Очищает кэш новостей (только для администраторов)
// @Tags         admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  AdminClearCacheResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/cache/clear [post]
func (h *Handlers) ClearAdminCache(w http.ResponseWriter, r *http.Request) {
	// TODO: реализовать логику очистки кэша
	log.Println("Cache cleared by admin")

	response := AdminClearCacheResponse{
		Success:   true,
		Message:   "Cache cleared successfully",
		Timestamp: time.Now().UTC(),
	}

	h.writeJSONResponse(w, response, http.StatusOK)
}

// PostLogin
// @Summary      Авторизация пользователя
// @Description  Авторизация по API ключу и получение JWT токена
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      LoginRequest  true  "Данные для авторизации"
// @Success      200      {object}  LoginResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Router       /login [post]
func (h *Handlers) PostLogin(authManager *auth.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var loginRequest LoginRequest

		if err := json.NewDecoder(r.Body).Decode(&loginRequest); err != nil {
			h.writeErrorResponse(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := authManager.ValidateAPIKey(loginRequest.APIKey)
		if err != nil {
			h.writeErrorResponse(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		token, err := authManager.GenerateJWT(user)
		if err != nil {
			h.writeErrorResponse(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}

		response := LoginResponse{
			Token:     token,
			UserID:    user.ID,
			IsAdmin:   user.IsAdmin,
			ExpiresIn: "24h",
		}

		h.writeJSONResponse(w, response, http.StatusOK)
	}
}

// GetValidateToken
// @Summary      Проверить токен
// @Description  Проверяет валидность JWT токена
// @Tags         auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  ValidateTokenResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /validate [get]
func (h *Handlers) GetValidateToken(authManager *auth.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := authManager.AuthenticateRequest(r)
		if err != nil {
			h.writeErrorResponse(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		response := ValidateTokenResponse{
			Valid:   true,
			UserID:  user.ID,
			IsAdmin: user.IsAdmin,
			Scopes:  user.Scopes,
		}

		h.writeJSONResponse(w, response, http.StatusOK)
	}
}

// formatBytes форматирует байты в человекочитаемый формат
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return strconv.FormatUint(b, 10) + "B"
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(b)/float64(div), 'f', 1, 64) + []string{"K", "M", "G", "T", "P", "E", "Z", "Y"}[exp] + "B"
}
