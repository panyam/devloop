package dagbump

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Preflight checks that p can be changed: a clean tree on the default
// branch. With pull set it fetches and fast-forwards when the clone is behind.
func Preflight(p *Project, pull bool) error {
	if p.LoadErr != nil {
		return p.LoadErr
	}
	if p.Dirty {
		return fmt.Errorf("%s has uncommitted changes in %s", p.Name, CompactPath(p.Dir))
	}
	if p.Branch != p.DefaultBranch {
		return fmt.Errorf("%s is on branch %q; switch %s to %q first", p.Name, p.Branch, CompactPath(p.Dir), p.DefaultBranch)
	}
	if !pull {
		return nil
	}
	if _, err := git(p.Dir, "fetch", "--tags", "origin"); err != nil {
		return err
	}
	behind, err := git(p.Dir, "rev-list", "--count", "HEAD..origin/"+p.DefaultBranch)
	if err == nil && behind != "0" {
		if _, err := git(p.Dir, "merge", "--ff-only", "origin/"+p.DefaultBranch); err != nil {
			return fmt.Errorf("%s is %s commit(s) behind origin and cannot fast-forward: %w", p.Name, behind, err)
		}
	}
	return nil
}

// FetchTags fetches tags for u's dependencies, so lag is measured against
// releases made from other clones or by CI.
func (g *Graph) FetchTags(u *Unit) {
	seen := map[*Project]bool{}
	for _, e := range g.Deps[u] {
		if p := e.To.Project; !seen[p] && p.LoadErr == nil {
			seen[p] = true
			_, _ = git(p.Dir, "fetch", "--tags", "--quiet", "origin")
		}
	}
}

// Context is what a release-notes writer needs about a unit.
type Context struct {
	Unit        string   `json:"unit"`
	Path        string   `json:"path"`
	Remote      string   `json:"remote"`
	Version     string   `json:"version"`
	Kind        string   `json:"kind"`
	Versioning  string   `json:"versioning"`
	Commits     []Commit `json:"commits"`
	Merges      []string `json:"merges"`
	DiffStat    string   `json:"diffstat"`
	ChangedDirs []string `json:"changed_dirs"`
	Modules     []string `json:"tagged_modules"`
	Npm         []string `json:"npm_packages"`
	Pending     []string `json:"pending_dep_updates"`
	Warnings    []string `json:"warnings"`
}

// Context gathers the change set since u's last release.
func (g *Graph) Context(u *Unit) (*Context, error) {
	p := u.Project
	if p.LoadErr != nil {
		return nil, p.LoadErr
	}
	c := &Context{Unit: u.Name, Path: CompactPath(p.Dir), Remote: p.Remote, Version: u.Version,
		Kind: u.Kind(), Versioning: map[bool]string{true: "lockstep", false: "changed"}[u.Lockstep()],
		Commits: u.Unreleased, Warnings: p.Warnings}
	for _, m := range u.Modules {
		c.Modules = append(c.Modules, dirOrDot(m.Dir))
	}
	for _, n := range u.PublicNpm() {
		c.Npm = append(c.Npm, n.Name+" ("+dirOrDot(n.Dir)+")")
	}
	for _, e := range g.LaggingDeps(u, false) {
		c.Pending = append(c.Pending, fmt.Sprintf("%s: %s %s -> %s", depFile(e), e.Module, e.Required, e.Latest))
	}
	base := p.baseTag(u)
	if base == "" {
		return c, nil
	}
	spec := append([]string{base + "..HEAD", "--"}, u.Pathspec()...)
	if out, err := git(p.Dir, append([]string{"log", "--merges", "--format=%h %s"}, spec...)...); err == nil && out != "" {
		c.Merges = strings.Split(out, "\n")
	}
	if out, err := git(p.Dir, append([]string{"diff", "--shortstat"}, spec...)...); err == nil {
		c.DiffStat = out
	}
	if out, err := git(p.Dir, append([]string{"diff", "--name-only"}, spec...)...); err == nil && out != "" {
		counts := map[string]int{}
		for _, f := range strings.Split(out, "\n") {
			counts[moduleDirOf(p, f)]++
		}
		for _, d := range sortedKeys(counts) {
			c.ChangedDirs = append(c.ChangedDirs, fmt.Sprintf("%s (%d files)", dirOrDot(d), counts[d]))
		}
	}
	return c, nil
}

func dirOrDot(d string) string {
	if d == "" {
		return "."
	}
	return d
}

func depFile(e *Edge) string {
	if e.Kind == "npm" {
		return path.Join(e.FromDir, "package.json")
	}
	return modFile(e.FromDir)
}

