package dagbump

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A repo that commits a go.work puts go build in workspace mode, where the go
// command refuses -mod=mod. Update and Release run their build check with
// goEnv, so it has to work there too.
func TestGoEnvBuildsInARepoWithGoWork(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	write("go.mod", "module example.com/withwork\n\ngo 1.21\n")
	write("go.work", "go 1.21\n\nuse .\n")
	write("main.go", "package main\n\nfunc main() {}\n")

	var out bytes.Buffer
	r := &Runner{Out: &out, Env: (&Graph{}).goEnv(), Base: dir}
	if err := r.Step(dir, "go build ./..."); err != nil {
		t.Fatalf("go build with goEnv in a repo with go.work: %v\n%s", err, out.String())
	}
}
