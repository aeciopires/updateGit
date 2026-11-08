package output

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/git"
)

// Run "go test ./internal/output -update" to regenerate the golden files.
var update = flag.Bool("update", false, "update the golden files")

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s: %v (run: go test ./internal/output -update)", path, err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func current(name, upstream string, ahead, behind int) *git.BranchInfo {
	return &git.BranchInfo{Name: name, Current: true, Upstream: upstream, Ahead: ahead, Behind: behind}
}

// sampleResults are the scenarios of the blocks.
func sampleResults() map[string]git.RepoResult {
	manyBranches := make([]git.BranchInfo, 0, 11)
	for i := range 11 {
		manyBranches = append(manyBranches, git.BranchInfo{Name: fmt.Sprintf("branch-%02d", i)})
	}
	return map[string]git.RepoResult{
		"ok": {
			Repo:   git.Repository{Name: "updateGit"},
			Status: git.StatusOK,
			Branches: git.RepoBranches{
				Current: current("main", "origin/main", 0, 0),
				Others:  []git.BranchInfo{{Name: "develop"}, {Name: "feature/parallel-pull", Ahead: 1}},
			},
			PullSummary: "Already up to date.",
			Output:      "Already up to date.\n",
			Duration:    800 * time.Millisecond,
		},
		"failed": {
			Repo:        git.Repository{Name: "private-repo"},
			Status:      git.StatusFailed,
			Branches:    git.RepoBranches{Current: current("develop", "origin/develop", 0, 3), Others: []git.BranchInfo{{Name: "main"}}},
			PullSummary: "fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			Output:      "remote: hello\nfatal: could not read Username for 'https://github.com': terminal prompts disabled\n",
			Reason:      "authentication required",
			Hint:        "Authentication required. Run sequentially (--git-parallel-enabled=false) or configure ssh-agent / a git credential helper.",
			Err:         errors.New("exit status 128"),
			Duration:    400 * time.Millisecond,
		},
		"timeout": {
			Repo:        git.Repository{Name: "huge-monorepo"},
			Status:      git.StatusTimeout,
			Branches:    git.RepoBranches{Current: current("main", "origin/main", 0, 120)},
			PullSummary: "Timeout after 5m0s.",
			Reason:      "timeout after 5m0s",
			Hint:        "Increase it with --git-timeout.",
			Duration:    5 * time.Minute,
		},
		"skipped-no-upstream": {
			Repo:        git.Repository{Name: "scratch"},
			Status:      git.StatusSkipped,
			Branches:    git.RepoBranches{Current: current("main", "", 0, 0)},
			PullSummary: "Skipped pull: current branch has no upstream.",
			Reason:      git.ReasonNoUpstream,
			Duration:    200 * time.Millisecond,
		},
		"detached": {
			Repo:        git.Repository{Name: "github/org-x/api"},
			Status:      git.StatusSkipped,
			Branches:    git.RepoBranches{Detached: "3f2a1c9", Others: []git.BranchInfo{{Name: "main"}, {Name: "draft"}}},
			PullSummary: "Skipped pull: HEAD is detached.",
			Reason:      git.ReasonDetachedHEAD,
			Duration:    100 * time.Millisecond,
		},
		"many-branches": {
			Repo:        git.Repository{Name: "github/helm-charts"},
			Status:      git.StatusOK,
			Branches:    git.RepoBranches{Current: current("release/7.1", "origin/release/7.1", 0, 0), Others: manyBranches},
			PullSummary: "Fast-forward: 4 files changed, 37 insertions(+), 5 deletions(-)",
			BackupPath:  "/backups/20261108-101502/github/helm-charts",
			Duration:    2100 * time.Millisecond,
		},
		"skipped-dirty": {
			Repo:        git.Repository{Name: "github/wip"},
			Status:      git.StatusSkipped,
			Branches:    git.RepoBranches{Current: current("feature/login", "origin/feature/login", 1, 0), Others: []git.BranchInfo{{Name: "main"}}},
			WorkingTree: git.WorkingTreeStatus{Modified: 2, Untracked: 1},
			PullSummary: "Skipped pull: files not committed in branch feature/login (2 modified, 1 untracked).",
			Reason:      git.ReasonUncommitted,
			Hint:        "Commit or stash the changes, or use --git-skip-dirty=false (with --backup-enabled --backup-strategy stash to keep them).",
			Duration:    100 * time.Millisecond,
		},
		"canceled-backup-error": {
			Repo:        git.Repository{Name: "slow"},
			Status:      git.StatusCanceled,
			Branches:    git.RepoBranches{Current: current("main", "origin/main", 2, 1)},
			PullSummary: "Canceled.",
			Reason:      "canceled",
			BackupErr:   errors.New("disk full"),
			BackupWait:  1500 * time.Millisecond,
			Duration:    3 * time.Second,
		},
	}
}

