package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGetRepoBranches(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)

	t.Run("up to date with upstream and other branches", func(t *testing.T) {
		repo := filepath.Join(base, "uptodate")
		cloneRepo(t, origin, repo)
		runGit(t, repo, "branch", "-q", "zeta")
		runGit(t, repo, "branch", "-q", "alpha")
		runGit(t, repo, "branch", "-q", "feature/x", "origin/feature/x")

		b, err := GetRepoBranches(repo)
		if err != nil {
			t.Fatal(err)
		}
		if b.Current == nil || b.Current.Name != "main" || b.Current.Upstream != "origin/main" || b.Current.State() != "up to date" {
			t.Fatalf("unexpected current branch: %+v", b.Current)
		}
		var names []string
		for _, o := range b.Others {
			names = append(names, o.Name)
		}
		want := []string{"alpha", "feature/x", "zeta"}
		if len(names) != len(want) {
			t.Fatalf("others = %v, want %v", names, want)
		}
		for i := range want {
			if names[i] != want[i] {
				t.Fatalf("others = %v, want %v (alphabetical, names with /)", names, want)
			}
		}
		if b.Others[1].Upstream != "origin/feature/x" {
			t.Errorf("feature/x upstream = %q", b.Others[1].Upstream)
		}
	})

	t.Run("ahead and behind", func(t *testing.T) {
		repo := filepath.Join(base, "aheadbehind")
		cloneRepo(t, origin, repo)
		// local commit (ahead 1)
		if err := os.WriteFile(filepath.Join(repo, "local.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", ".")
		runGit(t, repo, "commit", "-q", "-m", "local")

		b, _ := GetRepoBranches(repo)
		if b.Current.Ahead != 1 || b.Current.Behind != 0 || b.Current.State() != "ahead 1" {
			t.Errorf("state = %q (%+v), want ahead 1", b.Current.State(), b.Current)
		}

		// remote commits (behind 2 after fetch)
		pushCommit(t, base, origin, "remote1.txt")
		pushCommit(t, base, origin, "remote2.txt")
		runGit(t, repo, "fetch", "-q")
		b, _ = GetRepoBranches(repo)
		if b.Current.Ahead != 1 || b.Current.Behind != 2 || b.Current.State() != "ahead 1, behind 2" {
			t.Errorf("state = %q, want ahead 1, behind 2", b.Current.State())
		}
	})

	t.Run("behind only", func(t *testing.T) {
		repo := filepath.Join(base, "behind")
		cloneRepo(t, origin, repo)
		runGit(t, repo, "reset", "-q", "--hard", "HEAD~1")
		b, _ := GetRepoBranches(repo)
		if b.Current.Behind == 0 || b.Current.Ahead != 0 {
			t.Errorf("state = %q, want behind N", b.Current.State())
		}
	})

	t.Run("no upstream", func(t *testing.T) {
		repo := filepath.Join(base, "noupstream")
		initRepo(t, repo, "main")
		b, _ := GetRepoBranches(repo)
		if b.Current == nil || b.Current.Upstream != "" || b.Current.State() != "no upstream" {
			t.Errorf("unexpected: %+v", b.Current)
		}
	})

	t.Run("upstream gone", func(t *testing.T) {
		repo := filepath.Join(base, "gone")
		cloneRepo(t, origin, repo)
		runGit(t, repo, "checkout", "-q", "-b", "temp")
		runGit(t, repo, "push", "-q", "-u", "origin", "temp")
		runGit(t, repo, "push", "-q", "origin", "--delete", "temp")
		runGit(t, repo, "fetch", "-q", "--prune")
		b, _ := GetRepoBranches(repo)
		if b.Current == nil || !b.Current.Gone || b.Current.State() != "upstream gone" {
			t.Errorf("unexpected: %+v", b.Current)
		}
	})

	t.Run("detached HEAD", func(t *testing.T) {
		repo := filepath.Join(base, "detached")
		cloneRepo(t, origin, repo)
		runGit(t, repo, "checkout", "-q", "--detach")
		b, _ := GetRepoBranches(repo)
		if b.Current != nil || b.Detached == "" {
			t.Errorf("unexpected: current=%+v detached=%q", b.Current, b.Detached)
		}
		if len(b.Others) != 1 || b.Others[0].Name != "main" {
			t.Errorf("others = %+v, want [main]", b.Others)
		}
	})

	t.Run("repository without commits", func(t *testing.T) {
		repo := filepath.Join(base, "empty")
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "init", "-q", "-b", "trunk")
		b, err := GetRepoBranches(repo)
		if err != nil || b.Current == nil || b.Current.Name != "trunk" {
			t.Errorf("unexpected: %+v, %v", b.Current, err)
		}
	})
}

func TestGetRepoBranchesNotARepo(t *testing.T) {
	setupGitEnv(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	_, err := GetRepoBranches(t.TempDir())
	var gitErr *GitError
	if !errors.As(err, &gitErr) || gitErr.Operation != "for-each-ref" {
		t.Errorf("expected *GitError for-each-ref, got %v", err)
	}
}

func TestParseTrack(t *testing.T) {
	tests := []struct {
		in           string
		ahead, behin int
		gone         bool
	}{
		{"", 0, 0, false},
		{"gone", 0, 0, true},
		{"ahead 3", 3, 0, false},
		{"behind 2", 0, 2, false},
		{"ahead 1, behind 4", 1, 4, false},
		{"garbage", 0, 0, false},
	}
	for _, tt := range tests {
		a, b, g := parseTrack(tt.in)
		if a != tt.ahead || b != tt.behin || g != tt.gone {
			t.Errorf("parseTrack(%q) = %d,%d,%v", tt.in, a, b, g)
		}
	}
}

func TestBranchInfoState(t *testing.T) {
	tests := map[string]BranchInfo{
		"no upstream":       {Name: "x"},
		"upstream gone":     {Upstream: "o/x", Gone: true},
		"up to date":        {Upstream: "o/x"},
		"ahead 2":           {Upstream: "o/x", Ahead: 2},
		"behind 3":          {Upstream: "o/x", Behind: 3},
		"ahead 1, behind 2": {Upstream: "o/x", Ahead: 1, Behind: 2},
	}
	for want, b := range tests {
		if got := b.State(); got != want {
			t.Errorf("State() = %q, want %q", got, want)
		}
	}
}
