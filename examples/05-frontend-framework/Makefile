.PHONY: run deps build clean test lint format help dev

# Default target
run: deps
	@echo "Starting frontend framework development environment with devloop..."
	devloop -c .devloop.yaml

# Install all dependencies
deps:
	@echo "Installing dependencies for all workspaces..."
	@if ! command -v node >/dev/null 2>&1; then \
		echo "Error: Node.js is not installed"; \
		exit 1; \
	fi
	@echo "Installing root dependencies..."
	npm install
	@echo "Installing workspace dependencies..."
	npm run install:all
	@echo "All dependencies installed successfully!"

# Development without devloop
dev:
	@echo "Starting all development servers manually..."
	@echo "This will start React, Vue, and Storybook in parallel"
	npm run dev

# Build all projects
build:
	@echo "Building all projects..."
	npm run build:all
	@echo "All projects built successfully!"

# Clean all build artifacts and dependencies
clean:
	@echo "Cleaning all build artifacts and dependencies..."
	rm -rf node_modules
	rm -rf react-app/{node_modules,dist,build}
	rm -rf vue-components/{node_modules,dist}
	rm -rf storybook/{node_modules,storybook-static}
	rm -rf shared/node_modules
	rm -rf logs
	@echo "Clean completed!"

# Run all tests
test:
	@echo "Running all tests..."
	npm run test:all
	@echo "All tests completed!"

# Run linting
lint:
	@echo "Running linters..."
	npm run lint:all
	@echo "Linting completed!"

# Format all code
format:
	@echo "Formatting all code..."
	npm run format:all
	@echo "Formatting completed!"

# Type check all TypeScript
type-check:
	@echo "Running TypeScript type checking..."
	npm run type-check:all
	@echo "Type checking completed!"

# Install specific workspace
install-react:
	cd react-app && npm install

install-vue:
	cd vue-components && npm install

install-storybook:
	cd storybook && npm install

install-shared:
	cd shared && npm install

# Build specific projects
build-react:
	cd react-app && npm run build

build-vue:
	cd vue-components && npm run build

build-storybook:
	cd storybook && npm run build

# Development for specific projects
dev-react:
	cd react-app && npm run dev

dev-vue:
	cd vue-components && npm run dev

dev-storybook:
	cd storybook && npm run dev

# Check project health
health:
	@echo "Checking project health..."
	@echo "Node version: $(shell node --version)"
	@echo "NPM version: $(shell npm --version)"
	@echo "Checking package.json files..."
	@find . -name "package.json" -not -path "*/node_modules/*" | wc -l | xargs echo "Found package.json files:"
	@echo "Checking for common issues..."
	@if [ ! -d "node_modules" ]; then echo "⚠️  Root node_modules missing - run 'make deps'"; fi
	@if [ ! -d "react-app/node_modules" ]; then echo "⚠️  React app dependencies missing"; fi
	@if [ ! -d "vue-components/node_modules" ]; then echo "⚠️  Vue components dependencies missing"; fi
	@if [ ! -d "storybook/node_modules" ]; then echo "⚠️  Storybook dependencies missing"; fi
	@echo "Health check completed!"

# Show help
help:
	@echo "Available commands:"
	@echo "  make run         - Start development environment with devloop"
	@echo "  make deps        - Install all dependencies"
	@echo "  make dev         - Start all dev servers manually (without devloop)"
	@echo "  make build       - Build all projects"
	@echo "  make clean       - Clean all build artifacts and dependencies"
	@echo "  make test        - Run all tests"
	@echo "  make lint        - Run all linters"
	@echo "  make format      - Format all code"
	@echo "  make type-check  - Run TypeScript type checking"
	@echo "  make health      - Check project health"
	@echo ""
	@echo "Workspace-specific commands:"
	@echo "  make install-{react,vue,storybook,shared} - Install specific workspace deps"
	@echo "  make build-{react,vue,storybook}          - Build specific project"
	@echo "  make dev-{react,vue,storybook}            - Start specific dev server"
	@echo ""
	@echo "  make help        - Show this help message"