package output

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/git"
)

// DefaultMaxBranches is the number of "other branches" shown before "+N more".
const DefaultMaxBranches = 8

// headerWidth is the width of "<symbol> [n/N] <name>" before the duration.
const headerWidth = 60

// Options configures the rendering.
type Options struct {
	// Color enables ANSI colors and Unicode symbols. Without it the output is plain ASCII.
	Color bool
	// Debug shows all branches and the complete "git pull" output.
	Debug bool
	// MaxBranches is the number of other branches shown (default DefaultMaxBranches).
	MaxBranches int
}

type style struct {
	symbol string
	label  string
	color  string
}

func statusStyle(status git.Status) style {
	switch status {
	case git.StatusOK:
		return style{"✔", "[OK]", common.ANSIGreen}
	case git.StatusFailed:
		return style{"✘", "[FAIL]", common.ANSIRed}
	case git.StatusTimeout:
		return style{"⏱", "[TIMEOUT]", common.ANSIYellow}
	case git.StatusSkipped:
		return style{"⏭", "[SKIP]", common.ANSIYellow}
	default:
		return style{"⊘", "[CANCEL]", common.ANSIYellow}
	}
}

func paint(text, color string, enabled bool) string {
	if !enabled || color == "" || text == "" {
		return text
	}
	return color + text + common.ANSIReset
}

// FormatDuration formats a duration as "0.8s", "41.7s" or "5m0s".
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// RenderRepoResult renders the block of one repository. index is the completion
// order and total the number of repositories to update.
//
// With colors:
//
//	✔ [1/50] updateGit                                            0.8s
//	   ● main → origin/main · up to date
//	   ○ develop, feature/parallel-pull (ahead 1)
//	   ↓ Already up to date.
//
// Without colors:
//
//	[OK]      [1/50] updateGit (0.8s)
//	          * main -> origin/main (up to date)
//	          - others: develop, feature/parallel-pull (ahead 1)
//	          > Already up to date.
func RenderRepoResult(res git.RepoResult, index, total int, opts Options) string {
	if opts.MaxBranches <= 0 {
		opts.MaxBranches = DefaultMaxBranches
	}
	st := statusStyle(res.Status)
	var b strings.Builder

	counter := fmt.Sprintf("[%d/%d]", index, total)
	duration := FormatDuration(res.Duration)
	indent := "          "
	if opts.Color {
		indent = "   "
		head := fmt.Sprintf("%s %s %s", st.symbol, counter, res.Repo.Name)
		pad := headerWidth - len([]rune(head))
		if pad < 2 {
			pad = 2
		}
		fmt.Fprintf(&b, "%s %s %s%s%s\n",
			paint(st.symbol, st.color, true), counter, paint(res.Repo.Name, common.ANSIBold+st.color, true),
			strings.Repeat(" ", pad), paint(duration, common.ANSIGray, true))
	} else {
		fmt.Fprintf(&b, "%-9s %s %s (%s)\n", st.label, counter, res.Repo.Name, duration)
	}

	writeLine := func(symbol, plainSymbol, text string) {
		if opts.Color {
			fmt.Fprintf(&b, "%s%s %s\n", indent, symbol, text)
		} else {
			fmt.Fprintf(&b, "%s%s %s\n", indent, plainSymbol, text)
		}
	}

	if line := renderCurrentBranch(res.Branches, opts.Color); line != "" {
		writeLine(paint("●", common.ANSIBoldGreen, true), "*", line)
	}
	if line := renderOtherBranches(res.Branches.Others, opts); line != "" {
		if opts.Color {
			writeLine(paint("○", common.ANSIGray, true), "", line)
		} else {
			writeLine("", "-", "others: "+line)
		}
	}
	if res.BackupPath != "" {
		writeLine(paint("▣", common.ANSIGray, true), "#", paint("Backup: "+res.BackupPath, common.ANSIGray, opts.Color))
	}
	if res.BackupErr != nil {
		writeLine(paint("⚠", common.ANSIYellow, true), "!", paint("Backup failed: "+res.BackupErr.Error(), common.ANSIYellow, opts.Color))
	}
	if res.PullSummary != "" {
		color := common.ANSIGray
		switch res.Status {
		case git.StatusFailed:
			color = common.ANSIRed
		case git.StatusTimeout, git.StatusCanceled:
			color = common.ANSIYellow
		}
		writeLine(paint("↓", common.ANSIGray, true), ">", paint(res.PullSummary, color, opts.Color))
	}
	if res.Hint != "" {
		writeLine(paint("⚠", common.ANSIYellow, true), "!", paint(res.Hint, common.ANSIYellow, opts.Color))
	}
	if res.Status == git.StatusFailed && res.Output == "" && res.Err != nil {
		writeLine(paint("⚠", common.ANSIRed, true), "!", paint(res.Err.Error(), common.ANSIRed, opts.Color))
	}
	showOutput := strings.TrimSpace(res.Output) != "" &&
		(opts.Debug || res.Status == git.StatusFailed)
	if showOutput && strings.TrimSpace(res.Output) != res.PullSummary {
		writeLine(paint("┆", common.ANSIGray, true), "|", paint("git pull output:", common.ANSIGray, opts.Color))
		for _, line := range strings.Split(strings.TrimRight(res.Output, "\n"), "\n") {
			writeLine(paint("┆", common.ANSIGray, true), "|", "  "+paint(strings.TrimRight(line, "\r"), common.ANSIGray, opts.Color))
		}
	}
	if opts.Debug && res.BackupWait > 0 {
		writeLine(paint("┆", common.ANSIGray, true), "|", paint("Waited "+FormatDuration(res.BackupWait)+" for a backup slot", common.ANSIGray, opts.Color))
	}
	b.WriteString("\n")
	return b.String()
}

