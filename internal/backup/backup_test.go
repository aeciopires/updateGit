package backup

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitEnv(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
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

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "initial commit")
}

func TestNewBackupManager(t *testing.T) {
	base := t.TempDir()
	bm := NewBackupManager(base, StrategyCopy)

	if bm.Strategy != StrategyCopy {
		t.Errorf("Strategy = %q, want %q", bm.Strategy, StrategyCopy)
	}
	if bm.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
	if bm.BackupDir != filepath.Join(base, bm.Timestamp) {
		t.Errorf("BackupDir = %q, want %q", bm.BackupDir, filepath.Join(base, bm.Timestamp))
	}
	if info, err := os.Stat(bm.BackupDir); err != nil || !info.IsDir() {
		t.Errorf("backup directory was not created: %v", err)
	}

	stats := bm.GetBackupStats()
	if stats["backup_dir"] != bm.BackupDir || stats["strategy"] != StrategyCopy || stats["timestamp"] != bm.Timestamp {
		t.Errorf("unexpected stats: %v", stats)
	}
}

func TestNewBackupManagerDefaultDir(t *testing.T) {
	t.Chdir(t.TempDir())
	bm := NewBackupManager("", StrategyCopy)
	if !strings.HasPrefix(bm.BackupDir, "backups") {
		t.Errorf("BackupDir = %q, want it under ./backups", bm.BackupDir)
	}
}

func TestCreateCopyBackup(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")

	// Fake repository content: files, sub directory, symlink and a .git dir that must be skipped
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"README.md":   "readme",
		"sub/code.go": "package sub",
		".git/HEAD":   "ref: refs/heads/main",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "run.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("README.md", filepath.Join(repo, "link.md")); err != nil {
		t.Fatal(err)
	}

	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyCopy)
	info, err := bm.CreateBackup(repo, "repo")
	if err != nil {
		t.Fatalf("CreateBackup returned error: %v", err)
	}

	if info.Strategy != StrategyCopy || info.Repository != "repo" || info.OriginalPath != repo {
		t.Errorf("unexpected backup info: %+v", info)
	}
	if info.BackupPath != filepath.Join(bm.BackupDir, "repo") {
		t.Errorf("BackupPath = %q", info.BackupPath)
	}

	for _, name := range []string{"README.md", "sub/code.go"} {
		got, err := os.ReadFile(filepath.Join(info.BackupPath, name))
		if err != nil || string(got) != files[name] {
			t.Errorf("file %s not copied correctly: %q, %v", name, got, err)
		}
	}

	if st, err := os.Stat(filepath.Join(info.BackupPath, "run.sh")); err != nil || st.Mode().Perm()&0100 == 0 {
		t.Errorf("run.sh should keep the executable permission: %v", err)
	}

	if target, err := os.Readlink(filepath.Join(info.BackupPath, "link.md")); err != nil || target != "README.md" {
		t.Errorf("symlink not copied correctly: %q, %v", target, err)
	}

	if _, err := os.Stat(filepath.Join(info.BackupPath, ".git")); !os.IsNotExist(err) {
		t.Error(".git directory should not be copied")
	}
}

func TestCopyBackupNestedName(t *testing.T) {
	base := t.TempDir()
	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyCopy)

	// Two repositories with the same directory name in different paths
	for _, name := range []string{"github/api", "gitlab/api"} {
		repo := filepath.Join(base, "repos", filepath.FromSlash(name))
		if err := os.MkdirAll(repo, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "origin.txt"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := bm.CreateBackup(repo, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"github/api", "gitlab/api"} {
		got, err := os.ReadFile(filepath.Join(bm.BackupDir, filepath.FromSlash(name), "origin.txt"))
		if err != nil || string(got) != name {
			t.Errorf("backup of %s: %q, %v", name, got, err)
		}
	}
}

func TestCreateBackupUnknownStrategyUsesCopy(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}

	bm := NewBackupManager(filepath.Join(base, "backups"), BackupStrategy("unknown"))
	info, err := bm.CreateBackup(repo, "repo")
	if err != nil {
		t.Fatalf("CreateBackup returned error: %v", err)
	}
	if info.Strategy != StrategyCopy {
		t.Errorf("Strategy = %q, want %q", info.Strategy, StrategyCopy)
	}
}

