package dagbump

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// npmLock is one parsed lockfile. Locked versions are keyed by importer
// (the package folder relative to the lockfile's folder, "." for the same
// folder) and package name.
type npmLock struct {
	Manager string // pnpm | yarn | npm
	Dir     string // absolute folder holding the lockfile
	locked  map[string]map[string]string
	yarn    map[string]string // "name@spec" -> version
}

// findLock walks up from a package folder to the repo root and returns the
// manager and folder of the first lockfile it finds ("", "" for none).
func findLock(repoDir, pkgDir string) (manager, lockDir string) {
	for dir := pkgDir; ; dir = filepath.Dir(dir) {
		for _, c := range [][2]string{{"pnpm", "pnpm-lock.yaml"}, {"yarn", "yarn.lock"}, {"npm", "package-lock.json"}} {
			if _, err := os.Stat(filepath.Join(dir, c[1])); err == nil {
				return c[0], dir
			}
		}
		if dir == repoDir || len(dir) <= len(repoDir) {
			return "", ""
		}
	}
}

// npmPublishCmd publishes the package in the current folder. pnpm publish
// rewrites workspace: specs; yarn v1's publish prompts for a version, so
// yarn packages go through npm.
func npmPublishCmd(manager string) string {
	if manager == "pnpm" {
		return "pnpm publish"
	}
	return "npm publish"
}

var pkgVersionRe = regexp.MustCompile(`("version"\s*:\s*")[^"]*(")`)

// setPackageVersion rewrites the first "version" field of a package.json
// in place, keeping its formatting. No package manager runs, so workspace:
// specs and lifecycle scripts never get in the way.
func setPackageVersion(file, v string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	loc := pkgVersionRe.FindSubmatchIndex(data)
	if loc == nil {
		return fmt.Errorf("%s has no version field", file)
	}
	out := append(append(append([]byte{}, data[:loc[3]]...), v...), data[loc[4]:]...)
	return os.WriteFile(file, out, 0o644)
}

func loadLock(manager, dir string) (*npmLock, error) {
	l := &npmLock{Manager: manager, Dir: dir, locked: map[string]map[string]string{}, yarn: map[string]string{}}
	switch manager {
	case "pnpm":
		return l, l.loadPnpm(filepath.Join(dir, "pnpm-lock.yaml"))
	case "yarn":
		return l, l.loadYarn(filepath.Join(dir, "yarn.lock"))
	default:
		return l, l.loadNpm(filepath.Join(dir, "package-lock.json"))
	}
}

func (l *npmLock) set(importer, name, version string) {
	if l.locked[importer] == nil {
		l.locked[importer] = map[string]string{}
	}
	if i := strings.Index(version, "("); i >= 0 { // pnpm peer suffix: 0.7.7(solid-js@1.9.17)
		version = version[:i]
	}
	l.locked[importer][name] = version
}

// loadPnpm reads lockfile v6+ (importers, each dep {specifier, version})
// and v5 (top-level dependencies, plain versions).
func (l *npmLock) loadPnpm(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var raw map[string]yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	read := func(importer string, deps map[string]yaml.Node) {
		for name, n := range deps {
			switch n.Kind {
			case yaml.ScalarNode:
				l.set(importer, name, n.Value)
			case yaml.MappingNode:
				var d struct{ Version string }
				if n.Decode(&d) == nil {
					l.set(importer, name, d.Version)
				}
			}
		}
	}
	sections := []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"}
	if imp, ok := raw["importers"]; ok {
		var importers map[string]map[string]map[string]yaml.Node
		if err := imp.Decode(&importers); err != nil {
			return fmt.Errorf("%s importers: %w", file, err)
		}
		for importer, secs := range importers {
			for _, s := range sections {
				read(importer, secs[s])
			}
		}
		return nil
	}
	for _, s := range sections {
		if n, ok := raw[s]; ok {
			var deps map[string]yaml.Node
			if n.Decode(&deps) == nil {
				read(".", deps)
			}
		}
	}
	return nil
}

func (l *npmLock) loadNpm(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var pl struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &pl); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	for key, p := range pl.Packages {
		i := strings.LastIndex(key, "node_modules/")
		if i < 0 {
			continue
		}
		importer := strings.TrimSuffix(key[:i], "/")
		if importer == "" {
			importer = "."
		}
		l.set(importer, key[i+len("node_modules/"):], p.Version)
	}
	return nil
}