// moduleDirOf maps a file to the deepest Go module folder that holds it.
func moduleDirOf(p *Project, file string) string {
	best := ""
	for _, m := range p.Modules {
		if m.Dir != "" && strings.HasPrefix(file, m.Dir+"/") && len(m.Dir) > len(best) {
			best = m.Dir
		}
	}
	return best
}

// goEnv lets the go command see tags pushed moments ago: the projects'
// module prefixes skip the proxy and checksum database.
func (g *Graph) goEnv() []string {
	prefixes := map[string]bool{}
	for _, p := range g.Projects {
		if p.Remote != "" {
			prefixes[p.Remote] = true
		}
		for _, m := range p.Modules {
			prefixes[m.Path] = true
		}
	}
	roots := sortedKeys(prefixes)
	if priv := os.Getenv("GOPRIVATE"); priv != "" {
		roots = append([]string{priv}, roots...)
	}
	return []string{"GOPRIVATE=" + strings.Join(roots, ","), "GOFLAGS=-mod=mod"}
}

// UpdateOptions controls Update.
type UpdateOptions struct {
	NoCommit bool
	NoBuild  bool
	Push     bool
	// Wait is how long to wait for a dependency's new version to show up
	// on npm or to the go command (a CI publish takes minutes).
	Wait time.Duration
}

// Update moves every dependency of u on another unit to that unit's latest
// tag, tidies, builds and commits. It returns the commit subject, or ""
// when nothing needed updating.
func (g *Graph) Update(u *Unit, r *Runner, opts UpdateOptions) (string, error) {
	p := u.Project
	r.Env, r.Base = g.goEnv(), p.Dir
	edges := g.LaggingDeps(u, false)
	if len(edges) == 0 {
		r.Note("%s: dependencies are up to date", u.Name)
		return "", nil
	}
	if err := g.waitAvailable(edges, r, opts.Wait); err != nil {
		return "", err
	}
	touched := map[string]bool{}
	var npmCmds [][2]string // dir, command
	bumped := map[string]map[string]bool{}
	for _, e := range edges {
		if bumped[e.To.Name] == nil {
			bumped[e.To.Name] = map[string]bool{}
		}
		bumped[e.To.Name][e.Latest] = true
		switch e.Kind {
		case "go":
			if err := r.Step(filepath.Join(p.Dir, e.FromDir), fmt.Sprintf("go mod edit -require=%s@%s", e.Module, e.Latest)); err != nil {
				return "", err
			}
			touched[e.FromDir] = true
		case "npm":
			var pkg *NpmPackage
			for _, n := range u.Npm {
				if n.Dir == e.FromDir {
					pkg = n
				}
			}
			if pkg.PolicyErr != "" {
				return "", errors.New(pkg.PolicyErr)
			}
			d := pkg.Deps[e.Module]
			v := strings.TrimPrefix(e.Latest, "v")
			if pkg.Manager == "pnpm" && pkg.LockDir != "" {
				if r.DryRun {
					r.Note("allow %s@%s in pnpm-workspace.yaml minimumReleaseAgeExclude, if the file keeps that list", e.Module, v)
				} else if f, err := allowFreshVersion(pkg.LockDir, e.Module, v); err != nil {
					return "", err
				} else if f != "" {
					r.Note("added %s@%s to %s minimumReleaseAgeExclude", e.Module, v, CompactPath(f))
				}
			}
			npmCmds = append(npmCmds, [2]string{e.FromDir, npmAddCmd(pkg.Manager, e.Module, strings.TrimSpace(d.Spec), d.Section, v)})
		}
	}

	if len(touched) > 0 {
		if !p.HasConfig && hasMakeTarget(p.Dir, "tidy-all") {
			if err := r.Step(p.Dir, "make tidy-all"); err != nil {
				return "", err
			}
		} else if err := tidy(p, r, touched); err != nil {
			return "", err
		}
		if !opts.NoBuild {
			if u.Config.Check != "" {
				if err := r.Step(p.Dir, u.Config.Check); err != nil {
					return "", err
				}
			} else {
				for _, dir := range sortedKeys(touched) {
					if err := r.Step(filepath.Join(p.Dir, dir), "go build ./..."); err != nil {
						return "", err
					}
				}
			}
		}
	}
	for _, c := range npmCmds {
		if err := r.Step(filepath.Join(p.Dir, c[0]), c[1]); err != nil {
			return "", err
		}
	}

	var parts []string
	for _, name := range sortedKeys(bumped) {
		parts = append(parts, name+" "+strings.Join(sortedKeys(bumped[name]), "/"))
	}
	subject := "deps: bump " + strings.Join(parts, ", ")
	if opts.NoCommit {
		return subject, nil
	}
	if err := r.Step(p.Dir, "git add -u && git diff --cached --quiet || git commit -q -m "+shellQuote(subject)); err != nil {
		return "", err
	}
	if opts.Push {
		if err := r.Step(p.Dir, "git push origin HEAD:"+p.DefaultBranch); err != nil {
			return "", err
		}
	}
	return subject, nil
}

