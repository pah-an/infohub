package domain

import "time"

// News представляет новость из любого источника
type News struct {
	ID          string    `json:"id" example:"tech_news_1640995200_1"`
	Title       string    `json:"title" example:"Breaking: New Go Version Released"`
	Description string    `json:"description" example:"This breakthrough announcement changes the landscape..."`
	URL         string    `json:"url" example:"https://example.com/news/go-release"`
	Source      string    `json:"source" example:"Tech News"`
	PublishedAt time.Time `json:"published_at" example:"2024-01-01T12:00:00Z"`
}

// NewsList представляет список новостей
type NewsList []News

// SortByDate сортирует новости по дате (по убыванию)
func (nl NewsList) SortByDate() NewsList {
	// Простая сортировка пузырьком для демонстрации
	// В продакшене лучше использовать sort.Slice
	for i := 0; i < len(nl)-1; i++ {
		for j := 0; j < len(nl)-i-1; j++ {
			if nl[j].PublishedAt.Before(nl[j+1].PublishedAt) {
				nl[j], nl[j+1] = nl[j+1], nl[j]
			}
		}
	}
	return nl
}

// LimitTo ограничивает количество новостей
func (nl NewsList) LimitTo(limit int) NewsList {
	if len(nl) <= limit {
		return nl
	}
	return nl[:limit]
}
