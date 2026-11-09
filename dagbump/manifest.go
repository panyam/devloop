// Package dagbump finds the dependency DAG across a set of local git clones
// (Go modules and npm packages), works out which projects need a release
// when one of them changes, and runs the dependency updates and releases in
// order.
package dagbump

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest lists the projects in the DAG. Edges are not stored here: they
// are read from each project's go.mod and package.json files every run, plus
// any manual DependsOn entries.
type Manifest struct {
	// Owners limits discovery to remotes owned by these GitHub users/orgs.
	Owners []string `yaml:"owners,omitempty"`
	// Roots are the folders that discover scans (each child, and each
	// grandchild, that is a git clone).
	Roots []string `yaml:"roots,omitempty"`
	// IgnoreCommits are regexes on commit subjects that do not count as
	// unreleased changes (for example the restore commit some repos make
	// right after tagging).
	IgnoreCommits []string `yaml:"ignore_commits,omitempty"`
	// Tools sets the default per-ecosystem tool and whether to enforce it.
	Tools    ToolsConfig    `yaml:"tools,omitempty"`
	Projects []*ProjectSpec `yaml:"projects"`

	path   string
	exists bool
}

// ProjectSpec is one project's entry in the manifest. Only Name and Path are
// required. How the repo releases lives in its own .dagbump.yaml.
type ProjectSpec struct {
	Name   string `yaml:"name"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote,omitempty"`
	// Kind is "lib" (tagged releases) or "app" (dependency updates only).
	// Empty means auto: lib when the repo has a semver tag.
	Kind      string   `yaml:"kind,omitempty"`
	DependsOn []string `yaml:"depends_on,omitempty"`
	Skip      bool     `yaml:"skip,omitempty"`
}

// ToolsConfig picks the tool per ecosystem. In the manifest it is the
// default for repos that declare nothing; with Enforce, dagbump refuses to
// run any other tool in a repo whose .dagbump.yaml does not name its own.
// In a .dagbump.yaml it is that repo's choice, and Enforce is ignored.
type ToolsConfig struct {
	Npm     string `yaml:"npm,omitempty"` // npm | pnpm | yarn
	Enforce bool   `yaml:"enforce,omitempty"`
}

func (t ToolsConfig) validate(where string) error {
	switch t.Npm {
	case "", "npm", "pnpm", "yarn":
		return nil
	}
	return fmt.Errorf("%s: tools.npm %q must be npm, pnpm or yarn", where, t.Npm)
}

var defaultIgnoreCommits = []string{`^release: `}

// DefaultManifestPath is $DAGBUMP_MANIFEST, else ~/.config/dagbump/dagbump.yml.
func DefaultManifestPath() string {
	p, _ := ManifestPathSource()
	return p
}

// ManifestPathSource returns the default manifest path and where it came
// from: "$DAGBUMP_MANIFEST" or "default".
func ManifestPathSource() (string, string) {
	if p := os.Getenv("DAGBUMP_MANIFEST"); p != "" {
		return ExpandPath(p), "$DAGBUMP_MANIFEST"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "dagbump", "dagbump.yml"), "default"
}

// LoadManifest reads the manifest at path. A missing file gives an empty
// manifest with Exists() false.
func LoadManifest(path string) (*Manifest, error) {
	m := &Manifest{path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		m.IgnoreCommits = append([]string(nil), defaultIgnoreCommits...)
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	m.exists = true
	if err := m.Tools.validate(path); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range m.Projects {
		if p.Name == "" || p.Path == "" {
			return nil, fmt.Errorf("%s: every project needs a name and a path", path)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("%s: project %q is listed twice", path, p.Name)
		}
		seen[p.Name] = true
	}
	return m, nil
}

func (m *Manifest) Path() string { return m.path }
func (m *Manifest) Exists() bool { return m.exists }

// Save writes the manifest back, projects sorted by name.
func (m *Manifest) Save() error {
	sort.Slice(m.Projects, func(i, j int) bool { return m.Projects[i].Name < m.Projects[j].Name })
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	header := "# dagbump manifest: the projects in the release DAG.\n" +
		"# Edges come from each project's go.mod / package.json; depends_on adds manual ones.\n"
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.path, append([]byte(header), data...), 0o644); err != nil {
		return err
	}
	m.exists = true
	return nil
}

// Find returns the spec whose name matches, or whose path resolves to the
// same folder as name.
func (m *Manifest) Find(name string) *ProjectSpec {
	for _, p := range m.Projects {
		if p.Name == name {
			return p
		}
	}
	abs, err := filepath.Abs(ExpandPath(name))
	if err != nil {
		return nil
	}
	for _, p := range m.Projects {
		if ExpandPath(p.Path) == abs {
			return p
		}
	}
	return nil
}

func (m *Manifest) Remove(name string) bool {
	for i, p := range m.Projects {
		if p.Name == name {
			m.Projects = append(m.Projects[:i], m.Projects[i+1:]...)
			return true
		}
	}
	return false
}

// ExpandPath expands a leading ~ and cleans the path.
func ExpandPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Clean(p)
}

// CompactPath replaces the home folder prefix with ~.
func CompactPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
