package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aeciopires/updateGit/internal/backup"
	"github.com/aeciopires/updateGit/internal/common"
)

// Status is the result of the update of a repository.
type Status string

// Possible statuses of RepoResult.
const (
	StatusOK       Status = "OK"
	StatusFailed   Status = "FAILED"
	StatusTimeout  Status = "TIMEOUT"
	StatusSkipped  Status = "SKIPPED"
	StatusCanceled Status = "CANCELED"
)

// Skip reasons (RepoResult.Reason when Status is StatusSkipped).
const (
	ReasonNoUpstream    = "no upstream"
	ReasonDetachedHEAD  = "detached HEAD"
	ReasonUpstreamGone  = "upstream gone"
	ReasonUncommitted   = "uncommitted changes"
	reasonStatus        = "cannot read the working tree status"
	reasonAuth          = "authentication required"
	reasonLocalChanges  = "local changes would be overwritten"
	reasonDiverged      = "local and remote branches diverged"
	reasonConnection    = "connection closed by remote host"
	reasonResolveHost   = "could not resolve host"
	reasonConnect       = "could not connect to the remote"
	reasonBranches      = "cannot read branches"
	reasonCanceled      = "canceled"
	waitDelayAfterKill  = 2 * time.Second
	defaultTimeout      = 5 * time.Minute
	reasonUnknownFailed = "git pull failed"
)

// RepoResult is the result of the update of one repository. Workers never print
// anything: they return a RepoResult and the caller (Sink) prints it.
type RepoResult struct {
	Repo     Repository
	Status   Status
	Branches RepoBranches
	// PullSummary is one line summarizing the "git pull" output
	PullSummary string
	// Output is the complete "git pull" output (credentials masked)
	Output string
	// Reason is a short reason for FAILED/TIMEOUT/SKIPPED/CANCELED
	Reason string
	// Hint suggests how to fix a failure
	Hint string
	Err  error
	// WorkingTree describes the files not committed (filled when RunConfig.SkipDirty is enabled)
	WorkingTree WorkingTreeStatus
	// BackupPath is filled when a backup was created
	BackupPath string
	BackupErr  error
	// BackupWait is the time waiting for a backup slot (backup.max_concurrent)
	BackupWait time.Duration
	Duration   time.Duration
}

// EventKind identifies an Event.
type EventKind int

// Events sent while a repository is processed (used by the progress bar).
const (
	EventStarted EventKind = iota
	EventBackupWaiting
	EventBackupStarted
	EventBackupFinished
)

// Event is a notification sent by a worker.
type Event struct {
	Kind EventKind
	Repo string
}

// Backuper creates the backup of a repository (implemented by *backup.BackupManager).
type Backuper interface {
	CreateBackup(repoPath, repoName string) (*backup.BackupInfo, error)
}

// Sink receives the events and results. Its methods are always called from the
// goroutine that called Run (never concurrently), so it can write to the terminal.
type Sink interface {
	OnEvent(Event)
	OnResult(RepoResult)
}

// RunConfig configures Run.
type RunConfig struct {
	// Workers is the number of simultaneous "git pull" (1 = sequential and interactive)
	Workers int
	// Timeout of each "git pull" (default 5 minutes)
	Timeout time.Duration
	// Backup creates a backup before the pull of each repository (nil = disabled)
	Backup Backuper
	// BackupMaxConcurrent limits the simultaneous backups (independent of Workers)
	BackupMaxConcurrent int
	// SkipDirty skips the repositories with files not committed (staged, modified,
	// untracked or conflicted) to avoid losing work. The CLI enables it by default
	// (git.skip_dirty); the zero value keeps the repositories.
	SkipDirty bool
}

// pullFunc runs "git pull". It is a variable so the tests can replace it.
var pullFunc = runGitPull

// EffectiveWorkers returns the number of workers: 1 when parallel is disabled,
// otherwise maxConcurrent limited by the number of repositories.
func EffectiveWorkers(parallel bool, maxConcurrent, repos int) int {
	workers := 1
	if parallel {
		workers = maxConcurrent
	}
	if workers > repos {
		workers = repos
	}
	if workers < 1 {
		workers = 1
	}
	return workers
}

// EffectiveBackupLimit returns the number of simultaneous backups: limit, but never
// more than the number of workers.
func EffectiveBackupLimit(limit, workers int) int {
	if limit < 1 {
		limit = 1
	}
	if limit > workers {
		limit = workers
	}
	return limit
}

