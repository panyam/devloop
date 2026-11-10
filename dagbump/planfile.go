package dagbump

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

// DefaultPlanFile is where plan --write saves and apply reads.
const DefaultPlanFile = "dagbump.plan"

// PlanFile is the hand-off between dagbump plan, whoever writes the notes,
// and dagbump apply.
type PlanFile struct {
	Format   int         `yaml:"dagbump_plan"`
	Created  string      `yaml:"created"`
	Manifest string      `yaml:"manifest"`
	Seeds    []string    `yaml:"seeds,omitempty"`
	Changed  bool        `yaml:"changed,omitempty"`
	Units    []*PlanUnit `yaml:"units"`

	path string
}

// PlanUnit is one unit's entry. dagbump plan fills the context fields; the
// agent or person fills bump/version, notes, and optionally only and skip;
// dagbump apply fills status, released, tags and error.
type PlanUnit struct {
	Unit       string   `yaml:"unit"`
	Wave       int      `yaml:"wave"`
	Action     string   `yaml:"action"`
	Path       string   `yaml:"path"`
	Head       string   `yaml:"head"`
	Current    string   `yaml:"current,omitempty"`
	Versioning string   `yaml:"versioning"`
	HasConfig  bool     `yaml:"has_config"`
	WaitsFor   []string `yaml:"waits_for,omitempty"`
	Reasons    []string `yaml:"reasons,omitempty"`
	Updates    []string `yaml:"updates,omitempty"`
	Commits    []string `yaml:"commits,omitempty"`
	Changed    []string `yaml:"changed_dirs,omitempty"`
	DiffStat   string   `yaml:"diffstat,omitempty"`
	Warnings   []string `yaml:"warnings,omitempty"`

	Bump    string   `yaml:"bump"`
	Version string   `yaml:"version"`
	Only    []string `yaml:"only,omitempty"`
	Skip    bool     `yaml:"skip,omitempty"`
	Notes   string   `yaml:"notes"`

	Status   string   `yaml:"status"`
	Released string   `yaml:"released,omitempty"`
	Tags     []string `yaml:"tags,omitempty"`
	Error    string   `yaml:"error,omitempty"`
}

const planHeader = `# dagbump plan: written by "dagbump plan --write", run by "dagbump apply".
#
# For each unit with action: release, fill in:
#   bump: patch | minor | major     (or version: vX.Y.Z to pick the number)
#   notes: |
#     Markdown release notes. They become the tag annotation, the GitHub
#     release body, and the CHANGELOG / docs/releases entry.
# Optional per unit:
#   only: [ext/auth]   tag just these modules (and the unit's modules that require them)
#   skip: true         leave the unit out of this run
# Leave the other fields alone. apply refuses a unit whose repo has new
# commits since "head", since its notes would describe the wrong change set.
# "dagbump notes --agent '<cmd>'" can fill bump and notes by running an agent per unit.

`

// NewPlanFile builds a plan file from a computed plan.
func (g *Graph) NewPlanFile(plan *Plan, seeds []string, changed bool) (*PlanFile, error) {
	pf := &PlanFile{Format: 1, Created: time.Now().UTC().Format(time.RFC3339), Manifest: CompactPath(g.Manifest.Path()),
		Seeds: seeds, Changed: changed}
	inPlan := map[*Unit]bool{}
	for _, n := range plan.Nodes {
		inPlan[n.Unit] = true
	}
	for _, n := range plan.Nodes {
		u, p := n.Unit, n.Unit.Project
		pu := &PlanUnit{Unit: u.Name, Wave: n.Wave, Action: n.Action, Path: CompactPath(p.Dir), Current: u.Version,
			Versioning: map[bool]string{true: "lockstep", false: "changed"}[u.Lockstep()], HasConfig: p.HasConfig,
			Reasons: n.Reasons, Warnings: p.Warnings, Status: "pending"}
		head, err := headOf(p)
		if err != nil {
			return nil, err
		}
		pu.Head = head
		waits := map[string]bool{}
		for _, e := range g.Deps[u] {
			if inPlan[e.To] && e.To != u {
				waits[e.To.Name] = true
			}
		}
		pu.WaitsFor = sortedKeys(waits)
		for _, e := range n.Updates {
			to := e.Latest
			if !e.Lagging {
				to = e.To.Name + "'s new release"
			}
			pu.Updates = append(pu.Updates, fmt.Sprintf("%s: %s %s -> %s", depFile(e), e.Module, e.Required, to))
		}
		if n.Action == "release" {
			c, err := g.Context(u)
			if err != nil {
				return nil, err
			}
			for _, cm := range c.Commits {
				pu.Commits = append(pu.Commits, cm.Hash+" "+cm.Subject)
			}
			pu.Changed, pu.DiffStat = c.ChangedDirs, c.DiffStat
		}
		if n.Action == "none" {
			pu.Status = "done"
		}
		pf.Units = append(pf.Units, pu)
	}
	return pf, nil
}

