package dagbump

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// applyFixture: base and mid, each pushed to a bare remote. mid requires
// base through a local replace, so tidy works offline. A stub gh stands in
// for GitHub releases.
func applyFixture(t *testing.T) (*Manifest, string, string, string) {
	root := t.TempDir()
	base := repo(t, root, "base", map[string]string{"go.mod": gomod("example.com/base"), "base.go": "package base\n"}, "v0.1.0")
	commitFile(t, base, "more.go", "package base\n\nfunc New() {}\n")
	mid := repo(t, root, "mid", map[string]string{
		"go.mod":       gomod("example.com/mid", "example.com/base v0.1.0") + "\nreplace example.com/base => ../../base/main\n",
		"mid.go":       "package mid\n\nimport _ \"example.com/base\"\n",
		"CHANGELOG.md": "# Changelog\n\n## [v0.1.0] - 2026-01-01\n\nFirst.\n",
	}, "v0.1.0")
	m := manifestFor(t, root)
	for _, dir := range []string{base, mid} {
		bare := filepath.Join(root, "remotes", filepath.Base(filepath.Dir(dir))+".git")
		sh(t, root, "git init -q --bare -b main "+bare)
		sh(t, dir, "git remote set-url origin "+bare+" && git push -q origin main --tags && git fetch -q origin && git remote set-head origin main")
	}
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return m, root, base, mid
}

func writePlan(t *testing.T, m *Manifest, seed string) (*PlanFile, string) {
	t.Helper()
	g, err := BuildGraph(m)
	must(t, err)
	seeds, err := g.SeedsFor(seed)
	must(t, err)
	plan, err := g.Plan(PlanOptions{Seeds: seeds})
	must(t, err)
	pf, err := g.NewPlanFile(plan, []string{seed}, false)
	must(t, err)
	path := filepath.Join(t.TempDir(), DefaultPlanFile)
	must(t, pf.Save(path))
	pf, err = LoadPlanFile(path)
	must(t, err)
	return pf, path
}

func TestPlanFileNotesApply(t *testing.T) {
	m, root, base, mid := applyFixture(t)
	pf, path := writePlan(t, m, "base")
	if len(pf.Units) != 2 || pf.Units[0].Unit != "base" || pf.Units[1].Unit != "mid" {
		t.Fatalf("units = %+v", pf.Units)
	}
	if p := pf.Resolve(false); len(p) != 2 {
		t.Fatalf("an unfilled plan should have 2 problems, got %v", p)
	}
	if len(pf.Units[0].Commits) != 1 || pf.Units[1].WaitsFor[0] != "base" {
		t.Errorf("context: commits %v, waits_for %v", pf.Units[0].Commits, pf.Units[1].WaitsFor)
	}

	agent := `cat > /dev/null; printf -- '---\nbump: minor\n---\nNotes for %s.\n' "$DAGBUMP_UNIT"`
	must(t, FillNotes(pf, NotesOptions{Agent: agent, Jobs: 2, Out: &strings.Builder{}}))
	pf, err := LoadPlanFile(path)
	must(t, err)
	if pf.Units[1].Bump != "minor" || strings.TrimSpace(pf.Units[1].Notes) != "Notes for mid." {
		t.Fatalf("notes not filled: %+v", pf.Units[1])
	}

	out := &strings.Builder{}
	if err := Apply(m, pf, ApplyOptions{Out: out}); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	pf, err = LoadPlanFile(path)
	must(t, err)
	for _, pu := range pf.Units {
		if pu.Status != "done" || pu.Released != "v0.2.0" {
			t.Errorf("%s: status %s released %s", pu.Unit, pu.Status, pu.Released)
		}
	}
	for _, name := range []string{"base", "mid"} {
		if tags := gitOut(t, filepath.Join(root, "remotes", name+".git"), "tag", "-l", "v0.2.0"); tags != "v0.2.0" {
			t.Errorf("%s remote has no v0.2.0", name)
		}
	}
	if gm := gitOut(t, mid, "show", "v0.2.0:go.mod"); !strings.Contains(gm, "example.com/base v0.2.0") {
		t.Errorf("mid's release does not require base v0.2.0:\n%s", gm)
	}
	cl := gitOut(t, mid, "show", "v0.2.0:CHANGELOG.md")
	if !strings.Contains(cl, "Notes for mid.") || strings.Index(cl, "[v0.2.0]") > strings.Index(cl, "[v0.1.0]") {
		t.Errorf("CHANGELOG at the tag:\n%s", cl)
	}
	if msg := gitOut(t, base, "tag", "-l", "--format=%(contents)", "v0.2.0"); !strings.Contains(msg, "Notes for base.") {
		t.Errorf("base tag annotation = %q", msg)
	}
	// A second apply finds nothing left to do.
	must(t, Apply(m, pf, ApplyOptions{Out: &strings.Builder{}}))
}

