.PHONY: deps run clean

deps:
	@echo "Initializing Go modules for each service..."
	@cd shared && go mod init example.com/microservices/shared 2>/dev/null || true
	@cd auth-service && go mod init example.com/microservices/auth 2>/dev/null || true
	@cd auth-service && go mod edit -replace example.com/microservices/shared=../shared
	@cd auth-service && go mod tidy
	@cd user-service && go mod init example.com/microservices/user 2>/dev/null || true
	@cd user-service && go mod edit -replace example.com/microservices/shared=../shared
	@cd user-service && go mod tidy
	@cd api-gateway && go mod init example.com/microservices/gateway 2>/dev/null || true
	@cd api-gateway && go mod edit -replace example.com/microservices/shared=../shared
	@cd api-gateway && go mod tidy

run:
	@echo "Starting microservices with devloop..."
	@devloop -c .devloop.yaml

clean:
	@echo "Cleaning up..."
	@rm -f auth-service/auth-server
	@rm -f user-service/user-server
	@rm -f api-gateway/gateway