// waitAvailable polls until every target version resolves: go list -m for
// Go modules, npm view for packages. A tag pushed by dagbump resolves at
// once; a package published by CI can take minutes.
func (g *Graph) waitAvailable(edges []*Edge, r *Runner, wait time.Duration) error {
	if r.DryRun {
		return nil
	}
	pending := map[string][2]string{} // key -> kind, spec
	for _, e := range edges {
		switch e.Kind {
		case "go":
			if e.LocalReplace != "" {
				continue // tidy resolves it from the local folder
			}
			pending["go "+e.Module+"@"+e.Latest] = [2]string{"go", e.Module + "@" + e.Latest}
		case "npm":
			pending["npm "+e.Module+"@"+e.Latest] = [2]string{"npm", e.Module + "@" + strings.TrimPrefix(e.Latest, "v")}
		}
	}
	deadline := time.Now().Add(wait)
	for {
		for key, kv := range pending {
			var err error
			if kv[0] == "go" {
				_, err = run(os.TempDir(), r.Env, "go", "list", "-m", kv[1])
			} else {
				var out string
				out, err = run(os.TempDir(), nil, "npm", "view", kv[1], "version")
				if err == nil && out == "" {
					err = errors.New("not on npm yet")
				}
			}
			if err == nil {
				delete(pending, key)
			}
		}
		if len(pending) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("still not available after %s: %s", wait, strings.Join(sortedKeys(pending), ", "))
		}
		r.Note("waiting for %s", strings.Join(sortedKeys(pending), ", "))
		time.Sleep(15 * time.Second)
	}
}

// tidy runs go mod tidy in the touched modules and every in-repo module
// that requires one of them, dependencies first.
func tidy(p *Project, r *Runner, touched map[string]bool) error {
	for _, dir := range tidyOrder(p, touched) {
		if err := r.Step(filepath.Join(p.Dir, dir), "go mod tidy"); err != nil {
			return err
		}
	}
	return nil
}

func tidyOrder(p *Project, touched map[string]bool) []string {
	byPath := map[string]*GoModule{}
	for _, m := range p.Modules {
		byPath[m.Path] = m
	}
	need := map[string]bool{}
	for d := range touched {
		need[d] = true
	}
	for changed := true; changed; {
		changed = false
		for _, m := range p.Modules {
			if need[m.Dir] {
				continue
			}
			for req := range m.Requires {
				if dep, ok := byPath[req]; ok && need[dep.Dir] {
					need[m.Dir], changed = true, true
					break
				}
			}
		}
	}
	var order []string
	done := map[string]bool{}
	for len(done) < len(need) {
		progressed := false
		for _, d := range sortedKeys(need) {
			if done[d] {
				continue
			}
			ready := true
			for _, m := range p.Modules {
				if m.Dir != d {
					continue
				}
				for req := range m.Requires {
					if dep, ok := byPath[req]; ok && dep.Dir != d && need[dep.Dir] && !done[dep.Dir] {
						ready = false
					}
				}
			}
			if ready {
				done[d], progressed = true, true
				order = append(order, d)
			}
		}
		if !progressed { // in-repo cycle: tidy the rest in name order
			for _, d := range sortedKeys(need) {
				if !done[d] {
					done[d] = true
					order = append(order, d)
				}
			}
		}
	}
	return order
}

// NextVersion bumps v by patch, minor or major. A pre-release patch bump
// drops the suffix (v0.2.11-rc1 -> v0.2.11).
func NextVersion(v, bump string) (string, error) {
	if !semver.IsValid(v) {
		return "", fmt.Errorf("current version %q is not semver", v)
	}
	var maj, min, pat int
	if _, err := fmt.Sscanf(strings.SplitN(semver.Canonical(v), "-", 2)[0], "v%d.%d.%d", &maj, &min, &pat); err != nil {
		return "", err
	}
	pre := semver.Prerelease(v) != ""
	switch bump {
	case "patch":
		if !pre {
			pat++
		}
	case "minor":
		if !pre || pat != 0 {
			min, pat = min+1, 0
		}
	case "major":
		maj, min, pat = maj+1, 0, 0
	default:
		return "", fmt.Errorf("bump must be patch, minor or major, not %q", bump)
	}
	return fmt.Sprintf("v%d.%d.%d", maj, min, pat), nil
}

