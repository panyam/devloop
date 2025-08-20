
install:
	date > .LAST_BUILT_AT
	go build -o ${GOBIN}/devloop main.go

buf:
	buf generate

deps: install-tools
	@echo "✓ All dependencies installed successfully!"

versionup:
	git tag -a `cat VERSION` -m `cat VERSION`
	git push origin --tags
	git push

checks:
	go fmt ./...
	go vet ./...
	staticcheck ./...


test: checks
	go test ./...

# Test coverage targets
coverage:
	@echo "Running tests with coverage..."
	@mkdir -p reports
	go test ./... -coverprofile=reports/coverage.out
	@echo "Coverage data saved to reports/coverage.out"

coverage-html: coverage
	@echo "Generating HTML coverage report..."
	go tool cover -html=reports/coverage.out -o reports/coverage.html
	@echo "HTML coverage report saved to reports/coverage.html"

coverage-open: coverage-html
	@echo "Opening coverage report in browser..."
	@if command -v open >/dev/null 2>&1; then \
		open reports/coverage.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open reports/coverage.html; \
	else \
		echo "Please open reports/coverage.html in your browser"; \
	fi

# Specific test coverage targets
coverage-agent:
	@echo "Running agent tests with coverage..."
	@mkdir -p reports
	go test ./agent -coverprofile=reports/agent_coverage.out
	go tool cover -func=reports/agent_coverage.out | tail -1

coverage-agent-html: 
	@echo "Generating agent HTML coverage report..."
	@mkdir -p reports
	go test ./agent -coverprofile=reports/agent_coverage.out
	go tool cover -html=reports/agent_coverage.out -o reports/agent_coverage.html
	@echo "Agent HTML coverage report saved to reports/agent_coverage.html"

coverage-agent-open: coverage-agent-html
	@echo "Opening agent coverage report in browser..."
	@if command -v open >/dev/null 2>&1; then \
		open reports/agent_coverage.html; \
	elif command -v xdg-open >/dev/null 2>&1; then \
		xdg-open reports/agent_coverage.html; \
	else \
		echo "Please open reports/agent_coverage.html in your browser"; \
	fi

# LRO-specific test targets
test-lro:
	@echo "Running LRO Manager tests..."
	go test ./agent -run TestLROManager -v

test-scheduler:
	@echo "Running Scheduler integration tests..."
	go test ./agent -run TestScheduler -v

test-new-architecture: test-lro test-scheduler
	@echo "All new architecture tests completed"

# Coverage help target
coverage-help:
	@echo "Available coverage targets:"
	@echo "  make coverage           - Run all tests with coverage"
	@echo "  make coverage-html      - Generate HTML coverage report"
	@echo "  make coverage-open      - Generate and open HTML report in browser"
	@echo "  make coverage-agent     - Run agent tests with coverage summary"
	@echo "  make coverage-agent-html - Generate agent HTML coverage report"
	@echo "  make coverage-agent-open - Generate and open agent HTML report"
	@echo "  make coverage-summary   - Quick coverage summary for all packages"
	@echo "  make coverage-detailed  - Detailed coverage breakdown"
	@echo "  make coverage-clean     - Clean all coverage files"
	@echo ""
	@echo "All reports are saved to: reports/"

# Coverage summary targets
coverage-summary:
	@echo "Coverage Summary:"
	@mkdir -p reports
	@go test ./... -coverprofile=reports/coverage.out >/dev/null 2>&1
	@go tool cover -func=reports/coverage.out | tail -1
	@echo ""
	@echo "Agent Package Coverage:"
	@go test ./agent -coverprofile=reports/agent_coverage.out >/dev/null 2>&1
	@go tool cover -func=reports/agent_coverage.out | tail -1

coverage-detailed:
	@echo "Detailed Coverage Report:"
	@mkdir -p reports
	go test ./agent -coverprofile=reports/agent_coverage.out
	@echo ""
	@echo "Top covered functions:"
	@go tool cover -func=reports/agent_coverage.out | sort -k3 -nr | head -10
	@echo ""
	@echo "Low coverage functions (needs improvement):"
	@go tool cover -func=reports/agent_coverage.out | grep -E "0\.0%|[0-4][0-9]\..*%" | head -10

# Clean coverage files
coverage-clean:
	@echo "Cleaning coverage files..."
	rm -rf reports/
	@echo "Coverage files cleaned"

# Install required Go tools
install-tools:
	@echo "Installing required Go tools..."
	go install golang.org/x/tools/cmd/goyacc@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
	go get  google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go get honnef.co/go/tools/cmd/staticcheck
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "✓ Go tools installed"