// renderCurrentBranch renders "main → origin/main · up to date" (or the detached HEAD).
func renderCurrentBranch(branches git.RepoBranches, color bool) string {
	if branches.Current == nil {
		if branches.Detached == "" {
			return ""
		}
		return paint(fmt.Sprintf("(detached HEAD at %s)", branches.Detached), common.ANSIYellow, color)
	}
	current := branches.Current
	state := current.State()
	stateColor := common.ANSIYellow
	if state == "up to date" {
		stateColor = common.ANSIGreen
	}
	if !color {
		if current.Upstream == "" {
			return fmt.Sprintf("%s (%s)", current.Name, state)
		}
		return fmt.Sprintf("%s -> %s (%s)", current.Name, current.Upstream, state)
	}
	name := paint(current.Name, common.ANSIBoldGreen, true)
	if current.Upstream == "" {
		return fmt.Sprintf("%s · %s", name, paint(state, stateColor, true))
	}
	return fmt.Sprintf("%s %s · %s", name, paint("→ "+current.Upstream, common.ANSIGray, true), paint(state, stateColor, true))
}

// renderOtherBranches renders the other local branches; branches with commits not
// pushed are highlighted with "(ahead N)".
func renderOtherBranches(others []git.BranchInfo, opts Options) string {
	if len(others) == 0 {
		return ""
	}
	limit := len(others)
	if !opts.Debug && limit > opts.MaxBranches {
		limit = opts.MaxBranches
	}
	parts := make([]string, 0, limit+1)
	for _, branch := range others[:limit] {
		if branch.Ahead > 0 {
			parts = append(parts, paint(fmt.Sprintf("%s (ahead %d)", branch.Name, branch.Ahead), common.ANSIYellow, opts.Color))
		} else {
			parts = append(parts, branch.Name)
		}
	}
	if limit < len(others) {
		parts = append(parts, fmt.Sprintf("+%d more", len(others)-limit))
	}
	return strings.Join(parts, ", ")
}