// ReleaseOptions controls Release.
type ReleaseOptions struct {
	Bump    string
	Version string
	Notes   string // path to a release-notes file
	// Only tags just these modules (folders or module paths) and the
	// unit's modules that require them, whatever the unit's versioning.
	Only         []string
	NoPush       bool
	AllowLagging bool
	Force        bool
}

// ReleasePlan is what Release will tag.
type ReleasePlan struct {
	Version string
	Tags    []string // primary first when present
	Modules []*GoModule
	Npm     []*NpmPackage
}

// Release tags u at the next version and publishes it. With a .dagbump.yaml
// it runs the steps itself; without one it drives the repo's make targets
// (bump-root / bump-siblings / tag) when they exist.
func (g *Graph) Release(u *Unit, r *Runner, opts ReleaseOptions) (*ReleasePlan, error) {
	p := u.Project
	r.Env, r.Base = g.goEnv(), p.Dir
	if u.Kind() != "lib" {
		return nil, fmt.Errorf("%s is an app (no release tags); use update --push, or set kind: lib", u.Name)
	}
	if lag := g.LaggingDeps(u, true); len(lag) > 0 && !opts.AllowLagging {
		var s []string
		for _, e := range lag {
			s = append(s, fmt.Sprintf("%s: %s %s < %s", depFile(e), e.Module, e.Required, e.Latest))
		}
		return nil, fmt.Errorf("%s lags its dependencies (%s); run `dagbump update %s` first", u.Name, strings.Join(s, "; "), u.Name)
	}
	if len(u.Unreleased) == 0 && u.Version != "" && !opts.Force {
		return nil, fmt.Errorf("%s has nothing unreleased since %s", u.Name, u.Version)
	}
	v, err := nextFor(u, opts)
	if err != nil {
		return nil, err
	}
	notes := ""
	if opts.Notes != "" {
		if notes, err = filepath.Abs(opts.Notes); err != nil {
			return nil, err
		}
		if _, err := os.Stat(notes); err != nil {
			return nil, err
		}
	}
	r.Env = append(r.Env, "V="+v, "NOTES="+notes)
	if !p.HasConfig && len(p.Units) == 1 && (hasMakeTarget(p.Dir, "tag") || hasMakeTarget(p.Dir, "bump-root") || hasMakeTarget(p.Dir, "bump-siblings")) {
		return g.releaseMake(u, r, v, notes, opts)
	}
	rp, err := selectRelease(u, v, opts)
	if err != nil {
		return nil, err
	}
	return rp, g.releaseNative(u, r, rp, notes, opts)
}

func nextFor(u *Unit, opts ReleaseOptions) (string, error) {
	v := opts.Version
	if v == "" {
		if opts.Bump == "" {
			return "", errors.New("give --bump patch|minor|major or --version")
		}
		if u.Version == "" {
			return "", fmt.Errorf("%s has no release yet; give --version", u.Name)
		}
		var err error
		if v, err = NextVersion(u.Version, opts.Bump); err != nil {
			return "", err
		}
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", fmt.Errorf("version %q is not semver", v)
	}
	if u.Version != "" && semver.Compare(v, u.Version) <= 0 {
		return "", fmt.Errorf("%s is not above %s's current %s", v, u.Name, u.Version)
	}
	return v, nil
}

