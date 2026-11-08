package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// BranchInfo describes a local branch.
type BranchInfo struct {
	// Name of the local branch
	Name string
	// Current is true for the branch checked out (HEAD)
	Current bool
	// Upstream is the tracking branch (e.g. origin/main), empty if there is none
	Upstream string
	// Ahead is the number of local commits not pushed
	Ahead int
	// Behind is the number of remote commits not pulled (as of the last fetch)
	Behind int
	// Gone is true when the upstream branch was deleted in the remote
	Gone bool
}

// RepoBranches describes the branches of a repository.
type RepoBranches struct {
	// Current is the branch checked out; nil in detached HEAD
	Current *BranchInfo
	// Detached is the short SHA of HEAD when it is detached
	Detached string
	// Others are the other local branches (alphabetical order)
	Others []BranchInfo
}

// State returns the synchronization state with the upstream branch:
// "up to date", "ahead 2", "behind 3", "ahead 1, behind 2", "upstream gone" or "no upstream".
func (b BranchInfo) State() string {
	switch {
	case b.Upstream == "":
		return "no upstream"
	case b.Gone:
		return "upstream gone"
	case b.Ahead > 0 && b.Behind > 0:
		return fmt.Sprintf("ahead %d, behind %d", b.Ahead, b.Behind)
	case b.Ahead > 0:
		return fmt.Sprintf("ahead %d", b.Ahead)
	case b.Behind > 0:
		return fmt.Sprintf("behind %d", b.Behind)
	default:
		return "up to date"
	}
}

// GetRepoBranches returns the current branch (with upstream, ahead/behind) and the
// other local branches of a repository, using a single "git for-each-ref".
func GetRepoBranches(repoPath string) (RepoBranches, error) {
	return getRepoBranches(context.Background(), repoPath)
}

func getRepoBranches(ctx context.Context, repoPath string) (RepoBranches, error) {
	var result RepoBranches

	// %00 = NUL separator: branch names cannot contain it.
	out, err := gitOutput(ctx, repoPath, "for-each-ref",
		"--format=%(HEAD)%00%(refname:short)%00%(upstream:short)%00%(upstream:track,nobracket)",
		"refs/heads")
	if err != nil {
		return result, &GitError{Repository: repoPath, Operation: "for-each-ref", Err: err}
	}

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 4 {
			continue
		}
		branch := BranchInfo{
			Name:     fields[1],
			Current:  fields[0] == "*",
			Upstream: fields[2],
		}
		branch.Ahead, branch.Behind, branch.Gone = parseTrack(fields[3])
		if branch.Current {
			current := branch
			result.Current = &current
		} else {
			result.Others = append(result.Others, branch)
		}
	}

	if result.Current != nil {
		return result, nil
	}

	// No branch is checked out: unborn branch (repository without commits) or detached HEAD.
	if name, err := gitOutput(ctx, repoPath, "symbolic-ref", "--short", "-q", "HEAD"); err == nil && strings.TrimSpace(name) != "" {
		result.Current = &BranchInfo{Name: strings.TrimSpace(name), Current: true}
		return result, nil
	}
	if sha, err := gitOutput(ctx, repoPath, "rev-parse", "--short", "HEAD"); err == nil {
		result.Detached = strings.TrimSpace(sha)
	}
	return result, nil
}

// parseTrack parses %(upstream:track,nobracket): "", "gone", "ahead 1", "behind 2" or "ahead 1, behind 2".
func parseTrack(track string) (ahead, behind int, gone bool) {
	track = strings.TrimSpace(track)
	if track == "gone" {
		return 0, 0, true
	}
	for _, part := range strings.Split(track, ",") {
		fields := strings.Fields(part)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "ahead":
			ahead = n
		case "behind":
			behind = n
		}
	}
	return ahead, behind, false
}
