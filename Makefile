
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
