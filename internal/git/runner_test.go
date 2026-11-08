package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aeciopires/updateGit/internal/backup"
)

// fakePull replaces pullFunc during the test.
func fakePull(t *testing.T, fn func(ctx context.Context, repoPath string, interactive bool) (string, error)) {
	t.Helper()
	original := pullFunc
	pullFunc = fn
	t.Cleanup(func() { pullFunc = original })
}

// concurrency measures the peak of simultaneous calls.
type concurrency struct {
	current atomic.Int32
	peak    atomic.Int32
	calls   atomic.Int32
}

func (c *concurrency) enter() {
	c.calls.Add(1)
	n := c.current.Add(1)
	for {
		p := c.peak.Load()
		if n <= p || c.peak.CompareAndSwap(p, n) {
			return
		}
	}
}

func (c *concurrency) leave() { c.current.Add(-1) }

// cloneMany clones origin n times into base/repos and returns the repositories.
func cloneMany(t *testing.T, base, origin string, n int) []Repository {
	t.Helper()
	repos := make([]Repository, 0, n)
	for i := range n {
		name := fmt.Sprintf("repo-%02d", i)
		path := filepath.Join(base, "repos", name)
		cloneRepo(t, origin, path)
		repos = append(repos, Repository{Path: path, Name: name, IsValid: true})
	}
	return repos
}

func TestUpdateParallelManyRepos(t *testing.T) {
	if testing.Short() {
		t.Skip("load test (50 repositories)")
	}
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 50)
	pushCommit(t, base, origin, "new.txt")

	sink := &recordingSink{}
	results := Run(context.Background(), repos, RunConfig{Workers: 15, Timeout: time.Minute}, sink)

	if len(results) != 50 || len(sink.results) != 50 {
		t.Fatalf("got %d results (sink %d), want 50", len(results), len(sink.results))
	}
	for _, r := range results {
		if r.Status != StatusOK || !strings.HasPrefix(r.PullSummary, "Fast-forward") {
			t.Errorf("%s: status=%s summary=%q output=%q", r.Repo.Name, r.Status, r.PullSummary, r.Output)
		}
		if _, err := os.Stat(filepath.Join(r.Repo.Path, "new.txt")); err != nil {
			t.Errorf("%s was not updated: %v", r.Repo.Name, err)
		}
		if r.Branches.Current == nil || r.Branches.Current.State() != "up to date" {
			t.Errorf("%s: branches after pull = %+v", r.Repo.Name, r.Branches.Current)
		}
	}
}

func TestUpdateRespectsMaxConcurrent(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 60)

	for _, limit := range []int{15, 20, 50} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var c concurrency
			fakePull(t, func(_ context.Context, _ string, interactive bool) (string, error) {
				if interactive {
					t.Error("parallel mode must not be interactive")
				}
				c.enter()
				defer c.leave()
				time.Sleep(30 * time.Millisecond)
				return "Already up to date.\n", nil
			})
			results := Run(context.Background(), repos, RunConfig{Workers: limit}, nil)
			if len(results) != len(repos) {
				t.Fatalf("got %d results, want %d", len(results), len(repos))
			}
			if peak := c.peak.Load(); peak > int32(limit) || peak < 2 {
				t.Errorf("peak of simultaneous pulls = %d, want between 2 and %d", peak, limit)
			}
		})
	}
}

func TestUpdateSequentialWhenDisabled(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 5)

	if w := EffectiveWorkers(false, 10, len(repos)); w != 1 {
		t.Fatalf("EffectiveWorkers(parallel=false) = %d, want 1", w)
	}

	var c concurrency
	fakePull(t, func(_ context.Context, _ string, interactive bool) (string, error) {
		if !interactive {
			t.Error("sequential mode must be interactive (password prompts)")
		}
		c.enter()
		defer c.leave()
		time.Sleep(10 * time.Millisecond)
		return "", nil
	})
	Run(context.Background(), repos, RunConfig{Workers: 1}, nil)
	if c.peak.Load() != 1 || c.calls.Load() != 5 {
		t.Errorf("peak=%d calls=%d, want 1 and 5", c.peak.Load(), c.calls.Load())
	}
}

