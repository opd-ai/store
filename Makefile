.PHONY: help build run test docker-up docker-down docker-logs clean lint fmt vet install-deps generate-key rotate-key config-example

help:
	@echo "opd-ai/store development tasks"
	@echo ""
	@echo "Usage:"
	@echo "  make build           - Build the store binary"
	@echo "  make run             - Run the store server locally"
	@echo "  make test            - Run unit and integration tests"
	@echo "  make test-coverage   - Run tests with coverage report"
	@echo "  make docker-up       - Start Docker Compose services"
	@echo "  make docker-down     - Stop Docker Compose services"
	@echo "  make docker-logs     - View Docker logs"
	@echo "  make docker-build    - Build Docker image"
	@echo "  make lint            - Run linter (golangci-lint)"
	@echo "  make fmt             - Format code (gofmt)"
	@echo "  make vet             - Run go vet"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make install-deps    - Download dependencies"
	@echo "  make generate-key    - Generate a new encryption key"
	@echo "  make rotate-key      - Rotate encryption keys (requires OLD_KEY and NEW_KEY)"
	@echo "  make config-example  - Copy example config file to config.yaml"

build:
	@echo "Building store binary..."
	go build -o bin/store ./cmd/store

run: build
	@echo "Running store server..."
	./bin/store

test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

docker-build:
	@echo "Building Docker image..."
	docker build -f deployments/docker/Dockerfile -t opd-ai/store:latest .

docker-up:
	@echo "Starting Docker Compose services..."
	docker-compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Services started:"
	@docker-compose ps
	@echo ""
	@echo "API available at: http://localhost:8080"
	@echo "Paywall mock at: http://localhost:8081"

docker-down:
	@echo "Stopping Docker Compose services..."
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-test: docker-up
	@echo "Running integration tests with Docker..."
	go test -v ./test/...
	$(MAKE) docker-down

lint:
	@echo "Running linter..."
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...

vet:
	@echo "Running go vet..."
	go vet ./...

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean

install-deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

check: fmt lint vet test
	@echo "All checks passed!"

generate-key:
	@echo "Generating new encryption key..."
	@go run ./cmd/rotate-key -generate

rotate-key:
	@echo "Rotating encryption keys..."
	@if [ -z "$(NEW_KEY)" ]; then \
		echo "Error: NEW_KEY is required. Usage: make rotate-key NEW_KEY=<base64-key> [OLD_KEY=<base64-key>]"; \
		exit 1; \
	fi
	@go run ./cmd/rotate-key -old-key="$(OLD_KEY)" -new-key="$(NEW_KEY)"

config-example:
	@if [ -f config.yaml ]; then \
		echo "Warning: config.yaml already exists. Not overwriting."; \
		echo "To create a new config, delete config.yaml first or use: cp config.example.yaml config.yaml"; \
	else \
		cp config.example.yaml config.yaml; \
		echo "Created config.yaml from config.example.yaml"; \
		echo "Edit config.yaml to customize your configuration"; \
	fi
