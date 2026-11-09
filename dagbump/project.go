package dagbump

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// GoModule is one tracked go.mod in a project.
type GoModule struct {
	Path string // module path
	Dir  string // folder relative to the project root, "" for the root
	// TagPrefix is the tag prefix Go expects for this module: Dir, minus a
	// trailing major-version folder (v2, v3...) that the module path also ends in.
	TagPrefix string
	Requires  map[string]string // module path -> required version
	// LocalReplaces maps a replaced module path to the local folder it points at.
	LocalReplaces map[string]string
	// Published is true when the module has at least one tag of its own.
	Published bool
}

// NpmDep is one entry from package.json's dependency sections.
type NpmDep struct {
	Spec    string
	Section string // dependencies | devDependencies | peerDependencies
	Locked  string // version from package-lock.json, if any
}

// NpmPackage is one tracked package.json.
type NpmPackage struct {
	Dir     string // "" for the root
	Name    string
	Version string
	Private bool
	Deps    map[string]NpmDep
	// Manager is the tool dagbump runs for this package: npm, pnpm or yarn.
	Manager string
	// ManagerFrom says where Manager came from: .dagbump.yaml,
	// packageManager, lockfile, manifest or default.
	ManagerFrom string
	// LockDir is the absolute folder of the nearest lockfile ("" when there is none).
	LockDir string
	// PolicyErr is set when the manifest enforces another tool.
	PolicyErr string
}

// Commit is one unreleased commit.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
}

// Project is a manifest entry loaded from its clone.
type Project struct {
	Spec          *ProjectSpec
	Name          string
	Dir           string
	Remote        string
	Branch        string
	DefaultBranch string
	Dirty         bool
	Modules       []*GoModule
	Npm           []*NpmPackage
	// Config is the repo's .dagbump.yaml, or one inferred from its tags.
	Config *RepoConfig
	// HasConfig is true when Config came from a .dagbump.yaml.
	HasConfig bool
	Units     []*Unit
	Warnings  []string
	LoadErr   error

	tags      map[string][]string // tag prefix ("" for root) -> versions
	tagSet    map[string]bool
	locks     map[string]*npmLock
	tools     ToolsConfig // the manifest's
	npmViaCI  bool
	ignore    []*regexp.Regexp
	unitOfDir map[string]*Unit
}

// Unit is one version line inside a project.
type Unit struct {
	Project *Project
	Config  *UnitConfig
	// Name is the project name for the main unit, project/unit otherwise.
	Name    string
	Main    bool
	Modules []*GoModule   // tagged with the unit
	Follow  []*GoModule   // untagged; their pins move with the unit
	Npm     []*NpmPackage // public ones are versioned with the unit; private ones only consume
	// Version is the latest release of the unit ("" when there is none).
	Version    string
	Unreleased []Commit
}

// Kind is "lib" (released with tags) or "app" (only dependency updates).
func (u *Unit) Kind() string {
	p := u.Project
	if p.Spec != nil && p.Spec.Kind != "" && u.Main {
		return p.Spec.Kind
	}
	if p.Config != nil && p.Config.Kind != "" {
		return p.Config.Kind
	}
	if u.Version != "" || (p.HasConfig && (len(u.Modules) > 0 || len(u.PublicNpm()) > 0)) {
		return "lib"
	}
	if len(u.Modules) == 0 && p.RootModule() == nil && len(u.PublicNpm()) > 0 && u.PublicNpm()[0].Version != "" {
		return "lib"
	}
	return "app"
}

// Lockstep is true unless the unit uses changed versioning.
func (u *Unit) Lockstep() bool { return u.Config.Versioning != "changed" }

// PublicNpm returns the unit's packages that get its version.
func (u *Unit) PublicNpm() []*NpmPackage {
	var out []*NpmPackage
	for _, n := range u.Npm {
		if !n.Private {
			out = append(out, n)
		}
	}
	return out
}

// PrimaryPrefix is the tag prefix of the unit's release marker tag: "" for
// the main unit, else its first module's prefix or first package folder.
func (u *Unit) PrimaryPrefix() string {
	if u.Main {
		return ""
	}
	if len(u.Modules) > 0 {
		return u.Modules[0].TagPrefix
	}
	if pub := u.PublicNpm(); len(pub) > 0 {
		return pub[0].Dir
	}
	return ""
}