func TestUpdateWorkersCappedByRepoCount(t *testing.T) {
	tests := []struct {
		parallel           bool
		max, repos, expect int
	}{
		{true, 50, 3, 3},
		{true, 10, 100, 10},
		{true, 0, 5, 1},
		{false, 50, 3, 1},
		{true, 5, 0, 1},
	}
	for _, tt := range tests {
		if got := EffectiveWorkers(tt.parallel, tt.max, tt.repos); got != tt.expect {
			t.Errorf("EffectiveWorkers(%v, %d, %d) = %d, want %d", tt.parallel, tt.max, tt.repos, got, tt.expect)
		}
	}
	if Run(context.Background(), nil, RunConfig{Workers: 5}, nil) != nil {
		t.Error("Run without repositories should return nil")
	}
}

func TestPullTimeout(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 3)

	fakePull(t, func(ctx context.Context, repoPath string, _ bool) (string, error) {
		if strings.HasSuffix(repoPath, "repo-01") {
			<-ctx.Done() // hangs until the timeout
			return "", ctx.Err()
		}
		return "Already up to date.\n", nil
	})

	start := time.Now()
	results := Run(context.Background(), repos, RunConfig{Workers: 3, Timeout: 200 * time.Millisecond}, nil)
	if time.Since(start) > 5*time.Second {
		t.Fatal("timeout was not respected")
	}
	statuses := map[string]Status{}
	for _, r := range results {
		statuses[r.Repo.Name] = r.Status
		if r.Status == StatusTimeout && (r.Reason != "timeout after 200ms" || r.Hint == "") {
			t.Errorf("timeout result: reason=%q hint=%q", r.Reason, r.Hint)
		}
	}
	if statuses["repo-01"] != StatusTimeout || statuses["repo-00"] != StatusOK || statuses["repo-02"] != StatusOK {
		t.Errorf("statuses = %v", statuses)
	}
}

func TestPullFailureDoesNotStopOthers(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 4)
	runGit(t, repos[2].Path, "remote", "set-url", "origin", filepath.Join(base, "missing.git"))

	results := Run(context.Background(), repos, RunConfig{Workers: 4, Timeout: time.Minute}, nil)
	failed, ok := 0, 0
	for _, r := range results {
		switch r.Status {
		case StatusFailed:
			failed++
			var gitErr *GitError
			if r.Repo.Name != "repo-02" || !errors.As(r.Err, &gitErr) || !strings.HasPrefix(r.PullSummary, "fatal:") || r.Reason == "" {
				t.Errorf("unexpected failure: %+v", r)
			}
		case StatusOK:
			ok++
		}
	}
	if failed != 1 || ok != 3 {
		t.Errorf("failed=%d ok=%d, want 1 and 3", failed, ok)
	}
}

func TestSkipWhenNoUpstream(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)

	noUpstream := filepath.Join(base, "repos", "no-upstream")
	initRepo(t, noUpstream, "main")
	detached := filepath.Join(base, "repos", "detached")
	cloneRepo(t, origin, detached)
	runGit(t, detached, "checkout", "-q", "--detach")
	gone := filepath.Join(base, "repos", "gone")
	cloneRepo(t, origin, gone)
	runGit(t, gone, "checkout", "-q", "-b", "temp")
	runGit(t, gone, "push", "-q", "-u", "origin", "temp")
	runGit(t, gone, "push", "-q", "origin", "--delete", "temp")
	runGit(t, gone, "fetch", "-q", "--prune")

	var calls atomic.Int32
	fakePull(t, func(context.Context, string, bool) (string, error) {
		calls.Add(1)
		return "", nil
	})

	repos := []Repository{{Path: noUpstream, Name: "no-upstream"}, {Path: detached, Name: "detached"}, {Path: gone, Name: "gone"}}
	results := Run(context.Background(), repos, RunConfig{Workers: 3}, nil)
	want := map[string]string{"no-upstream": ReasonNoUpstream, "detached": ReasonDetachedHEAD, "gone": ReasonUpstreamGone}
	for _, r := range results {
		if r.Status != StatusSkipped || r.Reason != want[r.Repo.Name] || r.PullSummary == "" {
			t.Errorf("%s: status=%s reason=%q summary=%q", r.Repo.Name, r.Status, r.Reason, r.PullSummary)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("git pull was called %d times for skipped repositories", calls.Load())
	}
}

