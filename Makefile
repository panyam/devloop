
deps: install-tools
	@echo "✓ All dependencies installed successfully!"

install:
	go install main.go

buf:
	buf generate

versionup:
	git tag -a `cat VERSION` -m `cat VERSION`
	git push origin --tags
	git push

checks:
	go fmt ./...
	go vet ./...
	staticcheck ./...

test: checks testv1 testv2

testv1: checks
	DEVLOOP_ORCHESTRATOR_VERSION=v1 go test -v ./...

testv2: checks
	DEVLOOP_ORCHESTRATOR_VERSION=v2 go test -v ./...

# Install required Go tools
install-tools:
	@echo "Installing required Go tools..."
	go install golang.org/x/tools/cmd/goyacc@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install honnef.co/go/tools/cmd/staticcheck
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
	@echo "✓ Go tools installed"
