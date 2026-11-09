package dagbump

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/mod/semver"
)

// Edge says unit From depends on unit To.
type Edge struct {
	From, To *Unit
	Kind     string // go | npm | manual
	// Module is the dependency's module path or npm package name.
	Module string
	// FromDir is the folder of the requiring go.mod or package.json in From's project.
	FromDir  string
	Required string
	Latest   string
	Lagging  bool
	// Propagates is true when a release of To forces a release of From: the
	// requiring module is one From tags, or a public package's runtime dep.
	Propagates bool
	// LocalReplace is set when the requiring go.mod replaces the dependency
	// with a local folder.
	LocalReplace string
}

// Graph is the loaded DAG. Its nodes are units.
type Graph struct {
	Manifest *Manifest
	Projects []*Project
	Units    []*Unit
	Edges    []*Edge
	Deps     map[*Unit][]*Edge // edges out of a unit (its dependencies)
	Users    map[*Unit][]*Edge // edges into a unit (its dependents)
	byName   map[string]*Unit
	projects map[string]*Project
}

// BuildGraph loads every non-skipped project in the manifest and derives the edges.
func BuildGraph(m *Manifest) (*Graph, error) {
	var ignore []*regexp.Regexp
	for _, s := range m.IgnoreCommits {
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("ignore_commits %q: %w", s, err)
		}
		ignore = append(ignore, re)
	}
	var specs []*ProjectSpec
	for _, s := range m.Projects {
		if !s.Skip {
			specs = append(specs, s)
		}
	}
	projects := make([]*Project, len(specs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, s := range specs {
		wg.Add(1)
		go func(i int, s *ProjectSpec) {
			defer wg.Done()
			sem <- struct{}{}
			projects[i] = LoadProject(s, ignore, m.Tools)
			<-sem
		}(i, s)
	}
	wg.Wait()
	return NewGraph(m, projects)
}

// NewGraph derives the edges between already loaded projects.
func NewGraph(m *Manifest, projects []*Project) (*Graph, error) {
	g := &Graph{
		Manifest: m,
		Projects: projects,
		Deps:     map[*Unit][]*Edge{},
		Users:    map[*Unit][]*Edge{},
		byName:   map[string]*Unit{},
		projects: map[string]*Project{},
	}
	type ownedMod struct {
		u *Unit
		m *GoModule
	}
	goIndex := map[string]ownedMod{}
	npmIndex := map[string]*Unit{}
	for _, p := range projects {
		g.projects[p.Name] = p
		for _, u := range p.Units {
			g.Units = append(g.Units, u)
			g.byName[u.Name] = u
			for _, mod := range append(append([]*GoModule{}, u.Modules...), u.Follow...) {
				if prev, ok := goIndex[mod.Path]; ok && prev.u.Project != p {
					p.Warnings = append(p.Warnings, fmt.Sprintf("module %s is also in %s", mod.Path, prev.u.Project.Name))
					continue
				}
				goIndex[mod.Path] = ownedMod{u, mod}
			}
			for _, n := range u.PublicNpm() {
				npmIndex[n.Name] = u
			}
		}
	}

	for _, p := range projects {
		replaced := map[string]map[string]bool{} // dependency project -> files pointing it at a local folder
		for _, u := range p.Units {
			tagged := map[*GoModule]bool{}
			for _, m := range u.Modules {
				tagged[m] = true
			}
			for _, mod := range append(append([]*GoModule{}, u.Modules...), u.Follow...) {
				for _, req := range sortedKeys(mod.Requires) {
					o, ok := goIndex[req]
					if !ok || o.u == u {
						continue
					}
					e := &Edge{
						From: u, To: o.u, Kind: "go", Module: req, FromDir: mod.Dir,
						Required:     mod.Requires[req],
						Latest:       p2latest(o.u, o.m),
						Propagates:   tagged[mod],
						LocalReplace: mod.LocalReplaces[req],
					}
					e.Lagging = e.Latest != "" && semver.Compare(e.Latest, e.Required) > 0
					if e.LocalReplace != "" && e.Propagates && o.u.Project != p {
						if replaced[o.u.Project.Name] == nil {
							replaced[o.u.Project.Name] = map[string]bool{}
						}
						replaced[o.u.Project.Name][modFile(mod.Dir)] = true
					}
					g.add(e)
				}
			}
			for _, n := range u.Npm {
				for _, name := range sortedKeys(n.Deps) {
					to, ok := npmIndex[name]
					if !ok || to == u {
						continue
					}
					d := n.Deps[name]
					var pkg *NpmPackage
					for _, x := range to.PublicNpm() {
						if x.Name == name {
							pkg = x
						}
					}
					e := &Edge{From: u, To: to, Kind: "npm", Module: name, FromDir: n.Dir, Required: d.Spec,
						Latest: to.NpmLatest(pkg), Propagates: !n.Private && d.Section != "devDependencies"}
					if localNpmSpec(d.Spec) {
						e.LocalReplace = d.Spec
						if e.Propagates || n.Dir != "" {
							if replaced[to.Project.Name] == nil {
								replaced[to.Project.Name] = map[string]bool{}
							}
							replaced[to.Project.Name][path.Join(n.Dir, "package.json")] = true
						}
					} else if have := npmHave(d); have != "" && e.Latest != "" {
						e.Lagging = semver.Compare(e.Latest, have) > 0
					}
					g.add(e)
				}
			}
		}
		for _, name := range sortedKeys(replaced) {
			files := sortedKeys(replaced[name])
			where := files[0]
			if len(files) > 1 {
				where = fmt.Sprintf("%s and %d more", files[0], len(files)-1)
			}
			p.Warnings = append(p.Warnings, fmt.Sprintf("%s replaces %s with a local folder; builds use that clone, not the release", where, name))
		}
	}
	for _, p := range projects {
		from := p.MainUnit()
		if from == nil {
			continue
		}
		for _, dep := range p.Spec.DependsOn {
			to, ok := g.byName[dep]
			if !ok {
				p.Warnings = append(p.Warnings, fmt.Sprintf("depends_on %q is not in the manifest", dep))
				continue
			}
			g.add(&Edge{From: from, To: to, Kind: "manual", Module: dep, Propagates: true})
		}
	}
	return g, nil
}

// p2latest is the version a consumer should require for a module: its own
// latest tag. Modules of a changed-versioning unit can lag the unit.
func p2latest(u *Unit, m *GoModule) string { return u.Project.LatestFor(m.TagPrefix) }

func modFile(dir string) string {
	if dir == "" {
		return "go.mod"
	}
	return dir + "/go.mod"
}

// localNpmSpec is true for a spec that points at a folder, not a registry version.
func localNpmSpec(spec string) bool {
	for _, p := range []string{"link:", "file:", "workspace:", "portal:"} {
		if strings.HasPrefix(spec, p) {
			return true
		}
	}
	return false
}

// npmHave is the version a package currently gets: the locked one, else the
// floor of the spec. "*" with no lock counts as up to date.
func npmHave(d NpmDep) string {
	if d.Locked != "" {
		return "v" + d.Locked
	}
	spec := strings.TrimLeft(d.Spec, "^~>= ")
	if i := strings.IndexAny(spec, " <|"); i >= 0 {
		spec = spec[:i]
	}
	if spec == "" || spec == "*" || spec == "latest" {
		return ""
	}
	if v := "v" + spec; semver.IsValid(v) {
		return v
	}
	return ""
}

func (g *Graph) add(e *Edge) {
	g.Edges = append(g.Edges, e)
	g.Deps[e.From] = append(g.Deps[e.From], e)
	g.Users[e.To] = append(g.Users[e.To], e)
}

// Get finds a unit by name. A project name (or path) gives its main unit.
func (g *Graph) Get(name string) (*Unit, error) {
	if u, ok := g.byName[name]; ok {
		return u, nil
	}
	if p := g.Project(name); p != nil && p.MainUnit() != nil {
		return p.MainUnit(), nil
	}
	return nil, fmt.Errorf("no project or unit %q in %s", name, CompactPath(g.Manifest.Path()))
}

// Project finds a project by manifest name or path.
func (g *Graph) Project(name string) *Project {
	if p, ok := g.projects[name]; ok {
		return p
	}
	if s := g.Manifest.Find(name); s != nil {
		return g.projects[s.Name]
	}
	return nil
}

// LaggingDeps returns the edges out of u whose required version is behind
// the dependency's latest tag.
func (g *Graph) LaggingDeps(u *Unit, propagatingOnly bool) []*Edge {
	var out []*Edge
	for _, e := range g.Deps[u] {
		if e.Lagging && (e.Propagates || !propagatingOnly) {
			out = append(out, e)
		}
	}
	return out
}

// DependsOn lists the units u's tagged modules depend on.
func (g *Graph) DependsOn(u *Unit) []string {
	seen := map[string]bool{}
	for _, e := range g.Deps[u] {
		if e.Propagates {
			seen[e.To.Name] = true
		}
	}
	return sortedKeys(seen)
}

// Waves orders the given units into release waves: every unit comes after
// the units it depends on. Waves are sorted by name for stable output.
func (g *Graph) Waves(set map[*Unit]bool) ([][]*Unit, error) {
	waves, err := g.waves(set, false)
	if err == nil {
		return waves, nil
	}
	// A cycle through untagged modules (examples, tests) does not constrain
	// releases; retry with only the edges that propagate.
	if w, err2 := g.waves(set, true); err2 == nil {
		return w, nil
	}
	return nil, err
}

func (g *Graph) waves(set map[*Unit]bool, propagatingOnly bool) ([][]*Unit, error) {
	deps := map[*Unit]map[*Unit]bool{}
	for u := range set {
		deps[u] = map[*Unit]bool{}
		for _, e := range g.Deps[u] {
			if set[e.To] && (!propagatingOnly || e.Propagates) {
				deps[u][e.To] = true
			}
		}
	}
	var waves [][]*Unit
	done := map[*Unit]bool{}
	for len(done) < len(set) {
		var wave []*Unit
		for u := range set {
			if done[u] {
				continue
			}
			ready := true
			for d := range deps[u] {
				if !done[d] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, u)
			}
		}
		if len(wave) == 0 {
			var stuck []string
			for u := range set {
				if !done[u] {
					stuck = append(stuck, u.Name)
				}
			}
			sort.Strings(stuck)
			return nil, fmt.Errorf("dependency cycle among: %s", strings.Join(stuck, ", "))
		}
		sort.Slice(wave, func(i, j int) bool { return wave[i].Name < wave[j].Name })
		for _, u := range wave {
			done[u] = true
		}
		waves = append(waves, wave)
	}
	return waves, nil
}