func headOf(p *Project) (string, error) {
	ref := p.DefaultBranch
	if _, err := git(p.Dir, "rev-parse", "-q", "--verify", "refs/heads/"+ref); err != nil {
		ref = "HEAD"
	}
	return git(p.Dir, "rev-parse", "--short=12", ref)
}

// LoadPlanFile reads a plan file.
func LoadPlanFile(path string) (*PlanFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pf := &PlanFile{path: path}
	if err := yaml.Unmarshal(data, pf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if pf.Format != 1 {
		return nil, fmt.Errorf("%s: not a dagbump plan (dagbump_plan: %d)", path, pf.Format)
	}
	return pf, nil
}

// Save writes the plan file to path (or where it was loaded from).
func (pf *PlanFile) Save(path string) error {
	if path != "" {
		pf.path = path
	}
	var buf bytes.Buffer
	buf.WriteString(planHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(pf); err != nil {
		return err
	}
	return os.WriteFile(pf.path, buf.Bytes(), 0o644)
}

// Path is where the plan file lives.
func (pf *PlanFile) Path() string { return pf.path }

// Resolve checks the filled-in fields and works out each release's version.
// It returns one line per problem.
func (pf *PlanFile) Resolve(allowEmptyNotes bool) []string {
	var problems []string
	for _, pu := range pf.Units {
		if pu.Skip || pu.Action != "release" || pu.Status == "done" {
			continue
		}
		v := pu.Version
		switch {
		case v != "":
			if !strings.HasPrefix(v, "v") {
				v = "v" + v
			}
			if !semver.IsValid(v) {
				problems = append(problems, fmt.Sprintf("%s: version %q is not semver", pu.Unit, pu.Version))
				continue
			}
			if pu.Current != "" && semver.Compare(v, pu.Current) <= 0 {
				problems = append(problems, fmt.Sprintf("%s: version %s is not above %s", pu.Unit, v, pu.Current))
				continue
			}
			if pu.Bump != "" && pu.Current != "" {
				if want, err := NextVersion(pu.Current, pu.Bump); err == nil && want != v {
					problems = append(problems, fmt.Sprintf("%s: bump %s gives %s, but version says %s; clear one", pu.Unit, pu.Bump, want, v))
					continue
				}
			}
		case pu.Bump != "":
			if pu.Current == "" {
				problems = append(problems, fmt.Sprintf("%s: no release yet, so give version, not bump", pu.Unit))
				continue
			}
			var err error
			if v, err = NextVersion(pu.Current, pu.Bump); err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", pu.Unit, err))
				continue
			}
		default:
			problems = append(problems, fmt.Sprintf("%s: needs bump or version", pu.Unit))
			continue
		}
		pu.Version = v
		if strings.TrimSpace(pu.Notes) == "" && !allowEmptyNotes {
			problems = append(problems, fmt.Sprintf("%s: needs notes", pu.Unit))
		}
	}
	return problems
}

// ApplyOptions controls Apply.
type ApplyOptions struct {
	DryRun bool
	Wait   time.Duration
	Out    io.Writer
}

// Apply runs the plan file unit by unit in wave order: dependency updates,
// then the release. It reloads the graph before each unit, so each one sees
// the tags the earlier ones pushed, and it saves progress to the plan file
// after each unit, so a rerun picks up where a failure stopped.
func Apply(m *Manifest, pf *PlanFile, opts ApplyOptions) error {
	if problems := pf.Resolve(false); len(problems) > 0 {
		return fmt.Errorf("the plan is not ready:\n  %s", strings.Join(problems, "\n  "))
	}
	save := func() error {
		if opts.DryRun {
			return nil
		}
		return pf.Save("")
	}
	for _, pu := range pf.Units {
		if pu.Skip || pu.Status == "done" || pu.Action == "none" {
			continue
		}
		fmt.Fprintf(opts.Out, "\n== wave %d: %s %s\n", pu.Wave, pu.Action, pu.Unit)
		err := applyUnit(m, pu, opts, save)
		if err != nil {
			pu.Status, pu.Error = "failed", err.Error()
			if serr := save(); serr != nil {
				return errors.Join(err, serr)
			}
			return fmt.Errorf("%s: %w", pu.Unit, err)
		}
		pu.Error = ""
		if err := save(); err != nil {
			return err
		}
	}
	if opts.DryRun {
		fmt.Fprintln(opts.Out, "\n# dry run: later waves show their updates against today's tags, since nothing upstream was released")
	}
	return nil
}

func applyUnit(m *Manifest, pu *PlanUnit, opts ApplyOptions, save func() error) error {
	r := &Runner{DryRun: opts.DryRun, Out: opts.Out}
	g, err := BuildGraph(m)
	if err != nil {
		return err
	}
	u, err := g.Get(pu.Unit)
	if err != nil {
		return err
	}
	p := u.Project
	if err := Preflight(p, false); err != nil {
		return err
	}
	head, err := headOf(p)
	if err != nil {
		return err
	}
	if head != pu.Head {
		return fmt.Errorf("%s moved from %s to %s since the plan; rerun dagbump plan --write and redo its notes", p.Name, pu.Head, head)
	}
	if !opts.DryRun {
		if _, err := git(p.Dir, "fetch", "--tags", "--quiet", "origin"); err != nil {
			return err
		}
		if n, err := git(p.Dir, "rev-list", "--count", "HEAD..origin/"+p.DefaultBranch); err == nil && n != "0" {
			return fmt.Errorf("origin/%s of %s has %s commit(s) the plan has not seen; pull, rerun dagbump plan --write and redo its notes", p.DefaultBranch, p.Name, n)
		}
	}
	g.FetchTags(u)
	if g, err = BuildGraph(m); err != nil {
		return err
	}
	u, _ = g.Get(pu.Unit)

	if pu.Status == "pending" || pu.Status == "failed" && pu.Released == "" {
		if _, err := g.Update(u, r, UpdateOptions{Push: pu.Action == "update", Wait: opts.Wait}); err != nil {
			return err
		}
		if !opts.DryRun {
			if pu.Head, err = headOf(p); err != nil {
				return err
			}
			pu.Status = "updated"
			if err := save(); err != nil {
				return err
			}
		}
	}
	if pu.Action == "update" {
		pu.Status = "done"
		return nil
	}

	if g, err = BuildGraph(m); err != nil {
		return err
	}
	u, _ = g.Get(pu.Unit)
	if len(u.Unreleased) == 0 && len(g.LaggingDeps(u, true)) == 0 && !opts.DryRun {
		fmt.Fprintf(opts.Out, "  # %s has nothing to release (an upstream release was skipped?); marking done\n", u.Name)
		pu.Status = "done"
		return nil
	}
	notesFile, err := os.CreateTemp("", "dagbump-notes-*.md")
	if err != nil {
		return err
	}
	defer os.Remove(notesFile.Name())
	if _, err := notesFile.WriteString(strings.TrimSpace(pu.Notes) + "\n"); err != nil {
		return err
	}
	notesFile.Close()
	rp, err := g.Release(u, r, ReleaseOptions{Version: pu.Version, Notes: notesFile.Name(), Only: pu.Only,
		AllowLagging: opts.DryRun, Force: opts.DryRun})
	if err != nil {
		return err
	}
	if !opts.DryRun {
		pu.Status, pu.Released, pu.Tags = "done", rp.Version, rp.Tags
	}
	return nil
}

// NotesOptions controls FillNotes.
type NotesOptions struct {
	Agent string // shell command; reads the prompt on stdin, prints front matter + notes
	Jobs  int
	Force bool // redo units that already have notes
	Out   io.Writer
}

const notesPrompt = `Write release notes for %s, which goes from %s to its next version.
The JSON below describes the release: the commits since %s, the changed folders,
the dependency updates it carries, and the plan's other units with their versions.
Read diffs with "git show <hash>" in %s where a subject does not explain the change.

Pick the bump:
- major: a breaking change to a public API, at 1.0 or later
- minor: new API or behaviour; also a breaking change before 1.0
- patch: fixes, docs, internal changes, or a release that only moves dependencies

Reply with front matter, then the notes in markdown, and nothing else:

---
bump: minor
---
One short paragraph on why this release exists.

Then sections as needed: Breaking, Added, Changed, Fixed, Dependencies. One line
per change, grouped by theme. Breaking items say what to change. Write plain
GitHub refs (PR 813, issue 818). No headings above the sections.

%s
`

// FillNotes runs the agent once per release unit that has no notes, wave by
// wave, up to Jobs at a time within a wave. The agent gets the prompt on
// stdin and DAGBUMP_CONTEXT pointing at the same JSON. Its reply may start
// with front matter (bump: / version:), and the rest is the notes.
func FillNotes(pf *PlanFile, opts NotesOptions) error {
	if opts.Jobs < 1 {
		opts.Jobs = 1
	}
	planned := func() map[string]string {
		out := map[string]string{}
		for _, pu := range pf.Units {
			v := pu.Version
			if v == "" && pu.Bump != "" && pu.Current != "" {
				v, _ = NextVersion(pu.Current, pu.Bump)
			}
			if v != "" {
				out[pu.Unit] = v
			}
		}
		return out
	}
	maxWave := 0
	for _, pu := range pf.Units {
		if pu.Wave > maxWave {
			maxWave = pu.Wave
		}
	}
	var mu sync.Mutex
	for wave := 1; wave <= maxWave; wave++ {
		versions := planned()
		var todo []*PlanUnit
		for _, pu := range pf.Units {
			if pu.Wave == wave && pu.Action == "release" && !pu.Skip && pu.Status != "done" &&
				(opts.Force || strings.TrimSpace(pu.Notes) == "") {
				todo = append(todo, pu)
			}
		}
		sem := make(chan struct{}, opts.Jobs)
		var wg sync.WaitGroup
		var firstErr error
		for _, pu := range todo {
			wg.Add(1)
			go func(pu *PlanUnit) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				reply, err := runNotesAgent(opts.Agent, pu, versions)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					fmt.Fprintf(opts.Out, "  %s: %v\n", pu.Unit, err)
					if firstErr == nil {
						firstErr = fmt.Errorf("%s: %w", pu.Unit, err)
					}
					return
				}
				applyNotesReply(pu, reply, opts.Force)
				fmt.Fprintf(opts.Out, "  %s: bump %s, %d lines of notes\n", pu.Unit, dashIfEmpty(pu.Bump+pu.Version), strings.Count(pu.Notes, "\n")+1)
				if err := pf.Save(""); err != nil && firstErr == nil {
					firstErr = err
				}
			}(pu)
		}
		wg.Wait()
		if firstErr != nil {
			return firstErr
		}
	}
	return nil
}

