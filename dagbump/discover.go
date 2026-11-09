package dagbump

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Clone is one git checkout found under a root.
type Clone struct {
	Dir        string
	Remote     string
	Branch     string
	OnDefault  bool
	Clean      bool
	CommitTime int64
	TopLevel   bool // sits directly under a root, not in a worktree group
}

// Candidate groups the clones of one remote and names the one picked.
type Candidate struct {
	Name   string
	Remote string
	Chosen *Clone
	Others []*Clone
	// InManifest is the existing manifest entry for this remote, if any.
	InManifest *ProjectSpec
}

// Discover scans each root's children and grandchildren for git clones,
// groups them by remote, keeps the remotes whose owner is in owners (all
// when owners is empty) and that have a go.mod or package.json, and picks
// one clone per remote.
func Discover(roots, owners []string, m *Manifest) ([]*Candidate, error) {
	var clones []*Clone
	for _, root := range roots {
		root = ExpandPath(root)
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			dir := filepath.Join(root, e.Name())
			if isClone(dir) {
				clones = appendClone(clones, dir, true)
				continue
			}
			sub, _ := os.ReadDir(dir)
			for _, s := range sub {
				if s.IsDir() && isClone(filepath.Join(dir, s.Name())) {
					clones = appendClone(clones, filepath.Join(dir, s.Name()), false)
				}
			}
		}
	}

	ownerOK := map[string]bool{}
	for _, o := range owners {
		ownerOK[strings.ToLower(o)] = true
	}
	byRemote := map[string][]*Clone{}
	for _, c := range clones {
		if c.Remote == "" || (len(ownerOK) > 0 && !ownerOK[RemoteOwner(c.Remote)]) {
			continue
		}
		byRemote[c.Remote] = append(byRemote[c.Remote], c)
	}

	existing := map[string]*ProjectSpec{}
	for _, s := range m.Projects {
		r := s.Remote
		if r == "" {
			if url, err := git(ExpandPath(s.Path), "remote", "get-url", "origin"); err == nil {
				r = RemoteID(url)
			}
		}
		if r != "" {
			existing[r] = s
		}
	}

	var out []*Candidate
	for remote, cs := range byRemote {
		sort.Slice(cs, func(i, j int) bool { return cloneBetter(cs[i], cs[j]) })
		chosen := cs[0]
		if !hasManifestFile(chosen.Dir) {
			continue
		}
		c := &Candidate{Remote: remote, Chosen: chosen, Others: cs[1:], InManifest: existing[remote]}
		c.Name = remote[strings.LastIndex(remote, "/")+1:]
		out = append(out, c)
	}
	// Two owners with the same repo name get owner-qualified names.
	count := map[string]int{}
	for _, c := range out {
		count[c.Name]++
	}
	for _, c := range out {
		if count[c.Name] > 1 {
			c.Name = RemoteOwner(c.Remote) + "-" + c.Name
		}
		if c.InManifest != nil {
			c.Name = c.InManifest.Name
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Merge adds the new candidates to the manifest. Existing entries keep their
// path unless that folder is gone.
func (m *Manifest) Merge(cands []*Candidate) (added, moved []string) {
	for _, c := range cands {
		if s := c.InManifest; s != nil {
			if _, err := os.Stat(ExpandPath(s.Path)); err != nil {
				s.Path = CompactPath(c.Chosen.Dir)
				moved = append(moved, s.Name)
			}
			if s.Remote == "" {
				s.Remote = c.Remote
			}
			continue
		}
		m.Projects = append(m.Projects, &ProjectSpec{Name: c.Name, Path: CompactPath(c.Chosen.Dir), Remote: c.Remote})
		added = append(added, c.Name)
	}
	return added, moved
}

// NewSpec builds a manifest entry for a single clone.
func NewSpec(dir, name string) (*ProjectSpec, error) {
	abs, err := filepath.Abs(ExpandPath(dir))
	if err != nil {
		return nil, err
	}
	top, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	s := &ProjectSpec{Name: name, Path: CompactPath(top)}
	if url, err := git(top, "remote", "get-url", "origin"); err == nil {
		s.Remote = RemoteID(url)
	}
	if s.Name == "" {
		if s.Remote != "" {
			s.Name = s.Remote[strings.LastIndex(s.Remote, "/")+1:]
		} else {
			s.Name = filepath.Base(top)
		}
	}
	return s, nil
}

func isClone(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func hasManifestFile(dir string) bool {
	for _, f := range []string{"go.mod", "package.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

func appendClone(list []*Clone, dir string, top bool) []*Clone {
	c := &Clone{Dir: dir, TopLevel: top}
	if url, err := git(dir, "remote", "get-url", "origin"); err == nil {
		c.Remote = RemoteID(url)
	}
	c.Branch, _ = git(dir, "branch", "--show-current")
	c.OnDefault = c.Branch != "" && c.Branch == defaultBranch(dir)
	st, _ := git(dir, "status", "--porcelain", "--untracked-files=no")
	c.Clean = st == ""
	if t, err := git(dir, "log", "-1", "--format=%ct"); err == nil {
		c.CommitTime, _ = strconv.ParseInt(t, 10, 64)
	}
	return append(list, c)
}

// cloneBetter prefers a clone on the default branch, then one in a folder
// named main or master (or a top-level clone), then a clean one, then the
// one with the newest commit.
func cloneBetter(a, b *Clone) bool {
	score := func(c *Clone) int {
		s := 0
		if c.OnDefault {
			s += 4
		}
		if base := filepath.Base(c.Dir); base == "main" || base == "master" || c.TopLevel {
			s += 2
		}
		if c.Clean {
			s++
		}
		return s
	}
	if sa, sb := score(a), score(b); sa != sb {
		return sa > sb
	}
	return a.CommitTime > b.CommitTime
}
