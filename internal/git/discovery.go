package git

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
)

// MaxSearchDepth is the maximum number of directory levels searched for repositories.
const MaxSearchDepth = 6

// DiscoveryOptions configures DiscoverRepositories.
type DiscoveryOptions struct {
	// BaseDir is the directory where the search starts
	BaseDir string
	// MaxDepth is the number of levels searched: 1 = only BaseDir children (1 to MaxSearchDepth)
	MaxDepth int
	// ExcludeDirs are directory names never searched (e.g. node_modules)
	ExcludeDirs []string
	// OnProgress is called after each directory read with the partial counters (optional)
	OnProgress func(visitedDirs, reposFound int)
}

// DiscoveryResult is the result of the discovery stage, finished before any update.
type DiscoveryResult struct {
	// Repos found, sorted by Name (path relative to BaseDir)
	Repos []Repository
	// VisitedDirs is the number of directories read
	VisitedDirs int
	// IgnoredDirs are the directories skipped by ExcludeDirs (relative paths)
	IgnoredDirs []string
	// Warnings are non fatal problems (e.g. permission denied in a subdirectory)
	Warnings []string
	// MaxDepth is the depth effectively used
	MaxDepth int
	// Duration of the discovery
	Duration time.Duration
}

// DiscoverRepositories searches git repositories from BaseDir up to MaxDepth levels.
// The search never goes into a directory that is already a git repository, ignores
// hidden directories and ExcludeDirs, and does not follow symbolic links. The whole
// search finishes before returning, so the total of repositories is known before
// any update starts.
func DiscoverRepositories(opts DiscoveryOptions) (DiscoveryResult, error) {
	start := time.Now()
	result := DiscoveryResult{MaxDepth: clampDepth(opts.MaxDepth)}

	info, err := os.Stat(opts.BaseDir)
	if err != nil {
		return result, fmt.Errorf("cannot read base directory %s: %w", opts.BaseDir, err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("base directory %s is not a directory", opts.BaseDir)
	}

	exclude := make(map[string]bool, len(opts.ExcludeDirs))
	for _, name := range opts.ExcludeDirs {
		if name = strings.TrimSpace(name); name != "" {
			exclude[name] = true
		}
	}

	var scan func(dir string, level int) error
	scan = func(dir string, level int) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if dir == opts.BaseDir {
				return fmt.Errorf("cannot read base directory %s: %w", dir, err)
			}
			warning := fmt.Sprintf("cannot read directory %s: %v", dir, err)
			result.Warnings = append(result.Warnings, warning)
			common.Logger("warning", "Skipping directory: %s", warning)
			return nil
		}
		result.VisitedDirs++

		for _, entry := range entries {
			// Symbolic links are not followed (avoids loops and searches outside BaseDir)
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
				continue
			}
			name := entry.Name()
			path := filepath.Join(dir, name)
			if strings.HasPrefix(name, ".") {
				continue
			}
			if exclude[name] {
				result.IgnoredDirs = append(result.IgnoredDirs, relativeName(opts.BaseDir, path))
				common.Logger("debug", "Directory ignored (exclude_dirs). directory=%s", path)
				continue
			}
			if IsGitRepository(path) {
				// Registered and never walked: the search does not look inside repositories
				result.Repos = append(result.Repos, Repository{
					Path:    path,
					Name:    relativeName(opts.BaseDir, path),
					IsValid: true,
				})
				continue
			}
			if level < result.MaxDepth {
				if err := scan(path, level+1); err != nil {
					return err
				}
			}
		}

		if opts.OnProgress != nil {
			opts.OnProgress(result.VisitedDirs, len(result.Repos))
		}
		return nil
	}

	if err := scan(opts.BaseDir, 1); err != nil {
		return result, err
	}

	sort.Slice(result.Repos, func(i, j int) bool { return result.Repos[i].Name < result.Repos[j].Name })
	sort.Strings(result.IgnoredDirs)
	result.Duration = time.Since(start)
	return result, nil
}

// clampDepth keeps the depth between 1 and MaxSearchDepth.
func clampDepth(depth int) int {
	if depth < 1 {
		return 1
	}
	if depth > MaxSearchDepth {
		return MaxSearchDepth
	}
	return depth
}

// relativeName returns path relative to base with "/" separators.
func relativeName(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}