// selectRelease picks the modules and packages to tag. Lockstep: all of
// them. Changed (or --only): the changed ones plus every module of the unit
// that requires one of those, so no tagged module points at a stale sibling.
func selectRelease(u *Unit, v string, opts ReleaseOptions) (*ReleasePlan, error) {
	p := u.Project
	rp := &ReleasePlan{Version: v}
	pick := map[*GoModule]bool{}
	switch {
	case len(opts.Only) > 0:
		for _, o := range opts.Only {
			found := false
			for _, m := range u.Modules {
				if m.Dir == strings.Trim(o, "./") || (o == "." && m.Dir == "") || m.Path == o {
					pick[m], found = true, true
				}
			}
			if !found {
				return nil, fmt.Errorf("--only %q is not a tagged module of %s", o, u.Name)
			}
		}
	case u.Lockstep():
		for _, m := range u.Modules {
			pick[m] = true
		}
	default:
		for _, m := range u.Modules {
			changed, err := p.ModuleChanged(m)
			if err != nil {
				return nil, err
			}
			if changed {
				pick[m] = true
			}
		}
	}
	byPath := map[string]*GoModule{}
	for _, m := range u.Modules {
		byPath[m.Path] = m
	}
	for grew := true; grew; {
		grew = false
		for _, m := range u.Modules {
			if pick[m] {
				continue
			}
			for req := range m.Requires {
				if dep, ok := byPath[req]; ok && pick[dep] {
					pick[m], grew = true, true
					break
				}
			}
		}
	}
	for _, m := range u.Modules {
		if pick[m] {
			rp.Modules = append(rp.Modules, m)
		}
	}
	// Packages ride along in lockstep, or when their folder changed.
	base := p.baseTag(u)
	for _, n := range u.PublicNpm() {
		if u.Lockstep() && len(opts.Only) == 0 {
			rp.Npm = append(rp.Npm, n)
			continue
		}
		if base == "" {
			rp.Npm = append(rp.Npm, n)
			continue
		}
		if c, err := p.logSince(base, []string{dirOrDot(n.Dir)}); err == nil && len(c) > 0 {
			rp.Npm = append(rp.Npm, n)
		}
	}
	if len(rp.Modules) == 0 && len(rp.Npm) == 0 {
		return nil, fmt.Errorf("%s: no module or package changed since its last tag", u.Name)
	}
	// The release marker tag: the main unit's root tag in lockstep, or when
	// the root module is picked, or when there is no root module to move.
	primary := u.PrimaryPrefix()
	root := p.RootModule()
	wantPrimary := u.Lockstep() && len(opts.Only) == 0
	for _, m := range rp.Modules {
		if m.TagPrefix == primary {
			wantPrimary = true
		}
	}
	if u.Main && (root == nil || !contains(u.Modules, root)) {
		wantPrimary = true
	}
	seen := map[string]bool{}
	if wantPrimary {
		rp.Tags = append(rp.Tags, TagName(primary, v))
		seen[primary] = true
	}
	for _, m := range rp.Modules {
		if !seen[m.TagPrefix] {
			seen[m.TagPrefix] = true
			rp.Tags = append(rp.Tags, TagName(m.TagPrefix, v))
		}
	}
	for _, t := range rp.Tags {
		if p.HasTag(t) {
			return nil, fmt.Errorf("tag %s already exists in %s", t, p.Name)
		}
	}
	return rp, nil
}

func contains(list []*GoModule, m *GoModule) bool {
	for _, x := range list {
		if x == m {
			return true
		}
	}
	return false
}

const stripSubject = "release: installable tree for "

