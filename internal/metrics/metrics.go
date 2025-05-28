package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics содержит все метрики приложения
type Metrics struct {
	// HTTP метрики
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight *prometheus.GaugeVec

	// News collection метрики
	NewsCollected          *prometheus.CounterVec
	NewsCollectionErrors   *prometheus.CounterVec
	NewsCollectionDuration *prometheus.HistogramVec
	SourcesActive          *prometheus.GaugeVec
	NewsCacheSize          prometheus.Gauge

	// System метрики
	ApplicationInfo *prometheus.GaugeVec
	StartTime       prometheus.Gauge
}

// New создает новый набор метрик
func New(namespace, subsystem string) *Metrics {
	m := &Metrics{
		// HTTP метрики
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_requests_total",
				Help:      "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status_code"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		HTTPRequestsInFlight: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "http_requests_in_flight",
				Help:      "Number of HTTP requests currently being served",
			},
			[]string{"method", "endpoint"},
		),

		// News collection метрики
		NewsCollected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "news_collected_total",
				Help:      "Total number of news articles collected",
			},
			[]string{"source"},
		),
		NewsCollectionErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "news_collection_errors_total",
				Help:      "Total number of news collection errors",
			},
			[]string{"source", "error_type"},
		),
		NewsCollectionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "news_collection_duration_seconds",
				Help:      "News collection duration in seconds",
				Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
			},
			[]string{"source"},
		),
		SourcesActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "sources_active",
				Help:      "Number of active news sources",
			},
			[]string{"status"}, // "healthy", "error", "timeout"
		),
		NewsCacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "news_cache_size",
				Help:      "Number of news articles in cache",
			},
		),

		// System метрики
		ApplicationInfo: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "info",
				Help:      "Application information",
			},
			[]string{"version", "go_version", "git_commit"},
		),
		StartTime: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "start_time_seconds",
				Help:      "Application start time in seconds since Unix epoch",
			},
		),
	}

	// Устанавливаем время запуска
	m.StartTime.SetToCurrentTime()

	return m
}

// Register регистрирует все метрики в Prometheus
func (m *Metrics) Register() error {
	collectors := []prometheus.Collector{
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPRequestsInFlight,
		m.NewsCollected,
		m.NewsCollectionErrors,
		m.NewsCollectionDuration,
		m.SourcesActive,
		m.NewsCacheSize,
		m.ApplicationInfo,
		m.StartTime,
	}

	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// RecordHTTPRequest записывает метрики HTTP запроса
func (m *Metrics) RecordHTTPRequest(method, endpoint string, statusCode int, duration time.Duration) {
	m.HTTPRequestsTotal.WithLabelValues(method, endpoint, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

// RecordNewsCollected записывает метрику собранных новостей
func (m *Metrics) RecordNewsCollected(source string, count int) {
	m.NewsCollected.WithLabelValues(source).Add(float64(count))
}

// RecordNewsCollectionError записывает ошибку сбора новостей
func (m *Metrics) RecordNewsCollectionError(source, errorType string) {
	m.NewsCollectionErrors.WithLabelValues(source, errorType).Inc()
}

// RecordNewsCollectionDuration записывает время сбора новостей
func (m *Metrics) RecordNewsCollectionDuration(source string, duration time.Duration) {
	m.NewsCollectionDuration.WithLabelValues(source).Observe(duration.Seconds())
}

// SetSourceStatus устанавливает статус источника
func (m *Metrics) SetSourceStatus(status string, count int) {
	m.SourcesActive.WithLabelValues(status).Set(float64(count))
}

// SetNewsCacheSize устанавливает размер кэша новостей
func (m *Metrics) SetNewsCacheSize(size int) {
	m.NewsCacheSize.Set(float64(size))
}

// SetApplicationInfo устанавливает информацию о приложении
func (m *Metrics) SetApplicationInfo(version, goVersion, gitCommit string) {
	m.ApplicationInfo.WithLabelValues(version, goVersion, gitCommit).Set(1)
}

// Handler возвращает HTTP handler для метрик
func Handler() http.Handler {
	return promhttp.Handler()
}

// HTTPMiddleware создает middleware для записи HTTP метрик
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		endpoint := r.URL.Path

		m.HTTPRequestsInFlight.WithLabelValues(r.Method, endpoint).Inc()
		defer m.HTTPRequestsInFlight.WithLabelValues(r.Method, endpoint).Dec()

		ww := &statusWriter{ResponseWriter: w, statusCode: 200}

		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		m.RecordHTTPRequest(r.Method, endpoint, ww.statusCode, duration)
	})
}

// statusWriter для захвата статус кода
type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
