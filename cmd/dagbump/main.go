// Command dagbump finds the release DAG across local clones of Go modules
// and npm packages, says which projects need a release when one changes,
// and runs the dependency updates and releases in dependency order.
//
//	go install github.com/panyam/devloop/cmd/dagbump@latest
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/panyam/devloop/dagbump"
	"github.com/spf13/cobra"
)

var (
	manifestPath string
	jsonOut      bool
)

func main() {
	root := &cobra.Command{
		Use:   "dagbump",
		Short: "Cascade version bumps across dependent repos in dependency order",
		Long: `dagbump keeps a manifest of local clones ($DAGBUMP_MANIFEST, else ~/.config/dagbump/dagbump.yml),
reads the go.mod and package.json files in each to build the dependency DAG,
and works out which projects need a release when one of them changes.

Each repo can carry a .dagbump.yaml that says how it releases: which modules
and packages share a version, version files, hooks, publishing. dagbump release
then runs the steps itself, in place of make tag / tag-push / bump-root.

Typical flow:
  dagbump discover ~/newstack ~/projects --owner panyam --write
  dagbump init mcpkit --write      # propose and save mcpkit's .dagbump.yaml
  dagbump plan mcpkit --write      # mcpkit and everything downstream -> dagbump.plan
  # fill in bump and notes per unit in dagbump.plan (by hand, or dagbump notes --agent ...)
  dagbump apply --dry-run          # show every step
  dagbump apply --yes              # update, release, push, publish, wave by wave

  dagbump update chakra            # or one step at a time
  dagbump release chakra --version v0.4.0 --notes notes.md`,
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&manifestPath, "manifest", dagbump.DefaultManifestPath(), "manifest file ($DAGBUMP_MANIFEST)")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "print JSON")
	root.AddCommand(manifestCmd(), discoverCmd(), addCmd(), removeCmd(), linkCmd(), unlinkCmd(), initCmd(),
		listCmd(), graphCmd(), planCmd(), notesCmd(), applyCmd(), contextCmd(), updateCmd(), releaseCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func loadManifest(mustExist bool) (*dagbump.Manifest, error) {
	m, err := dagbump.LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if mustExist && !m.Exists() {
		return nil, fmt.Errorf("no manifest at %s; run `dagbump discover <roots> --write` or `dagbump add <path>`", manifestPath)
	}
	return m, nil
}

func loadGraph() (*dagbump.Graph, error) {
	m, err := loadManifest(true)
	if err != nil {
		return nil, err
	}
	return dagbump.BuildGraph(m)
}

// loadGraphFor loads the graph for a command aimed at one project. With no
// name it uses the repo holding the current folder, adding it to the
// in-memory manifest when it is not listed (or when there is no manifest).
func loadGraphFor(name string) (*dagbump.Graph, string, error) {
	m, err := loadManifest(false)
	if err != nil {
		return nil, "", err
	}
	if name == "" {
		cwd, _ := os.Getwd()
		spec, err := dagbump.NewSpec(cwd, "")
		if err != nil {
			return nil, "", fmt.Errorf("not in a git repo; name a project")
		}
		if s := m.Find(dagbump.ExpandPath(spec.Path)); s != nil {
			name = s.Name
		} else {
			if m.Find(spec.Name) != nil {
				spec.Name += "-local"
			}
			m.Projects = append(m.Projects, spec)
			name = spec.Name
		}
	} else if !m.Exists() {
		return nil, "", fmt.Errorf("no manifest at %s; run `dagbump discover <roots> --write`", manifestPath)
	}
	g, err := dagbump.BuildGraph(m)
	return g, name, err
}

func optArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func table() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0) }

// ---- manifest commands ----

