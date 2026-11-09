package dagbump

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func run(dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(out.String()), fmt.Errorf("%s %s (in %s): %v: %s",
			name, strings.Join(args, " "), CompactPath(dir), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func git(dir string, args ...string) (string, error) { return run(dir, nil, "git", args...) }

// Runner executes the mutating steps of update and release. In dry-run mode
// it only prints them.
type Runner struct {
	DryRun bool
	Out    io.Writer
	Env    []string
	// Base is the project folder; steps in a sub-folder print it as a prefix.
	Base string
}

// Step prints and runs a shell command in dir, streaming its output.
func (r *Runner) Step(dir, shell string) error {
	where := ""
	if rel, err := filepath.Rel(r.Base, dir); r.Base != "" && err == nil && rel != "." {
		where = "(" + rel + ") "
	}
	fmt.Fprintf(r.Out, "  %s$ %s\n", where, shell)
	if r.DryRun {
		return nil
	}
	cmd := exec.Command("sh", "-c", shell)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), r.Env...)
	cmd.Stdout = r.Out
	cmd.Stderr = r.Out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%q in %s: %w", shell, CompactPath(dir), err)
	}
	return nil
}

// Note prints an informational line.
func (r *Runner) Note(format string, args ...any) {
	fmt.Fprintf(r.Out, "  # "+format+"\n", args...)
}

// shellQuote quotes s for sh when it contains anything but safe characters.
func shellQuote(s string) string {
	if s != "" && regexp.MustCompile(`^[A-Za-z0-9_./@:=,+-]+$`).MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// hasMakeTarget reports whether dir/Makefile defines target.
func hasMakeTarget(dir, target string) bool {
	data, err := os.ReadFile(dir + "/Makefile")
	if err != nil {
		return false
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(target) + `\s*:([^=]|$)`)
	return re.Match(data)
}

var remoteRe = regexp.MustCompile(`^(?:[a-z+]+://)?(?:[^@/]+@)?([^:/]+)[:/](.+?)(?:\.git)?/?$`)

// RemoteID normalizes a git remote URL to host/owner/name in lower case, so
// ssh and https clones of one repo compare equal.
func RemoteID(url string) string {
	m := remoteRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return strings.ToLower(url)
	}
	return strings.ToLower(m[1] + "/" + m[2])
}

// RemoteOwner returns the owner segment of a RemoteID.
func RemoteOwner(id string) string {
	parts := strings.Split(id, "/")
	if len(parts) >= 3 {
		return parts[1]
	}
	return ""
}
