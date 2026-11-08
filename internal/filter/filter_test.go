package filter

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewFilter(t *testing.T) {
	f, err := NewFilter([]string{"repo-a", "repo-b"})
	if err != nil {
		t.Fatalf("NewFilter returned error: %v", err)
	}
	if len(f.SkipRepos) != 2 || !f.SkipRepos["repo-a"] || !f.SkipRepos["repo-b"] {
		t.Errorf("unexpected SkipRepos: %v", f.SkipRepos)
	}
}

func TestNewFilterEmpty(t *testing.T) {
	f, err := NewFilter(nil)
	if err != nil {
		t.Fatalf("NewFilter returned error: %v", err)
	}
	if f.SkipRepos == nil || len(f.SkipRepos) != 0 {
		t.Errorf("SkipRepos = %v, want empty non-nil map", f.SkipRepos)
	}
	if !f.ShouldProcess("any-repo") {
		t.Error("empty filter should process every repository")
	}
}

func TestShouldProcess(t *testing.T) {
	f, _ := NewFilter([]string{"skip-me"})

	if f.ShouldProcess("skip-me") {
		t.Error("ShouldProcess(skip-me) = true, want false")
	}
	if !f.ShouldProcess("keep-me") {
		t.Error("ShouldProcess(keep-me) = false, want true")
	}
	// Matching is exact (no globbing, case sensitive)
	if !f.ShouldProcess("Skip-Me") {
		t.Error("ShouldProcess(Skip-Me) = false, want true (case sensitive)")
	}
}

func TestSkipReposMatchesRelativePath(t *testing.T) {
	f, _ := NewFilter([]string{"repo", "github/exact"})

	tests := map[string]bool{
		"repo":            false, // directory name
		"github/repo":     false, // directory name of a nested repository
		"github/exact":    false, // relative path
		"gitlab/exact":    true,  // same directory name, other path: only the path was listed
		"github/other":    true,
		"github/repo-two": true,
	}
	for name, want := range tests {
		if got := f.ShouldProcess(name); got != want {
			t.Errorf("ShouldProcess(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestFilterRepositories(t *testing.T) {
	f, _ := NewFilter([]string{"b", "d"})

	got := f.FilterRepositories([]string{"a", "b", "c", "d"})
	want := []string{"a", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterRepositories = %v, want %v", got, want)
	}

	if got := f.FilterRepositories(nil); len(got) != 0 {
		t.Errorf("FilterRepositories(nil) = %v, want empty", got)
	}
}

func TestGetStats(t *testing.T) {
	f, _ := NewFilter([]string{"a", "b", "c"})
	stats := f.GetStats()
	if stats["skip_count"] != 3 {
		t.Errorf("skip_count = %v, want 3", stats["skip_count"])
	}
}

func TestFilterError(t *testing.T) {
	err := &FilterError{Pattern: "foo*", Err: errors.New("bad pattern")}
	want := "filter pattern 'foo*' error: bad pattern"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}
