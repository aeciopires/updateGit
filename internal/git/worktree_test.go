package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetWorkingTreeStatus(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()

	repo := filepath.Join(base, "repo")
	initRepo(t, repo, "main")
	writeFile(t, filepath.Join(repo, ".gitignore"), "*.log\n")
	writeFile(t, filepath.Join(repo, "b.txt"), "b\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "more files")

	status, err := GetWorkingTreeStatus(repo)
	if err != nil || status.Dirty() || status.String() != "clean" {
		t.Fatalf("clean repository: %+v (%s), %v", status, status, err)
	}

	// Ignored files are not counted
	writeFile(t, filepath.Join(repo, "debug.log"), "x")
	if status, _ = GetWorkingTreeStatus(repo); status.Dirty() {
		t.Errorf("ignored file counted: %+v", status)
	}

	writeFile(t, filepath.Join(repo, "README.md"), "changed\n") // modified
	writeFile(t, filepath.Join(repo, "b.txt"), "staged\n")      // staged
	runGit(t, repo, "add", "b.txt")
	writeFile(t, filepath.Join(repo, "new1.txt"), "x") // untracked
	writeFile(t, filepath.Join(repo, "new2.txt"), "x") // untracked

	status, err = GetWorkingTreeStatus(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := WorkingTreeStatus{Staged: 1, Modified: 1, Untracked: 2}
	if status != want || !status.Dirty() || status.String() != "1 staged, 1 modified, 2 untracked" {
		t.Errorf("status = %+v (%s), want %+v", status, status, want)
	}

	// File staged and modified again: counted in both
	writeFile(t, filepath.Join(repo, "b.txt"), "staged and modified\n")
	if status, _ = GetWorkingTreeStatus(repo); status.Staged != 1 || status.Modified != 2 {
		t.Errorf("staged+modified: %+v", status)
	}
}

func TestGetWorkingTreeStatusConflict(t *testing.T) {
	setupGitEnv(t)
	repo := filepath.Join(t.TempDir(), "repo")
	initRepo(t, repo, "main")
	runGit(t, repo, "checkout", "-q", "-b", "other")
	writeFile(t, filepath.Join(repo, "README.md"), "other\n")
	runGit(t, repo, "commit", "-q", "-am", "other")
	runGit(t, repo, "checkout", "-q", "main")
	writeFile(t, filepath.Join(repo, "README.md"), "main\n")
	runGit(t, repo, "commit", "-q", "-am", "main")
	cmdErr := func() error {
		_, err := gitOutput(t.Context(), repo, "merge", "other")
		return err
	}()
	if cmdErr == nil {
		t.Fatal("expected a merge conflict")
	}

	status, err := GetWorkingTreeStatus(repo)
	if err != nil || status.Conflicted != 1 || status.String() != "1 conflicted" {
		t.Errorf("conflict: %+v (%s), %v", status, status, err)
	}
}

func TestGetWorkingTreeStatusNotARepo(t *testing.T) {
	setupGitEnv(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	_, err := GetWorkingTreeStatus(t.TempDir())
	var gitErr *GitError
	if !errors.As(err, &gitErr) || gitErr.Operation != "status" {
		t.Errorf("expected *GitError status, got %v", err)
	}
}
