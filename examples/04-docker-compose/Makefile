.PHONY: run deps build down logs clean test help

# Default target
run: deps
	@echo "Starting Docker Compose development environment with devloop..."
	devloop -c .devloop.yaml

# Install dependencies and prepare environment
deps:
	@echo "Setting up Docker Compose development environment..."
	@if ! command -v docker-compose >/dev/null 2>&1; then \
		echo "Error: docker-compose is not installed"; \
		exit 1; \
	fi
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Error: docker is not installed"; \
		exit 1; \
	fi
	@echo "Creating required directories..."
	@mkdir -p data logs migrations api frontend worker nginx config
	@echo "Creating sample environment file..."
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@echo "Building initial containers..."
	docker-compose build
	@echo "Dependencies installed successfully!"

# Build all containers
build:
	@echo "Building all Docker containers..."
	docker-compose build

# Start services without devloop
start:
	@echo "Starting services..."
	docker-compose up -d
	@echo "Services started. Access at:"
	@echo "  Frontend: http://localhost:3000"
	@echo "  API: http://localhost:8080/api"
	@echo "  Database: localhost:5432"

# Stop all services
down:
	@echo "Stopping all services..."
	docker-compose down

# Stop and remove all data
clean:
	@echo "Stopping services and removing all data..."
	docker-compose down -v --remove-orphans
	docker system prune -f

# View logs from all services
logs:
	docker-compose logs -f

# Run tests
test:
	@echo "Running tests..."
	docker-compose exec -T api go test ./...
	docker-compose exec -T worker python -m pytest tests/
	@echo "All tests completed!"

# Health check
health:
	@echo "Checking service health..."
	@curl -f http://localhost:8080/health || echo "API health check failed"
	@curl -f http://localhost:3000 || echo "Frontend health check failed"
	docker-compose exec -T db pg_isready -U postgres || echo "Database health check failed"
	docker-compose exec -T redis redis-cli ping || echo "Redis health check failed"

# Show help
help:
	@echo "Available commands:"
	@echo "  make run     - Start development environment with devloop"
	@echo "  make deps    - Install dependencies and setup environment"
	@echo "  make build   - Build all Docker containers"
	@echo "  make start   - Start services without devloop"
	@echo "  make down    - Stop all services"
	@echo "  make clean   - Stop services and remove all data"
	@echo "  make logs    - View logs from all services"
	@echo "  make test    - Run all tests"
	@echo "  make health  - Check service health"
	@echo "  make help    - Show this help message"