func TestRenderRepoResult(t *testing.T) {
	for name, res := range sampleResults() {
		for _, color := range []bool{true, false} {
			mode := "plain"
			if color {
				mode = "color"
			}
			t.Run(name+"-"+mode, func(t *testing.T) {
				got := RenderRepoResult(res, 3, 50, Options{Color: color})
				golden(t, "block-"+name+"-"+mode, got)
				if !color && ansiRegex.MatchString(got) {
					t.Error("plain block must not contain ANSI codes")
				}
			})
		}
	}
}

func TestRenderDebugShowsOutput(t *testing.T) {
	res := sampleResults()["ok"]
	res.Output = "Updating a..b\nFast-forward\n a | 1 +\n 1 file changed, 1 insertion(+)\n"
	res.PullSummary = "Fast-forward: 1 file changed, 1 insertion(+)"
	golden(t, "block-debug-plain", RenderRepoResult(res, 1, 1, Options{Debug: true}))

	// Without debug the output of a successful pull is not shown
	if strings.Contains(RenderRepoResult(res, 1, 1, Options{}), "git pull output") {
		t.Error("output shown without debug")
	}
}

func TestRenderBranchTruncation(t *testing.T) {
	res := sampleResults()["many-branches"]
	plain := RenderRepoResult(res, 1, 1, Options{})
	if !strings.Contains(plain, "+3 more") || strings.Contains(plain, "branch-10") {
		t.Errorf("expected 8 branches and +3 more:\n%s", plain)
	}
	debug := RenderRepoResult(res, 1, 1, Options{Debug: true})
	if strings.Contains(debug, "more") || !strings.Contains(debug, "branch-10") {
		t.Errorf("debug must show all branches:\n%s", debug)
	}
	limited := RenderRepoResult(res, 1, 1, Options{MaxBranches: 2})
	if !strings.Contains(limited, "+9 more") {
		t.Errorf("MaxBranches=2:\n%s", limited)
	}
}

func TestRenderSymbolsFallback(t *testing.T) {
	for _, res := range sampleResults() {
		plain := RenderRepoResult(res, 1, 1, Options{Color: false})
		for _, symbol := range []string{"✔", "✘", "●", "⏭", "⏱", "○", "↓", "⚠", "→", "·"} {
			if strings.Contains(plain, symbol) {
				t.Errorf("plain output contains %q:\n%s", symbol, plain)
			}
		}
	}
}

func sampleSummary() Summary {
	results := sampleResults()
	return Summary{
		Results: []git.RepoResult{
			results["ok"], results["failed"], results["timeout"], results["skipped-no-upstream"],
			results["detached"], results["many-branches"], results["canceled-backup-error"], results["skipped-dirty"],
		},
		Total:    10,
		Filtered: []string{"old-project", "broken-repo"},
		Duration: 41700 * time.Millisecond,
	}
}

func TestRenderSummary(t *testing.T) {
	s := sampleSummary()
	golden(t, "summary-color", RenderSummary(s, Options{Color: true}))
	golden(t, "summary-plain", RenderSummary(s, Options{}))

	counts, notStarted := s.Counts()
	if counts[git.StatusOK] != 2 || counts[git.StatusSkipped] != 3 || notStarted != 2 || !s.HasFailures() {
		t.Errorf("counts=%v notStarted=%d", counts, notStarted)
	}

	clean := Summary{Results: []git.RepoResult{sampleResults()["ok"]}, Total: 1, Duration: time.Second}
	if clean.HasFailures() {
		t.Error("no failures expected")
	}
	golden(t, "summary-clean-plain", RenderSummary(clean, Options{}))
}

