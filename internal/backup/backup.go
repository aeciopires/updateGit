// Package backup provides functionality to manage backups of git repositories.
// It supports different backup strategies such as copying files or using git stash.
// The package allows creating, restoring, and cleaning up backups.
package backup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
)

// BackupStrategy represents different backup approaches
type BackupStrategy string

const (
	StrategyStash BackupStrategy = "stash"
	StrategyCopy  BackupStrategy = "copy"
)

// BackupManager handles repository backups
type BackupManager struct {
	BackupDir string
	Strategy  BackupStrategy
	Timestamp string
}

// BackupInfo contains information about a backup
type BackupInfo struct {
	Repository   string
	BackupPath   string
	Strategy     BackupStrategy
	Timestamp    time.Time
	OriginalPath string
}

// BackupError represents a backup operation error
type BackupError struct {
	Repository string
	Operation  string
	Err        error
}

func (e *BackupError) Error() string {
	return fmt.Sprintf("backup %s failed for repository '%s': %v", e.Operation, e.Repository, e.Err)
}

// NewBackupManager creates a new backup manager
func NewBackupManager(backupDir string, strategy BackupStrategy) *BackupManager {
	timestamp := time.Now().Format("20060102-150405")

	if backupDir == "" {
		backupDir = "./backups"
	}

	fullBackupDir := filepath.Join(backupDir, timestamp)
	if err := os.MkdirAll(fullBackupDir, config.PermissionDir); err != nil {
		common.Logger("fatal", "Failed to create backup directory. error=%v", err)
	}

	manager := &BackupManager{
		BackupDir: fullBackupDir,
		Strategy:  strategy,
		Timestamp: timestamp,
	}

	common.Logger("info", "Backup manager initialized. backup_dir=%s strategy=%s timestamp=%s", fullBackupDir, strategy, timestamp)

	return manager
}

// CreateBackup creates a backup of the specified repository
func (bm *BackupManager) CreateBackup(repoPath, repoName string) (*BackupInfo, error) {
	common.Logger("info", "Creating repository backup. repository=%s path=%s strategy=%s", repoName, repoPath, bm.Strategy)

	switch bm.Strategy {
	case StrategyStash:
		return bm.createStashBackup(repoPath, repoName)
	case StrategyCopy:
		return bm.createCopyBackup(repoPath, repoName)
	default:
		return bm.createCopyBackup(repoPath, repoName)
	}
}

// createStashBackup creates a git stash backup
func (bm *BackupManager) createStashBackup(repoPath, repoName string) (*BackupInfo, error) {
	if !bm.hasUncommittedChanges(repoPath) {
		common.Logger("debug", "No uncommitted changes, skipping stash backup. repository=%s", repoName)
		return &BackupInfo{
			Repository:   repoName,
			BackupPath:   "git-stash",
			Strategy:     StrategyStash,
			Timestamp:    time.Now(),
			OriginalPath: repoPath,
		}, nil
	}

	stashMessage := fmt.Sprintf("updateGit backup %s", bm.Timestamp)
	cmd := exec.Command("git", "stash", "push", "-u", "-m", stashMessage)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, &BackupError{Repository: repoName, Operation: "git stash", Err: fmt.Errorf("%v: %s", err, string(out))}
	}
	common.Logger("info", "Git stash backup created. repository=%s message=%s", repoName, stashMessage)

	return &BackupInfo{
		Repository:   repoName,
		BackupPath:   fmt.Sprintf("stash: %s", stashMessage),
		Strategy:     StrategyStash,
		Timestamp:    time.Now(),
		OriginalPath: repoPath,
	}, nil
}

// createCopyBackup creates a file system copy backup
func (bm *BackupManager) createCopyBackup(repoPath, repoName string) (*BackupInfo, error) {
	backupPath := filepath.Join(bm.BackupDir, repoName)
	common.Logger("debug", "Attempting copy backup. repo_name='%s', backup_path='%s'", repoName, backupPath)

	if err := os.MkdirAll(backupPath, config.PermissionDir); err != nil {
		return nil, &BackupError{Repository: repoName, Operation: "create directory", Err: err}
	}

	if err := bm.copyRepository(repoPath, backupPath); err != nil {
		return nil, &BackupError{Repository: repoName, Operation: "copy files", Err: err}
	}

	common.Logger("debug", "Finished copy backup for repository '%s'", repoName)

	return &BackupInfo{
		Repository:   repoName,
		BackupPath:   backupPath,
		Strategy:     StrategyCopy,
		Timestamp:    time.Now(),
		OriginalPath: repoPath,
	}, nil
}