// Summary is the data of the final summary.
type Summary struct {
	// Results of the repositories processed
	Results []git.RepoResult
	// Total is the number of repositories to update (after the filter)
	Total int
	// Filtered are the repositories skipped by --skip-repos
	Filtered []string
	// Duration of the whole execution
	Duration time.Duration
}

// Counts returns the number of results by status and the repositories not started
// (canceled before starting).
func (s Summary) Counts() (counts map[git.Status]int, notStarted int) {
	counts = make(map[git.Status]int)
	for _, r := range s.Results {
		counts[r.Status]++
	}
	notStarted = s.Total - len(s.Results)
	if notStarted < 0 {
		notStarted = 0
	}
	return counts, notStarted
}

// HasFailures tells if a repository failed or timed out.
func (s Summary) HasFailures() bool {
	counts, _ := s.Counts()
	return counts[git.StatusFailed] > 0 || counts[git.StatusTimeout] > 0
}

// RenderSummary renders the final summary.
func RenderSummary(s Summary, opts Options) string {
	counts, notStarted := s.Counts()
	canceled := counts[git.StatusCanceled] + notStarted
	var b strings.Builder

	results := append([]git.RepoResult(nil), s.Results...)
	sort.Slice(results, func(i, j int) bool { return results[i].Repo.Name < results[j].Repo.Name })

	var failed []git.RepoResult
	skipped := map[string][]string{}
	var skipReasons []string
	var canceledNames []string
	for _, r := range results {
		switch r.Status {
		case git.StatusFailed, git.StatusTimeout:
			failed = append(failed, r)
		case git.StatusSkipped:
			if _, ok := skipped[r.Reason]; !ok {
				skipReasons = append(skipReasons, r.Reason)
			}
			skipped[r.Reason] = append(skipped[r.Reason], r.Repo.Name)
		case git.StatusCanceled:
			canceledNames = append(canceledNames, r.Repo.Name)
		}
	}
	sort.Strings(skipReasons)
	filtered := append([]string(nil), s.Filtered...)
	sort.Strings(filtered)

	if opts.Color {
		line := strings.Repeat("═", 28)
		fmt.Fprintf(&b, "%s %s %s\n", line, paint("Summary", common.ANSIBold, true), line)
		fmt.Fprintf(&b, "  Total %d   %s   %s   %s   %s",
			s.Total,
			paint(fmt.Sprintf("✔ Updated %d", counts[git.StatusOK]), common.ANSIGreen, true),
			paint(fmt.Sprintf("✘ Failed %d", counts[git.StatusFailed]), colorIf(counts[git.StatusFailed] > 0, common.ANSIRed), true),
			paint(fmt.Sprintf("⏱ Timeout %d", counts[git.StatusTimeout]), colorIf(counts[git.StatusTimeout] > 0, common.ANSIYellow), true),
			paint(fmt.Sprintf("⏭ Skipped %d", counts[git.StatusSkipped]), colorIf(counts[git.StatusSkipped] > 0, common.ANSIYellow), true))
		if canceled > 0 {
			fmt.Fprintf(&b, "   %s", paint(fmt.Sprintf("⊘ Canceled %d", canceled), common.ANSIYellow, true))
		}
		fmt.Fprintf(&b, "   %s\n", paint("⌛ "+FormatDuration(s.Duration), common.ANSIGray, true))
		if len(failed) > 0 {
			b.WriteString("  Failed:\n")
			width := 0
			for _, r := range failed {
				width = max(width, len([]rune(r.Repo.Name)))
			}
			for _, r := range failed {
				st := statusStyle(r.Status)
				fmt.Fprintf(&b, "    %s %-*s   %s\n", paint(st.symbol, st.color, true), width, r.Repo.Name, paint(r.Reason, st.color, true))
			}
		}
		for _, reason := range skipReasons {
			fmt.Fprintf(&b, "  Skipped (%s): %s\n", reason, strings.Join(skipped[reason], ", "))
		}
		if len(canceledNames) > 0 {
			fmt.Fprintf(&b, "  Canceled: %s\n", strings.Join(canceledNames, ", "))
		}
		if notStarted > 0 {
			fmt.Fprintf(&b, "  Not started (canceled): %d\n", notStarted)
		}
		if len(filtered) > 0 {
			fmt.Fprintf(&b, "  Filtered out (--skip-repos): %s\n", strings.Join(filtered, ", "))
		}
		fmt.Fprintf(&b, "%s\n", strings.Repeat("═", 65))
		return b.String()
	}

	line := strings.Repeat("=", 20)
	fmt.Fprintf(&b, "%s Summary %s\n", line, line)
	fmt.Fprintf(&b, "Total %d | Updated %d | Failed %d | Timeout %d | Skipped %d",
		s.Total, counts[git.StatusOK], counts[git.StatusFailed], counts[git.StatusTimeout], counts[git.StatusSkipped])
	if canceled > 0 {
		fmt.Fprintf(&b, " | Canceled %d", canceled)
	}
	fmt.Fprintf(&b, " | %s\n", FormatDuration(s.Duration))
	if len(failed) > 0 {
		parts := make([]string, 0, len(failed))
		for _, r := range failed {
			parts = append(parts, fmt.Sprintf("%s (%s)", r.Repo.Name, r.Reason))
		}
		fmt.Fprintf(&b, "Failed: %s\n", strings.Join(parts, ", "))
	}
	for _, reason := range skipReasons {
		fmt.Fprintf(&b, "Skipped (%s): %s\n", reason, strings.Join(skipped[reason], ", "))
	}
	if len(canceledNames) > 0 {
		fmt.Fprintf(&b, "Canceled: %s\n", strings.Join(canceledNames, ", "))
	}
	if notStarted > 0 {
		fmt.Fprintf(&b, "Not started (canceled): %d\n", notStarted)
	}
	if len(filtered) > 0 {
		fmt.Fprintf(&b, "Filtered out (--skip-repos): %s\n", strings.Join(filtered, ", "))
	}
	fmt.Fprintf(&b, "%s\n", strings.Repeat("=", 49))
	return b.String()
}