// NpmPublish is direct, ci or none.
func (u *Unit) NpmPublish() string {
	if u.Config.Publish.Npm != "" {
		return u.Config.Publish.Npm
	}
	if u.Project.npmViaCI {
		return "ci"
	}
	return "direct"
}

// Dirs returns every folder the unit owns.
func (u *Unit) Dirs() []string {
	var out []string
	for _, m := range u.Modules {
		out = append(out, m.Dir)
	}
	for _, m := range u.Follow {
		out = append(out, m.Dir)
	}
	for _, n := range u.Npm {
		out = append(out, n.Dir)
	}
	return out
}

// Pathspec returns git pathspecs for the unit's files: its folders, minus
// folders nested inside them that belong to another unit.
func (u *Unit) Pathspec() []string {
	mine := map[string]bool{}
	for _, d := range u.Dirs() {
		mine[d] = true
	}
	var specs []string
	if mine[""] {
		specs = append(specs, ".")
	} else {
		specs = append(specs, sortedKeys(mine)...)
	}
	for _, other := range u.Project.Units {
		if other == u {
			continue
		}
		for _, d := range other.Dirs() {
			if d != "" && !mine[d] {
				specs = append(specs, ":(exclude)"+d)
			}
		}
	}
	return specs
}

func (p *Project) RootModule() *GoModule {
	for _, m := range p.Modules {
		if m.Dir == "" {
			return m
		}
	}
	return nil
}

// MainUnit is the unit that carries the root tag.
func (p *Project) MainUnit() *Unit {
	if len(p.Units) == 0 {
		return nil
	}
	return p.Units[0]
}

// LatestFor returns the highest stable version tagged with prefix, or the
// highest pre-release when no stable one exists.
func (p *Project) LatestFor(prefix string) string {
	var best, bestPre string
	for _, v := range p.tags[prefix] {
		if semver.Prerelease(v) == "" {
			if best == "" || semver.Compare(v, best) > 0 {
				best = v
			}
		} else if bestPre == "" || semver.Compare(v, bestPre) > 0 {
			bestPre = v
		}
	}
	if best != "" {
		return best
	}
	return bestPre
}

// HasTag reports whether the tag exists locally.
func (p *Project) HasTag(tag string) bool { return p.tagSet[tag] }

// NpmLatest is the version consumers of a package should depend on.
func (u *Unit) NpmLatest(n *NpmPackage) string {
	if v := u.Version; v != "" {
		return v
	}
	if n.Version != "" {
		return "v" + n.Version
	}
	return ""
}

// TagName gives the git tag for a module prefix at version v.
func TagName(prefix, v string) string {
	if prefix == "" {
		return v
	}
	return prefix + "/" + v
}

var majorDirRe = regexp.MustCompile(`^v[0-9]+$`)

// LoadProject reads a project's clone: git state, tags, go.mod and
// package.json files, and its .dagbump.yaml. Errors are kept on the project
// so one broken clone does not stop the rest of the graph from loading.
func LoadProject(spec *ProjectSpec, ignore []*regexp.Regexp, tools ToolsConfig) *Project {
	p := &Project{Spec: spec, Name: spec.Name, Dir: ExpandPath(spec.Path), tags: map[string][]string{}, tagSet: map[string]bool{},
		ignore: ignore, tools: tools}
	if err := p.load(); err != nil {
		p.LoadErr = err
	}
	return p
}

