package dagbump

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// notesTargets resolves where u's notes go, relative to the repo root, for
// version v. Either may be "".
func notesTargets(u *Unit, v string) (changelog, file string) {
	p, c := u.Project, u.Config.Notes
	changelog = c.Changelog
	if changelog == "" {
		if _, err := os.Stat(filepath.Join(p.Dir, "CHANGELOG.md")); err == nil {
			changelog = "CHANGELOG.md"
		}
	}
	file = c.File
	if file == "" {
		if st, err := os.Stat(filepath.Join(p.Dir, "docs", "releases")); err == nil && st.IsDir() {
			file = "docs/releases/{V}.md"
		}
	}
	if changelog == "none" {
		changelog = ""
	}
	if file == "none" {
		file = ""
	}
	return changelog, strings.ReplaceAll(file, "{V}", v)
}

// writeNotes puts the notes into the repo: a new entry at the top of the
// changelog (above the first "## " heading) and a per-release file. It
// prints what it writes; in dry-run mode it writes nothing.
func writeNotes(u *Unit, r *Runner, v, notesPath string) error {
	if notesPath == "" {
		return nil
	}
	changelog, file := notesTargets(u, v)
	if changelog == "" && file == "" {
		return nil
	}
	body, err := os.ReadFile(notesPath)
	if err != nil {
		return err
	}
	notes := strings.TrimSpace(string(body)) + "\n"
	p := u.Project
	date := time.Now().Format("2006-01-02")
	if file != "" {
		r.Note("write notes to %s", file)
		if !r.DryRun {
			full := filepath.Join(p.Dir, file)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(full, []byte(fmt.Sprintf("# %s (%s)\n\n%s", v, date, notes)), 0o644); err != nil {
				return err
			}
		}
	}
	if changelog != "" {
		r.Note("add a %s entry to %s", v, changelog)
		if !r.DryRun {
			full := filepath.Join(p.Dir, changelog)
			old, _ := os.ReadFile(full)
			entry := fmt.Sprintf("## [%s] - %s\n\n%s", v, date, notes)
			if file != "" {
				entry += fmt.Sprintf("\nFull notes: [%s](%s)\n", file, file)
			}
			text := string(old)
			switch i := strings.Index(text, "\n## "); {
			case text == "":
				text = "# Changelog\n\n" + entry
			case i >= 0:
				text = text[:i+1] + entry + "\n" + text[i+1:]
			default:
				text = strings.TrimRight(text, "\n") + "\n\n" + entry
			}
			if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
				return err
			}
		}
	}
	if r.DryRun {
		return nil
	}
	// New files are untracked, and the release commit uses git add -u.
	var paths []string
	for _, f := range []string{changelog, file} {
		if f != "" {
			paths = append(paths, shellQuote(f))
		}
	}
	return r.Step(p.Dir, "git add -- "+strings.Join(paths, " "))
}
