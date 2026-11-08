package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitEnv isolates git from the user/system configuration and sets an identity for commits.
func setupGitEnv(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	clearGitHookEnv(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

// gitHookEnv lists variables git exports to hooks (e.g. GIT_INDEX_FILE=.git/index
// during "git commit"). When the tests run from a pre-commit hook they would
// point the test repositories to the wrong index/repository, so they are cleared.
var gitHookEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX",
	"GIT_EDITOR", "GIT_AUTHOR_DATE", "GIT_REFLOG_ACTION",
}

func clearGitHookEnv(t *testing.T) {
	t.Helper()
	for _, k := range gitHookEnv {
		t.Setenv(k, "") // registers the restore of the original value
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

// initRepo creates a git repository with one commit on the given branch.
func initRepo(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial commit")
}

func TestIsGitRepository(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()

	repo := filepath.Join(base, "repo")
	initRepo(t, repo, "main")
	plain := filepath.Join(base, "plain")
	if err := os.Mkdir(plain, 0755); err != nil {
		t.Fatal(err)
	}

	if !IsGitRepository(repo) {
		t.Error("IsGitRepository(repo) = false, want true")
	}
	if IsGitRepository(plain) {
		t.Error("IsGitRepository(plain) = true, want false")
	}
	if IsGitRepository(filepath.Join(base, "missing")) {
		t.Error("IsGitRepository(missing) = true, want false")
	}

	// git worktree: ".git" is a file ("gitdir: ...")
	worktree := filepath.Join(base, "worktree")
	runGit(t, repo, "worktree", "add", "-q", "-b", "wt", worktree)
	if info, err := os.Stat(filepath.Join(worktree, ".git")); err != nil || info.IsDir() {
		t.Fatalf(".git of the worktree should be a file: %v", err)
	}
	if !IsGitRepository(worktree) {
		t.Error("IsGitRepository(worktree) = false, want true")
	}

	// ".git" file that is not a gitdir reference
	fake := filepath.Join(base, "fake")
	if err := os.MkdirAll(fake, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fake, ".git"), []byte("not a repo"), 0644); err != nil {
		t.Fatal(err)
	}
	if IsGitRepository(fake) {
		t.Error("IsGitRepository(fake) = true, want false")
	}
}

func TestGetCurrentBranch(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()

	repo := filepath.Join(base, "repo")
	initRepo(t, repo, "main")

	branch, err := GetCurrentBranch(repo)
	if err != nil || branch != "main" {
		t.Errorf("GetCurrentBranch = %q, %v; want main, nil", branch, err)
	}

	// Branch names with slashes must be kept intact
	runGit(t, repo, "checkout", "-q", "-b", "feature/my-change")
	branch, err = GetCurrentBranch(repo)
	if err != nil || branch != "feature/my-change" {
		t.Errorf("GetCurrentBranch = %q, %v; want feature/my-change, nil", branch, err)
	}

	// Detached HEAD has no symbolic ref
	runGit(t, repo, "checkout", "-q", "--detach")
	branch, err = GetCurrentBranch(repo)
	if err == nil || branch != "unknown" {
		t.Errorf("GetCurrentBranch (detached) = %q, %v; want unknown and an error", branch, err)
	}
	var gitErr *GitError
	if !errors.As(err, &gitErr) || gitErr.Operation != "symbolic-ref" {
		t.Errorf("expected *GitError with operation symbolic-ref, got %v", err)
	}
}

func TestGetBranches(t *testing.T) {
	setupGitEnv(t)
	repo := filepath.Join(t.TempDir(), "repo")
	initRepo(t, repo, "main")
	runGit(t, repo, "branch", "develop")

	out, err := GetBranches(repo)
	if err != nil {
		t.Fatalf("GetBranches returned error: %v", err)
	}
	if !strings.Contains(out, "main") || !strings.Contains(out, "develop") {
		t.Errorf("GetBranches output %q should contain main and develop", out)
	}

	if _, err := GetBranches(t.TempDir()); err == nil {
		t.Error("GetBranches should fail outside a git repository")
	}
}

func TestFindRepositories(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()

	initRepo(t, filepath.Join(base, "repo-a"), "main")
	initRepo(t, filepath.Join(base, "repo-b"), "develop")
	if err := os.Mkdir(filepath.Join(base, "not-a-repo"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := FindRepositories(base)
	if err != nil {
		t.Fatalf("FindRepositories returned error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("found %d repositories, want 2: %+v", len(repos), repos)
	}

	got := map[string]string{}
	for _, r := range repos {
		if !r.IsValid {
			t.Errorf("repository %s should be valid", r.Name)
		}
		if r.Path != filepath.Join(base, r.Name) {
			t.Errorf("repository %s has path %s", r.Name, r.Path)
		}
		got[r.Name] = r.CurrentBranch
	}
	if got["repo-a"] != "main" || got["repo-b"] != "develop" {
		t.Errorf("unexpected branches: %v", got)
	}
}

func TestPullRepository(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()

	// origin (bare) <- upstream clone pushes a new commit; local clone pulls it
	source := filepath.Join(base, "source")
	initRepo(t, source, "main")
	origin := filepath.Join(base, "origin.git")
	runGit(t, base, "clone", "-q", "--bare", source, origin)

	local := filepath.Join(base, "local")
	runGit(t, base, "clone", "-q", origin, local)

	upstream := filepath.Join(base, "upstream")
	runGit(t, base, "clone", "-q", origin, upstream)
	if err := os.WriteFile(filepath.Join(upstream, "new.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, upstream, "add", ".")
	runGit(t, upstream, "commit", "-q", "-m", "new file")
	runGit(t, upstream, "push", "-q", "origin", "main")

	if err := PullRepository(local); err != nil {
		t.Fatalf("PullRepository returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "new.txt")); err != nil {
		t.Errorf("new.txt should exist after pull: %v", err)
	}
}

func TestPullRepositoryWithoutRemote(t *testing.T) {
	setupGitEnv(t)
	repo := filepath.Join(t.TempDir(), "repo")
	initRepo(t, repo, "main")

	err := PullRepository(repo)
	var gitErr *GitError
	if !errors.As(err, &gitErr) || gitErr.Operation != "pull" {
		t.Errorf("expected *GitError with operation pull, got %v", err)
	}
}

func TestGitError(t *testing.T) {
	err := &GitError{Repository: "/tmp/repo", Operation: "pull", Err: errors.New("boom")}
	want := "git pull failed for repository '/tmp/repo': boom"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestGitErrorUnwrapAndHelpers(t *testing.T) {
	inner := errors.New("boom")
	err := &GitError{Repository: "r", Operation: "pull", Err: inner}
	if !errors.Is(err, inner) {
		t.Error("errors.Is must find the wrapped error")
	}
	if got := relativeName("relative/base", "/absolute/path/repo"); got != "repo" {
		t.Errorf("relativeName with incompatible paths = %q, want repo", got)
	}
	if _, err := FindRepositories(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("FindRepositories with a missing directory must fail")
	}
}