func TestCreateCopyBackupMissingRepo(t *testing.T) {
	base := t.TempDir()
	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyCopy)

	_, err := bm.CreateBackup(filepath.Join(base, "missing"), "missing")
	var backupErr *BackupError
	if !errors.As(err, &backupErr) || backupErr.Operation != "copy files" {
		t.Errorf("expected *BackupError with operation 'copy files', got %v", err)
	}
}

func TestCreateStashBackup(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	initRepo(t, repo)

	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyStash)

	// Clean repository: nothing to stash
	info, err := bm.CreateBackup(repo, "repo")
	if err != nil {
		t.Fatalf("CreateBackup (clean) returned error: %v", err)
	}
	if info.BackupPath != "git-stash" {
		t.Errorf("BackupPath (clean) = %q, want git-stash", info.BackupPath)
	}
	if out := runGit(t, repo, "stash", "list"); out != "" {
		t.Errorf("stash list should be empty, got %q", out)
	}

	// Uncommitted and untracked changes are stashed
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "untracked.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}

	info, err = bm.CreateBackup(repo, "repo")
	if err != nil {
		t.Fatalf("CreateBackup (dirty) returned error: %v", err)
	}
	if info.Strategy != StrategyStash || !strings.Contains(info.BackupPath, "updateGit backup "+bm.Timestamp) {
		t.Errorf("unexpected backup info: %+v", info)
	}
	if out := runGit(t, repo, "stash", "list"); !strings.Contains(out, "updateGit backup "+bm.Timestamp) {
		t.Errorf("stash not created, stash list: %q", out)
	}
	if out := runGit(t, repo, "status", "--porcelain"); out != "" {
		t.Errorf("working tree should be clean after stash, got %q", out)
	}
}

func TestNotImplementedOperations(t *testing.T) {
	bm := NewBackupManager(t.TempDir(), StrategyCopy)

	if err := bm.RestoreBackup(&BackupInfo{Repository: "repo"}); err == nil {
		t.Error("RestoreBackup should return an error (not implemented)")
	}
	if err := bm.CleanupOldBackups(7); err == nil {
		t.Error("CleanupOldBackups should return an error (not implemented)")
	}
}

func TestBackupError(t *testing.T) {
	err := &BackupError{Repository: "repo", Operation: "copy files", Err: errors.New("boom")}
	want := "backup copy files failed for repository 'repo': boom"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestCopyBackupUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(repo, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyCopy)
	_, err := bm.CreateBackup(repo, "repo")
	var backupErr *BackupError
	if !errors.As(err, &backupErr) || backupErr.Operation != "copy files" {
		t.Errorf("unreadable file: %v", err)
	}
}

func TestCopyBackupDestinationNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyCopy)
	if err := os.Chmod(bm.BackupDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bm.BackupDir, 0o755) })
	_, err := bm.CreateBackup(repo, "repo")
	var backupErr *BackupError
	if !errors.As(err, &backupErr) || backupErr.Operation != "create directory" {
		t.Errorf("read-only destination: %v", err)
	}

	// copyFile errors: missing source and unwritable destination
	if err := bm.copyFile(filepath.Join(repo, "missing"), filepath.Join(base, "dst")); err == nil {
		t.Error("missing source must fail")
	}
	if err := bm.copyFile(filepath.Join(repo, "a.txt"), filepath.Join(bm.BackupDir, "x", "a.txt")); err == nil {
		t.Error("unwritable destination must fail")
	}
}

func TestStashBackupOutsideRepository(t *testing.T) {
	setupGitEnv(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	base := t.TempDir()
	dir := filepath.Join(base, "not-a-repo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	bm := NewBackupManager(filepath.Join(base, "backups"), StrategyStash)
	// "git status" fails => changes are assumed => "git stash" fails with a BackupError
	if bm.hasUncommittedChanges(dir) != true {
		t.Error("an error in git status must assume changes")
	}
	_, err := bm.CreateBackup(dir, "not-a-repo")
	var backupErr *BackupError
	if !errors.As(err, &backupErr) || backupErr.Operation != "git stash" {
		t.Errorf("stash outside a repository: %v", err)
	}
}
