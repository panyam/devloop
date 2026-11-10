package dagbump

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lib publishes @acme/lib 1.1.0; app (an npm project) locks it at 1.0.0.
func toolsFixture(t *testing.T, appFiles map[string]string) *Manifest {
	root := t.TempDir()
	repo(t, root, "lib", map[string]string{"package.json": `{"name": "@acme/lib", "version": "1.1.0"}` + "\n"}, "v1.1.0")
	files := map[string]string{
		"package.json":      `{"name": "app", "private": true, "dependencies": {"@acme/lib": "^1.0.0"}}` + "\n",
		"package-lock.json": `{"packages": {"node_modules/@acme/lib": {"version": "1.0.0"}}}` + "\n",
	}
	for k, v := range appFiles {
		files[k] = v
	}
	repo(t, root, "app", files)
	return manifestFor(t, root)
}

func appPkg(t *testing.T, g *Graph) *NpmPackage {
	t.Helper()
	u, err := g.Get("app")
	must(t, err)
	return u.Npm[0]
}

func TestToolPrecedence(t *testing.T) {
	m := toolsFixture(t, nil)
	g, err := BuildGraph(m)
	must(t, err)
	if n := appPkg(t, g); n.Manager != "npm" || n.ManagerFrom != "lockfile" {
		t.Errorf("lockfile: %s from %s", n.Manager, n.ManagerFrom)
	}
	app, _ := g.Get("app")
	out := &bytes.Buffer{}
	_, err = g.Update(app, &Runner{DryRun: true, Out: out}, UpdateOptions{})
	must(t, err)
	if !strings.Contains(out.String(), "npm install '@acme/lib@^1.1.0' --save-prod") {
		t.Errorf("update steps:\n%s", out)
	}

	// packageManager beats the lockfile, and the mismatch is reported.
	m = toolsFixture(t, map[string]string{"package.json": `{"name": "app", "private": true, "packageManager": "pnpm@10.0.0", "dependencies": {"@acme/lib": "^1.0.0"}}` + "\n"})
	g, err = BuildGraph(m)
	must(t, err)
	if n := appPkg(t, g); n.Manager != "pnpm" || n.ManagerFrom != "packageManager" {
		t.Errorf("packageManager: %s from %s", n.Manager, n.ManagerFrom)
	}
	app, _ = g.Get("app")
	if w := strings.Join(app.Project.Warnings, ";"); !strings.Contains(w, "lockfile is npm's") {
		t.Errorf("no mismatch warning: %s", w)
	}
}

func TestToolEnforcement(t *testing.T) {
	m := toolsFixture(t, nil)
	m.Tools = ToolsConfig{Npm: "pnpm", Enforce: true}
	g, err := BuildGraph(m)
	must(t, err)
	app, _ := g.Get("app")
	if _, err := g.Update(app, &Runner{DryRun: true, Out: &bytes.Buffer{}}, UpdateOptions{}); err == nil ||
		!strings.Contains(err.Error(), "enforces pnpm") {
		t.Fatalf("update under enforcement: err = %v", err)
	}

	// The repo opts out by naming its tool in .dagbump.yaml.
	m = toolsFixture(t, map[string]string{RepoConfigFile: "tools: {npm: npm}\n"})
	m.Tools = ToolsConfig{Npm: "pnpm", Enforce: true}
	g, err = BuildGraph(m)
	must(t, err)
	if n := appPkg(t, g); n.PolicyErr != "" || n.ManagerFrom != RepoConfigFile {
		t.Errorf("opt-out: %s from %s, policy %q", n.Manager, n.ManagerFrom, n.PolicyErr)
	}

	// Without a lockfile or declaration, the manifest default applies.
	root := t.TempDir()
	repo(t, root, "bare", map[string]string{"package.json": `{"name": "bare", "version": "0.1.0"}` + "\n"}, "v0.1.0")
	m = manifestFor(t, root)
	m.Tools = ToolsConfig{Npm: "pnpm"}
	g, err = BuildGraph(m)
	must(t, err)
	u, _ := g.Get("bare")
	if n := u.Npm[0]; n.Manager != "pnpm" || n.ManagerFrom != "manifest" {
		t.Errorf("default: %s from %s", n.Manager, n.ManagerFrom)
	}
}

func TestSetPackageVersion(t *testing.T) {
	f := filepath.Join(t.TempDir(), "package.json")
	in := "{\n    \"name\": \"x\",\n    \"version\": \"0.7.7\",\n    \"dependencies\": {\"y\": \"workspace:*\", \"version\": \"1\"}\n}\n"
	must(t, os.WriteFile(f, []byte(in), 0o644))
	must(t, setPackageVersion(f, "0.8.0"))
	got, _ := os.ReadFile(f)
	if want := strings.Replace(in, "0.7.7", "0.8.0", 1); string(got) != want {
		t.Errorf("got:\n%s", got)
	}
}