func (p *Project) load() error {
	if st, err := os.Stat(p.Dir); err != nil || !st.IsDir() {
		return fmt.Errorf("folder %s does not exist", CompactPath(p.Dir))
	}
	url, err := git(p.Dir, "remote", "get-url", "origin")
	if err == nil {
		p.Remote = RemoteID(url)
	}
	p.Branch, _ = git(p.Dir, "branch", "--show-current")
	p.DefaultBranch = defaultBranch(p.Dir)
	status, err := git(p.Dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return err
	}
	p.Dirty = status != ""
	if p.Branch != p.DefaultBranch {
		p.Warnings = append(p.Warnings, fmt.Sprintf("on branch %q, not %q", p.Branch, p.DefaultBranch))
	}
	if p.Dirty {
		p.Warnings = append(p.Warnings, "has uncommitted changes")
	}

	tagList, err := git(p.Dir, "tag", "-l")
	if err != nil {
		return err
	}
	for _, t := range strings.Fields(tagList) {
		p.tagSet[t] = true
		prefix, v := "", t
		if i := strings.LastIndex(t, "/"); i >= 0 {
			prefix, v = t[:i], t[i+1:]
		}
		if semver.IsValid(v) {
			p.tags[prefix] = append(p.tags[prefix], v)
		}
	}

	cfg, err := LoadRepoConfig(p.Dir)
	if err != nil {
		return err
	}
	p.HasConfig = cfg != nil
	if cfg == nil {
		cfg = &RepoConfig{}
	}
	p.Config = cfg

	files, err := git(p.Dir, "ls-files", "--", ":(glob)**/go.mod", ":(glob)**/package.json")
	if err != nil {
		return err
	}
	for _, f := range strings.Fields(files) {
		if strings.Contains(f, "node_modules/") || strings.Contains(f, "/testdata/") || strings.HasPrefix(f, "testdata/") {
			continue
		}
		var err error
		if path.Base(f) == "package.json" {
			err = p.loadNpm(f)
		} else {
			var m *GoModule
			if m, err = p.loadGoMod(f); err == nil {
				p.Modules = append(p.Modules, m)
			}
		}
		if err != nil {
			p.Warnings = append(p.Warnings, err.Error())
		}
	}
	sort.Slice(p.Modules, func(i, j int) bool { return p.Modules[i].Dir < p.Modules[j].Dir })
	sort.Slice(p.Npm, func(i, j int) bool { return p.Npm[i].Dir < p.Npm[j].Dir })
	p.npmViaCI = workflowsPublishNpm(p.Dir)

	for _, s := range cfg.IgnoreCommits {
		re, err := regexp.Compile(s)
		if err != nil {
			return fmt.Errorf("%s ignore_commits %q: %w", RepoConfigFile, s, err)
		}
		p.ignore = append(p.ignore, re)
	}
	p.buildUnits()
	for _, u := range p.Units {
		if err := p.loadUnreleased(u); err != nil {
			return err
		}
	}
	return nil
}

// buildUnits assigns modules and packages to units. Explicit globs win;
// the main unit takes what is left: the root module and tagged modules as
// members, the rest as followers, every package.json.
func (p *Project) buildUnits() {
	cfgs := p.Config.AllUnits()
	p.unitOfDir = map[string]*Unit{}
	for i, c := range cfgs {
		u := &Unit{Project: p, Config: c, Main: i == 0, Name: p.Name}
		if !u.Main {
			name := c.Name
			if name == "" {
				name = fmt.Sprint(i)
			}
			u.Name = p.Name + "/" + name
		}
		p.Units = append(p.Units, u)
	}
	claimed := map[string]bool{}
	for _, m := range p.Modules {
		for _, u := range p.Units {
			if matchAny(u.Config.Modules, m.Dir) {
				u.Modules = append(u.Modules, m)
				claimed[m.Dir] = true
				break
			}
			if matchAny(u.Config.Follow, m.Dir) {
				u.Follow = append(u.Follow, m)
				claimed[m.Dir] = true
				break
			}
		}
	}
	main := p.Units[0]
	for _, m := range p.Modules {
		if claimed[m.Dir] {
			continue
		}
		if main.Config.Modules == nil && (m.Published || m.Dir == "") {
			main.Modules = append(main.Modules, m)
		} else {
			main.Follow = append(main.Follow, m)
		}
	}
	for _, n := range p.Npm {
		owner := main
		for _, u := range p.Units {
			if matchAny(u.Config.Npm, n.Dir) {
				owner = u
				break
			}
		}
		if owner == main && main.Config.Npm != nil && !matchAny(main.Config.Npm, n.Dir) && !n.Private {
			// The main unit lists its packages; an unlisted public one only consumes.
			n.Private = true
		}
		owner.Npm = append(owner.Npm, n)
	}
	for _, u := range p.Units {
		for _, d := range u.Dirs() {
			p.unitOfDir[d] = u
		}
		u.Version = p.unitVersion(u)
	}
}