func TestApplyRefusesMovedRepo(t *testing.T) {
	m, _, base, _ := applyFixture(t)
	pf, path := writePlan(t, m, "base")
	for _, pu := range pf.Units {
		pu.Bump, pu.Notes = "patch", "notes\n"
	}
	commitFile(t, base, "late.go", "package base\n")
	err := Apply(m, pf, ApplyOptions{Out: &strings.Builder{}})
	if err == nil || !strings.Contains(err.Error(), "since the plan") {
		t.Fatalf("err = %v", err)
	}
	pf, _ = LoadPlanFile(path)
	if pf.Units[0].Status != "failed" || pf.Units[0].Error == "" {
		t.Errorf("base: status %s error %q", pf.Units[0].Status, pf.Units[0].Error)
	}
}

func TestPnpmLockAndAgeGuard(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "pnpm-lock.yaml"), []byte(`lockfileVersion: '9.0'
importers:
  .:
    dependencies:
      tlex:
        specifier: ^1.1.0
        version: 1.1.0
  web:
    dependencies:
      '@panyam/tsappkit':
        specifier: ^0.7.3
        version: 0.7.3(solid-js@1.9.17)
      shared:
        specifier: workspace:*
        version: link:../shared
`), 0o644))
	l, err := loadLock("pnpm", dir)
	must(t, err)
	if v := l.Locked(filepath.Join(dir, "web"), "@panyam/tsappkit", "^0.7.3"); v != "0.7.3" {
		t.Errorf("web tsappkit = %q", v)
	}
	if v := l.Locked(dir, "tlex", "^1.1.0"); v != "1.1.0" {
		t.Errorf("root tlex = %q", v)
	}
	if got := npmAddCmd("pnpm", "@panyam/tsappkit", "^0.7.3", "devDependencies", "0.7.7"); got != "pnpm add '@panyam/tsappkit@^0.7.7' -D" {
		t.Errorf("pnpm add = %q", got)
	}

	ws := filepath.Join(dir, "pnpm-workspace.yaml")
	must(t, os.WriteFile(ws, []byte("packages:\n  - web\nminimumReleaseAgeExclude:\n  - '@panyam/tsappkit@0.7.3 || 0.7.7'\nonlyBuiltDependencies:\n  - esbuild\n"), 0o644))
	_, err = allowFreshVersion(dir, "@panyam/tsappkit", "0.7.8")
	must(t, err)
	_, err = allowFreshVersion(dir, "@panyam/massrelay", "0.3.7")
	must(t, err)
	_, err = allowFreshVersion(dir, "@panyam/tsappkit", "0.7.8") // already there: no change
	must(t, err)
	data, _ := os.ReadFile(ws)
	want := "packages:\n  - web\nminimumReleaseAgeExclude:\n  - '@panyam/massrelay@0.3.7'\n  - '@panyam/tsappkit@0.7.3 || 0.7.7 || 0.7.8'\nonlyBuiltDependencies:\n  - esbuild\n"
	if string(data) != want {
		t.Errorf("pnpm-workspace.yaml:\n%s\nwant:\n%s", data, want)
	}
}
