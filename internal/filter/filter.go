// Package filter provides repository filtering capabilities based on include/exclude patterns and skip lists.
// It allows selective processing of repositories based on user-defined criteria.
package filter

import (
	"regexp"

	"github.com/aeciopires/updateGit/internal/common"
)

// Filter represents repository filtering configuration
type Filter struct {
	IncludePattern *regexp.Regexp
	ExcludePattern *regexp.Regexp
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
func NewFilter(includePattern, excludePattern string, skipRepos []string) (*Filter, error) {
	filter := &Filter{
		SkipRepos: make(map[string]bool),
	}

	// Compile include pattern
	if includePattern != "" {
		regex, err := regexp.Compile(includePattern)
		if err != nil {
			return nil, &FilterError{Pattern: includePattern, Err: err}
		}
		filter.IncludePattern = regex
		common.Logger("debug", "Include pattern compiled. pattern=%s", includePattern)
	}

	// Compile exclude pattern
	if excludePattern != "" {
		regex, err := regexp.Compile(excludePattern)
		if err != nil {
			return nil, &FilterError{Pattern: excludePattern, Err: err}
		}
		filter.ExcludePattern = regex
		common.Logger("debug", "Exclude pattern compiled. pattern=%s", excludePattern)
	}

	// Build skip repos map
	for _, repo := range skipRepos {
		filter.SkipRepos[repo] = true
		common.Logger("debug", "Repository added to skip list. repository=%s", repo)
	}

	common.Logger("info", "Repository filter configured. include_pattern=%s exclude_pattern=%s skip_count=%d",
		includePattern, excludePattern, len(skipRepos))

	return filter, nil
}

// ShouldProcess determines if a repository should be processed based on filter criteria
func (f *Filter) ShouldProcess(repoName string) bool {
	// Check skip list first
	if f.SkipRepos[repoName] {
		common.Logger("debug", "Repository skipped (in skip list). repository=%s", repoName)
		return false
	}

	// Check exclude pattern
	if f.ExcludePattern != nil && f.ExcludePattern.MatchString(repoName) {
		common.Logger("debug", "Repository excluded by pattern. repository=%s", repoName)
		return false
	}

	// Check include pattern (if specified, repo must match)
	if f.IncludePattern != nil && !f.IncludePattern.MatchString(repoName) {
		common.Logger("debug", "Repository not included by pattern. repository=%s", repoName)
		return false
	}

	common.Logger("debug", "Repository passes filter criteria. repository=%s", repoName)
	return true
}

// GetStats returns filtering statistics
func (f *Filter) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"has_include_pattern": f.IncludePattern != nil,
		"has_exclude_pattern": f.ExcludePattern != nil,
		"skip_count":          len(f.SkipRepos),
	}

	if f.IncludePattern != nil {
		stats["include_pattern"] = f.IncludePattern.String()
	}
	if f.ExcludePattern != nil {
		stats["exclude_pattern"] = f.ExcludePattern.String()
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