func (g *Graph) releaseNative(u *Unit, r *Runner, rp *ReleasePlan, notes string, opts ReleaseOptions) error {
	p, v, c := u.Project, rp.Version, u.Config
	fmt.Fprintf(r.Out, "release %s %s -> %s (%s)\n  tags: %s\n", u.Name, dashIfEmpty(u.Version), v,
		map[bool]string{true: "lockstep", false: "changed"}[u.Lockstep()], strings.Join(rp.Tags, " "))

	// Fail before writing anything when a package would publish with a tool
	// the manifest rules out.
	if u.NpmPublish() == "direct" {
		for _, n := range rp.Npm {
			if n.PolicyErr != "" {
				return errors.New(n.PolicyErr)
			}
		}
	}

	// Fail before writing anything when a tagged module replaces something
	// outside the repo: the strip step cannot make that tree installable.
	if c.StripReplaces {
		for _, m := range rp.Modules {
			for old, dir := range m.LocalReplaces {
				if !insideRepo(m.Dir, dir) {
					return fmt.Errorf("%s replaces %s => %s, outside the repo; strip_replaces cannot drop it", modFile(m.Dir), old, dir)
				}
			}
		}
	}

	// 1. version files and package versions
	files := c.VersionFiles
	if len(files) == 0 {
		if _, err := git(p.Dir, "ls-files", "--error-unmatch", "VERSION"); err == nil {
			files = []string{"VERSION"}
		}
	}
	for _, f := range files {
		val := v
		if data, err := os.ReadFile(filepath.Join(p.Dir, f)); err == nil && !strings.HasPrefix(strings.TrimSpace(string(data)), "v") {
			val = strings.TrimPrefix(v, "v")
		}
		if err := r.Step(p.Dir, fmt.Sprintf("printf '%%s\\n' %s > %s", val, shellQuote(f))); err != nil {
			return err
		}
	}
	for _, n := range rp.Npm {
		file := path.Join(n.Dir, "package.json")
		r.Note("set %s version to %s", file, strings.TrimPrefix(v, "v"))
		if !r.DryRun {
			if err := setPackageVersion(filepath.Join(p.Dir, file), strings.TrimPrefix(v, "v")); err != nil {
				return err
			}
		}
	}

	// 2. pins: every tagged module requires the other modules being tagged
	// at V, with an in-repo replace so tidy resolves V locally. Followers
	// move per pin_followers.
	tagged := map[string]*GoModule{}
	for _, m := range rp.Modules {
		tagged[m.Path] = m
	}
	touched := map[string]bool{}
	isMember := map[*GoModule]bool{}
	for _, m := range u.Modules {
		isMember[m] = true
	}
	for _, m := range append(append([]*GoModule{}, u.Modules...), u.Follow...) {
		var edits []string
		for _, req := range sortedKeys(m.Requires) {
			dep, ok := tagged[req]
			if !ok || dep == m || m.Requires[req] == "v0.0.0" {
				continue
			}
			if !isMember[m] {
				switch c.PinFollowers {
				case "none":
					continue
				case "all":
				default:
					if dep.Dir != "" {
						continue
					}
				}
			}
			edits = append(edits, "-require="+req+"@"+v)
			if _, has := m.LocalReplaces[req]; !has {
				edits = append(edits, "-replace="+req+"="+relDir(m.Dir, dep.Dir))
			}
		}
		if len(edits) > 0 {
			if err := r.Step(filepath.Join(p.Dir, m.Dir), "go mod edit "+strings.Join(edits, " ")); err != nil {
				return err
			}
			touched[m.Dir] = true
		}
	}
	if len(touched) > 0 {
		if err := tidy(p, r, touched); err != nil {
			return err
		}
	}

	// 3. notes into the repo, hooks, then the release commit
	if err := writeNotes(u, r, v, notes); err != nil {
		return err
	}
	for _, h := range c.Hooks.PreTag {
		if err := r.Step(p.Dir, h); err != nil {
			return err
		}
	}
	if err := r.Step(p.Dir, "git add -u && git diff --cached --quiet || git commit -q -m "+shellQuote("release: "+v)); err != nil {
		return err
	}

	// 4. strip in-repo replaces for the tagged commit
	stripped := false
	if c.StripReplaces {
		for _, m := range rp.Modules {
			var drops []string
			for _, old := range sortedKeys(m.LocalReplaces) {
				drops = append(drops, "-dropreplace="+old)
			}
			if len(drops) > 0 {
				if err := r.Step(filepath.Join(p.Dir, m.Dir), "go mod edit "+strings.Join(drops, " ")); err != nil {
					return err
				}
				stripped = true
			}
		}
		if stripped {
			if err := r.Step(p.Dir, "git commit -q -a -m "+shellQuote(stripSubject+v)); err != nil {
				return err
			}
		}
	}

	// 5. tags: the first carries the notes
	for i, t := range rp.Tags {
		cmd := fmt.Sprintf("git tag -a %s -m %s", t, t)
		if i == 0 && notes != "" {
			cmd = fmt.Sprintf("git tag -a %s -F %s", t, shellQuote(notes))
		}
		if err := r.Step(p.Dir, cmd); err != nil {
			return err
		}
	}

	// 6. restore the replaces on the branch
	if stripped {
		if err := r.Step(p.Dir, `git checkout HEAD^ -- $(git diff --name-only HEAD^ HEAD -- '*go.mod') && git commit -q -m `+
			shellQuote("release: restore in-repo replaces after "+v)); err != nil {
			return err
		}
	}
	for _, h := range c.Hooks.PostTag {
		if err := r.Step(p.Dir, h); err != nil {
			return err
		}
	}
	if opts.NoPush {
		r.Note("stopped before push (--no-push); tags for %s are local", v)
		return nil
	}

	// 7. push: branch, the first tag alone (GitHub raises no push event for
	// a push carrying many tags), then the rest together.
	if err := pushTags(p, r, rp.Tags); err != nil {
		return err
	}

	// 8. publish
	switch u.NpmPublish() {
	case "direct":
		for _, n := range npmOrder(rp.Npm) {
			if err := r.Step(filepath.Join(p.Dir, n.Dir), npmPublishCmd(n.Manager)); err != nil {
				return err
			}
		}
	case "ci":
		if len(rp.Npm) > 0 {
			r.Note("npm packages publish from CI on %s; `dagbump update` on dependents waits for them", rp.Tags[0])
		}
	}
	if c.Publish.GitHubRelease == nil || *c.Publish.GitHubRelease {
		if err := ghRelease(p, r, rp.Tags[0], v, notes); err != nil {
			return err
		}
	}
	for _, h := range c.Hooks.PostRelease {
		if err := r.Step(p.Dir, h); err != nil {
			return err
		}
	}
	return nil
}

