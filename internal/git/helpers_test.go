package git

import (
	"os"
	"path/filepath"
	"testing"
)

// newOrigin creates a bare repository with one commit on main and the branches
// develop and feature/x. It returns its path.
func newOrigin(t *testing.T, base string) string {
	t.Helper()
	source := filepath.Join(base, "source")
	initRepo(t, source, "main")
	runGit(t, source, "branch", "develop")
	runGit(t, source, "branch", "feature/x")
	origin := filepath.Join(base, "origin.git")
	runGit(t, base, "clone", "-q", "--bare", source, origin)
	return origin
}

// cloneRepo clones origin into dest (creating the parent directories).
func cloneRepo(t *testing.T, origin, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, filepath.Dir(dest), "clone", "-q", origin, dest)
}

// pushCommit pushes a new commit to origin (main) from a temporary clone.
func pushCommit(t *testing.T, base, origin, file string) {
	t.Helper()
	upstream := filepath.Join(base, "upstream-"+file)
	cloneRepo(t, origin, upstream)
	if err := os.WriteFile(filepath.Join(upstream, file), []byte(file+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, upstream, "add", ".")
	runGit(t, upstream, "commit", "-q", "-m", "add "+file)
	runGit(t, upstream, "push", "-q", "origin", "main")
}

// recordingSink collects the events and results sent by Run.
type recordingSink struct {
	events  []Event
	results []RepoResult
}

func (s *recordingSink) OnEvent(e Event)         { s.events = append(s.events, e) }
func (s *recordingSink) OnResult(res RepoResult) { s.results = append(s.results, res) }
