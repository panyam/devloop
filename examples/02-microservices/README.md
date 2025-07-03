# Microservices Development Example

This example demonstrates devloop managing a microservices architecture with multiple Go services, shared libraries, and an API gateway.

## What's Included

- **API Gateway**: Routes requests to appropriate microservices (port 20201)
- **Auth Service**: Handles authentication and JWT tokens (port 20202)
- **User Service**: Manages user data and profiles (port 20203)
- **Shared Libraries**: Common utilities and types used across services

## Prerequisites

- Go 1.20 or higher
- devloop installed (`go install github.com/panyam/devloop@latest`)

## Quick Start

1. Install dependencies:
   ```bash
   make deps
   ```

2. Run with devloop:
   ```bash
   make run
   # Or directly: devloop -c .devloop.yaml
   ```

3. Test the services:
   ```bash
   # Health check
   curl http://localhost:20201/health

   # Get auth token
   curl -X POST http://localhost:20201/auth/login \
     -H "Content-Type: application/json" \
     -d '{"username":"demo","password":"demo123"}'

   # Get user info (requires token from above)
   curl http://localhost:20201/users/123 \
     -H "Authorization: Bearer <token>"
   ```

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Client    │────▶│ API Gateway │────▶│Auth Service │
│             │     │    :20201   │     │    :20202   │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                           ▼
                    ┌─────────────┐     ┌─────────────┐
                    │User Service │     │   Shared    │
                    │    :20203    │────▶│  Libraries  │
                    └─────────────┘     └─────────────┘
```

## Service Communication

- API Gateway routes `/auth/*` to Auth Service
- API Gateway routes `/users/*` to User Service
- Services communicate via HTTP/REST
- Shared libraries provide common types and utilities

## Try It Out

1. **Modify shared libraries** (`shared/types.go`):
   - Add a new field to the User struct
   - Watch all services rebuild automatically

2. **Update the auth service** (`auth-service/main.go`):
   - Change the JWT expiration time
   - Only the auth service restarts

3. **Add a new endpoint** to any service:
   - The specific service rebuilds
   - Other services remain running

## Development Tips

- Each service has its own log prefix for easy identification
- Services are started in dependency order (shared → services → gateway)
- Use `run_on_init: true` ensures all services start immediately
- Process groups ensure clean shutdown of all related processes

## Troubleshooting

- **Port conflicts**: Ensure ports 20201-20203 are available
- **Service discovery**: Services use hardcoded ports for simplicity
- **Compilation errors**: Check shared library changes don't break services
