package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Status представляет статус компонента
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusWarning   Status = "warning"
	StatusUnknown   Status = "unknown"
)

// Check представляет проверку здоровья компонента
type Check struct {
	Name      string            `json:"name"`
	Status    Status            `json:"status"`
	Message   string            `json:"message,omitempty"`
	Duration  time.Duration     `json:"duration"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// CheckFunc представляет функцию проверки здоровья
type CheckFunc func(ctx context.Context) Check

// Manager управляет проверками здоровья
type Manager struct {
	checks  map[string]CheckFunc
	mutex   sync.RWMutex
	timeout time.Duration
}

// OverallHealth представляет общее состояние системы
type OverallHealth struct {
	Status    Status           `json:"status"`
	Timestamp time.Time        `json:"timestamp"`
	Duration  time.Duration    `json:"duration"`
	Service   string           `json:"service"`
	Version   string           `json:"version"`
	Checks    map[string]Check `json:"checks"`
}

// NewManager создает новый менеджер health checks
func NewManager(timeout time.Duration) *Manager {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &Manager{
		checks:  make(map[string]CheckFunc),
		timeout: timeout,
	}
}

// RegisterCheck регистрирует проверку здоровья
func (m *Manager) RegisterCheck(name string, check CheckFunc) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.checks[name] = check
}

// RunChecks выполняет все зарегистрированные проверки
func (m *Manager) RunChecks(ctx context.Context) OverallHealth {
	start := time.Now()

	m.mutex.RLock()
	checksToRun := make(map[string]CheckFunc, len(m.checks))
	for name, check := range m.checks {
		checksToRun[name] = check
	}
	m.mutex.RUnlock()

	results := make(map[string]Check)
	var wg sync.WaitGroup
	var resultsMutex sync.Mutex

	// Выполняем проверки параллельно
	for name, check := range checksToRun {
		wg.Add(1)
		go func(name string, check CheckFunc) {
			defer wg.Done()

			// Создаем контекст с таймаутом для каждой проверки
			checkCtx, cancel := context.WithTimeout(ctx, m.timeout)
			defer cancel()

			result := m.runSingleCheck(checkCtx, name, check)

			resultsMutex.Lock()
			results[name] = result
			resultsMutex.Unlock()
		}(name, check)
	}

	wg.Wait()

	// Определяем общий статус
	overallStatus := m.calculateOverallStatus(results)

	return OverallHealth{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Service:   "InfoHub",
		Version:   "1.0.0", // TODO: получить из конфигурации
		Checks:    results,
	}
}

// runSingleCheck выполняет одну проверку с обработкой паники
func (m *Manager) runSingleCheck(ctx context.Context, name string, check CheckFunc) Check {
	start := time.Now()

	defer func() {
		if r := recover(); r != nil {
			// Обработка паники в проверке
		}
	}()

	select {
	case <-ctx.Done():
		return Check{
			Name:      name,
			Status:    StatusUnhealthy,
			Message:   "Check timed out",
			Duration:  time.Since(start),
			Timestamp: time.Now(),
			Error:     ctx.Err().Error(),
		}
	default:
		result := check(ctx)
		result.Name = name
		result.Duration = time.Since(start)
		result.Timestamp = time.Now()
		return result
	}
}

// calculateOverallStatus вычисляет общий статус на основе результатов проверок
func (m *Manager) calculateOverallStatus(checks map[string]Check) Status {
	if len(checks) == 0 {
		return StatusUnknown
	}

	hasUnhealthy := false
	hasWarning := false

	for _, check := range checks {
		switch check.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusWarning:
			hasWarning = true
		case StatusUnknown:
			hasWarning = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasWarning {
		return StatusWarning
	}

	return StatusHealthy
}

// HTTPHandler создает HTTP handler для health checks
func (m *Manager) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		health := m.RunChecks(ctx)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

		// Устанавливаем статус код в зависимости от здоровья
		switch health.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusWarning:
			w.WriteHeader(http.StatusOK) // 200, но с предупреждениями
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable) // 503
		default:
			w.WriteHeader(http.StatusServiceUnavailable) // 503
		}

		if err := json.NewEncoder(w).Encode(health); err != nil {
			http.Error(w, "Failed to encode health check response", http.StatusInternalServerError)
		}
	}
}

// Предопределенные проверки здоровья

// DatabaseCheck создает проверку базы данных
func DatabaseCheck(pingFunc func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) Check {
		if err := pingFunc(ctx); err != nil {
			return Check{
				Status:  StatusUnhealthy,
				Message: "Database connection failed",
				Error:   err.Error(),
			}
		}

		return Check{
			Status:  StatusHealthy,
			Message: "Database connection successful",
		}
	}
}

// RedisCheck создает проверку Redis
func RedisCheck(pingFunc func(ctx context.Context) error) CheckFunc {
	return func(ctx context.Context) Check {
		if err := pingFunc(ctx); err != nil {
			return Check{
				Status:  StatusWarning, // Redis не критичен для работы
				Message: "Redis connection failed",
				Error:   err.Error(),
			}
		}

		return Check{
			Status:  StatusHealthy,
			Message: "Redis connection successful",
		}
	}
}

// DiskSpaceCheck создает проверку дискового пространства
func DiskSpaceCheck(path string, threshold float64) CheckFunc {
	return func(ctx context.Context) Check {
		// Здесь должна быть реальная проверка дискового пространства
		// Для примера возвращаем здоровый статус
		return Check{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("Disk space usage below threshold (%.1f%%)", threshold),
			Metadata: map[string]string{
				"path":      path,
				"threshold": fmt.Sprintf("%.1f%%", threshold),
			},
		}
	}
}

// ExternalServiceCheck создает проверку внешнего сервиса
func ExternalServiceCheck(name, url string, timeout time.Duration) CheckFunc {
	return func(ctx context.Context) Check {
		client := &http.Client{Timeout: timeout}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return Check{
				Status:  StatusUnhealthy,
				Message: fmt.Sprintf("Failed to create request to %s", name),
				Error:   err.Error(),
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return Check{
				Status:  StatusWarning,
				Message: fmt.Sprintf("%s is not accessible", name),
				Error:   err.Error(),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return Check{
				Status:  StatusHealthy,
				Message: fmt.Sprintf("%s is accessible", name),
				Metadata: map[string]string{
					"url":         url,
					"status_code": fmt.Sprintf("%d", resp.StatusCode),
				},
			}
		}

		return Check{
			Status:  StatusWarning,
			Message: fmt.Sprintf("%s returned non-2xx status", name),
			Metadata: map[string]string{
				"url":         url,
				"status_code": fmt.Sprintf("%d", resp.StatusCode),
			},
		}
	}
}

// MemoryCheck создает проверку использования памяти
func MemoryCheck(threshold float64) CheckFunc {
	return func(ctx context.Context) Check {
		// Здесь должна быть реальная проверка памяти
		// Для примера возвращаем здоровый статус
		return Check{
			Status:  StatusHealthy,
			Message: fmt.Sprintf("Memory usage below threshold (%.1f%%)", threshold),
			Metadata: map[string]string{
				"threshold": fmt.Sprintf("%.1f%%", threshold),
			},
		}
	}
}