func discoverCmd() *cobra.Command {
	var owners []string
	var write bool
	c := &cobra.Command{
		Use:   "discover [roots...]",
		Short: "Scan folders for clones and add them to the manifest",
		Long: `Scans each root's children and grandchildren (project/ and project/<worktree>/)
for git clones with a go.mod or package.json, groups them by remote, and picks
one clone per repo: on the default branch, in a folder named main/master, clean,
newest commit. Without --write it only prints what it would add. Existing
manifest entries keep their path.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(false)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				args = m.Roots
			}
			if len(args) == 0 {
				return fmt.Errorf("give the folders to scan, e.g. dagbump discover ~/newstack ~/projects")
			}
			if len(owners) == 0 {
				owners = m.Owners
			}
			cands, err := dagbump.Discover(args, owners, m)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cands)
			}
			w := table()
			fmt.Fprintln(w, "NAME\tCLONE\tSTATUS\tOTHER CLONES")
			for _, c := range cands {
				status := "new"
				if c.InManifest != nil {
					status = "in manifest"
				}
				var others []string
				for _, o := range c.Others {
					others = append(others, dagbump.CompactPath(o.Dir))
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, dagbump.CompactPath(c.Chosen.Dir), status, strings.Join(others, " "))
			}
			w.Flush()
			if !write {
				fmt.Println("\n(dry run; add --write to save)")
				return nil
			}
			var roots []string
			for _, a := range args {
				roots = append(roots, dagbump.CompactPath(dagbump.ExpandPath(a)))
			}
			m.Roots = mergeStrings(m.Roots, roots)
			m.Owners = mergeStrings(m.Owners, owners)
			added, moved := m.Merge(cands)
			if err := m.Save(); err != nil {
				return err
			}
			fmt.Printf("\n%s: added %d, repointed %d (%s)\n", manifestPath, len(added), len(moved), strings.Join(append(added, moved...), ", "))
			return nil
		},
	}
	c.Flags().StringSliceVar(&owners, "owner", nil, "only remotes owned by these GitHub users/orgs (repeatable)")
	c.Flags().BoolVar(&write, "write", false, "save to the manifest")
	return c
}

func manifestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "manifest",
		Short: "Show which manifest is in use and where its path came from",
		Long: `The manifest is --manifest, else $DAGBUMP_MANIFEST, else ~/.config/dagbump/dagbump.yml.
Point $DAGBUMP_MANIFEST at another file to switch lists, e.g. personal and work.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			from := "--manifest"
			if !cmd.Flags().Changed("manifest") {
				_, from = dagbump.ManifestPathSource()
			}
			m, err := loadManifest(false)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(map[string]any{"path": manifestPath, "from": from, "exists": m.Exists(),
					"projects": len(m.Projects), "roots": m.Roots, "tools": m.Tools})
			}
			fmt.Printf("%s (from %s)\n", dagbump.CompactPath(manifestPath), from)
			if !m.Exists() {
				fmt.Println("  does not exist yet; dagbump discover <roots> --write creates it")
				return nil
			}
			fmt.Printf("  %d projects, roots %s", len(m.Projects), strings.Join(m.Roots, " "))
			if m.Tools.Npm != "" {
				fmt.Printf(", npm tool %s", m.Tools.Npm)
				if m.Tools.Enforce {
					fmt.Print(" (enforced)")
				}
			}
			fmt.Println()
			return nil
		},
	}
}

func mergeStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(a, b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func addCmd() *cobra.Command {
	var name, kind string
	c := &cobra.Command{
		Use:   "add <path>",
		Short: "Add one clone to the manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(false)
			if err != nil {
				return err
			}
			s, err := dagbump.NewSpec(args[0], name)
			if err != nil {
				return err
			}
			s.Kind = kind
			if old := m.Find(s.Name); old != nil {
				return fmt.Errorf("%s is already in the manifest at %s", s.Name, old.Path)
			}
			m.Projects = append(m.Projects, s)
			if err := m.Save(); err != nil {
				return err
			}
			fmt.Printf("added %s (%s)\n", s.Name, s.Path)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "project name (default: repo name)")
	c.Flags().StringVar(&kind, "kind", "", "lib or app (default: auto)")
	return c
}

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a project from the manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(true)
			if err != nil {
				return err
			}
			if !m.Remove(args[0]) {
				return fmt.Errorf("no project %q", args[0])
			}
			return m.Save()
		},
	}
}

func linkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "link <dependent> <dependency>",
		Short: "Add a manual edge the go.mod/package.json files do not show",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(true)
			if err != nil {
				return err
			}
			from, to := m.Find(args[0]), m.Find(args[1])
			if from == nil || to == nil {
				return fmt.Errorf("both projects must be in the manifest")
			}
			from.DependsOn = mergeStrings(from.DependsOn, []string{to.Name})
			return m.Save()
		},
	}
}

func unlinkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink <dependent> <dependency>",
		Short: "Remove a manual edge",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(true)
			if err != nil {
				return err
			}
			from := m.Find(args[0])
			if from == nil {
				return fmt.Errorf("no project %q", args[0])
			}
			var keep []string
			for _, d := range from.DependsOn {
				if d != args[1] {
					keep = append(keep, d)
				}
			}
			from.DependsOn = keep
			return m.Save()
		},
	}
}