func runNotesAgent(agent string, pu *PlanUnit, versions map[string]string) (string, error) {
	ctx := map[string]any{
		"unit": pu.Unit, "path": pu.Path, "current": pu.Current, "versioning": pu.Versioning,
		"commits": pu.Commits, "changed_dirs": pu.Changed, "diffstat": pu.DiffStat,
		"dependency_updates": pu.Updates, "waits_for": pu.WaitsFor, "planned_versions": versions,
	}
	data, _ := json.MarshalIndent(ctx, "", "  ")
	tmp, err := os.CreateTemp("", "dagbump-context-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	tmp.Write(data)
	tmp.Close()
	dir := ExpandPath(pu.Path)
	prompt := fmt.Sprintf(notesPrompt, pu.Unit, dashIfEmpty(pu.Current), dashIfEmpty(pu.Current), dir, "```json\n"+string(data)+"\n```")
	cmd := exec.Command("sh", "-c", agent)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DAGBUMP_CONTEXT="+tmp.Name(), "DAGBUMP_UNIT="+pu.Unit)
	cmd.Stdin = strings.NewReader(prompt)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("agent failed: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	if strings.TrimSpace(out.String()) == "" {
		return "", errors.New("agent printed nothing")
	}
	return out.String(), nil
}

// applyNotesReply splits optional front matter (bump:, version:) from the
// notes. Front matter only fills fields that are empty, unless force.
func applyNotesReply(pu *PlanUnit, reply string, force bool) {
	body := strings.TrimSpace(reply)
	if strings.HasPrefix(body, "---") {
		rest := strings.TrimPrefix(body, "---")
		if i := strings.Index(rest, "\n---"); i >= 0 {
			var fm struct{ Bump, Version string }
			if yaml.Unmarshal([]byte(rest[:i]), &fm) == nil {
				if fm.Bump != "" && (pu.Bump == "" || force) && pu.Version == "" {
					pu.Bump = fm.Bump
				}
				if fm.Version != "" && (pu.Version == "" || force) {
					pu.Version = fm.Version
				}
			}
			body = strings.TrimSpace(rest[i+4:])
		}
	}
	pu.Notes = body + "\n"
}

// planPath resolves the plan file argument.
func PlanPath(arg string) string {
	if arg == "" {
		return DefaultPlanFile
	}
	if st, err := os.Stat(arg); err == nil && st.IsDir() {
		return filepath.Join(arg, DefaultPlanFile)
	}
	return arg
}
