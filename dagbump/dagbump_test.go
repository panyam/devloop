package dagbump

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteID(t *testing.T) {
	for in, want := range map[string]string{
		"git@github.com:panyam/mcpkit.git":      "github.com/panyam/mcpkit",
		"https://github.com/panyam/Agni.git":    "github.com/panyam/agni",
		"ssh://git@github.com/panyam/tlex":      "github.com/panyam/tlex",
		"https://github.com/panyam/goutils/":    "github.com/panyam/goutils",
		"https://gitlab.com/group/sub/proj.git": "gitlab.com/group/sub/proj",
	} {
		if got := RemoteID(in); got != want {
			t.Errorf("RemoteID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextVersion(t *testing.T) {
	cases := []struct{ v, bump, want string }{
		{"v0.1.14", "patch", "v0.1.15"},
		{"v0.1.14", "minor", "v0.2.0"},
		{"v0.1.14", "major", "v1.0.0"},
		{"v0.2.11-rc1", "patch", "v0.2.11"},
		{"v0.2.11-rc1", "minor", "v0.3.0"},
		{"v0.3.0-rc1", "minor", "v0.3.0"},
	}
	for _, c := range cases {
		got, err := NextVersion(c.v, c.bump)
		if err != nil || got != c.want {
			t.Errorf("NextVersion(%s, %s) = %s, %v; want %s", c.v, c.bump, got, err, c.want)
		}
	}
	if _, err := NextVersion("v1.0.0", "huge"); err == nil {
		t.Error("expected an error for an unknown bump")
	}
}

func TestGlobMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, dir string
		want         bool
	}{
		{".", "", true}, {".", "ext", false}, {"ext/*", "ext/auth", true}, {"ext/*", "ext/auth/go", false},
		{"ext/**", "ext/auth/go", true}, {"experimental/**", "experimental/ext/rooms", true}, {"cmd/common", "cmd/common", true},
	} {
		if got := globMatch(c.pattern, c.dir); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v", c.pattern, c.dir, got)
		}
	}
}

// repo creates root/name/main as a git repo with the files committed and
// the tags placed on that commit.
func repo(t *testing.T, root, name string, files map[string]string, tags ...string) string {
	t.Helper()
	dir := filepath.Join(root, name, "main")
	for f, body := range files {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644))
	}
	sh(t, dir, "git init -q -b main && git remote add origin git@github.com:acme/"+name+".git && git add -A && git commit -q -m init")
	for _, tag := range tags {
		sh(t, dir, "git tag "+tag)
	}
	return dir
}

func commitFile(t *testing.T, dir, file, msg string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, file), []byte(msg), 0o644))
	sh(t, dir, "git add -A && git commit -q -m "+shellQuote(msg))
}

func manifestFor(t *testing.T, root string) *Manifest {
	t.Helper()
	m := &Manifest{path: filepath.Join(root, "manifest.yaml"), IgnoreCommits: defaultIgnoreCommits}
	cands, err := Discover([]string{root}, []string{"acme"}, m)
	must(t, err)
	m.Merge(cands)
	return m
}

func gomod(path string, requires ...string) string {
	s := "module " + path + "\n\ngo 1.22\n"
	for _, r := range requires {
		s += "\nrequire " + r + "\n"
	}
	return s
}

// base <- mid <- app. base has a commit after v0.1.0; app has no tags.
func cascadeFixture(t *testing.T) *Manifest {
	root := t.TempDir()
	base := repo(t, root, "base", map[string]string{"go.mod": gomod("example.com/base")}, "v0.1.0")
	commitFile(t, base, "f.go", "feat: new api")
	repo(t, root, "mid", map[string]string{"go.mod": gomod("example.com/mid", "example.com/base v0.1.0")}, "v0.2.0")
	repo(t, root, "app", map[string]string{"go.mod": gomod("example.com/app", "example.com/mid v0.2.0")})
	return manifestFor(t, root)
}

func TestPlanCascade(t *testing.T) {
	m := cascadeFixture(t)
	g, err := BuildGraph(m)
	must(t, err)
	base, _ := g.Get("base")

	plan, err := g.Plan(PlanOptions{Seeds: []*Unit{base}})
	must(t, err)
	if got := planString(plan); got != "1:base:release 2:mid:release 3:app:update" {
		t.Fatalf("plan = %q", got)
	}

	// Release base locally (no push); the plan then shows mid lagging.
	rp, err := g.Release(base, &Runner{Out: &bytes.Buffer{}}, ReleaseOptions{Bump: "minor", NoPush: true})
	must(t, err)
	if rp.Version != "v0.2.0" {
		t.Fatalf("released %s, want v0.2.0", rp.Version)
	}
	g, err = BuildGraph(m)
	must(t, err)
	base, _ = g.Get("base")
	mid, _ := g.Get("mid")
	plan, err = g.Plan(PlanOptions{Seeds: []*Unit{base}})
	must(t, err)
	if got := planString(plan); got != "1:base:none 2:mid:release 3:app:update" {
		t.Fatalf("plan after base release = %q", got)
	}
	if _, err := g.Release(mid, &Runner{DryRun: true, Out: &bytes.Buffer{}}, ReleaseOptions{Bump: "patch"}); err == nil ||
		!strings.Contains(err.Error(), "lags") {
		t.Fatalf("release of a lagging unit: err = %v", err)
	}
	out := &bytes.Buffer{}
	subject, err := g.Update(mid, &Runner{DryRun: true, Out: out}, UpdateOptions{})
	must(t, err)
	if subject != "deps: bump base v0.2.0" || !strings.Contains(out.String(), "go mod edit -require=example.com/base@v0.2.0") {
		t.Fatalf("update: %q\n%s", subject, out)
	}
}