func pushTags(p *Project, r *Runner, tags []string) error {
	if err := r.Step(p.Dir, "git push origin HEAD:"+p.DefaultBranch); err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	if err := r.Step(p.Dir, "git push origin "+tags[0]); err != nil {
		return err
	}
	if len(tags) > 1 {
		return r.Step(p.Dir, "git push origin "+strings.Join(tags[1:], " "))
	}
	return nil
}

func ghRelease(p *Project, r *Runner, tag, v, notes string) error {
	cmd := fmt.Sprintf("gh release view %s >/dev/null 2>&1 || gh release create %s --verify-tag --title %s", tag, tag, tag)
	if notes != "" {
		cmd += " --notes-file " + shellQuote(notes)
	} else {
		cmd += " --generate-notes"
	}
	if semver.Prerelease(v) != "" {
		cmd += " --prerelease"
	}
	return r.Step(p.Dir, cmd)
}

// npmOrder sorts packages so each comes after the packages it depends on.
func npmOrder(pkgs []*NpmPackage) []*NpmPackage {
	names := map[string]bool{}
	for _, n := range pkgs {
		names[n.Name] = true
	}
	var out []*NpmPackage
	done := map[string]bool{}
	for len(out) < len(pkgs) {
		progressed := false
		for _, n := range pkgs {
			if done[n.Name] {
				continue
			}
			ready := true
			for d := range n.Deps {
				if names[d] && !done[d] && d != n.Name {
					ready = false
				}
			}
			if ready {
				done[n.Name], progressed = true, true
				out = append(out, n)
			}
		}
		if !progressed {
			for _, n := range pkgs {
				if !done[n.Name] {
					done[n.Name] = true
					out = append(out, n)
				}
			}
		}
	}
	return out
}