var yarnHeaderRe = regexp.MustCompile(`^"?([^\s].*?)"?:\s*$`)
var yarnVersionRe = regexp.MustCompile(`^\s+version:?\s+"?([^"\s]+)"?`)

// loadYarn reads yarn v1 and berry lockfiles: a header line naming the
// "name@spec" entries, then an indented version line.
func (l *npmLock) loadYarn(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var keys []string
	for _, line := range strings.Split(string(data), "\n") {
		if m := yarnHeaderRe.FindStringSubmatch(line); m != nil && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "#") {
			keys = nil
			for _, k := range strings.Split(m[1], ",") {
				keys = append(keys, strings.Trim(strings.TrimSpace(k), `"`))
			}
			continue
		}
		if m := yarnVersionRe.FindStringSubmatch(line); m != nil {
			for _, k := range keys {
				k = strings.Replace(k, "@npm:", "@", 1)
				l.yarn[k] = m[1]
			}
			keys = nil
		}
	}
	return nil
}

// Locked returns the version the lockfile resolves name to for the package
// in folder pkgDir (absolute), given its spec.
func (l *npmLock) Locked(pkgDir, name, spec string) string {
	if l.Manager == "yarn" {
		return l.yarn[name+"@"+spec]
	}
	importer, err := filepath.Rel(l.Dir, pkgDir)
	if err != nil {
		return ""
	}
	if v := l.locked[filepath.ToSlash(importer)][name]; v != "" {
		return v
	}
	return l.locked["."][name] // npm workspaces hoist to the root
}

// npmAddCmd is the command that moves a dependency to version v (no "v"
// prefix) in the package's folder, keeping its dependency section.
func npmAddCmd(manager, name, spec, section, v string) string {
	if spec == "*" || spec == "" || spec == "latest" {
		if manager == "yarn" {
			return "yarn upgrade " + shellQuote(name)
		}
		return manager + " update " + shellQuote(name)
	}
	target := shellQuote(name + "@^" + v)
	switch manager {
	case "pnpm":
		return "pnpm add " + target + map[string]string{"devDependencies": " -D", "peerDependencies": " --save-peer"}[section]
	case "yarn":
		return "yarn add " + target + map[string]string{"devDependencies": " -D", "peerDependencies": " -P"}[section]
	}
	return "npm install " + target + map[string]string{"dependencies": " --save-prod", "devDependencies": " --save-dev", "peerDependencies": " --save-peer"}[section]
}

var excludeKeyRe = regexp.MustCompile(`(?m)^minimumReleaseAgeExclude:\s*$`)

// allowFreshVersion adds name@v to pnpm-workspace.yaml's
// minimumReleaseAgeExclude list, when the file keeps one, so pnpm installs
// a version published minutes ago. An existing entry for the package gets
// " || v" appended; otherwise a new entry goes first in the list. It
// returns the file it changed, or "".
func allowFreshVersion(lockDir, name, v string) (string, error) {
	file := filepath.Join(lockDir, "pnpm-workspace.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		return "", nil
	}
	text := string(data)
	loc := excludeKeyRe.FindStringIndex(text)
	if loc == nil {
		return "", nil
	}
	entryRe := regexp.MustCompile(`(?m)^(\s*-\s*)(['"]?)` + regexp.QuoteMeta(name) + `@([^'"\n]*)(['"]?)\s*$`)
	if m := entryRe.FindStringSubmatchIndex(text); m != nil {
		versions := text[m[6]:m[7]]
		for _, have := range strings.Split(versions, "||") {
			if strings.TrimSpace(have) == v {
				return "", nil
			}
		}
		text = text[:m[7]] + " || " + v + text[m[7]:]
	} else {
		indent := "  - "
		if im := regexp.MustCompile(`(?m)^(\s*-\s*)`).FindStringSubmatch(text[loc[1]:]); im != nil {
			indent = strings.TrimLeft(im[1], "\n")
		}
		text = text[:loc[1]] + "\n" + indent + "'" + name + "@" + v + "'" + text[loc[1]:]
	}
	return file, os.WriteFile(file, []byte(text), 0o644)
}
