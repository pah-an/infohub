package storage

import (
	"encoding/json"
	"os"

	"github.com/pah-an/infohub/internal/domain"
)

// FileCache реализует кэширование новостей в файл
type FileCache struct {
	filePath string
}

// NewFileCache создает новый файловый кэш
func NewFileCache(filePath string) *FileCache {
	return &FileCache{
		filePath: filePath,
	}
}

// SaveNews сохраняет новости в файл
func (f *FileCache) SaveNews(news domain.NewsList) error {
	data, err := json.MarshalIndent(news, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(f.filePath, data, 0644)
}

// GetLatestNews загружает новости из файла
func (f *FileCache) GetLatestNews(limit int) (domain.NewsList, error) {
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.NewsList{}, nil
		}
		return nil, err
	}

	var news domain.NewsList
	if err = json.Unmarshal(data, &news); err != nil {
		return nil, err
	}

	return news.LimitTo(limit), nil
}