func TestRenderHidesSuccessAtWarningLevel(t *testing.T) {
	var console, file bytes.Buffer
	p := NewPrinter(3, nil,
		Destination{Writer: &console, Opts: Options{}, MinLevel: common.LevelWarning},
		Destination{Writer: &file, Opts: Options{}, MinLevel: common.LevelInfo},
		Destination{Writer: nil},
	)
	results := sampleResults()
	p.OnEvent(git.Event{Kind: git.EventStarted, Repo: "x"})
	p.OnResult(results["ok"])
	p.OnResult(results["skipped-no-upstream"])
	p.OnResult(results["failed"])
	p.PrintSummary(Summary{Results: []git.RepoResult{results["ok"], results["skipped-no-upstream"], results["failed"]}, Total: 3})

	if strings.Contains(console.String(), "updateGit (") || strings.Contains(console.String(), "scratch (") {
		t.Errorf("warning level must hide the success blocks:\n%s", console.String())
	}
	if !strings.Contains(console.String(), "[3/3] private-repo") || !strings.Contains(console.String(), "Summary") {
		t.Errorf("failures and summary must be shown:\n%s", console.String())
	}
	if !strings.Contains(file.String(), "[1/3] updateGit") {
		t.Errorf("the info destination must show all blocks:\n%s", file.String())
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		800 * time.Millisecond:   "0.8s",
		41700 * time.Millisecond: "41.7s",
		5 * time.Minute:          "5m0s",
		90500 * time.Millisecond: "1m31s",
	}
	for d, want := range tests {
		if got := FormatDuration(d); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// newTestProgress returns a Progress with a fixed clock.
func newTestProgress(total, backupLimit int, color bool, width int) (*Progress, *time.Time) {
	p := NewProgress(total, backupLimit, color, func() int { return width })
	start := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	now := start
	p.start = start
	p.now = func() time.Time { return now }
	return p, &now
}

func TestProgressRender(t *testing.T) {
	for _, color := range []bool{true, false} {
		mode := map[bool]string{true: "color", false: "plain"}[color]
		for _, width := range []int{60, 80, 200} {
			p, now := newTestProgress(50, 4, color, width)
			golden(t, fmt.Sprintf("progress-0-%s-%d", mode, width), p.Line())

			for i := range 21 {
				name := fmt.Sprintf("repo-%02d", i)
				p.OnEvent(git.Event{Kind: git.EventStarted, Repo: name})
				status := git.StatusOK
				switch i {
				case 3:
					status = git.StatusFailed
				case 7:
					status = git.StatusSkipped
				}
				p.OnResult(git.RepoResult{Repo: git.Repository{Name: name}, Status: status})
			}
			for _, name := range []string{"infra-live", "docs-site", "api", "web", "worker"} {
				p.OnEvent(git.Event{Kind: git.EventStarted, Repo: name})
			}
			p.OnEvent(git.Event{Kind: git.EventBackupStarted})
			p.OnEvent(git.Event{Kind: git.EventBackupStarted})
			*now = now.Add(12 * time.Second)
			line := p.Line()
			golden(t, fmt.Sprintf("progress-42-%s-%d", mode, width), line)
			if visible := len([]rune(ansiRegex.ReplaceAllString(line, ""))); visible > width-1 {
				t.Errorf("width %d: visible length %d wraps the line", width, visible)
			}

			for i := 21; i < 50; i++ {
				p.OnResult(git.RepoResult{Repo: git.Repository{Name: fmt.Sprint(i)}, Status: git.StatusOK})
			}
			p.OnEvent(git.Event{Kind: git.EventBackupFinished})
			p.OnEvent(git.Event{Kind: git.EventBackupFinished})
			p.OnEvent(git.Event{Kind: git.EventBackupFinished}) // never negative
			golden(t, fmt.Sprintf("progress-100-%s-%d", mode, width), p.Line())
		}
	}
}

func TestProgressETA(t *testing.T) {
	p, now := newTestProgress(10, 0, false, 200)
	if !strings.Contains(p.Line(), "ETA --:--") {
		t.Errorf("ETA before the first result: %q", p.Line())
	}
	*now = now.Add(10 * time.Second)
	for i := range 2 {
		p.OnResult(git.RepoResult{Repo: git.Repository{Name: fmt.Sprint(i)}, Status: git.StatusOK})
	}
	// 2 done in 10s => 5s each => 8 remaining = 40s
	if !strings.Contains(p.Line(), "ETA ~00:40") {
		t.Errorf("ETA: %q", p.Line())
	}
	if strings.Contains(p.Line(), "backup") {
		t.Error("backup segment shown with backup disabled")
	}
	if clock(2*time.Hour+3*time.Minute+4*time.Second) != "02:03:04" {
		t.Error("clock with hours")
	}
}

func TestProgressModes(t *testing.T) {
	tests := []struct {
		mode     string
		terminal bool
		workers  int
		want     bool
	}{
		{"auto", true, 10, true},
		{"auto", false, 10, false},
		{"auto", true, 1, false}, // sequential: possible password prompts
		{"always", false, 1, true},
		{"never", true, 10, false},
	}
	for _, tt := range tests {
		if got := ProgressEnabled(tt.mode, tt.terminal, tt.workers); got != tt.want {
			t.Errorf("ProgressEnabled(%s, %v, %d) = %v", tt.mode, tt.terminal, tt.workers, got)
		}
	}
	if !SpinnerEnabled("auto", true) || SpinnerEnabled("auto", false) || !SpinnerEnabled("always", false) || SpinnerEnabled("never", true) {
		t.Error("SpinnerEnabled")
	}
}

func TestSpinner(t *testing.T) {
	s := NewSpinner("/home/user/git", 2, false, func() int { return 200 })
	s.Update(14, 61)
	line := s.Line()
	if !strings.Contains(line, "Scanning /home/user/git (levels 1-2) - 14 directories - 61 repositories found...") {
		t.Errorf("spinner line: %q", line)
	}
	colored := NewSpinner("/x", 3, true, func() int { return 30 })
	if visible := len([]rune(ansiRegex.ReplaceAllString(colored.Line(), ""))); visible > 29 {
		t.Errorf("spinner wraps the line: %d", visible)
	}
}

// fdBuffer is a buffer with a file descriptor (not a terminal).
type fdBuffer struct{ bytes.Buffer }

func (*fdBuffer) Fd() uintptr { return 1 << 20 }

func TestTerminalWriterInterleaving(t *testing.T) {
	var out bytes.Buffer
	sync := common.NewSyncWriter(&out)
	term := NewTerminal(sync)
	term.SetBar(func() string { return "BAR" })

	var wg = new(syncWaitGroup)
	for i := range 20 {
		wg.Go(func() {
			for j := range 5 {
				_, _ = fmt.Fprintf(term, "goroutine %02d line %d\n", i, j)
				term.Redraw()
			}
		})
	}
	wg.Wait()
	term.StopBar()

	// Every line written is complete and the bar is always erased before it
	text := strings.ReplaceAll(out.String(), clearLine+"BAR", "")
	text = strings.ReplaceAll(text, clearLine, "")
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) != 100 {
		t.Fatalf("got %d lines, want 100", len(lines))
	}
	lineRegex := regexp.MustCompile(`^goroutine \d{2} line \d$`)
	for _, line := range lines {
		if !lineRegex.MatchString(line) {
			t.Fatalf("broken line: %q", line)
		}
	}
	if !strings.HasSuffix(out.String(), clearLine) {
		t.Error("StopBar must erase the bar")
	}
	if term.Fd() != ^uintptr(0) {
		t.Error("Fd of a writer without descriptor")
	}
	if NewTerminal(&fdBuffer{}).Fd() != 1<<20 {
		t.Error("Fd must delegate to the output")
	}
}

func TestProgressNotInLogFile(t *testing.T) {
	var console, file bytes.Buffer
	term := NewTerminal(&console)
	progress := NewProgress(2, 0, true, nil)
	term.SetBar(progress.Line)
	p := NewPrinter(2, progress,
		Destination{Writer: term, Opts: Options{Color: true}},
		Destination{Writer: &file, Opts: Options{}},
	)
	stop := StartTicker(term, time.Millisecond)
	p.OnResult(sampleResults()["ok"])
	time.Sleep(10 * time.Millisecond)
	stop()
	stop() // idempotent
	term.StopBar()
	p.PrintSummary(Summary{Results: []git.RepoResult{sampleResults()["ok"]}, Total: 2})

	if strings.ContainsAny(file.String(), "\r\x1b") {
		t.Errorf("log file contains control sequences: %q", file.String())
	}
	if !strings.Contains(console.String(), "1/2") {
		t.Error("the progress bar was not drawn in the terminal")
	}
}

func TestTerminalWidthFallback(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "x")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if w := TerminalWidth(f)(); w != 80 {
		t.Errorf("width of a regular file = %d, want 80", w)
	}
}

// syncWaitGroup is sync.WaitGroup (alias to keep the test readable).
type syncWaitGroup = sync.WaitGroup

func TestSmallHelpers(t *testing.T) {
	if s := NewSpinner("/x", 2, false, nil); s.width() != 80 {
		t.Error("default spinner width")
	}
	if p := NewProgress(1, 0, false, nil); p.width() != 80 {
		t.Error("default progress width")
	}
	// More results than the total (never negative)
	s := Summary{Results: []git.RepoResult{{Status: git.StatusOK}, {Status: git.StatusOK}}, Total: 1}
	if _, notStarted := s.Counts(); notStarted != 0 {
		t.Errorf("notStarted = %d", notStarted)
	}
	// Printer with progress bar: events are forwarded
	progress := NewProgress(1, 1, false, func() int { return 200 })
	p := NewPrinter(1, progress)
	p.OnEvent(git.Event{Kind: git.EventStarted, Repo: "a"})
	p.OnEvent(git.Event{Kind: git.EventBackupStarted, Repo: "a"})
	if !strings.Contains(progress.Line(), "running 1: a") || !strings.Contains(progress.Line(), "backup 1/1") {
		t.Errorf("events not forwarded: %q", progress.Line())
	}
}