func colorIf(cond bool, color string) string {
	if cond {
		return color
	}
	return ""
}

// Destination is where the Printer writes (terminal or log file).
type Destination struct {
	Writer io.Writer
	Opts   Options
	// MinLevel: with common.LevelWarning or higher, the blocks of the repositories
	// updated/skipped are omitted (only failures and the summary are written).
	MinLevel common.Level
}

// Printer implements git.Sink: it writes each result to the terminal and to the
// log file, and updates the progress bar. Its methods are called by git.Run from a
// single goroutine.
type Printer struct {
	dests    []Destination
	total    int
	done     int
	progress *Progress
}

// NewPrinter returns a Printer for total repositories. progress may be nil.
func NewPrinter(total int, progress *Progress, dests ...Destination) *Printer {
	return &Printer{dests: dests, total: total, progress: progress}
}

// OnEvent updates the progress bar.
func (p *Printer) OnEvent(e git.Event) {
	if p.progress != nil {
		p.progress.OnEvent(e)
	}
}

// OnResult writes the block of a repository.
func (p *Printer) OnResult(res git.RepoResult) {
	p.done++
	if p.progress != nil {
		p.progress.OnResult(res)
	}
	important := res.Status == git.StatusFailed || res.Status == git.StatusTimeout || res.Status == git.StatusCanceled
	for _, d := range p.dests {
		if d.Writer == nil || (!important && d.MinLevel >= common.LevelWarning) {
			continue
		}
		_, _ = io.WriteString(d.Writer, RenderRepoResult(res, p.done, p.total, d.Opts))
	}
}

// PrintSummary writes the summary to all destinations (always, independent of the level).
func (p *Printer) PrintSummary(s Summary) {
	for _, d := range p.dests {
		if d.Writer == nil {
			continue
		}
		_, _ = io.WriteString(d.Writer, RenderSummary(s, d.Opts))
	}
}