func TestSkipDirtyRepository(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 4)
	writeFile(t, filepath.Join(repos[0].Path, "README.md"), "modified\n") // modified
	writeFile(t, filepath.Join(repos[1].Path, "new.txt"), "new\n")        // untracked
	writeFile(t, filepath.Join(repos[2].Path, "staged.txt"), "staged\n")  // staged
	runGit(t, repos[2].Path, "add", "staged.txt")
	// repos[3] is clean

	var pulled sync.Map
	fakePull(t, func(_ context.Context, repoPath string, _ bool) (string, error) {
		pulled.Store(filepath.Base(repoPath), true)
		return "Already up to date.\n", nil
	})
	bk := &fakeBackuper{}

	results := Run(context.Background(), repos, RunConfig{Workers: 4, SkipDirty: true, Backup: bk, BackupMaxConcurrent: 2}, nil)
	byName := map[string]RepoResult{}
	for _, r := range results {
		byName[r.Repo.Name] = r
	}
	wantStatus := map[string]string{"repo-00": "1 modified", "repo-01": "1 untracked", "repo-02": "1 staged"}
	for name, desc := range wantStatus {
		r := byName[name]
		if r.Status != StatusSkipped || r.Reason != ReasonUncommitted || r.WorkingTree.String() != desc ||
			!strings.Contains(r.PullSummary, "branch main ("+desc+")") || r.Hint == "" {
			t.Errorf("%s: status=%s reason=%q tree=%s summary=%q", name, r.Status, r.Reason, r.WorkingTree, r.PullSummary)
		}
		if _, ok := pulled.Load(name); ok {
			t.Errorf("%s: git pull must not run with files not committed", name)
		}
	}
	if byName["repo-03"].Status != StatusOK {
		t.Errorf("clean repository: %+v", byName["repo-03"])
	}
	// Skipped before the backup: only the clean repository is backed up
	if calls := bk.c.calls.Load(); calls != 1 {
		t.Errorf("backups = %d, want 1 (dirty repositories are skipped before the backup)", calls)
	}

	// SkipDirty disabled (previous behavior): every repository is updated
	results = Run(context.Background(), repos, RunConfig{Workers: 4}, nil)
	for _, r := range results {
		if r.Status != StatusOK {
			t.Errorf("SkipDirty=false: %s status %s", r.Repo.Name, r.Status)
		}
	}
}

func TestSkipDirtyStatusError(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 1)
	// A corrupted index makes "git status" fail while the branches are readable
	writeFile(t, filepath.Join(repos[0].Path, ".git", "index"), "corrupted")
	results := Run(context.Background(), repos, RunConfig{Workers: 1, SkipDirty: true}, nil)
	if results[0].Status != StatusFailed || results[0].Reason != reasonStatus {
		t.Errorf("unexpected: status=%s reason=%q err=%v", results[0].Status, results[0].Reason, results[0].Err)
	}
}