// Run updates the repositories with cfg.Workers simultaneous "git pull". The list of
// repositories must be final (discovered and filtered). Results are sent to sink
// as soon as each repository finishes. When ctx is canceled (Ctrl+C), the running
// pulls are killed, the pending repositories are not started and the partial results
// are returned.
func Run(ctx context.Context, repos []Repository, cfg RunConfig, sink Sink) []RepoResult {
	if len(repos) == 0 {
		return nil
	}
	workers := EffectiveWorkers(true, cfg.Workers, len(repos))
	interactive := workers == 1
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	var backupSem chan struct{}
	if cfg.Backup != nil {
		backupSem = make(chan struct{}, EffectiveBackupLimit(cfg.BackupMaxConcurrent, workers))
	}

	jobs := make(chan Repository)
	messages := make(chan any, workers*4)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for repo := range jobs {
				if ctx.Err() != nil {
					continue // canceled: do not start pending repositories
				}
				emit := func(e Event) { messages <- e }
				messages <- processRepo(ctx, repo, cfg, interactive, backupSem, emit)
			}
		})
	}

	go func() {
		defer close(jobs)
		for _, repo := range repos {
			select {
			case <-ctx.Done():
				return
			case jobs <- repo:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(messages)
	}()

	results := make([]RepoResult, 0, len(repos))
	for msg := range messages {
		switch v := msg.(type) {
		case Event:
			if sink != nil {
				sink.OnEvent(v)
			}
		case RepoResult:
			results = append(results, v)
			if sink != nil {
				sink.OnResult(v)
			}
		}
	}
	return results
}

// processRepo updates one repository: checks the upstream, creates the backup
// (limited by backupSem), runs "git pull" with timeout and reads the branches.
func processRepo(ctx context.Context, repo Repository, cfg RunConfig, interactive bool, backupSem chan struct{}, emit func(Event)) RepoResult {
	start := time.Now()
	result := RepoResult{Repo: repo}
	finish := func() RepoResult {
		result.Duration = time.Since(start)
		return result
	}

	emit(Event{Kind: EventStarted, Repo: repo.Name})

	// The read-only git commands (branches and status) share the timeout of the
	// repository, so a hung file system never blocks a worker forever.
	readCtx, cancelRead := context.WithTimeout(ctx, cfg.Timeout)
	defer cancelRead()

	branches, err := getRepoBranches(readCtx, repo.Path)
	if err != nil {
		result.Status, result.Reason, result.Err = StatusFailed, reasonBranches, err
		return finish()
	}
	result.Branches = branches

	switch {
	case branches.Current == nil:
		result.Status, result.Reason = StatusSkipped, ReasonDetachedHEAD
		result.PullSummary = "Skipped pull: HEAD is detached."
		return finish()
	case branches.Current.Upstream == "":
		result.Status, result.Reason = StatusSkipped, ReasonNoUpstream
		result.PullSummary = "Skipped pull: current branch has no upstream."
		return finish()
	case branches.Current.Gone:
		result.Status, result.Reason = StatusSkipped, ReasonUpstreamGone
		result.PullSummary = "Skipped pull: upstream branch was deleted in the remote."
		return finish()
	}

	// Files not committed: skipped before the backup and the pull, to avoid losing work
	if cfg.SkipDirty {
		status, err := getWorkingTreeStatus(readCtx, repo.Path)
		if err != nil {
			result.Status, result.Reason, result.Err = StatusFailed, reasonStatus, err
			return finish()
		}
		result.WorkingTree = status
		if status.Dirty() {
			result.Status, result.Reason = StatusSkipped, ReasonUncommitted
			result.PullSummary = fmt.Sprintf("Skipped pull: files not committed in branch %s (%s).", branches.Current.Name, status)
			result.Hint = "Commit or stash the changes, or use --git-skip-dirty=false (with --backup-enabled --backup-strategy stash to keep them)."
			return finish()
		}
	}

	if cfg.Backup != nil && backupSem != nil {
		emit(Event{Kind: EventBackupWaiting, Repo: repo.Name})
		waitStart := time.Now()
		select {
		case backupSem <- struct{}{}:
		case <-ctx.Done():
			result.Status, result.Reason = StatusCanceled, reasonCanceled
			return finish()
		}
		result.BackupWait = time.Since(waitStart)
		emit(Event{Kind: EventBackupStarted, Repo: repo.Name})
		info, err := cfg.Backup.CreateBackup(repo.Path, repo.Name)
		<-backupSem
		emit(Event{Kind: EventBackupFinished, Repo: repo.Name})
		if err != nil {
			// Same behavior as before: the failure is reported and the pull continues
			result.BackupErr = err
			common.Logger("error", "Failed to create backup. repository=%s error=%v", repo.Name, err)
		} else if info != nil {
			result.BackupPath = info.BackupPath
		}
	}

	pullCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	output, err := pullFunc(pullCtx, repo.Path, interactive)
	result.Output = common.MaskCredentials(output)

	switch {
	case err == nil:
		result.Status = StatusOK
		result.PullSummary = SummarizePullOutput(result.Output)
		// Refresh the branches: ahead/behind after the pull
		refreshCtx, cancelRefresh := context.WithTimeout(ctx, cfg.Timeout)
		defer cancelRefresh()
		if updated, err := getRepoBranches(refreshCtx, repo.Path); err == nil {
			result.Branches = updated
		}
	case ctx.Err() != nil:
		result.Status, result.Reason, result.Err = StatusCanceled, reasonCanceled, ctx.Err()
		result.PullSummary = "Canceled."
	case errors.Is(pullCtx.Err(), context.DeadlineExceeded):
		result.Status = StatusTimeout
		result.Reason = fmt.Sprintf("timeout after %s", cfg.Timeout)
		result.PullSummary = fmt.Sprintf("Timeout after %s.", cfg.Timeout)
		result.Hint = "Increase it with --git-timeout."
		result.Err = pullCtx.Err()
	default:
		result.Status = StatusFailed
		result.Err = &GitError{Repository: repo.Path, Operation: "pull", Err: err}
		result.PullSummary = SummarizePullOutput(result.Output)
		result.Reason, result.Hint = classifyFailure(result.Output, interactive)
	}
	return finish()
}

// runGitPull runs "git pull" capturing the output. In parallel mode (not
// interactive) git never asks for passwords: the repository fails instead.
func runGitPull(ctx context.Context, repoPath string, interactive bool) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "pull")
	cmd.Dir = repoPath
	cmd.Env = pullEnv(interactive, os.Environ())
	// ssh may keep the output pipes open after git is killed by the timeout
	cmd.WaitDelay = waitDelayAfterKill
	if interactive {
		// git and ssh read passwords from the terminal (/dev/tty)
		cmd.Stdin = os.Stdin
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pullEnv returns the environment of "git pull". LC_ALL=C keeps the git messages
// in English (they are parsed). In non interactive mode the prompts are disabled
// and ssh uses BatchMode, unless the user defined GIT_SSH_COMMAND.
func pullEnv(interactive bool, environ []string) []string {
	env := make([]string, 0, len(environ)+3)
	env = append(env, environ...)
	env = append(env, "LC_ALL=C")
	if interactive {
		return env
	}
	env = append(env, "GIT_TERMINAL_PROMPT=0")
	if !hasEnv(environ, "GIT_SSH_COMMAND") {
		env = append(env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
	}
	return env
}

func hasEnv(environ []string, name string) bool {
	prefix := name + "="
	for _, kv := range environ {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

var statLine = regexp.MustCompile(`^\d+ files? changed`)

// SummarizePullOutput returns one line summarizing the "git pull" output:
// "Already up to date.", "Fast-forward: 3 files changed, ...", the first
// "fatal:"/"error:" line or the last line.
func SummarizePullOutput(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}

	stat := ""
	for _, line := range lines {
		if strings.HasPrefix(line, "fatal:") || strings.HasPrefix(line, "error:") {
			return line
		}
		if stat == "" && statLine.MatchString(line) {
			stat = line
		}
	}

	joined := strings.Join(lines, "\n")
	switch {
	case strings.Contains(joined, "Already up to date") || strings.Contains(joined, "Already up-to-date"):
		return "Already up to date."
	case strings.Contains(joined, "Fast-forward"):
		if stat != "" {
			return "Fast-forward: " + stat
		}
		return "Fast-forward"
	case strings.Contains(joined, "Merge made by"):
		if stat != "" {
			return "Merge: " + stat
		}
		return "Merge"
	case strings.Contains(joined, "Successfully rebased"):
		return "Rebased"
	}
	return lines[len(lines)-1]
}

// classifyFailure returns a short reason and a hint for a failed "git pull".
func classifyFailure(output string, interactive bool) (reason, hint string) {
	out := strings.ToLower(output)
	switch {
	case containsAny(out, "could not read username", "could not read password", "terminal prompts disabled",
		"authentication failed", "permission denied (publickey", "host key verification failed",
		"permission denied, please try again"):
		if interactive {
			return reasonAuth, "Check your credentials (ssh-agent or a git credential helper)."
		}
		return reasonAuth, "Authentication required. Run sequentially (--git-parallel-enabled=false) or configure ssh-agent / a git credential helper."
	case containsAny(out, "would be overwritten by merge", "please commit your changes or stash them",
		"your local changes to the following files"):
		return reasonLocalChanges, "Commit or stash the local changes (by default, repositories with files not committed are skipped)."
	case containsAny(out, "not possible to fast-forward", "divergent branches", "need to specify how to reconcile"):
		return reasonDiverged, "Resolve it manually (git pull --rebase or git merge)."
	case containsAny(out, "connection closed by remote host", "kex_exchange_identification", "connection reset by peer"):
		return reasonConnection, "Too many simultaneous SSH connections? Reduce --git-max-concurrent or use SSH ControlMaster."
	case containsAny(out, "could not resolve host", "could not resolve hostname"):
		return reasonResolveHost, "Check your network connection and DNS."
	case containsAny(out, "failed to connect", "connection refused", "connection timed out", "could not connect"):
		return reasonConnect, "Check your network connection and the remote URL (git remote -v)."
	}
	if summary := SummarizePullOutput(output); summary != "" {
		return strings.TrimPrefix(strings.TrimPrefix(summary, "fatal: "), "error: "), ""
	}
	return reasonUnknownFailed, ""
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