func initCmd() *cobra.Command {
	var write bool
	c := &cobra.Command{
		Use:   "init [project]",
		Short: "Propose a .dagbump.yaml for a repo from its tags and files",
		Long: `Writes one lockstep unit holding the modules tagged at the latest release,
every public package.json, VERSION if tracked, strip_replaces when the repo has
scripts/release-replaces.sh, publish.npm: ci when a workflow publishes to npm,
and a verify-submodule-deps hook when the Makefile has that target. Review it,
then replace the repo's make tag / tag-push / bump-root with dagbump release.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, name, err := loadGraphFor(optArg(args))
			if err != nil {
				return err
			}
			u, err := g.Get(name)
			if err != nil {
				return err
			}
			p := u.Project
			if p.HasConfig {
				return fmt.Errorf("%s already has %s", p.Name, dagbump.RepoConfigFile)
			}
			cfg, notes := dagbump.InferConfig(p, g.Manifest.Owners)
			data, err := cfg.Marshal()
			if err != nil {
				return err
			}
			target := filepath.Join(p.Dir, dagbump.RepoConfigFile)
			if !write {
				fmt.Printf("# %s\n%s", dagbump.CompactPath(target), data)
			}
			for _, n := range notes {
				fmt.Fprintf(os.Stderr, "note: %s\n", n)
			}
			if !write {
				fmt.Println("\n(dry run; add --write to save)")
				return nil
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				return err
			}
			fmt.Printf("wrote %s\n", dagbump.CompactPath(target))
			return nil
		},
	}
	c.Flags().BoolVar(&write, "write", false, "write the file into the repo")
	return c
}

// ---- views ----

type unitView struct {
	Name       string           `json:"name"`
	Project    string           `json:"project"`
	Path       string           `json:"path"`
	Remote     string           `json:"remote"`
	Kind       string           `json:"kind"`
	Versioning string           `json:"versioning"`
	Config     bool             `json:"has_config"`
	Version    string           `json:"version"`
	Modules    []string         `json:"tagged_modules"`
	Npm        []string         `json:"npm_packages"`
	NpmTools   []string         `json:"npm_tools,omitempty"`
	Unreleased []dagbump.Commit `json:"unreleased"`
	Lagging    []edgeView       `json:"lagging"`
	DependsOn  []string         `json:"depends_on"`
	Warnings   []string         `json:"warnings"`
	Error      string           `json:"error,omitempty"`
}

type edgeView struct {
	To         string `json:"to"`
	Kind       string `json:"kind"`
	Module     string `json:"module"`
	In         string `json:"in"`
	Required   string `json:"required"`
	Latest     string `json:"latest"`
	Lagging    bool   `json:"lagging"`
	Propagates bool   `json:"propagates"`
}

func viewEdge(e *dagbump.Edge) edgeView {
	in := e.FromDir
	switch e.Kind {
	case "go":
		in = filepath.Join(e.FromDir, "go.mod")
	case "npm":
		in = filepath.Join(e.FromDir, "package.json")
	}
	return edgeView{To: e.To.Name, Kind: e.Kind, Module: e.Module, In: in, Required: e.Required,
		Latest: e.Latest, Lagging: e.Lagging, Propagates: e.Propagates}
}

func viewUnit(g *dagbump.Graph, u *dagbump.Unit) unitView {
	p := u.Project
	v := unitView{Name: u.Name, Project: p.Name, Path: dagbump.CompactPath(p.Dir), Remote: p.Remote, Kind: u.Kind(),
		Versioning: "lockstep", Config: p.HasConfig, Version: u.Version, Unreleased: u.Unreleased,
		Warnings: p.Warnings, DependsOn: g.DependsOn(u)}
	if !u.Lockstep() {
		v.Versioning = "changed"
	}
	for _, m := range u.Modules {
		v.Modules = append(v.Modules, dot(m.Dir))
	}
	for _, n := range u.PublicNpm() {
		v.Npm = append(v.Npm, n.Name)
	}
	if p.LoadErr != nil {
		v.Error = p.LoadErr.Error()
	}
	v.Lagging = []edgeView{}
	for _, e := range g.LaggingDeps(u, false) {
		v.Lagging = append(v.Lagging, viewEdge(e))
	}
	v.Warnings = append([]string{}, v.Warnings...)
	v.Unreleased = append([]dagbump.Commit{}, v.Unreleased...)
	v.DependsOn = append([]string{}, v.DependsOn...)
	for _, n := range u.Npm {
		v.NpmTools = append(v.NpmTools, fmt.Sprintf("%s: %s (%s)", filepath.Join(n.Dir, "package.json"), n.Manager, n.ManagerFrom))
	}
	return v
}

func dot(d string) string {
	if d == "" {
		return "."
	}
	return d
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show every unit: version, unreleased commits, lagging deps, warnings",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := loadGraph()
			if err != nil {
				return err
			}
			var views []unitView
			for _, u := range g.Units {
				views = append(views, viewUnit(g, u))
			}
			sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
			if jsonOut {
				return printJSON(views)
			}
			w := table()
			fmt.Fprintln(w, "NAME\tKIND\tVERSION\tMODS\tNPM\tUNRELEASED\tLAGGING\tCONFIG\tNOTES")
			for _, v := range views {
				notes := strings.Join(v.Warnings, "; ")
				if v.Error != "" {
					notes = "ERROR: " + v.Error
				}
				cfg := "-"
				if v.Config {
					cfg = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n", v.Name, v.Kind, dash(v.Version), len(v.Modules), len(v.Npm),
					len(v.Unreleased), len(v.Lagging), cfg, notes)
			}
			return w.Flush()
		},
	}
}

func graphCmd() *cobra.Command {
	var dotOut, all bool
	c := &cobra.Command{
		Use:   "graph",
		Short: "Print the whole DAG in release waves (or as Graphviz dot)",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := loadGraph()
			if err != nil {
				return err
			}
			set := map[*dagbump.Unit]bool{}
			for _, u := range g.Units {
				if all || len(g.Deps[u]) > 0 || len(g.Users[u]) > 0 {
					set[u] = true
				}
			}
			if dotOut {
				fmt.Println("digraph dagbump {\n  rankdir=BT;")
				seen := map[string]bool{}
				for _, e := range g.Edges {
					key := e.From.Name + "->" + e.To.Name
					if seen[key] || !e.Propagates {
						continue
					}
					seen[key] = true
					style := ""
					if e.Lagging {
						style = ` [color=red]`
					}
					fmt.Printf("  %q -> %q%s;\n", e.From.Name, e.To.Name, style)
				}
				fmt.Println("}")
				return nil
			}
			waves, err := g.Waves(set)
			if err != nil {
				return err
			}
			if jsonOut {
				var out [][]unitView
				for _, w := range waves {
					var row []unitView
					for _, u := range w {
						row = append(row, viewUnit(g, u))
					}
					out = append(out, row)
				}
				return printJSON(out)
			}
			for i, w := range waves {
				fmt.Printf("wave %d\n", i+1)
				for _, u := range w {
					lag := ""
					if n := len(g.LaggingDeps(u, false)); n > 0 {
						lag = fmt.Sprintf("  [%d lagging]", n)
					}
					fmt.Printf("  %-22s %-9s <- %s%s\n", u.Name, dash(u.Version), dash(strings.Join(g.DependsOn(u), ", ")), lag)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dotOut, "dot", false, "Graphviz output (red = lagging)")
	c.Flags().BoolVar(&all, "all", false, "include units with no edges")
	return c
}

// ---- plan / context ----

type planNodeView struct {
	Wave       int              `json:"wave"`
	Unit       string           `json:"unit"`
	Project    string           `json:"project"`
	Path       string           `json:"path"`
	Kind       string           `json:"kind"`
	Versioning string           `json:"versioning"`
	Config     bool             `json:"has_config"`
	Seed       bool             `json:"seed"`
	Action     string           `json:"action"`
	Version    string           `json:"version"`
	Reasons    []string         `json:"reasons"`
	Updates    []edgeView       `json:"updates"`
	Unreleased []dagbump.Commit `json:"unreleased"`
	Warnings   []string         `json:"warnings"`
}

func planCmd() *cobra.Command {
	var changed, write bool
	var exclude []string
	var out string
	c := &cobra.Command{
		Use:   "plan [projects or units...]",
		Short: "Which units need a release, and in what order",
		Long: `Takes the seeds (a project name seeds all its units; project/unit seeds one), or
--changed for every lib with unreleased commits or lagging deps, plus everything
downstream, and orders them into waves. Each unit's action is release (lib),
update (app: deps only) or none. It reads live state, so rerun it after each
release to see what is left.

--write saves the plan to dagbump.plan (or --out): each unit's commits, changed
folders and dependency updates, with empty bump and notes fields to fill in.
dagbump apply then runs it.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			g, err := loadGraph()
			if err != nil {
				return err
			}
			opts := dagbump.PlanOptions{Changed: changed, Exclude: map[string]bool{}}
			for _, x := range exclude {
				opts.Exclude[x] = true
			}
			for _, a := range args {
				us, err := g.SeedsFor(a)
				if err != nil {
					return err
				}
				opts.Seeds = append(opts.Seeds, us...)
			}
			if len(opts.Seeds) == 0 && !changed {
				return fmt.Errorf("name the projects to start from, or pass --changed")
			}
			plan, err := g.Plan(opts)
			if err != nil {
				return err
			}
			if write {
				pf, err := g.NewPlanFile(plan, args, changed)
				if err != nil {
					return err
				}
				path := dagbump.PlanPath(out)
				if err := pf.Save(path); err != nil {
					return err
				}
				releases := 0
				for _, pu := range pf.Units {
					if pu.Action == "release" {
						releases++
					}
				}
				fmt.Printf("wrote %s: %d unit(s), %d release(s) need bump and notes\n", path, len(pf.Units), releases)
				if !jsonOut {
					return nil
				}
			}
			var views []planNodeView
			for _, n := range plan.Nodes {
				u := n.Unit
				v := planNodeView{Wave: n.Wave, Unit: u.Name, Project: u.Project.Name, Path: dagbump.CompactPath(u.Project.Dir),
					Kind: u.Kind(), Versioning: "lockstep", Config: u.Project.HasConfig, Seed: n.Seed, Action: n.Action,
					Version: u.Version, Reasons: n.Reasons, Unreleased: u.Unreleased, Warnings: u.Project.Warnings}
				if !u.Lockstep() {
					v.Versioning = "changed"
				}
				for _, e := range n.Updates {
					v.Updates = append(v.Updates, viewEdge(e))
				}
				views = append(views, v)
			}
			if jsonOut {
				return printJSON(views)
			}
			wave := 0
			for _, v := range views {
				if v.Wave != wave {
					wave = v.Wave
					fmt.Printf("wave %d\n", wave)
				}
				seed := ""
				if v.Seed {
					seed = " (seed)"
				}
				fmt.Printf("  %-8s %s %s%s\n", v.Action, v.Unit, dash(v.Version), seed)
				for _, r := range v.Reasons {
					fmt.Printf("      - %s\n", r)
				}
				printUpdates(v.Updates)
				for _, w := range v.Warnings {
					fmt.Printf("      ! %s\n", w)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&changed, "changed", false, "seed with every lib that has unreleased commits or lagging deps")
	c.Flags().StringSliceVar(&exclude, "exclude", nil, "leave these projects or units (and what only they pull in) out")
	c.Flags().BoolVar(&write, "write", false, "save the plan file for notes and apply")
	c.Flags().StringVar(&out, "out", "", "plan file path (default ./dagbump.plan)")
	return c
}

func notesCmd() *cobra.Command {
	var opts dagbump.NotesOptions
	c := &cobra.Command{
		Use:   "notes [plan file]",
		Short: "Fill bump and notes in a plan file by running an agent per release",
		Long: `Runs --agent once per release unit with empty notes, wave by wave, up to --jobs
at a time within a wave, in the unit's repo folder. The agent reads a prompt on
stdin: the instructions, then the unit's context as JSON (also in the file named
by $DAGBUMP_CONTEXT). It prints optional front matter (bump: minor) and then the
notes. The plan file is saved after each unit.

  dagbump notes --agent 'claude -p --permission-mode plan'

Plan mode lets the agent read the repo and run git show, and stops it editing.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Agent == "" {
				return fmt.Errorf("give --agent, e.g. --agent 'claude -p --permission-mode plan'")
			}
			pf, err := dagbump.LoadPlanFile(dagbump.PlanPath(optArg(args)))
			if err != nil {
				return err
			}
			opts.Out = os.Stdout
			return dagbump.FillNotes(pf, opts)
		},
	}
	c.Flags().StringVar(&opts.Agent, "agent", "", "shell command that reads the prompt on stdin and prints the notes")
	c.Flags().IntVar(&opts.Jobs, "jobs", 2, "agents to run at once within a wave")
	c.Flags().BoolVar(&opts.Force, "force", false, "redo units that already have notes")
	return c
}

func applyCmd() *cobra.Command {
	var dryRun, yes, allowEmpty bool
	var wait time.Duration
	c := &cobra.Command{
		Use:   "apply [plan file]",
		Short: "Run a plan file: updates and releases, wave by wave",
		Long: `Checks the plan (every release has bump or version, and notes), then for each
unit in order: refuses if its repo moved since the plan, moves its deps to the
latest tags, and releases it with the plan's version and notes. Progress goes
back into the plan file after each unit, so a rerun resumes after a failure.

Without --dry-run or --yes it only checks the plan and prints the releases.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := dagbump.PlanPath(optArg(args))
			pf, err := dagbump.LoadPlanFile(path)
			if err != nil {
				return err
			}
			problems := pf.Resolve(allowEmpty)
			w := table()
			fmt.Fprintln(w, "WAVE	UNIT	ACTION	FROM	TO	ONLY	STATUS")
			for _, pu := range pf.Units {
				action := pu.Action
				if pu.Skip {
					action = "skip"
				}
				fmt.Fprintf(w, "%d	%s	%s	%s	%s	%s	%s\n", pu.Wave, pu.Unit, action, dash(pu.Current), dash(pu.Version),
					dash(strings.Join(pu.Only, ",")), pu.Status)
			}
			w.Flush()
			if len(problems) > 0 {
				return fmt.Errorf("%s is not ready:\n  %s", path, strings.Join(problems, "\n  "))
			}
			if !dryRun && !yes {
				fmt.Println("\nplan is complete. --dry-run shows every step; --yes runs it.")
				return nil
			}
			m, err := loadManifest(true)
			if err != nil {
				return err
			}
			return dagbump.Apply(m, pf, dagbump.ApplyOptions{DryRun: dryRun, Wait: wait, Out: os.Stdout})
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print every step, change nothing")
	c.Flags().BoolVar(&yes, "yes", false, "run it: tags, pushes and publishes")
	c.Flags().BoolVar(&allowEmpty, "allow-empty-notes", false, "release units whose notes are empty")
	c.Flags().DurationVar(&wait, "wait", 10*time.Minute, "how long update waits for new versions to resolve")
	return c
}

// printUpdates prints one line per (module, from, to), counting the files
// that share it.
func printUpdates(updates []edgeView) {
	type key struct{ module, from, to string }
	var order []key
	files := map[key][]string{}
	for _, u := range updates {
		k := key{u.Module, u.Required, latestOrPending(u)}
		if files[k] == nil {
			order = append(order, k)
		}
		files[k] = append(files[k], u.In)
	}
	for _, k := range order {
		where := files[k][0]
		if n := len(files[k]); n > 1 {
			where = fmt.Sprintf("%d files", n)
		}
		fmt.Printf("      ~ %s %s -> %s  (%s)\n", k.module, k.from, k.to, where)
	}
}

func latestOrPending(u edgeView) string {
	if u.Lagging {
		return u.Latest
	}
	return "(new release)"
}

func contextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context [project or unit]",
		Short: "Change set since the last release, for writing notes",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, name, err := loadGraphFor(optArg(args))
			if err != nil {
				return err
			}
			u, err := g.Get(name)
			if err != nil {
				return err
			}
			c, err := g.Context(u)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(c)
			}
			fmt.Printf("%s %s (%s, %s) at %s\n", c.Unit, dash(c.Version), c.Kind, c.Versioning, c.Path)
			fmt.Printf("diff: %s\n", dash(c.DiffStat))
			section := func(title string, lines []string) {
				if len(lines) == 0 {
					return
				}
				fmt.Printf("\n%s:\n", title)
				for _, l := range lines {
					fmt.Printf("  %s\n", l)
				}
			}
			var commits []string
			for _, cm := range c.Commits {
				commits = append(commits, cm.Hash+" "+cm.Subject)
			}
			section("commits", commits)
			section("merges", c.Merges)
			section("changed module folders", c.ChangedDirs)
			section("tagged modules", c.Modules)
			section("npm packages", c.Npm)
			section("pending dependency updates", c.Pending)
			section("warnings", c.Warnings)
			return nil
		},
	}
}

