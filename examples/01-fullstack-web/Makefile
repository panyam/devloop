.PHONY: deps run clean

deps:
	@echo "Installing dependencies..."
	@cd backend && go mod init example.com/fullstack-backend 2>/dev/null || true
	@cd backend && go mod tidy
	@cd frontend && npm init -y 2>/dev/null || true
	@cd frontend && npm install --save-dev http-server

run:
	@echo "Starting devloop orchestrator..."
	@devloop -c .devloop.yaml

clean:
	@echo "Cleaning up..."
	@rm -f backend/main
	@rm -rf frontend/node_modules
	@rm -f docs/api.md