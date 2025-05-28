package aggregator

import (
	"context"
	"log"
	"sync"

	"github.com/pah-an/infohub/internal/domain"
)

// Aggregator агрегирует новости из разных источников
type Aggregator struct {
	news       domain.NewsList
	mutex      sync.RWMutex
	repository domain.NewsRepository
}

// New создает новый агрегатор
func New(repo domain.NewsRepository) *Aggregator {
	return &Aggregator{
		news:       make(domain.NewsList, 0),
		repository: repo,
	}
}

// Start запускает агрегатор для прослушивания каналов
func (a *Aggregator) Start(ctx context.Context, newsChannel <-chan domain.NewsList, errorChannel <-chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		case newNews := <-newsChannel:
			a.addNews(newNews)
		case err := <-errorChannel:
			log.Printf("Collector error: %v", err)
		}
	}
}

// addNews добавляет новые новости в хранилище
func (a *Aggregator) addNews(newNews domain.NewsList) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.news = append(a.news, newNews...)

	a.removeDuplicates()

	a.news = a.news.SortByDate()

	if len(a.news) > 1000 {
		a.news = a.news[:1000]
	}

	if a.repository != nil {
		if err := a.repository.SaveNews(a.news); err != nil {
			log.Printf("Error saving news to repository: %v", err)
		}
	}
}

// removeDuplicates удаляет дубликаты новостей по ID
func (a *Aggregator) removeDuplicates() {
	seen := make(map[string]bool)
	uniqueNews := make(domain.NewsList, 0, len(a.news))

	for _, news := range a.news {
		if !seen[news.ID] {
			seen[news.ID] = true
			uniqueNews = append(uniqueNews, news)
		}
	}

	a.news = uniqueNews
}

// GetLatestNews возвращает последние новости
func (a *Aggregator) GetLatestNews(limit int) domain.NewsList {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.news.LimitTo(limit)
}

// GetNews возвращает все новости
func (a *Aggregator) GetNews() domain.NewsList {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.news
}

// LoadFromRepository загружает новости из репозитория при старте
func (a *Aggregator) LoadFromRepository() error {
	if a.repository == nil {
		return nil
	}

	news, err := a.repository.GetLatestNews(1000)
	if err != nil {
		return err
	}

	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.news = news.SortByDate()
	return nil
}

// SaveToRepository сохраняет текущие новости в репозиторий
func (a *Aggregator) SaveToRepository() error {
	if a.repository == nil {
		return nil
	}

	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a.repository.SaveNews(a.news)
}
