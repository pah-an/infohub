package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/pah-an/infohub/internal/domain"
)

// RedisCache реализует кэширование через Redis
type RedisCache struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// Config содержит конфигурацию Redis
type Config struct {
	Address  string        `yaml:"address" json:"address"`
	Password string        `yaml:"password" json:"password"`
	DB       int           `yaml:"db" json:"db"`
	TTL      time.Duration `yaml:"ttl" json:"ttl"`
	Prefix   string        `yaml:"prefix" json:"prefix"`
	Enabled  bool          `yaml:"enabled" json:"enabled"`
}

// NewRedisCache создает новый Redis кэш
func NewRedisCache(config Config) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	if config.TTL == 0 {
		config.TTL = 5 * time.Minute
	}

	if config.Prefix == "" {
		config.Prefix = "infohub:"
	}

	return &RedisCache{
		client: client,
		prefix: config.Prefix,
		ttl:    config.TTL,
	}, nil
}

// Set сохраняет данные в кэш
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	if ttl == 0 {
		ttl = r.ttl
	}

	fullKey := r.prefix + key
	return r.client.Set(ctx, fullKey, data, ttl).Err()
}

// Get получает данные из кэша
func (r *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	fullKey := r.prefix + key
	data, err := r.client.Get(ctx, fullKey).Result()
	if err != nil {
		if err == redis.Nil {
			return ErrCacheMiss
		}
		return fmt.Errorf("failed to get from cache: %w", err)
	}

	if err = json.Unmarshal([]byte(data), dest); err != nil {
		return fmt.Errorf("failed to unmarshal cached value: %w", err)
	}

	return nil
}

// Delete удаляет данные из кэша
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	fullKey := r.prefix + key
	return r.client.Del(ctx, fullKey).Err()
}

// Exists проверяет существование ключа в кэше
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := r.prefix + key
	result, err := r.client.Exists(ctx, fullKey).Result()
	return result > 0, err
}

// SetTTL устанавливает TTL для ключа
func (r *RedisCache) SetTTL(ctx context.Context, key string, ttl time.Duration) error {
	fullKey := r.prefix + key
	return r.client.Expire(ctx, fullKey, ttl).Err()
}

// GetTTL получает TTL ключа
func (r *RedisCache) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	fullKey := r.prefix + key
	return r.client.TTL(ctx, fullKey).Result()
}

// Keys получает все ключи по паттерну
func (r *RedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	fullPattern := r.prefix + pattern
	keys, err := r.client.Keys(ctx, fullPattern).Result()
	if err != nil {
		return nil, err
	}

	// Убираем префикс из ключей
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = key[len(r.prefix):]
	}

	return result, nil
}

// Close закрывает соединение с Redis
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// Ping проверяет соединение с Redis
func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// MemoryCache реализует in-memory кэширование
type MemoryCache struct {
	data   map[string]*cacheItem
	mutex  *sync.RWMutex
	ttl    time.Duration
	ticker *time.Ticker
	done   chan bool
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewMemoryCache создает новый in-memory кэш
func NewMemoryCache(ttl time.Duration, cleanupInterval time.Duration) *MemoryCache {
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	if cleanupInterval == 0 {
		cleanupInterval = 10 * time.Minute
	}

	cache := &MemoryCache{
		data:   make(map[string]*cacheItem),
		mutex:  &sync.RWMutex{},
		ttl:    ttl,
		ticker: time.NewTicker(cleanupInterval),
		done:   make(chan bool),
	}

	go cache.cleanup()

	return cache
}

// Set сохраняет данные в memory кэш
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if ttl == 0 {
		ttl = m.ttl
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.data[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Get получает данные из memory кэша
func (m *MemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	item, exists := m.data[key]
	if !exists || time.Now().After(item.expiresAt) {
		return ErrCacheMiss
	}

	data, err := json.Marshal(item.value)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Delete удаляет данные из memory кэша
func (m *MemoryCache) Delete(ctx context.Context, key string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.data, key)
	return nil
}

// cleanup очищает устаревшие элементы
func (m *MemoryCache) cleanup() {
	for {
		select {
		case <-m.ticker.C:
			m.mutex.Lock()
			now := time.Now()
			for key, item := range m.data {
				if now.After(item.expiresAt) {
					delete(m.data, key)
				}
			}
			m.mutex.Unlock()
		case <-m.done:
			return
		}
	}
}

// Close закрывает memory кэш
func (m *MemoryCache) Close() error {
	m.ticker.Stop()
	m.done <- true
	return nil
}

// Cache определяет интерфейс кэша
type Cache interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string, dest interface{}) error
	Delete(ctx context.Context, key string) error
	Close() error
}

// ErrCacheMiss возвращается когда ключ не найден в кэше
var ErrCacheMiss = fmt.Errorf("cache miss")

// NewsCacheKey генерирует ключ для кэширования новостей
func NewsCacheKey(limit int) string {
	return fmt.Sprintf("news:latest:%d", limit)
}

// CachedNewsRepository добавляет кэширование к репозиторию новостей
type CachedNewsRepository struct {
	cache Cache
	repo  domain.NewsRepository
	ttl   time.Duration
}

// NewCachedNewsRepository создает репозиторий с кэшированием
func NewCachedNewsRepository(cache Cache, repo domain.NewsRepository, ttl time.Duration) *CachedNewsRepository {
	return &CachedNewsRepository{
		cache: cache,
		repo:  repo,
		ttl:   ttl,
	}
}

// GetLatestNews получает новости с кэшированием
func (c *CachedNewsRepository) GetLatestNews(limit int) (domain.NewsList, error) {
	ctx := context.Background()
	key := NewsCacheKey(limit)

	// Пытаемся получить из кэша
	var cachedNews domain.NewsList
	if err := c.cache.Get(ctx, key, &cachedNews); err == nil {
		return cachedNews, nil
	}

	// Если в кэше нет, получаем из репозитория
	news, err := c.repo.GetLatestNews(limit)
	if err != nil {
		return nil, err
	}

	// Сохраняем в кэш
	if err = c.cache.Set(ctx, key, news, c.ttl); err != nil {
		// Логируем ошибку, но не возвращаем её
		fmt.Printf("Failed to cache news: %v\n", err)
	}

	return news, nil
}

// SaveNews сохраняет новости и инвалидирует кэш
func (c *CachedNewsRepository) SaveNews(news domain.NewsList) error {
	ctx := context.Background()

	// Сохраняем в репозиторий
	if err := c.repo.SaveNews(news); err != nil {
		return err
	}

	// Инвалидируем кэш
	keys := []string{
		NewsCacheKey(10),
		NewsCacheKey(50),
		NewsCacheKey(100),
		NewsCacheKey(500),
		NewsCacheKey(1000),
	}

	for _, key := range keys {
		if err := c.cache.Delete(ctx, key); err != nil {
			fmt.Printf("Failed to invalidate cache key %s: %v\n", key, err)
		}
	}

	return nil
}
