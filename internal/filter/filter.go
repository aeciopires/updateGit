// Package filter provides repository filtering capabilities based on skip lists.
// It allows selective processing of repositories based on user-defined criteria.
package filter

import (
	"github.com/aeciopires/updateGit/internal/common"
)

// Filter represents repository filtering configuration
type Filter struct {
	SkipRepos      map[string]bool
}

// FilterError represents a filtering error
type FilterError struct {
	Pattern string
	Err     error
}

func (e *FilterError) Error() string {
	return "filter pattern '" + e.Pattern + "' error: " + e.Err.Error()
}

// NewFilter creates a new repository filter with the given patterns
func NewFilter(skipRepos []string) (*Filter, error) {
	filter := &Filter{
		SkipRepos: make(map[string]bool),
	}

	// Build skip repos map
	for _, repo := range skipRepos {
		filter.SkipRepos[repo] = true
		common.Logger("debug", "Repository added to skip list. repository=%s", repo)
	}

	common.Logger("info", "Repository filter configured. skip_count=%d", len(skipRepos))

	return filter, nil
}

// ShouldProcess determines if a repository should be processed based on filter criteria
func (f *Filter) ShouldProcess(repoName string) bool {
	// Check skip list first
	if f.SkipRepos[repoName] {
		common.Logger("debug", "Repository skipped (in skip list). repository=%s", repoName)
		return false
	}

	common.Logger("debug", "Repository passes filter criteria. repository=%s", repoName)
	return true
}

// GetStats returns filtering statistics
func (f *Filter) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"skip_count":          len(f.SkipRepos),
	}

	return stats
}

// FilterRepositories applies the filter to a list of repository names
func (f *Filter) FilterRepositories(repos []string) []string {
	var filtered []string
	for _, repo := range repos {
		if f.ShouldProcess(repo) {
			filtered = append(filtered, repo)
		}
	}

	common.Logger("info", "Repository filtering completed. total=%d filtered=%d skipped=%d",
		len(repos), len(filtered), len(repos)-len(filtered))

	return filtered
}
