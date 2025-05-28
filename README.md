# InfoHub API

News aggregator app (technical assignment)
Сервис агрегации новостей из различных источников с REST API.

## Функции

- 📰 Сбор новостей из внешних источников
- ⚡ Кэширование с Redis/памятью
- 🔐 JWT аутентификация
- 📊 Prometheus метрики
- 🏥 Health checks
- 📖 Swagger документация

## Быстрый старт

### Локальный запуск

```bash
# Клонируем репозиторий
git clone https://github.com/pah-an/infohub.git
cd infohub

# Запускаем
go run cmd/infohub/main.go
```

### Docker

```bash
# Сборка и запуск
docker-compose up --build
```

## API Endpoints

| Метод | Путь | Описание |
|-------|------|----------|
| `GET` | `/api/v1/news` | Получить новости |
| `GET` | `/api/v1/healthz` | Проверка здоровья |
| `GET` | `/health` | Детальная проверка |
| `GET` | `/metrics` | Prometheus метрики |
| `GET` | `/swagger/` | API документация |
| `POST` | `/auth/login` | Авторизация |

### Пример использования

```bash
# Получить новости
curl "http://localhost:8080/api/v1/news?limit=10"

# С аутентификацией
curl -H "X-API-Key: your-key" "http://localhost:8080/api/v1/news"
```

## Конфигурация

Основные настройки в `configs/config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: "8080"

auth:
  enabled: true
  api_keys:
    "your-api-key": "Description"

redis:
  enabled: true
  address: "localhost:6379"

sources:
  - name: "Tech News"
    url: "https://api.example.com/news"
    interval: 30s
```

## Переменные окружения

- `CONFIG_PATH` - Путь к конфигу (по умолчанию: `configs/config.yaml`)
- `LOG_LEVEL` - Уровень логирования (`debug`, `info`, `warn`, `error`)
- `REDIS_ADDRESS` - Адрес Redis сервера

## Развертывание

### Docker 

```bash
docker build -t infohub .
docker run -p 8080:8080 -v $(pwd)/configs:/app/configs infohub
```

## Тестирование

```bash
# Юнит тесты
go test ./...

# С покрытием
go test -cover ./...
```

## Мониторинг

- **Метрики**: `/metrics` (Prometheus формат)
- **Health**: `/health` (статус сервисов)
- **Админка**: `/admin/` (веб интерфейс)

## Лицензия

MIT License
