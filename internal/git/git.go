// Package git provides functions to manage and update git repositories.
// It includes repository discovery, branch information, and the parallel
// execution of "git pull" (see runner.go).
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aeciopires/updateGit/internal/common"
)

// Repository represents a git repository with its metadata
type Repository struct {
	// Path is the absolute path of the repository
	Path string
	// Name is the path relative to the base directory (e.g. "updateGit" or "github/helm-charts")
	Name string
	// CurrentBranch is only filled by FindRepositories (kept for compatibility)
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

// Unwrap returns the original error.
func (e *GitError) Unwrap() error { return e.Err }

// IsGitRepository checks if a directory contains a git repository: ".git" can be
// a directory (normal clone) or a file starting with "gitdir:" (worktree or submodule).
func IsGitRepository(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		common.Logger("debug", "Not a git repository. path=%s", path)
		return false
	}
	if info.IsDir() {
		common.Logger("debug", "Found git repository. repository=%s", path)
		return true
	}
	content, err := os.ReadFile(gitPath)
	if err == nil && bytes.HasPrefix(bytes.TrimSpace(content), []byte("gitdir:")) {
		common.Logger("debug", "Found git worktree/submodule. repository=%s", path)
		return true
	}
	common.Logger("debug", "Not a git repository. path=%s", path)
	return false
}

// gitOutput runs a git command in repoPath and returns its standard output.
func gitOutput(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	return string(out), err
}

// GetCurrentBranch returns the current branch name for a repository
func GetCurrentBranch(repoPath string) (string, error) {
	output, err := gitOutput(context.Background(), repoPath, "symbolic-ref", "HEAD")
	if err != nil {
		common.Logger("debug", "Failed to get current branch. repository=%s error=%v", repoPath, err)
		return "unknown", &GitError{
			Repository: repoPath,
			Operation:  "symbolic-ref",
			Err:        err,
		}
	}

	branchRef := strings.TrimSpace(output)
	parts := strings.Split(branchRef, "/")
	if len(parts) >= 3 {
		branchName := strings.Join(parts[2:], "/")
		common.Logger("debug", "Current branch detected. repository=%s branch=%s", repoPath, branchName)
		return branchName, nil
	}

	return branchRef, nil
}

// GetBranches returns all local branches for a repository (output of "git branch")
func GetBranches(repoPath string) (string, error) {
	output, err := gitOutput(context.Background(), repoPath, "branch")
	if err != nil {
		return "", &GitError{
			Repository: repoPath,
			Operation:  "branch",
			Err:        err,
		}
	}

	return output, nil
}

// PullRepository executes git pull on a repository, connected to the terminal.
//
// Deprecated: kept for compatibility. The "pull" command uses Run (runner.go),
// which captures the output and applies a timeout.
func PullRepository(repoPath string) error {
	common.Logger("info", "Executing git pull. repository=%s", repoPath)

	cmd := exec.CommandContext(context.Background(), "git", "pull")
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

// FindRepositories discovers the git repositories in the first level of baseDir
// and fills their current branch. Kept for compatibility: the "pull" command uses
// DiscoverRepositories (discovery.go).
func FindRepositories(baseDir string) ([]Repository, error) {
	result, err := DiscoverRepositories(DiscoveryOptions{BaseDir: baseDir, MaxDepth: 1})
	if err != nil {
		return nil, err
	}
	for i := range result.Repos {
		branch, err := GetCurrentBranch(result.Repos[i].Path)
		if err != nil {
			common.Logger("warning", "Could not determine current branch. repository=%s error=%v", result.Repos[i].Path, err)
		}
		result.Repos[i].CurrentBranch = branch
	}
	return result.Repos, nil
}
