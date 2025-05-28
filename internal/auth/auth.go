package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config содержит конфигурацию аутентификации
type Config struct {
	JWTSecret   string            `yaml:"jwt_secret" json:"jwt_secret"`
	JWTTTL      time.Duration     `yaml:"jwt_ttl" json:"jwt_ttl"`
	APIKeys     map[string]string `yaml:"api_keys" json:"api_keys"` // key -> description
	AdminAPIKey string            `yaml:"admin_api_key" json:"admin_api_key"`
	Enabled     bool              `yaml:"enabled" json:"enabled"`
	PublicPaths []string          `yaml:"public_paths" json:"public_paths"`
}

// Manager управляет аутентификацией
type Manager struct {
	config    Config
	jwtSecret []byte
}

// Claims представляет JWT claims
type Claims struct {
	UserID  string   `json:"user_id"`
	APIKey  string   `json:"api_key"`
	IsAdmin bool     `json:"is_admin"`
	Scopes  []string `json:"scopes"`
	jwt.RegisteredClaims
}

// User представляет пользователя
type User struct {
	ID      string   `json:"id"`
	APIKey  string   `json:"api_key"`
	IsAdmin bool     `json:"is_admin"`
	Scopes  []string `json:"scopes"`
}

// NewManager создает новый менеджер аутентификации
func NewManager(config Config) (*Manager, error) {
	if config.JWTSecret == "" {
		return nil, fmt.Errorf("JWT secret is required")
	}

	if config.JWTTTL == 0 {
		config.JWTTTL = 24 * time.Hour // по умолчанию 24 часа
	}

	if config.APIKeys == nil {
		config.APIKeys = make(map[string]string)
	}

	return &Manager{
		config:    config,
		jwtSecret: []byte(config.JWTSecret),
	}, nil
}

// GenerateAPIKey генерирует новый API ключ
func GenerateAPIKey() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return ""
	}
	return "infohub_" + hex.EncodeToString(bytes)
}

// ValidateAPIKey проверяет API ключ
func (m *Manager) ValidateAPIKey(apiKey string) (*User, error) {
	if !m.config.Enabled {
		return &User{
			ID:      "anonymous",
			APIKey:  "none",
			IsAdmin: false,
			Scopes:  []string{"read"},
		}, nil
	}

	// Проверяем admin API key
	if apiKey == m.config.AdminAPIKey && m.config.AdminAPIKey != "" {
		return &User{
			ID:      "admin",
			APIKey:  apiKey,
			IsAdmin: true,
			Scopes:  []string{"read", "write", "admin"},
		}, nil
	}

	// Проверяем обычные API keys
	if description, exists := m.config.APIKeys[apiKey]; exists {
		return &User{
			ID:      description,
			APIKey:  apiKey,
			IsAdmin: false,
			Scopes:  []string{"read"},
		}, nil
	}

	return nil, fmt.Errorf("invalid API key")
}

// GenerateJWT генерирует JWT токен
func (m *Manager) GenerateJWT(user *User) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:  user.ID,
		APIKey:  user.APIKey,
		IsAdmin: user.IsAdmin,
		Scopes:  user.Scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(m.config.JWTTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "infohub",
			Subject:   user.ID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.jwtSecret)
}

// ValidateJWT проверяет JWT токен
func (m *Manager) ValidateJWT(tokenString string) (*User, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return &User{
			ID:      claims.UserID,
			APIKey:  claims.APIKey,
			IsAdmin: claims.IsAdmin,
			Scopes:  claims.Scopes,
		}, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// AuthenticateRequest аутентифицирует HTTP запрос
func (m *Manager) AuthenticateRequest(r *http.Request) (*User, error) {
	// Проверяем публичные пути
	for _, path := range m.config.PublicPaths {
		if strings.HasPrefix(r.URL.Path, path) {
			return &User{
				ID:      "public",
				APIKey:  "none",
				IsAdmin: false,
				Scopes:  []string{"read"},
			}, nil
		}
	}

	// Пытаемся получить API ключ из заголовка
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		// Пытаемся получить из query параметра
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey != "" {
		return m.ValidateAPIKey(apiKey)
	}

	// Пытаемся получить JWT из заголовка Authorization
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return m.ValidateJWT(parts[1])
		}
	}

	if !m.config.Enabled {
		return &User{
			ID:      "anonymous",
			APIKey:  "none",
			IsAdmin: false,
			Scopes:  []string{"read"},
		}, nil
	}

	return nil, fmt.Errorf("authentication required")
}

// HasScope проверяет, есть ли у пользователя определенная область доступа
func (u *User) HasScope(scope string) bool {
	for _, s := range u.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// RequireScope создает middleware для проверки области доступа
func (m *Manager) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := m.AuthenticateRequest(r)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.HasScope(scope) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Добавляем пользователя в контекст
			r.Header.Set("X-User-ID", user.ID)
			r.Header.Set("X-User-Admin", fmt.Sprintf("%t", user.IsAdmin))

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin создает middleware для проверки прав администратора
func (m *Manager) RequireAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, err := m.AuthenticateRequest(r)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !user.IsAdmin {
				http.Error(w, "Admin access required", http.StatusForbidden)
				return
			}

			r.Header.Set("X-User-ID", user.ID)
			r.Header.Set("X-User-Admin", "true")

			next.ServeHTTP(w, r)
		})
	}
}
