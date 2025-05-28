package logger

import (
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger представляет настроенный логгер
type Logger struct {
	*logrus.Logger
}

// Config содержит конфигурацию логгера
type Config struct {
	Level      string `yaml:"level" json:"level"`
	Format     string `yaml:"format" json:"format"` // "json" или "text"
	Output     string `yaml:"output" json:"output"` // "stdout", "stderr" или путь к файлу
	TimeFormat string `yaml:"time_format" json:"time_format"`
}

// New создает новый настроенный логгер
func New(config Config) *Logger {
	logger := logrus.New()

	// Настройка уровня логирования
	level, err := logrus.ParseLevel(config.Level)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Настройка формата
	if config.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: getTimeFormat(config.TimeFormat),
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: getTimeFormat(config.TimeFormat),
			ForceColors:     true,
		})
	}

	// Настройка вывода
	switch config.Output {
	case "stderr":
		logger.SetOutput(os.Stderr)
	case "stdout", "":
		logger.SetOutput(os.Stdout)
	default:
		// Попытка открыть файл
		if file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			logger.SetOutput(file)
		} else {
			logger.SetOutput(os.Stdout)
			logger.Warnf("Failed to open log file %s, using stdout: %v", config.Output, err)
		}
	}

	return &Logger{Logger: logger}
}

// getTimeFormat возвращает формат времени по умолчанию или заданный
func getTimeFormat(format string) string {
	if format == "" {
		return time.RFC3339
	}
	return format
}

// WithField добавляет поле к логу
func (l *Logger) WithField(key string, value interface{}) *logrus.Entry {
	return l.Logger.WithField(key, value)
}

// WithFields добавляет несколько полей к логу
func (l *Logger) WithFields(fields logrus.Fields) *logrus.Entry {
	return l.Logger.WithFields(fields)
}

// WithError добавляет ошибку к логу
func (l *Logger) WithError(err error) *logrus.Entry {
	return l.Logger.WithError(err)
}

// WithComponent добавляет компонент к логу
func (l *Logger) WithComponent(component string) *logrus.Entry {
	return l.Logger.WithField("component", component)
}

// WithRequestID добавляет ID запроса к логу
func (l *Logger) WithRequestID(requestID string) *logrus.Entry {
	return l.Logger.WithField("request_id", requestID)
}

// Default возвращает логгер по умолчанию
func Default() *Logger {
	return New(Config{
		Level:      "info",
		Format:     "text",
		Output:     "stdout",
		TimeFormat: time.RFC3339,
	})
}