// ---- mutating commands ----

// prepare loads the graph, fetches dependency tags, preflights the project
// and reloads, so the run sees the latest tags and a synced clone.
func prepare(arg string, dryRun bool) (*dagbump.Graph, *dagbump.Unit, error) {
	g, name, err := loadGraphFor(arg)
	if err != nil {
		return nil, nil, err
	}
	u, err := g.Get(name)
	if err != nil {
		return nil, nil, err
	}
	g.FetchTags(u)
	if err := dagbump.Preflight(u.Project, !dryRun); err != nil {
		return nil, nil, err
	}
	if g, err = dagbump.BuildGraph(g.Manifest); err != nil {
		return nil, nil, err
	}
	u, err = g.Get(name)
	return g, u, err
}

func updateCmd() *cobra.Command {
	var dryRun bool
	var opts dagbump.UpdateOptions
	c := &cobra.Command{
		Use:   "update [project or unit]",
		Short: "Move a unit's deps on other units to their latest tags, tidy, build, commit",
		Long: `Waits (up to --wait) for each target version to resolve: go list -m for Go
modules, npm view for packages, since CI publishes can take minutes. With no
argument it updates the repo holding the current folder.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, u, err := prepare(optArg(args), dryRun)
			if err != nil {
				return err
			}
			r := &dagbump.Runner{DryRun: dryRun, Out: os.Stdout}
			fmt.Printf("update %s\n", u.Name)
			subject, err := g.Update(u, r, opts)
			if err != nil {
				return err
			}
			if subject != "" {
				fmt.Printf("done: %s\n", subject)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the steps only")
	c.Flags().BoolVar(&opts.NoCommit, "no-commit", false, "leave the changes uncommitted")
	c.Flags().BoolVar(&opts.NoBuild, "no-build", false, "skip the build check")
	c.Flags().BoolVar(&opts.Push, "push", false, "push the commit (for apps, which have no release step)")
	c.Flags().DurationVar(&opts.Wait, "wait", 10*time.Minute, "how long to wait for new versions to resolve")
	return c
}

func releaseCmd() *cobra.Command {
	var dryRun bool
	var opts dagbump.ReleaseOptions
	c := &cobra.Command{
		Use:   "release [project or unit]",
		Short: "Tag the next version, push, and publish",
		Long: `Refuses when the tree is dirty, the clone is off its default branch, a
dependency lags its latest tag (run update first), or nothing is unreleased.
With no argument it releases the repo holding the current folder.

With a .dagbump.yaml, dagbump runs the steps:
  1. write version files; set each package.json version
  2. pin the unit's modules to V on each other (adding in-repo replaces), tidy
  3. pre_tag hooks; commit "release: V"
  4. strip_replaces: commit the tree without in-repo replaces
  5. tag: the marker tag (root, carries the notes), then <dir>/V per module
  6. restore the replaces; post_tag hooks
  7. push the branch, the marker tag alone, then the rest
  8. npm publish (publish.npm: direct), gh release, post_release hooks

lockstep units tag every module; changed units (or --only) tag the modules
changed since their own last tag, plus the unit's modules that require them.

Without a .dagbump.yaml it drives make bump-root / bump-siblings / tag when
the Makefile has them, else plain git tag.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			g, u, err := prepare(optArg(args), dryRun)
			if err != nil {
				return err
			}
			r := &dagbump.Runner{DryRun: dryRun, Out: os.Stdout}
			rp, err := g.Release(u, r, opts)
			if err != nil {
				return err
			}
			verb := "released"
			if dryRun {
				verb = "dry run: would release"
			}
			fmt.Printf("%s %s %s (%s)\n", verb, u.Name, rp.Version, strings.Join(rp.Tags, " "))
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print the steps only")
	c.Flags().StringVar(&opts.Bump, "bump", "", "patch, minor or major")
	c.Flags().StringVar(&opts.Version, "version", "", "explicit version instead of --bump")
	c.Flags().StringVar(&opts.Notes, "notes", "", "release-notes file: marker tag annotation and GitHub release body")
	c.Flags().StringSliceVar(&opts.Only, "only", nil, "tag only these modules (folders) and the unit's modules that require them")
	c.Flags().BoolVar(&opts.NoPush, "no-push", false, "stop after tagging locally")
	c.Flags().BoolVar(&opts.AllowLagging, "allow-lagging", false, "release even if deps lag their latest tags")
	c.Flags().BoolVar(&opts.Force, "force", false, "release even with no unreleased commits")
	return c
}
