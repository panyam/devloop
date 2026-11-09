package dagbump

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RepoConfigFile is the per-repo release config, at the repo root.
const RepoConfigFile = ".dagbump.yaml"

// RepoConfig describes how one repo releases. A repo with one version line
// puts the unit fields at the top level; a repo with several lists them
// under units. The first unit is the main unit: it carries the root tag and
// takes every module and package no other unit claims.
type RepoConfig struct {
	UnitConfig `yaml:",inline"`
	// Kind is lib or app for the whole repo; empty means auto.
	Kind string `yaml:"kind,omitempty"`
	// Tools names this repo's tools, overriding detection and the manifest.
	Tools         ToolsConfig   `yaml:"tools,omitempty"`
	IgnoreCommits []string      `yaml:"ignore_commits,omitempty"`
	Units         []*UnitConfig `yaml:"units,omitempty"`
}

// UnitConfig is one version line: the modules and packages tagged together.
type UnitConfig struct {
	Name string `yaml:"name,omitempty"`
	// Versioning is lockstep (every module tagged at every release) or
	// changed (one version counter, only changed modules and their in-unit
	// dependents tagged).
	Versioning string `yaml:"versioning,omitempty"`
	// Modules are the go.mod folders tagged with the unit ("." is the root).
	// Globs: * matches one path segment, ** any number. Default: every
	// module that has a tag of its own.
	Modules PathList `yaml:"modules,omitempty"`
	// Follow are untagged go.mod folders (examples, docs sites) whose pins
	// on the unit's modules move with each release. Default for the main
	// unit: every module nobody else claims.
	Follow []string `yaml:"follow,omitempty"`
	// PinFollowers says which pins a follower moves at a release: root
	// (default; only its require of the root module), all (every module
	// being tagged) or none. Requires at the v0.0.0 placeholder never move.
	PinFollowers string `yaml:"pin_followers,omitempty"`
	// Npm are package.json folders whose version is the unit's version.
	// Default for the main unit: every non-private package.json; [] for none.
	Npm PathList `yaml:"npm,omitempty"`
	// VersionFiles hold the bare version string. Default: VERSION if tracked.
	VersionFiles []string `yaml:"version_files,omitempty"`
	// StripReplaces drops in-repo replace directives from the tagged
	// modules in the tagged commit, and restores them in the next commit,
	// so `go install pkg@V` works.
	StripReplaces bool          `yaml:"strip_replaces,omitempty"`
	Publish       PublishConfig `yaml:"publish,omitempty"`
	Notes         NotesConfig   `yaml:"notes,omitempty"`
	Hooks         HooksConfig   `yaml:"hooks,omitempty"`
	// Check runs after a dependency update instead of `go build ./...`.
	Check string `yaml:"check,omitempty"`
}

// PublishConfig says how releases reach their registries.
type PublishConfig struct {
	// Npm is direct (dagbump runs npm publish), ci (a workflow publishes on
	// the tag; dependents wait for npm to show it) or none.
	Npm           string `yaml:"npm,omitempty"`
	GitHubRelease *bool  `yaml:"github_release,omitempty"`
}

// NotesConfig says where release notes land in the repo. Empty fields are
// auto: CHANGELOG.md when the repo has one, docs/releases/{V}.md when the
// repo has a docs/releases folder. "none" turns a target off.
type NotesConfig struct {
	Changelog string `yaml:"changelog,omitempty"`
	File      string `yaml:"file,omitempty"`
}

// HooksConfig are shell commands run in the repo root with V and NOTES set.
type HooksConfig struct {
	PreTag      []string `yaml:"pre_tag,omitempty"`
	PostTag     []string `yaml:"post_tag,omitempty"`
	PostRelease []string `yaml:"post_release,omitempty"`
}

// PathList is a list of folder globs where an empty list ([]) means "none"
// and a missing one means "the default".
type PathList []string

// IsZero lets yaml omit only a missing list, so [] survives a round trip.
func (l PathList) IsZero() bool { return l == nil }

// AllUnits returns the units, the top-level one when units is empty.
func (c *RepoConfig) AllUnits() []*UnitConfig {
	if len(c.Units) > 0 {
		return c.Units
	}
	return []*UnitConfig{&c.UnitConfig}
}

// LoadRepoConfig reads dir/.dagbump.yaml. It returns nil, nil when there is none.
func LoadRepoConfig(dir string) (*RepoConfig, error) {
	data, err := os.ReadFile(filepath.Join(dir, RepoConfigFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := &RepoConfig{}
	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("%s: %w", RepoConfigFile, err)
	}
	if len(c.Units) > 0 && (len(c.Modules) > 0 || len(c.Npm) > 0) {
		return nil, fmt.Errorf("%s: put modules and npm either at the top level or under units, not both", RepoConfigFile)
	}
	if err := c.Tools.validate(RepoConfigFile); err != nil {
		return nil, err
	}
	for _, u := range c.AllUnits() {
		switch u.Versioning {
		case "", "lockstep", "changed":
		default:
			return nil, fmt.Errorf("%s: versioning %q must be lockstep or changed", RepoConfigFile, u.Versioning)
		}
		switch u.PinFollowers {
		case "", "root", "all", "none":
		default:
			return nil, fmt.Errorf("%s: pin_followers %q must be root, all or none", RepoConfigFile, u.PinFollowers)
		}
		switch u.Publish.Npm {
		case "", "direct", "ci", "none":
		default:
			return nil, fmt.Errorf("%s: publish.npm %q must be direct, ci or none", RepoConfigFile, u.Publish.Npm)
		}
	}
	return c, nil
}

// Marshal renders the config as YAML.
func (c *RepoConfig) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	header := "# dagbump release config: `dagbump release` reads this instead of Make targets.\n" +
		"# Fields: https://github.com/panyam/devloop/tree/main/cmd/dagbump#per-repo-config\n"
	return append([]byte(header), data...), nil
}

// globMatch matches a module folder ("" for the root) against a pattern
// where "." is the root, * is one segment and ** is any number.
func globMatch(pattern, dir string) bool {
	pattern = strings.TrimSuffix(strings.TrimPrefix(pattern, "./"), "/")
	if pattern == "." || pattern == "" {
		return dir == ""
	}
	q := regexp.QuoteMeta(pattern)
	q = strings.ReplaceAll(q, `\*\*`, "\x00")
	q = strings.ReplaceAll(q, `\*`, `[^/]*`)
	q = strings.ReplaceAll(q, "\x00", `.*`)
	ok, _ := regexp.MatchString("^"+q+"$", dir)
	return ok
}

func matchAny(patterns []string, dir string) bool {
	for _, p := range patterns {
		if globMatch(p, dir) {
			return true
		}
	}
	return false
}