func TestManualEdgeCycle(t *testing.T) {
	m := cascadeFixture(t)
	m.Find("base").DependsOn = []string{"app"}
	g, err := BuildGraph(m)
	must(t, err)
	base, _ := g.Get("base")
	if _, err := g.Plan(PlanOptions{Seeds: []*Unit{base}}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected a cycle error, got %v", err)
	}
}

// kit: root module, ext/a and ext/b sub-modules (b requires a, both require
// the root), a public npm package in ts/, a VERSION file, all at 0.1.0.
func kitFiles(config string) map[string]string {
	files := map[string]string{
		"go.mod":       gomod("example.com/kit"),
		"ext/a/go.mod": gomod("example.com/kit/ext/a", "example.com/kit v0.1.0") + "\nreplace example.com/kit => ../..\n",
		"ext/b/go.mod": gomod("example.com/kit/ext/b", "example.com/kit/ext/a v0.1.0") +
			"\nreplace example.com/kit/ext/a => ../a\n\nreplace example.com/kit => ../..\n",
		"examples/demo/go.mod":  gomod("example.com/kit/examples/demo", "example.com/kit/ext/a v0.1.0") + "\nreplace example.com/kit/ext/a => ../../ext/a\n\nreplace example.com/kit => ../..\n",
		"kit.go":                "package kit\n",
		"ext/a/a.go":            "package a\n\nimport _ \"example.com/kit\"\n",
		"ext/b/b.go":            "package b\n\nimport _ \"example.com/kit/ext/a\"\n",
		"examples/demo/main.go": "package main\n\nimport _ \"example.com/kit/ext/a\"\n\nfunc main() {}\n",
		"ts/package.json":       `{"name": "@acme/kit", "version": "0.1.0"}` + "\n",
		"VERSION":               "0.1.0\n",
	}
	if config != "" {
		files[RepoConfigFile] = config
	}
	return files
}

func kitRepo(t *testing.T, config string) (*Manifest, string) {
	root := t.TempDir()
	dir := repo(t, root, "kit", kitFiles(config), "v0.1.0", "ext/a/v0.1.0", "ext/b/v0.1.0")
	return manifestFor(t, root), dir
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, args...)
	must(t, err)
	return out
}

func TestInferConfig(t *testing.T) {
	m, _ := kitRepo(t, "")
	g, err := BuildGraph(m)
	must(t, err)
	u, _ := g.Get("kit")
	// @acme/kit is not on npm under acme, so init leaves it out.
	cfg, notes := InferConfig(u.Project, []string{"acme"})
	if got := strings.Join(cfg.Modules, " "); got != ". ext/a ext/b" {
		t.Errorf("modules = %q", got)
	}
	if cfg.Npm == nil || len(cfg.Npm) != 0 {
		t.Errorf("npm = %#v, want an explicit empty list (notes %v)", cfg.Npm, notes)
	}
	data, _ := cfg.Marshal()
	if !strings.Contains(string(data), "npm: []") {
		t.Errorf("marshalled config lost npm: []:\n%s", data)
	}
	if got := strings.Join(cfg.VersionFiles, " "); got != "VERSION" {
		t.Errorf("version_files = %q", got)
	}
}