// unitVersion is the highest release among the unit's marker and module tags.
func (p *Project) unitVersion(u *Unit) string {
	prefixes := []string{u.PrimaryPrefix()}
	for _, m := range u.Modules {
		prefixes = append(prefixes, m.TagPrefix)
	}
	best := ""
	for _, pr := range prefixes {
		if v := p.LatestFor(pr); v != "" && (best == "" || semver.Compare(v, best) > 0) {
			best = v
		}
	}
	if best == "" && len(u.Modules) == 0 {
		if pub := u.PublicNpm(); len(pub) > 0 && pub[0].Version != "" && semver.IsValid("v"+pub[0].Version) {
			best = "v" + pub[0].Version
		}
	}
	return best
}

// baseTag finds the tag of the unit's current version to diff from.
func (p *Project) baseTag(u *Unit) string {
	if u.Version == "" {
		return ""
	}
	prefixes := []string{u.PrimaryPrefix()}
	for _, m := range u.Modules {
		prefixes = append(prefixes, m.TagPrefix)
	}
	for _, pr := range prefixes {
		if t := TagName(pr, u.Version); p.HasTag(t) {
			return t
		}
	}
	return ""
}

func (p *Project) logSince(base string, pathspec []string) ([]Commit, error) {
	ref := p.DefaultBranch
	if _, err := git(p.Dir, "rev-parse", "-q", "--verify", "refs/heads/"+ref); err != nil {
		ref = "HEAD"
	}
	args := []string{"log", "--format=%h%x09%s", base + ".." + ref, "--"}
	out, err := git(p.Dir, append(args, pathspec...)...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		hash, subject, _ := strings.Cut(line, "\t")
		skip := false
		for _, re := range p.ignore {
			if re.MatchString(subject) {
				skip = true
				break
			}
		}
		if !skip {
			commits = append(commits, Commit{Hash: hash, Subject: subject})
		}
	}
	return commits, nil
}

func (p *Project) loadUnreleased(u *Unit) error {
	base := p.baseTag(u)
	if base == "" {
		return nil
	}
	c, err := p.logSince(base, u.Pathspec())
	u.Unreleased = c
	return err
}

// ModuleChanged reports whether m has commits since its own latest tag,
// not counting folders of other modules nested inside it. A module that was
// never tagged counts as changed.
func (p *Project) ModuleChanged(m *GoModule) (bool, error) {
	v := p.LatestFor(m.TagPrefix)
	if v == "" {
		return true, nil
	}
	spec := []string{"."}
	if m.Dir != "" {
		spec = []string{m.Dir}
	}
	for _, o := range p.Modules {
		if o != m && (m.Dir == "" || strings.HasPrefix(o.Dir, m.Dir+"/")) && o.Dir != "" {
			spec = append(spec, ":(exclude)"+o.Dir)
		}
	}
	c, err := p.logSince(TagName(m.TagPrefix, v), spec)
	return len(c) > 0, err
}

func (p *Project) loadGoMod(rel string) (*GoModule, error) {
	full := filepath.Join(p.Dir, rel)
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	f, err := modfile.Parse(full, data, nil)
	if err != nil {
		return nil, err
	}
	if f.Module == nil {
		return nil, fmt.Errorf("%s has no module line", rel)
	}
	dir := path.Dir(filepath.ToSlash(rel))
	if dir == "." {
		dir = ""
	}
	m := &GoModule{
		Path:          f.Module.Mod.Path,
		Dir:           dir,
		TagPrefix:     dir,
		Requires:      map[string]string{},
		LocalReplaces: map[string]string{},
	}
	if base := path.Base(dir); dir != "" && majorDirRe.MatchString(base) && strings.HasSuffix(m.Path, "/"+base) {
		m.TagPrefix = path.Dir(dir)
		if m.TagPrefix == "." {
			m.TagPrefix = ""
		}
	}
	for _, r := range f.Require {
		m.Requires[r.Mod.Path] = r.Mod.Version
	}
	for _, r := range f.Replace {
		if r.New.Version == "" {
			m.LocalReplaces[r.Old.Path] = r.New.Path
		}
	}
	m.Published = len(p.tags[m.TagPrefix]) > 0
	return m, nil
}

