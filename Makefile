
deps:
	go install honnef.co/go/tools/cmd/staticcheck

versionup:
	git tag -a `cat VERSION`
	git push origin --tags
	git push

test:
	go fmt ./...
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck ./...
	go test -v ./...