// PlanNode is one unit in a plan.
type PlanNode struct {
	Unit *Unit
	Wave int
	Seed bool
	// Action is release (tag a new version), update (dependency bump only,
	// for apps) or none.
	Action  string
	Reasons []string
	// Updates are the dependency edges to move to their latest versions.
	Updates []*Edge
}

// Plan is the ordered set of releases.
type Plan struct {
	Nodes []*PlanNode // in wave order
}

// PlanOptions picks the seeds of a plan.
type PlanOptions struct {
	Seeds   []*Unit
	Changed bool // add every lib with unreleased commits or lagging deps as a seed
	Exclude map[string]bool
}

// SeedsFor resolves a name to units: a unit name gives that unit, a project
// name gives all of the project's units.
func (g *Graph) SeedsFor(name string) ([]*Unit, error) {
	if u, ok := g.byName[name]; ok && !u.Main {
		return []*Unit{u}, nil
	}
	if p := g.Project(name); p != nil {
		return p.Units, nil
	}
	if u, ok := g.byName[name]; ok {
		return []*Unit{u}, nil
	}
	return nil, fmt.Errorf("no project or unit %q", name)
}

func (g *Graph) excluded(opts PlanOptions, u *Unit) bool {
	return opts.Exclude[u.Name] || opts.Exclude[u.Project.Name]
}

