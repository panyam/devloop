# OpenTelemetry Integration Example Makefile

.PHONY: help install-otel check-deps clean test

help:
	@echo "Available commands:"
	@echo "  install-otel  - Install OpenTelemetry Collector"
	@echo "  check-deps    - Check if required dependencies are installed"
	@echo "  clean         - Clean up generated files and logs"
	@echo "  test          - Test the integration (requires otelcol)"

# Install OpenTelemetry Collector
install-otel:
	@echo "Installing OpenTelemetry Collector..."
	@if command -v brew >/dev/null 2>&1; then \
		echo "Using Homebrew..."; \
		brew install opentelemetry-collector; \
	else \
		echo "Downloading binary..."; \
		curl -L -o otelcol \
			https://github.com/open-telemetry/opentelemetry-collector-releases/releases/latest/download/otelcol_$$(uname -s | tr '[:upper:]' '[:lower:]')_$$(uname -m); \
		chmod +x otelcol; \
		sudo mv otelcol /usr/local/bin/; \
	fi
	@echo "OpenTelemetry Collector installed successfully"

# Check dependencies
check-deps:
	@echo "Checking dependencies..."
	@command -v otelcol >/dev/null 2>&1 || { echo "❌ otelcol not found. Run 'make install-otel'"; exit 1; }
	@command -v go >/dev/null 2>&1 || { echo "❌ Go not found"; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "❌ Node.js not found"; exit 1; }
	@command -v python3 >/dev/null 2>&1 || { echo "❌ Python 3 not found"; exit 1; }
	@command -v devloop >/dev/null 2>&1 || { echo "❌ devloop not found"; exit 1; }
	@echo "✅ All dependencies found"

# Clean up
clean:
	@echo "Cleaning up..."
	@rm -rf backend/main
	@rm -rf frontend/node_modules
	@rm -rf otel-logs
	@rm -rf logs
	@echo "Cleanup complete"

# Test the integration
test: check-deps
	@echo "Testing OpenTelemetry integration..."
	@echo "This will:"
	@echo "1. Start the OTel Collector"
	@echo "2. Start backend, frontend, and worker services"
	@echo "3. Show telemetry output in the console"
	@echo ""
	@echo "Press Ctrl+C to stop all services"
	@echo ""
	@echo "Starting devloop..."
	devloop