func (p *Project) loadNpm(rel string) error {
	full := filepath.Join(p.Dir, rel)
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	var pj struct {
		Name             string            `json:"name"`
		Version          string            `json:"version"`
		Private          bool              `json:"private"`
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		PackageManager   string            `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	dir := path.Dir(filepath.ToSlash(rel))
	if dir == "." {
		dir = ""
	}
	n := &NpmPackage{Dir: dir, Name: pj.Name, Version: pj.Version, Private: pj.Private || pj.Name == "", Deps: map[string]NpmDep{}}
	for section, deps := range map[string]map[string]string{
		"peerDependencies": pj.PeerDependencies, "devDependencies": pj.DevDependencies, "dependencies": pj.Dependencies,
	} {
		for name, spec := range deps {
			n.Deps[name] = NpmDep{Spec: spec, Section: section}
		}
	}
	pkgDir := filepath.Dir(full)
	var lockMgr string
	lockMgr, n.LockDir = findLock(p.Dir, pkgDir)
	p.pickManager(n, rel, lockMgr, pj.PackageManager)
	if n.LockDir != "" {
		if p.locks == nil {
			p.locks = map[string]*npmLock{}
		}
		lock, ok := p.locks[n.LockDir]
		if !ok {
			var err error
			if lock, err = loadLock(n.Manager, n.LockDir); err != nil {
				p.Warnings = append(p.Warnings, err.Error())
				lock = nil
			}
			p.locks[n.LockDir] = lock
		}
		if lock != nil {
			for name, d := range n.Deps {
				d.Locked = lock.Locked(pkgDir, name, d.Spec)
				if strings.HasPrefix(d.Locked, "link:") || strings.HasPrefix(d.Locked, "file:") {
					d.Locked = ""
				}
				n.Deps[name] = d
			}
		}
	}
	p.Npm = append(p.Npm, n)
	return nil
}

// pickManager chooses the package's tool: the repo's .dagbump.yaml, then
// package.json's packageManager, then the lockfile, then the manifest's
// default, then npm. A declared tool that disagrees with the lockfile, or a
// tool the manifest's enforced choice rules out, is recorded.
func (p *Project) pickManager(n *NpmPackage, rel, lockMgr, packageManager string) {
	declaredPM := ""
	for _, m := range []string{"pnpm", "yarn", "npm"} {
		if strings.HasPrefix(packageManager, m+"@") || packageManager == m {
			declaredPM = m
		}
	}
	switch {
	case p.Config.Tools.Npm != "":
		n.Manager, n.ManagerFrom = p.Config.Tools.Npm, RepoConfigFile
	case declaredPM != "":
		n.Manager, n.ManagerFrom = declaredPM, "packageManager"
	case n.LockDir != "":
		n.Manager, n.ManagerFrom = lockMgr, "lockfile"
	case p.tools.Npm != "":
		n.Manager, n.ManagerFrom = p.tools.Npm, "manifest"
	default:
		n.Manager, n.ManagerFrom = "npm", "default"
	}
	if n.LockDir != "" && lockMgr != n.Manager {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%s: %s says %s, but the lockfile is %s's", rel, n.ManagerFrom, n.Manager, lockMgr))
	}
	if p.tools.Enforce && p.tools.Npm != "" && n.Manager != p.tools.Npm && n.ManagerFrom != RepoConfigFile {
		n.PolicyErr = fmt.Sprintf("%s uses %s (from %s); the manifest enforces %s. Switch it (%s), or set tools.npm in its %s to opt out",
			rel, n.Manager, n.ManagerFrom, p.tools.Npm, switchHint(n.Manager, p.tools.Npm), RepoConfigFile)
		p.Warnings = append(p.Warnings, n.PolicyErr)
	}
}

func switchHint(from, to string) string {
	if to == "pnpm" {
		return "pnpm import reads the " + from + " lockfile; commit pnpm-lock.yaml and drop the old one"
	}
	return "regenerate the lockfile with " + to
}

var npmPublishRe = regexp.MustCompile(`(npm|pnpm|yarn) publish|npm-publish`)

// workflowsPublishNpm reports whether a GitHub workflow publishes to npm.
func workflowsPublishNpm(dir string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, ".github", "workflows", "*.y*ml"))
	for _, f := range files {
		if data, err := os.ReadFile(f); err == nil && npmPublishRe.Match(data) {
			return true
		}
	}
	return false
}

func defaultBranch(dir string) string {
	if ref, err := git(dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(ref, "origin/")
	}
	for _, b := range []string{"main", "master"} {
		if _, err := git(dir, "rev-parse", "-q", "--verify", "refs/heads/"+b); err == nil {
			return b
		}
	}
	return "main"
}