// copyRepository copies the repository files to the backup directory
func (bm *BackupManager) copyRepository(src, dst string) error {
	common.Logger("debug", "Starting repository copy walk. src='%s'", src)
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			common.Logger("error", "Error accessing path '%s' during walk: %v", path, err)
			return err
		}

		relPath, relErr := filepath.Rel(src, path)
		if relErr != nil {
			common.Logger("error", "Could not get relative path for '%s': %v", path, relErr)
			return relErr
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() && info.Name() == ".git" {
			common.Logger("debug", "Skipping .git directory: '%s'", path)
			return filepath.SkipDir
		}

		if info.Mode()&os.ModeSymlink != 0 {
			common.Logger("debug", "Copying symlink: '%s' -> '%s'", path, dstPath)
			target, err := os.Readlink(path)
			if err != nil { return err }
			if err := os.MkdirAll(filepath.Dir(dstPath), config.PermissionDir); err != nil { return err }
			_ = os.Remove(dstPath)
			return os.Symlink(target, dstPath)
		}

		if info.IsDir() {
			common.Logger("debug", "Creating directory: '%s'", dstPath)
			return os.MkdirAll(dstPath, info.Mode())
		}

		common.Logger("debug", "Attempting to copy file: '%s' -> '%s'", path, dstPath)
		return bm.copyFile(path, dstPath)
	})

	if err != nil {
		common.Logger("error", "File walk finished with error: %v", err)
	} else {
		common.Logger("debug", "File walk completed successfully for src='%s'", src)
	}
	return err
}

// copyFile copies a single file from source to destination
func (bm *BackupManager) copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), config.PermissionDir); err != nil {
		common.Logger("error", "copyFile: Failed to create parent dir for '%s': %v", dst, err)
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		common.Logger("error", "copyFile: Failed to stat src '%s': %v", src, err)
		return err
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		common.Logger("error", "copyFile: Failed to open src '%s': %v", src, err)
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		common.Logger("error", "copyFile: Failed to open dst '%s': %v", dst, err)
		return err
	}
	defer destFile.Close()

	bytesCopied, err := io.Copy(destFile, sourceFile)
	if err != nil {
		common.Logger("error", "copyFile: Failed during io.Copy for '%s': %v", src, err)
		return err
	}

	common.Logger("debug", "Successfully copied %d bytes for file: %s", bytesCopied, dst)
	return os.Chmod(dst, srcInfo.Mode())
}

// hasUncommittedChanges checks if there are uncommitted changes in the repository
func (bm *BackupManager) hasUncommittedChanges(repoPath string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		common.Logger("warn", "Failed to detect repo status, assuming changes exist. path=%s err=%v", repoPath, err)
		return true
	}
	return len(out) > 0
}

// RestoreBackup restores a backup for a repository
func (bm *BackupManager) RestoreBackup(backupInfo *BackupInfo) error {
	common.Logger("info", "Restore functionality not yet implemented. repository=%s backup_path=%s strategy=%s",
		backupInfo.Repository, backupInfo.BackupPath, backupInfo.Strategy)
	return fmt.Errorf("restore functionality not yet implemented")
}

// CleanupOldBackups removes backups older than the specified number of days
func (bm *BackupManager) CleanupOldBackups(days int) error {
	common.Logger("info", "Backup cleanup not yet implemented. retention_days=%d", days)
	return fmt.Errorf("cleanup functionality not yet implemented")
}

// GetBackupStats returns statistics about the backup manager
func (bm *BackupManager) GetBackupStats() map[string]interface{} {
	return map[string]interface{}{
		"backup_dir": bm.BackupDir,
		"strategy":   bm.Strategy,
		"timestamp":  bm.Timestamp,
	}
}