// relDir is the relative path from module folder from to module folder to.
func relDir(from, to string) string {
	rel, err := filepath.Rel(filepath.Join("/", from), filepath.Join("/", to))
	if err != nil {
		return to
	}
	if rel == "." {
		return "./"
	}
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

// insideRepo reports whether a replace target, relative to module folder
// dir, stays inside the repo.
func insideRepo(dir, target string) bool {
	if !strings.HasPrefix(target, ".") {
		return false
	}
	return !strings.HasPrefix(path.Clean(path.Join(dir, target)), "..")
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// releaseMake drives a repo without .dagbump.yaml through its make targets.
func (g *Graph) releaseMake(u *Unit, r *Runner, v, notes string, opts ReleaseOptions) (*ReleasePlan, error) {
	p := u.Project
	rp := &ReleasePlan{Version: v, Tags: []string{v}}
	if p.HasTag(v) {
		return nil, fmt.Errorf("tag %s already exists in %s", v, p.Name)
	}
	fmt.Fprintf(r.Out, "release %s %s -> %s (make targets; add %s to let dagbump run the steps)\n", u.Name, u.Version, v, RepoConfigFile)
	if _, err := git(p.Dir, "ls-files", "--error-unmatch", "VERSION"); err == nil {
		if err := r.Step(p.Dir, "printf '%s\\n' "+v+" > VERSION"); err != nil {
			return nil, err
		}
	}
	switch {
	case hasMakeTarget(p.Dir, "bump-root"):
		if err := r.Step(p.Dir, "make bump-root V="+v); err != nil {
			return nil, err
		}
	case hasMakeTarget(p.Dir, "bump-siblings"):
		if err := r.Step(p.Dir, "make bump-siblings V="+v); err != nil {
			return nil, err
		}
	}
	if err := writeNotes(u, r, v, notes); err != nil {
		return nil, err
	}
	if err := r.Step(p.Dir, "git add -u && git diff --cached --quiet || git commit -q -m "+shellQuote("release: "+v)); err != nil {
		return nil, err
	}
	if hasMakeTarget(p.Dir, "tag") {
		cmd := fmt.Sprintf("make tag V=%s VERSION=%s", v, v)
		if notes != "" {
			cmd += " NOTES=" + shellQuote(notes)
		}
		if err := r.Step(p.Dir, cmd); err != nil {
			return nil, err
		}
	} else {
		cmd := fmt.Sprintf("git tag -a %s -m %s", v, v)
		if notes != "" {
			cmd = fmt.Sprintf("git tag -a %s -F %s", v, shellQuote(notes))
		}
		if err := r.Step(p.Dir, cmd); err != nil {
			return nil, err
		}
	}
	if opts.NoPush {
		r.Note("stopped before push (--no-push); tags for %s are local", v)
		return rp, nil
	}
	if err := r.Step(p.Dir, "git push origin HEAD:"+p.DefaultBranch+" && git push origin "+v); err != nil {
		return nil, err
	}
	if err := r.Step(p.Dir, fmt.Sprintf(`subs=$(git tag -l '*/%s'); [ -z "$subs" ] || git push origin $subs`, v)); err != nil {
		return nil, err
	}
	return rp, ghRelease(p, r, v, v, notes)
}

// InferConfig proposes a .dagbump.yaml from the repo's tags and files. The
// main unit holds the modules tagged at the latest release. A module tagged
// on its own older line gets its own unit. Each public package.json is
// checked on npm: one not published under an owner is left out, and one
// whose version differs from the repo's gets its own unit.
func InferConfig(p *Project, owners []string) (*RepoConfig, []string) {
	var notes []string
	main := UnitConfig{Versioning: "lockstep"}
	var extra []*UnitConfig
	latest := p.LatestFor("")
	for _, m := range p.Modules {
		v := p.LatestFor(m.TagPrefix)
		switch {
		case v == "":
		case latest == "" || v == latest:
			main.Modules = append(main.Modules, dirOrDot(m.Dir))
		default:
			extra = append(extra, &UnitConfig{Name: path.Base(m.Dir), Modules: PathList{m.Dir}})
			notes = append(notes, fmt.Sprintf("%s is tagged on its own line (%s, the repo is at %s): own unit %q",
				m.Dir, v, latest, path.Base(m.Dir)))
		}
	}
	sawPublic := false
	for _, n := range p.Npm {
		if n.Private {
			continue
		}
		sawPublic = true
		pub, owned, err := npmPublished(n.Name, owners)
		switch {
		case err != nil:
			notes = append(notes, fmt.Sprintf("%s (%s): npm lookup failed (%v); left out", n.Name, dirOrDot(n.Dir), err))
		case pub == "":
			notes = append(notes, fmt.Sprintf("%s (%s) is not on npm; left out (mark it \"private\": true, or add it to npm:)", n.Name, dirOrDot(n.Dir)))
		case !owned:
			notes = append(notes, fmt.Sprintf("%s (%s) is on npm under someone else; left out (rename it, or mark it \"private\": true)", n.Name, dirOrDot(n.Dir)))
		case latest != "" && n.Version != strings.TrimPrefix(latest, "v"):
			name := path.Base(n.Dir)
			if n.Dir == "" {
				name = "npm"
			}
			extra = append(extra, &UnitConfig{Name: name, Npm: PathList{dirOrDot(n.Dir)}})
			notes = append(notes, fmt.Sprintf("%s (%s) is at %s while the repo is at %s: own unit %q", n.Name, dirOrDot(n.Dir), n.Version, latest, name))
		default:
			main.Npm = append(main.Npm, dirOrDot(n.Dir))
		}
	}
	if sawPublic && main.Npm == nil {
		main.Npm = PathList{} // the repo has package.json files, none of them released with it
	}
	if _, err := git(p.Dir, "ls-files", "--error-unmatch", "VERSION"); err == nil {
		main.VersionFiles = []string{"VERSION"}
	}
	if _, err := os.Stat(filepath.Join(p.Dir, "scripts", "release-replaces.sh")); err == nil {
		main.StripReplaces = true
	}
	if len(main.Npm) > 0 && p.npmViaCI {
		main.Publish.Npm = "ci"
	}
	if hasMakeTarget(p.Dir, "verify-submodule-deps") {
		main.Hooks.PreTag = []string{"make -s verify-submodule-deps"}
	}
	sort.Strings(main.Modules)
	c := &RepoConfig{}
	if len(extra) == 0 {
		c.UnitConfig = main
	} else {
		for _, e := range extra {
			if len(e.Npm) > 0 && p.npmViaCI {
				e.Publish.Npm = "ci"
			}
		}
		c.Units = append([]*UnitConfig{&main}, extra...)
	}
	return c, notes
}

// npmPublished looks a package up on the registry. It returns the latest
// version ("" when the name is not on npm) and whether a maintainer is one
// of owners.
func npmPublished(name string, owners []string) (string, bool, error) {
	out, err := run(os.TempDir(), nil, "npm", "view", name, "version", "maintainers", "--json")
	if err != nil {
		if strings.Contains(err.Error(), "E404") {
			return "", false, nil
		}
		return "", false, err
	}
	var info struct {
		Version     string   `json:"version"`
		Maintainers []string `json:"maintainers"`
	}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", false, err
	}
	for _, m := range info.Maintainers {
		user := strings.Fields(m)
		for _, o := range owners {
			if len(user) > 0 && strings.EqualFold(user[0], o) {
				return info.Version, true, nil
			}
		}
	}
	return info.Version, false, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
