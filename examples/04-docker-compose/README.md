# Docker Compose Development Example

This example demonstrates using `devloop` to orchestrate a multi-container development environment with Docker Compose, including databases, web services, and background workers.

## What's Included

- **PostgreSQL Database**: Persistent database with volume mounting
- **Redis Cache**: In-memory caching and message broker
- **API Server**: Go REST API with database migrations
- **Background Worker**: Python task processor using Celery
- **Web Frontend**: React application with hot reloading
- **Nginx Proxy**: Load balancer and static file serving

## Prerequisites

- Docker and Docker Compose installed
- devloop installed (`go install github.com/panyam/devloop@latest`)
- Node.js 16+ (for local frontend development)
- Go 1.20+ (for local API development)

## Quick Start

1. **Start the environment:**
   ```bash
   make run
   # Or directly: devloop -c .devloop.yaml
   ```

2. **Access services:**
   - Frontend: http://localhost:3000
   - API: http://localhost:8080/api
   - Database: localhost:5432 (postgres/postgres)
   - Redis: localhost:6379

## Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    React    │───▶│    Nginx    │───▶│   Go API    │
│   Frontend  │    │   Proxy     │    │   Server    │
│   :3000     │    │   :8080     │    │   :8000     │
└─────────────┘    └─────────────┘    └──────┬──────┘
                                              │
┌─────────────┐    ┌─────────────┐           │
│   Python    │◀───│    Redis    │◀──────────┘
│   Worker    │    │   Cache     │
│   (Celery)  │    │   :6379     │
└─────────────┘    └─────────────┘
                           │
                   ┌─────────────┐
                   │ PostgreSQL  │
                   │  Database   │
                   │   :5432     │
                   └─────────────┘
```

## Development Workflow

### 1. Container Management
- **Docker Compose changes**: Rebuilds and restarts affected services
- **Dockerfile changes**: Rebuilds specific container images
- **Environment changes**: Restarts containers with new configuration

### 2. Application Development
- **Go API changes**: Rebuilds API container and restarts service
- **React frontend changes**: Hot reloads through webpack dev server
- **Python worker changes**: Restarts Celery worker processes
- **Database migrations**: Automatically applied when schema changes

### 3. Configuration Updates
- **Nginx config changes**: Reloads proxy configuration
- **Docker network changes**: Recreates network infrastructure
- **Volume mount changes**: Remounts development volumes

## Try It Out

1. **Modify the API** (`api/main.go`):
   - Add a new endpoint
   - Watch Docker rebuild the API container
   - Test the endpoint at http://localhost:8080/api

2. **Update the frontend** (`frontend/src/App.js`):
   - Change the UI components
   - See instant hot reload in the browser
   - No container restart needed

3. **Add a background task** (`worker/tasks.py`):
   - Create a new Celery task
   - Worker container restarts automatically
   - Test task execution through API

4. **Modify database schema** (`migrations/003_add_table.sql`):
   - Add a new migration file
   - Migration runs automatically in database container

## Container Services

### API Server (Go)
- REST API with CRUD operations
- Database connection pooling
- Redis session storage
- Health check endpoints

### Frontend (React)
- Modern React with hooks
- Material-UI components
- Hot reloading for development
- Production build optimization

### Background Worker (Python/Celery)
- Async task processing
- Redis as message broker
- Email notifications
- File processing tasks

### Database (PostgreSQL)
- Persistent data storage
- Automatic migration system
- Development seed data
- Connection pooling

### Cache (Redis)
- Session storage
- Application caching
- Message queue for workers
- Real-time features

### Proxy (Nginx)
- Load balancing
- Static file serving
- SSL termination
- Request routing

## Environment Variables

```bash
# Database
POSTGRES_DB=devloop_app
POSTGRES_USER=postgres
POSTGRES_PASSWORD=postgres

# Redis
REDIS_URL=redis://redis:6379/0

# API
API_PORT=8000
JWT_SECRET=dev-secret-key

# Worker
CELERY_BROKER_URL=redis://redis:6379/0
CELERY_RESULT_BACKEND=redis://redis:6379/0
```

## Volume Mounts

```yaml
volumes:
  - ./api:/app/api:rw          # API source code
  - ./frontend:/app/frontend:rw # Frontend source code  
  - ./worker:/app/worker:rw     # Worker source code
  - ./nginx:/etc/nginx:ro       # Nginx configuration
  - ./data:/var/lib/postgresql  # Database persistence
```

## Development Commands

```bash
# View logs from all services
docker-compose logs -f

# Shell into API container
docker-compose exec api sh

# Run database migrations
docker-compose exec api go run migrate.go

# Scale worker processes
docker-compose up --scale worker=3

# Rebuild specific service
docker-compose build api

# Reset database
docker-compose down -v && docker-compose up
```

## Production Considerations

### 1. Security
- Remove development secrets
- Use proper SSL certificates
- Implement proper authentication
- Secure container networking

### 2. Performance
- Use multi-stage Docker builds
- Optimize container images
- Configure resource limits
- Implement health checks

### 3. Monitoring
- Add logging aggregation
- Implement metrics collection
- Set up alerting
- Monitor container health

## Troubleshooting

**Port conflicts:**
```bash
# Check what's using ports
lsof -i :3000 -i :8080 -i :5432 -i :6379
# Kill conflicting processes
docker-compose down
```

**Container build failures:**
- Check Dockerfile syntax
- Verify base image availability
- Review build context size
- Check network connectivity

**Database connection issues:**
- Verify PostgreSQL is running
- Check connection string
- Review database logs
- Confirm network connectivity

**Frontend not updating:**
- Check if hot reload is enabled
- Verify volume mounts
- Review webpack configuration
- Clear browser cache

## Extensions

### Adding New Services
```yaml
# Add to docker-compose.yml
elasticsearch:
  image: elasticsearch:7.14.0
  environment:
    - discovery.type=single-node
  ports:
    - "9200:9200"
```

### Custom Dockerfile
```dockerfile
# Add to api/Dockerfile
FROM golang:1.20-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o api main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/api .
CMD ["./api"]
```

### Additional Workers
```bash
# Scale specific services
docker-compose up --scale worker=5 --scale api=2
```

## Next Steps

- Explore Kubernetes deployment
- Add service mesh (Istio)
- Implement distributed tracing
- Set up CI/CD pipeline with containers