func TestReleaseLockstep(t *testing.T) {
	m, dir := kitRepo(t, "modules: [., ext/*]\nnpm: [ts]\nstrip_replaces: true\npublish: {npm: ci}\n")
	commitFile(t, dir, "ext/a/doc.go", "package a\n")
	g, err := BuildGraph(m)
	must(t, err)
	u, _ := g.Get("kit")
	if got := len(u.Unreleased); got != 1 {
		t.Fatalf("unreleased = %d", got)
	}
	notes := filepath.Join(t.TempDir(), "notes.md")
	must(t, os.WriteFile(notes, []byte("kit v0.2.0: notes\n"), 0o644))
	out := &bytes.Buffer{}
	rp, err := g.Release(u, &Runner{Out: out}, ReleaseOptions{Bump: "minor", Notes: notes, NoPush: true})
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := strings.Join(rp.Tags, " "); got != "v0.2.0 ext/a/v0.2.0 ext/b/v0.2.0" {
		t.Fatalf("tags = %q", got)
	}
	// The tagged tree: pins at v0.2.0, no in-repo replaces, versions written.
	if gm := gitOut(t, dir, "show", "ext/b/v0.2.0:ext/b/go.mod"); !strings.Contains(gm, "example.com/kit/ext/a v0.2.0") || strings.Contains(gm, "replace") {
		t.Errorf("ext/b/go.mod at the tag:\n%s", gm)
	}
	if v := gitOut(t, dir, "show", "v0.2.0:VERSION"); v != "0.2.0" {
		t.Errorf("VERSION at tag = %q", v)
	}
	var pj struct{ Version string }
	if raw := gitOut(t, dir, "show", "v0.2.0:ts/package.json"); json.Unmarshal([]byte(raw), &pj) != nil || pj.Version != "0.2.0" {
		t.Errorf("ts/package.json version at tag = %q", pj.Version)
	}
	// The branch: replaces restored, follower pins moved, notes on the root tag.
	if gm := gitOut(t, dir, "show", "HEAD:ext/b/go.mod"); !strings.Contains(gm, "replace example.com/kit/ext/a => ../a") {
		t.Errorf("ext/b/go.mod on main lost its replace:\n%s", gm)
	}
	// A follower keeps its sibling pins by default (pin_followers: root).
	if gm := gitOut(t, dir, "show", "HEAD:examples/demo/go.mod"); !strings.Contains(gm, "example.com/kit/ext/a v0.1.0") {
		t.Errorf("examples/demo's sibling pin moved:\n%s", gm)
	}
	if msg := gitOut(t, dir, "tag", "-l", "--format=%(contents)", "v0.2.0"); !strings.Contains(msg, "notes") {
		t.Errorf("root tag annotation = %q", msg)
	}
	if subj := gitOut(t, dir, "log", "-3", "--format=%s"); subj != "release: restore in-repo replaces after v0.2.0\nrelease: installable tree for v0.2.0\nrelease: v0.2.0" {
		t.Errorf("release commits:\n%s", subj)
	}
	// Release commits do not count as unreleased work.
	g, err = BuildGraph(m)
	must(t, err)
	u, _ = g.Get("kit")
	if u.Version != "v0.2.0" || len(u.Unreleased) != 0 {
		t.Errorf("after release: version %s, unreleased %v", u.Version, u.Unreleased)
	}
}

func TestReleaseChangedAndOnly(t *testing.T) {
	m, dir := kitRepo(t, "versioning: changed\nmodules: [., ext/*]\n")
	commitFile(t, dir, "ext/a/doc.go", "package a\n")
	g, err := BuildGraph(m)
	must(t, err)
	u, _ := g.Get("kit")
	rp, err := selectRelease(u, "v0.2.0", ReleaseOptions{})
	must(t, err)
	// a changed; b requires a, so b is retagged; the root is untouched.
	if got := strings.Join(rp.Tags, " "); got != "ext/a/v0.2.0 ext/b/v0.2.0" {
		t.Errorf("changed tags = %q", got)
	}
	rp, err = selectRelease(u, "v0.2.0", ReleaseOptions{Only: []string{"ext/b"}})
	must(t, err)
	if got := strings.Join(rp.Tags, " "); got != "ext/b/v0.2.0" {
		t.Errorf("--only tags = %q", got)
	}
	if _, err := selectRelease(u, "v0.2.0", ReleaseOptions{Only: []string{"nope"}}); err == nil {
		t.Error("--only with an unknown module should fail")
	}
}

func TestUnitsInOneRepo(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":             gomod("example.com/kit"),
		"agent/go.mod":       gomod("example.com/kit/agent", "example.com/kit v0.1.0") + "\nreplace example.com/kit => ../\n",
		RepoConfigFile:       "units:\n  - modules: [.]\n  - name: agent\n    modules: [agent]\n",
		"consumer/README.md": "",
	}
	dir := repo(t, root, "kit", files, "v0.1.0", "agent/v0.5.0")
	repo(t, root, "user", map[string]string{"go.mod": gomod("example.com/user", "example.com/kit/agent v0.5.0")}, "v1.0.0")
	commitFile(t, dir, "core.go", "package kit\n")
	g, err := BuildGraph(manifestFor(t, root))
	must(t, err)
	seeds, err := g.SeedsFor("kit")
	must(t, err)
	plan, err := g.Plan(PlanOptions{Seeds: seeds})
	must(t, err)
	if got := planString(plan); got != "1:kit:release 2:kit/agent:release 3:user:release" {
		t.Fatalf("plan = %q", got)
	}
	agent, _ := g.Get("kit/agent")
	if agent.Version != "v0.5.0" || len(agent.Unreleased) != 0 {
		t.Errorf("agent unit: version %s, unreleased %v (core.go must not count)", agent.Version, agent.Unreleased)
	}
}

func planString(p *Plan) string {
	var parts []string
	for _, n := range p.Nodes {
		parts = append(parts, fmt.Sprintf("%d:%s:%s", n.Wave, n.Unit.Name, n.Action))
	}
	return strings.Join(parts, " ")
}

func sh(t *testing.T, dir, script string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
