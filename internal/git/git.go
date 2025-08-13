// Package git provides functions to manage and update git repositories.
// It includes repository discovery, branch management, and git operations.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
)

// UpdateConfig holds configuration for updating repositories.
type UpdateConfig struct {
	BaseDir       string
	Parallel      ParallelUpdateConfig
	BackupEnabled bool
	BackupManager interface{}
	Filter        interface{}
}

// ParallelUpdateConfig holds parallel update settings.
type ParallelUpdateConfig struct {
	Enabled       bool
	MaxConcurrent int
	Timeout       time.Duration
}

// Repository represents a git repository with its metadata
type Repository struct {
	Path          string
	Name          string
	CurrentBranch string
	IsValid       bool
}

// GitError represents a git operation error
type GitError struct {
	Repository string
	Operation  string
	Err        error
}

func (e *GitError) Error() string {
	return fmt.Sprintf("git %s failed for repository '%s': %v", e.Operation, e.Repository, e.Err)
}

// IsGitRepository checks if a directory contains a git repository
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		common.Logger("debug", "Found git repository. repository=%s", path)
		return true
	}

	common.Logger("debug", "Not a git repository. path=%s", path)
	return false
}

// GetCurrentBranch returns the current branch name for a repository
func GetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		common.Logger("debug", "Failed to get current branch. repository=%s error=%v", repoPath, err)
		return "unknown", &GitError{
			Repository: repoPath,
			Operation:  "symbolic-ref",
			Err:        err,
		}
	}

	branchRef := strings.TrimSpace(string(output))
	parts := strings.Split(branchRef, "/")
	if len(parts) >= 3 {
		branchName := strings.Join(parts[2:], "/")
		common.Logger("debug", "Current branch detected. repository=%s branch=%s", repoPath, branchName)
		return branchName, nil
	}

	return branchRef, nil
}

// GetBranches returns all local branches for a repository
func GetBranches(repoPath string) (string, error) {
	cmd := exec.Command("git", "branch")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", &GitError{
			Repository: repoPath,
			Operation:  "branch",
			Err:        err,
		}
	}

	return string(output), nil
}

// PullRepository executes git pull on a repository
func PullRepository(repoPath string) error {
	common.Logger("info", "Executing git pull. repository=%s", repoPath)

	cmd := exec.Command("git", "pull")
	cmd.Dir = repoPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return &GitError{
			Repository: repoPath,
			Operation:  "pull",
			Err:        err,
		}
	}

	common.Logger("info", "Git pull completed successfully. repository=%s", repoPath)
	return nil
}

// FindRepositories discovers all git repositories in a base directory
func FindRepositories(baseDir string) ([]Repository, error) {
	common.Logger("info", "Scanning for git repositories. baseDir=%s", baseDir)

	var repositories []Repository

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		common.Logger("fatal", "Failed to read directory '%s': %v", baseDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(baseDir, entry.Name())

		if IsGitRepository(repoPath) {
			currentBranch, err := GetCurrentBranch(repoPath)
			if err != nil {
				common.Logger("warning", "Could not determine current branch. repository=%s error=%v", repoPath, err)
			}

			repo := Repository{
				Path:          repoPath,
				Name:          entry.Name(),
				CurrentBranch: currentBranch,
				IsValid:       true,
			}

			repositories = append(repositories, repo)
			common.Logger("debug", "Repository added to update list. repository=%s branch=%s", repoPath, currentBranch)
		} else {
			common.Logger("debug", "Skipping non-git directory. directory=%s", repoPath)
		}
	}

	common.Logger("info", "Git repositories found. count=%d", len(repositories))
	return repositories, nil
}

// UpdateRepositories updates all git repositories in the specified directory
func UpdateRepositories(baseDir string) error {
	return UpdateRepositoriesWithConfig(UpdateConfig{BaseDir: baseDir})
}

// UpdateRepositoriesWithConfig updates repositories with backup/filter/parallel support
func UpdateRepositoriesWithConfig(cfg UpdateConfig) error {
	repositories, err := FindRepositories(cfg.BaseDir)
	if err != nil {
		common.Logger("fatal", "Failed to find repositories: %v", err)
	}
	if len(repositories) == 0 {
		common.Logger("warning", "No git repositories found. baseDir=%s", cfg.BaseDir)
		return nil
	}

	// Apply filter if set
	if cfg.Filter != nil {
		if f, ok := cfg.Filter.(interface {
			Match(repoName string) bool
		}); ok {
			var filtered []Repository
			for _, r := range repositories {
				if f.Match(r.Name) {
					filtered = append(filtered, r)
				} else {
					common.Logger("debug", "Repository excluded by filter. repository=%s", r.Name)
				}
			}
			repositories = filtered
		}
	}

	successCount := 0
	errorCount := 0

	for _, repo := range repositories {
		fmt.Println("------------- BEGIN -------------")
		common.Logger("info", "Updating repository. repository=%s path=%s branch=%s", repo.Name, repo.Path, repo.CurrentBranch)

		if branches, err := GetBranches(repo.Path); err == nil {
			common.Logger("debug", "Local branches:\n%s", branches)
		}

		// Backup if enabled
		if cfg.BackupEnabled && cfg.BackupManager != nil {
			if bm, ok := cfg.BackupManager.(interface {
				CreateBackup(repoPath, repoName string) error
			}); ok {
				if err := bm.CreateBackup(repo.Path, repo.Name); err != nil {
					common.Logger("error", "Failed to create backup. repository=%s error=%v", repo.Name, err)
				}
			}
		}

		fmt.Printf("[INFO] Updating repository: '%s' on branch '%s'\n", repo.Name, repo.CurrentBranch)
		fmt.Println("If necessary, enter login/password when prompted.")

		if err := PullRepository(repo.Path); err != nil {
			common.Logger("error", "Failed to update repository. repository=%s error=%v", repo.Name, err)
			errorCount++
		} else {
			successCount++
		}

		fmt.Println("---------------------------------")
		fmt.Println()
		fmt.Println()
	}

	common.Logger("info", "Repository update completed. total=%d success=%d errors=%d", len(repositories), successCount, errorCount)

	if errorCount > 0 {
		common.Logger("fatal", "Update completed with %d errors out of %d repositories", errorCount, len(repositories))
	}
	return nil
}