// Plan works out which units need a release because of the seeds: the seeds
// plus everything downstream of them, in dependency order. It reads live
// state, so running it again mid-cascade shows what is left.
func (g *Graph) Plan(opts PlanOptions) (*Plan, error) {
	set := map[*Unit]bool{}
	seed := map[*Unit]bool{}
	var queue []*Unit
	addSeed := func(u *Unit) {
		if g.excluded(opts, u) || set[u] {
			return
		}
		set[u], seed[u] = true, true
		queue = append(queue, u)
	}
	for _, u := range opts.Seeds {
		addSeed(u)
	}
	if opts.Changed {
		for _, u := range g.Units {
			if u.Project.LoadErr == nil && u.Kind() == "lib" && (len(u.Unreleased) > 0 || len(g.LaggingDeps(u, true)) > 0) {
				addSeed(u)
			}
		}
	}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, e := range g.Users[u] {
			if e.Propagates && !set[e.From] && !g.excluded(opts, e.From) {
				set[e.From] = true
				queue = append(queue, e.From)
			}
		}
	}

	waves, err := g.Waves(set)
	if err != nil {
		return nil, err
	}
	plan := &Plan{}
	action := map[*Unit]string{}
	for wi, wave := range waves {
		for _, u := range wave {
			n := &PlanNode{Unit: u, Wave: wi + 1, Seed: seed[u], Action: "none"}
			trigger := false
			if len(u.Unreleased) > 0 {
				trigger = true
				n.Reasons = append(n.Reasons, fmt.Sprintf("%d unreleased commit(s) since %s", len(u.Unreleased), u.Version))
			}
			seen := map[string]bool{}
			for _, e := range g.Deps[u] {
				upstreamReleasing := set[e.To] && action[e.To] == "release" && !(e.Kind == "npm" && e.LocalReplace != "")
				if !e.Lagging && !upstreamReleasing {
					continue
				}
				if e.Propagates {
					trigger = true
				}
				key := e.FromDir + "|" + e.Module
				if !seen[key] {
					seen[key] = true
					n.Updates = append(n.Updates, e)
				}
				if upstreamReleasing && e.Propagates && !seen["r|"+e.To.Name] {
					seen["r|"+e.To.Name] = true
					n.Reasons = append(n.Reasons, fmt.Sprintf("depends on %s, which is being released", e.To.Name))
				} else if e.Lagging && e.Propagates && !seen["l|"+e.Module] {
					seen["l|"+e.Module] = true
					n.Reasons = append(n.Reasons, fmt.Sprintf("requires %s %s, latest is %s", e.Module, e.Required, e.Latest))
				}
			}
			if u.Project.LoadErr != nil {
				n.Reasons = append(n.Reasons, "load error: "+u.Project.LoadErr.Error())
			}
			if trigger {
				if u.Kind() == "lib" {
					n.Action = "release"
				} else {
					n.Action = "update"
				}
			} else if n.Seed {
				n.Reasons = append(n.Reasons, "nothing unreleased")
			}
			action[u] = n.Action
			plan.Nodes = append(plan.Nodes, n)
		}
	}
	return plan, nil
}
