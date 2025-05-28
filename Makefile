.PHONY: build run test clean mock-server deps swagger docker k8s monitor

# Variables
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT ?= $(shell git rev-parse HEAD)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
IMAGE_NAME ?= infohub
REGISTRY ?= ghcr.io/pah-an
NAMESPACE ?= infohub

# Build commands
build:
	@echo "Building InfoHub v$(VERSION)..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
		-ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)" \
		-o bin/infohub cmd/infohub/main.go

build-local:
	@echo "Building InfoHub for local development..."
	go build -ldflags "-X main.version=$(VERSION) -X main.gitCommit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)" \
		-o bin/infohub cmd/infohub/main.go

# Development commands
run:
	@echo "Running InfoHub..."
	go run cmd/infohub/main.go

run-dev:
	@echo "Running InfoHub in development mode..."
	LOG_LEVEL=debug go run cmd/infohub/main.go

mock-server:
	@echo "Starting mock news server..."
	go run tools/mock_server.go

# Dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download
	go mod verify

install-tools:
	@echo "Installing development tools..."
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2
	go install golang.org/x/vuln/cmd/govulncheck@latest

# Documentation
swagger:
	@echo "Generating Swagger documentation..."
	swag init -g cmd/infohub/main.go -o docs --parseInternal

# Testing
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test
	@echo "Generating coverage report..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-integration:
	@echo "Running integration tests..."
	go test -v -tags=integration ./tests/...

test-benchmark:
	@echo "Running benchmark tests..."
	go test -bench=. -benchmem ./...

test-all: test test-integration test-benchmark

# Code quality
lint:
	@echo "Running linter..."
	golangci-lint run --timeout=5m

security:
	@echo "Running security checks..."
	govulncheck ./...

format:
	@echo "Formatting code..."
	go fmt ./...

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) .

docker-build-multiarch:
	@echo "Building multi-architecture Docker image..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		-t $(REGISTRY)/$(IMAGE_NAME):latest \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--push .

docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 --rm $(IMAGE_NAME):$(VERSION)

docker-compose-up:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

docker-compose-down:
	@echo "Stopping services..."
	docker-compose down

docker-compose-logs:
	@echo "Showing logs..."
	docker-compose logs -f

# Release commands
release-patch:
	@echo "Creating patch release..."
	./scripts/release.sh patch

release-minor:
	@echo "Creating minor release..."
	./scripts/release.sh minor

release-major:
	@echo "Creating major release..."
	./scripts/release.sh major

# Clean commands
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f news_cache.json
	rm -f coverage.out coverage.html
	rm -rf docs/swagger.json docs/swagger.yaml

clean-docker:
	@echo "Cleaning Docker resources..."
	docker system prune -f
	docker image prune -f

clean-all: clean clean-docker

# Utility commands
version:
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

health-check:
	@echo "Checking service health..."
	curl -f http://localhost:8080/health || exit 1

api-test:
	@echo "Testing API endpoints..."
	curl -s http://localhost:8080/api | jq .
	curl -s http://localhost:8080/api/v1/healthz | jq .

# Help
help:
	@echo "InfoHub Makefile"
	@echo ""
	@echo "Development Commands:"
	@echo "  run             - Run the application"
	@echo "  run-dev         - Run in development mode"
	@echo "  mock-server     - Start mock news server"
	@echo ""
	@echo "Build Commands:"
	@echo "  build           - Build production binary"
	@echo "  build-local     - Build for local development"
	@echo "  docker-build    - Build Docker image"
	@echo "  swagger         - Generate API documentation"
	@echo ""
	@echo "Testing Commands:"
	@echo "  test            - Run unit tests"
	@echo "  test-integration - Run integration tests"
	@echo "  test-benchmark  - Run benchmark tests"
	@echo "  lint            - Run code linter"
	@echo "  security        - Run security checks"
	@echo ""
	@echo "Docker Commands:"
	@echo "  docker-compose-up   - Start all services"
	@echo "  docker-compose-down - Stop all services"
	@echo "  docker-run          - Run in Docker container"
	@echo ""
	@echo "Utility Commands:"
	@echo "  version         - Show version info"
	@echo "  health-check    - Check service health"
	@echo "  clean           - Clean build artifacts"
	@echo "  help            - Show this help"

# Default target
.DEFAULT_GOAL := help
