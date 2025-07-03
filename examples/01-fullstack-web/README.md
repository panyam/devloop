# Full-Stack Web Application Example

This example demonstrates devloop orchestrating a typical full-stack web development environment with multiple components running in parallel.

## What's Included

- **Go Backend API**: Simple TODO API with GET and POST endpoints
- **Frontend**: Vanilla JavaScript single-page application
- **Database Migrations**: SQL migration scripts that run on changes
- **API Documentation**: Auto-generated docs from templates

## Prerequisites

- Go 1.20 or higher
- Node.js 14 or higher
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

3. Access the application:
   - Frontend: http://localhost:3000
   - Backend API: http://localhost:20202/api/todos

## What to Expect

When you run `make run`, devloop will start all services immediately:

```
[devloop] Starting orchestrator...
[devloop] Executing rule "backend" on initialization (run_on_init: true)
[devloop] Executing rule "frontend" on initialization (run_on_init: true)
[devloop] Executing rule "db" on initialization (run_on_init: true)
[devloop] Executing rule "docs" on initialization (run_on_init: true)
[backend]   Building Go server...
[backend]   Starting server on http://localhost:20202
[frontend]  Starting web server...
[db]        Checking for pending migrations...
[docs]      Generating API documentation...
```

Devloop will continue monitoring for file changes and automatically rebuild/restart services as needed.

## Try It Out

1. **Modify the backend** (`backend/main.go`):
   - Add a new field to the TODO struct
   - Watch devloop rebuild and restart the server

2. **Update the frontend** (`frontend/app.js`):
   - Change the UI or add a feature
   - See instant updates without manual restart

3. **Add a migration** (`migrations/002_add_field.sql`):
   - Create a new SQL file
   - Watch the migration runner execute automatically

4. **Update API docs** (`docs/api_template.md`):
   - Modify the template
   - See regenerated documentation

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Frontend  │────▶│   Backend   │────▶│  Database   │
│  localhost  │     │     API     │     │ (in-memory) │
│    :3000    │     │    :20202   │     │             │
└─────────────┘     └─────────────┘     └─────────────┘
```

## Key Files

- `.devloop.yaml` - Orchestration configuration
- `backend/main.go` - Go API server
- `frontend/app.js` - JavaScript application
- `migrations/*.sql` - Database migrations

## Troubleshooting

- **Port already in use**: Kill existing processes or change ports in the code
- **Dependencies missing**: Run `make deps` again
- **Changes not detected**: Check glob patterns in `.devloop.yaml`
