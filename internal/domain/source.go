package domain

import "time"

// Source представляет источник новостей
type Source struct {
	Name     string        `yaml:"name" json:"name"`
	URL      string        `yaml:"url" json:"url"`
	Interval time.Duration `yaml:"interval" json:"interval"`
}

// SourceRepository определяет интерфейс для работы с источниками
type SourceRepository interface {
	GetSources() []Source
}

// NewsRepository определяет интерфейс для работы с новостями
type NewsRepository interface {
	SaveNews(news NewsList) error
	GetLatestNews(limit int) (NewsList, error)
}

// NewsCollector определяет интерфейс для сбора новостей
type NewsCollector interface {
	CollectFromSource(source Source) (NewsList, error)
}
