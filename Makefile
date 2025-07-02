
deps: install-tools
	@echo "✓ All dependencies installed successfully!"

buf:
	buf generate

versionup:
	git tag -a `cat VERSION`
	git push origin --tags
	git push

test:
	go fmt ./...
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck ./...
	go test -v ./...

# Install required Go tools
install-tools:
	@echo "Installing required Go tools..."
	go install golang.org/x/tools/cmd/goyacc@latest
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install honnef.co/go/tools/cmd/staticcheck
	@echo "✓ Go tools installed"