func TestBranchErrorIsFailure(t *testing.T) {
	setupGitEnv(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	results := Run(context.Background(), []Repository{{Path: t.TempDir(), Name: "broken"}}, RunConfig{Workers: 1}, nil)
	if len(results) != 1 || results[0].Status != StatusFailed || results[0].Reason != reasonBranches {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestNonInteractiveEnvInParallel(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/home/x"}

	env := pullEnv(true, base)
	if hasEnv(env, "GIT_TERMINAL_PROMPT") || hasEnv(env, "GIT_SSH_COMMAND") || !hasEnv(env, "LC_ALL") {
		t.Errorf("interactive env = %v", env)
	}

	env = pullEnv(false, base)
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_TERMINAL_PROMPT=0") || !strings.Contains(joined, "GIT_SSH_COMMAND=ssh -o BatchMode=yes") {
		t.Errorf("non interactive env = %v", env)
	}

	// The GIT_SSH_COMMAND of the user is preserved
	custom := append(append([]string{}, base...), "GIT_SSH_COMMAND=ssh -i ~/.ssh/key")
	env = pullEnv(false, custom)
	if strings.Contains(strings.Join(env, "\n"), "BatchMode") {
		t.Errorf("GIT_SSH_COMMAND of the user was overridden: %v", env)
	}
}

func TestRunGitPullNonInteractive(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repo := filepath.Join(base, "repo")
	cloneRepo(t, origin, repo)
	pushCommit(t, base, origin, "x.txt")

	out, err := runGitPull(context.Background(), repo, false)
	if err != nil || !strings.Contains(out, "Fast-forward") {
		t.Errorf("runGitPull = %q, %v", out, err)
	}
}

func TestCancelStopsPendingRepos(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 6)

	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	fakePull(t, func(ctx context.Context, _ string, _ bool) (string, error) {
		if started.Add(1) == 2 {
			cancel() // Ctrl+C while 2 pulls are running
		}
		<-ctx.Done()
		return "", ctx.Err()
	})

	done := make(chan []RepoResult)
	go func() { done <- Run(ctx, repos, RunConfig{Workers: 2}, nil) }()
	select {
	case results := <-done:
		if len(results) > 2 {
			t.Errorf("pending repositories were started: %d results", len(results))
		}
		for _, r := range results {
			if r.Status != StatusCanceled {
				t.Errorf("%s: status %s, want CANCELED", r.Repo.Name, r.Status)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the cancellation (deadlock?)")
	}
	if started.Load() > 2 {
		t.Errorf("started %d pulls after the cancellation", started.Load())
	}
}

func TestBackupInParallel(t *testing.T) {
	setupGitEnv(t)
	for _, strategy := range []backup.BackupStrategy{backup.StrategyCopy, backup.StrategyStash} {
		t.Run(string(strategy), func(t *testing.T) {
			base := t.TempDir()
			origin := newOrigin(t, base)
			repos := cloneMany(t, base, origin, 20)
			for _, r := range repos {
				if err := os.WriteFile(filepath.Join(r.Path, "local.txt"), []byte("change"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			fakePull(t, func(context.Context, string, bool) (string, error) { return "Already up to date.\n", nil })

			bm := backup.NewBackupManager(filepath.Join(base, "backups"), strategy)
			sink := &recordingSink{}
			results := Run(context.Background(), repos, RunConfig{Workers: 10, Backup: bm, BackupMaxConcurrent: 4}, sink)
			if len(results) != 20 {
				t.Fatalf("got %d results", len(results))
			}
			for _, r := range results {
				if r.BackupErr != nil || r.BackupPath == "" {
					t.Errorf("%s: backup path=%q err=%v", r.Repo.Name, r.BackupPath, r.BackupErr)
				}
				if strategy == backup.StrategyCopy {
					if _, err := os.Stat(filepath.Join(r.BackupPath, "local.txt")); err != nil {
						t.Errorf("%s: copy backup missing: %v", r.Repo.Name, err)
					}
				} else if out := runGit(t, r.Repo.Path, "stash", "list"); !strings.Contains(out, "updateGit backup") {
					t.Errorf("%s: stash not created", r.Repo.Name)
				}
			}
			kinds := map[EventKind]int{}
			for _, e := range sink.events {
				kinds[e.Kind]++
			}
			if kinds[EventStarted] != 20 || kinds[EventBackupStarted] != 20 || kinds[EventBackupFinished] != 20 || kinds[EventBackupWaiting] != 20 {
				t.Errorf("events = %v", kinds)
			}
		})
	}
}

// fakeBackuper measures the simultaneous backups.
type fakeBackuper struct {
	c     concurrency
	sleep time.Duration
	err   error
}

func (f *fakeBackuper) CreateBackup(repoPath, repoName string) (*backup.BackupInfo, error) {
	f.c.enter()
	defer f.c.leave()
	time.Sleep(f.sleep)
	if f.err != nil {
		return nil, f.err
	}
	return &backup.BackupInfo{Repository: repoName, BackupPath: "/backup/" + repoName, OriginalPath: repoPath}, nil
}

func TestBackupMaxConcurrent(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 20)

	var pulls concurrency
	fakePull(t, func(context.Context, string, bool) (string, error) {
		pulls.enter()
		defer pulls.leave()
		time.Sleep(150 * time.Millisecond)
		return "", nil
	})
	bk := &fakeBackuper{sleep: 20 * time.Millisecond}

	results := Run(context.Background(), repos, RunConfig{Workers: 20, Backup: bk, BackupMaxConcurrent: 3}, nil)
	if len(results) != 20 {
		t.Fatalf("got %d results", len(results))
	}
	if peak := bk.c.peak.Load(); peak > 3 || peak < 1 {
		t.Errorf("peak of simultaneous backups = %d, want <= 3", peak)
	}
	if peak := pulls.peak.Load(); peak <= 3 {
		t.Errorf("peak of simultaneous pulls = %d, want > 3 (independent of the backup limit)", peak)
	}
	waited := false
	for _, r := range results {
		waited = waited || r.BackupWait > 0
	}
	if !waited {
		t.Error("some repository should have waited for a backup slot")
	}
}

func TestBackupFailureContinuesPull(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 1)
	fakePull(t, func(context.Context, string, bool) (string, error) { return "Already up to date.\n", nil })

	results := Run(context.Background(), repos, RunConfig{Workers: 1, Backup: &fakeBackuper{err: errors.New("disk full")}, BackupMaxConcurrent: 1}, nil)
	if results[0].Status != StatusOK || results[0].BackupErr == nil {
		t.Errorf("unexpected: %+v", results[0])
	}
}

func TestBackupLimitCappedByWorkers(t *testing.T) {
	tests := [][3]int{{10, 4, 4}, {3, 20, 3}, {0, 4, 1}, {4, 4, 4}}
	for _, tt := range tests {
		if got := EffectiveBackupLimit(tt[0], tt[1]); got != tt[2] {
			t.Errorf("EffectiveBackupLimit(%d, %d) = %d, want %d", tt[0], tt[1], got, tt[2])
		}
	}
}

func TestBackupLimitDoesNotBlockWithoutBackup(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 6)
	var pulls concurrency
	fakePull(t, func(context.Context, string, bool) (string, error) {
		pulls.enter()
		defer pulls.leave()
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})
	sink := &recordingSink{}
	Run(context.Background(), repos, RunConfig{Workers: 6, BackupMaxConcurrent: 1}, sink)
	if pulls.peak.Load() < 2 {
		t.Errorf("peak = %d: the backup limit must not apply without backup", pulls.peak.Load())
	}
	for _, e := range sink.events {
		if e.Kind != EventStarted {
			t.Errorf("unexpected backup event without backup: %+v", e)
		}
	}
}

func TestBackupWaitCancel(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	repos := cloneMany(t, base, origin, 4)
	fakePull(t, func(context.Context, string, bool) (string, error) { return "", nil })

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	bk := &fakeBackuper{sleep: 300 * time.Millisecond}

	done := make(chan []RepoResult)
	go func() { done <- Run(ctx, repos, RunConfig{Workers: 4, Backup: bk, BackupMaxConcurrent: 1}, nil) }()
	select {
	case results := <-done:
		canceled := 0
		for _, r := range results {
			if r.Status == StatusCanceled {
				canceled++
			}
		}
		if canceled < 3 {
			t.Errorf("canceled = %d, want >= 3 (waiting for the backup slot)", canceled)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock while waiting for a backup slot")
	}
}

func TestDiscoveryCompletesBeforeUpdate(t *testing.T) {
	setupGitEnv(t)
	base := t.TempDir()
	origin := newOrigin(t, base)
	cloneMany(t, base, origin, 8)
	initRepo(t, filepath.Join(base, "repos", "group", "nested"), "main")

	var mu sync.Mutex
	var discoveryEnd, firstPull time.Time
	discovery, err := DiscoverRepositories(DiscoveryOptions{
		BaseDir:  filepath.Join(base, "repos"),
		MaxDepth: 2,
		OnProgress: func(int, int) {
			mu.Lock()
			discoveryEnd = time.Now()
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	fakePull(t, func(context.Context, string, bool) (string, error) {
		mu.Lock()
		if firstPull.IsZero() {
			firstPull = time.Now()
		}
		mu.Unlock()
		return "", nil
	})

	sink := &recordingSink{}
	Run(context.Background(), discovery.Repos, RunConfig{Workers: 4}, sink)
	if len(discovery.Repos) != 9 {
		t.Fatalf("discovered %d repositories, want 9", len(discovery.Repos))
	}
	if !firstPull.After(discoveryEnd) {
		t.Error("a pull started before the discovery finished")
	}
	// The total known before the update is the number of results
	if len(sink.results) != len(discovery.Repos) {
		t.Errorf("results = %d, total = %d", len(sink.results), len(discovery.Repos))
	}
}

func TestSummarizePullOutput(t *testing.T) {
	tests := []struct {
		name, output, want string
	}{
		{"empty", "", ""},
		{"up to date", "Already up to date.\n", "Already up to date."},
		{"fast-forward", "Updating a1b2c3d..e4f5a6b\nFast-forward\n README.md | 2 +-\n 3 files changed, 10 insertions(+), 2 deletions(-)\n",
			"Fast-forward: 3 files changed, 10 insertions(+), 2 deletions(-)"},
		{"fast-forward without stat", "Fast-forward\n", "Fast-forward"},
		{"merge", "Merge made by the 'ort' strategy.\n a | 1 +\n 1 file changed, 1 insertion(+)\n", "Merge: 1 file changed, 1 insertion(+)"},
		{"merge without stat", "Merge made by the 'ort' strategy.\n", "Merge"},
		{"rebase", "Successfully rebased and updated refs/heads/main.\n", "Rebased"},
		{"fatal", "remote: x\nfatal: could not read Username for 'https://github.com': terminal prompts disabled\n",
			"fatal: could not read Username for 'https://github.com': terminal prompts disabled"},
		{"error", "error: Your local changes to the following files would be overwritten by merge:\n\ta.txt\nAborting\n",
			"error: Your local changes to the following files would be overwritten by merge:"},
		{"other", "something\nlast line\n", "last line"},
	}
	for _, tt := range tests {
		if got := SummarizePullOutput(tt.output); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestClassifyFailure(t *testing.T) {
	tests := []struct {
		output      string
		interactive bool
		reason      string
		hintPart    string
	}{
		{"fatal: could not read Username for 'https://github.com': terminal prompts disabled", false, reasonAuth, "Run sequentially"},
		{"git@github.com: Permission denied (publickey).", true, reasonAuth, "credentials"},
		{"error: Your local changes to the following files would be overwritten by merge", false, reasonLocalChanges, "stash"},
		{"fatal: Not possible to fast-forward, aborting.", false, reasonDiverged, "rebase"},
		{"kex_exchange_identification: Connection closed by remote host", false, reasonConnection, "ControlMaster"},
		{"fatal: unable to access 'https://x/': Could not resolve host: x", false, reasonResolveHost, "DNS"},
		{"fatal: unable to access 'https://127.0.0.1:9/': Failed to connect to 127.0.0.1 port 9", false, reasonConnect, "remote URL"},
		{"fatal: something else", false, "something else", ""},
		{"", false, reasonUnknownFailed, ""},
	}
	for _, tt := range tests {
		reason, hint := classifyFailure(tt.output, tt.interactive)
		if reason != tt.reason || !strings.Contains(hint, tt.hintPart) {
			t.Errorf("%q: reason=%q hint=%q", tt.output, reason, hint)
		}
	}
}
