package git

import (
	"context"
	"fmt"
	"strings"
)

// WorkingTreeStatus counts the files not committed in a repository
// (output of "git status --porcelain"; ignored files are not counted).
type WorkingTreeStatus struct {
	// Staged files (added to the index, not committed)
	Staged int
	// Modified tracked files (not added to the index)
	Modified int
	// Untracked files (new files not ignored by .gitignore)
	Untracked int
	// Conflicted files (unfinished merge/rebase)
	Conflicted int
}

// Dirty tells if there is any file not committed.
func (s WorkingTreeStatus) Dirty() bool {
	return s.Staged+s.Modified+s.Untracked+s.Conflicted > 0
}

// String describes the files not committed, e.g. "2 modified, 1 staged, 3 untracked".
func (s WorkingTreeStatus) String() string {
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(s.Conflicted, "conflicted")
	add(s.Staged, "staged")
	add(s.Modified, "modified")
	add(s.Untracked, "untracked")
	if len(parts) == 0 {
		return "clean"
	}
	return strings.Join(parts, ", ")
}

// GetWorkingTreeStatus returns the files not committed of a repository.
func GetWorkingTreeStatus(repoPath string) (WorkingTreeStatus, error) {
	return getWorkingTreeStatus(context.Background(), repoPath)
}

func getWorkingTreeStatus(ctx context.Context, repoPath string) (WorkingTreeStatus, error) {
	var status WorkingTreeStatus
	out, err := gitOutput(ctx, repoPath, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return status, &GitError{Repository: repoPath, Operation: "status", Err: err}
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		x, y := line[0], line[1]
		switch {
		case x == '?' && y == '?':
			status.Untracked++
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			status.Conflicted++
		default:
			if x != ' ' {
				status.Staged++
			}
			if y != ' ' {
				status.Modified++
			}
		}
	}
	return status, nil
}
