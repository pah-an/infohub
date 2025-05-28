package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/pah-an/infohub/internal/domain"
)

// Collector реализует сбор новостей из источников
type Collector struct {
	client   *http.Client
	sources  []domain.Source
	interval time.Duration
}

// New создает новый коллектор
func New(sources []domain.Source, interval time.Duration) *Collector {
	return &Collector{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		sources:  sources,
		interval: interval,
	}
}

// Start запускает периодический сбор новостей
func (c *Collector) Start(ctx context.Context, newsChannel chan<- domain.NewsList, errorChannel chan<- error) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Первый сбор сразу при запуске
	c.collectFromAllSources(ctx, newsChannel, errorChannel)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectFromAllSources(ctx, newsChannel, errorChannel)
		}
	}
}

// collectFromAllSources собирает новости со всех источников параллельно
func (c *Collector) collectFromAllSources(ctx context.Context, newsChannel chan<- domain.NewsList, errorChannel chan<- error) {
	var wg sync.WaitGroup

	for _, source := range c.sources {
		wg.Add(1)

		go func(src domain.Source) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
				news, err := c.CollectFromSource(src)
				if err != nil {
					select {
					case errorChannel <- fmt.Errorf("error collecting from %s: %w", src.Name, err):
					case <-ctx.Done():
					}
					return
				}

				if len(news) > 0 {
					select {
					case newsChannel <- news:
					case <-ctx.Done():
					}
				}
			}
		}(source)
	}

	wg.Wait()
}

// CollectFromSource собирает новости из одного источника
func (c *Collector) CollectFromSource(source domain.Source) (domain.NewsList, error) {
	req, err := http.NewRequest("GET", source.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, source.URL)
	}

	// Предполагаем, что API возвращает массив новостей в JSON
	var apiResponse struct {
		Articles []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			URL         string `json:"url"`
			PublishedAt string `json:"publishedAt"`
		} `json:"articles"`
	}

	if err = json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, err
	}

	var news domain.NewsList
	for i, article := range apiResponse.Articles {
		publishedAt, err := time.Parse(time.RFC3339, article.PublishedAt)
		if err != nil {
			// Если не удается распарсить дату, используем текущее время
			publishedAt = time.Now()
		}

		news = append(news, domain.News{
			ID:          fmt.Sprintf("%s_%d_%d", source.Name, time.Now().Unix(), i),
			Title:       article.Title,
			Description: article.Description,
			URL:         article.URL,
			Source:      source.Name,
			PublishedAt: publishedAt,
		})
	}

	return news, nil
}
