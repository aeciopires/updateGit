package output

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/git"
)

const barCells = 24

var (
	spinnerUnicode = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerASCII   = []string{"|", "/", "-", "\\"}
)

// segment is a piece of the progress line with its color.
type segment struct {
	text  string
	color string
}

// joinSegments joins the segments, truncating the visible text to width-1
// characters so the line never wraps.
func joinSegments(segments []segment, width int, color bool) string {
	remaining := width - 1
	if remaining < 10 {
		remaining = 10
	}
	var b strings.Builder
	for _, seg := range segments {
		if remaining <= 0 {
			break
		}
		runes := []rune(seg.text)
		if len(runes) > remaining {
			runes = append(runes[:remaining-1], '…')
		}
		b.WriteString(paint(string(runes), seg.color, color))
		remaining -= len(runes)
	}
	return b.String()
}

// Progress is the state of the live progress bar of the update stage. It is
// safe for concurrent use (events come from git.Run, Line from the terminal ticker).
type Progress struct {
	mu            sync.Mutex
	total         int
	done          int
	counts        map[git.Status]int
	running       []string
	backupRunning int
	backupLimit   int
	start         time.Time
	frame         int
	color         bool
	width         func() int
	now           func() time.Time
}

// NewProgress returns the progress of total repositories. backupLimit is the
// limit of simultaneous backups (0 = backup disabled).
func NewProgress(total, backupLimit int, color bool, width func() int) *Progress {
	if width == nil {
		width = func() int { return 80 }
	}
	return &Progress{
		total:       total,
		counts:      make(map[git.Status]int),
		backupLimit: backupLimit,
		start:       time.Now(),
		color:       color,
		width:       width,
		now:         time.Now,
	}
}

// OnEvent updates the running repositories and backups.
func (p *Progress) OnEvent(e git.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch e.Kind {
	case git.EventStarted:
		p.running = append(p.running, e.Repo)
	case git.EventBackupStarted:
		p.backupRunning++
	case git.EventBackupFinished:
		if p.backupRunning > 0 {
			p.backupRunning--
		}
	}
}

// OnResult counts a finished repository.
func (p *Progress) OnResult(res git.RepoResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	p.counts[res.Status]++
	for i, name := range p.running {
		if name == res.Repo.Name {
			p.running = append(p.running[:i], p.running[i+1:]...)
			break
		}
	}
}

// Line returns the progress line, e.g.:
//
//	⠼ [████████████░░░░░░░░░░░░] 21/50  42%  ✔ 19 ✘ 1 ⏱ 0 ⏭ 1 │ running 15: a, b, c +12 │ backup 2/4 │ 00:12 · ETA ~00:17
func (p *Progress) Line() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.frame++
	percent := 0
	if p.total > 0 {
		percent = p.done * 100 / p.total
	}
	filled := 0
	if p.total > 0 {
		filled = p.done * barCells / p.total
	}
	elapsed := p.now().Sub(p.start)
	eta := "--:--"
	if p.done > 0 && p.done < p.total {
		eta = "~" + clock(elapsed/time.Duration(p.done)*time.Duration(p.total-p.done))
	} else if p.done >= p.total {
		eta = "00:00"
	}

	failed := p.counts[git.StatusFailed] > 0 || p.counts[git.StatusTimeout] > 0
	barColor := common.ANSIGreen
	if failed {
		barColor = common.ANSIYellow
	}

	var segs []segment
	if p.color {
		segs = append(segs,
			segment{spinnerUnicode[p.frame%len(spinnerUnicode)] + " ", common.ANSICyan},
			segment{"[" + strings.Repeat("█", filled), barColor},
			segment{strings.Repeat("░", barCells-filled) + "]", common.ANSIGray},
			segment{fmt.Sprintf(" %d/%d  %d%%  ", p.done, p.total, percent), ""},
			segment{fmt.Sprintf("✔ %d ", p.counts[git.StatusOK]), common.ANSIGreen},
			segment{fmt.Sprintf("✘ %d ", p.counts[git.StatusFailed]), colorIf(p.counts[git.StatusFailed] > 0, common.ANSIRed)},
			segment{fmt.Sprintf("⏱ %d ", p.counts[git.StatusTimeout]), colorIf(p.counts[git.StatusTimeout] > 0, common.ANSIYellow)},
			segment{fmt.Sprintf("⏭ %d", p.counts[git.StatusSkipped]), colorIf(p.counts[git.StatusSkipped] > 0, common.ANSIYellow)},
		)
	} else {
		segs = append(segs,
			segment{spinnerASCII[p.frame%len(spinnerASCII)] + " ", ""},
			segment{"[" + strings.Repeat("#", filled) + strings.Repeat(".", barCells-filled) + "]", ""},
			segment{fmt.Sprintf(" %d/%d %d%% ok %d fail %d timeout %d skip %d", p.done, p.total, percent,
				p.counts[git.StatusOK], p.counts[git.StatusFailed], p.counts[git.StatusTimeout], p.counts[git.StatusSkipped]), ""},
		)
	}

	sep := " │ "
	if !p.color {
		sep = " | "
	}
	if len(p.running) > 0 {
		segs = append(segs, segment{sep + "running " + runningList(p.running), common.ANSIGray})
	}
	if p.backupLimit > 0 {
		segs = append(segs, segment{fmt.Sprintf("%sbackup %d/%d", sep, p.backupRunning, p.backupLimit), common.ANSIGray})
	}
	dot := " · "
	if !p.color {
		dot = " "
	}
	segs = append(segs, segment{fmt.Sprintf("%s%s%sETA %s", sep, clock(elapsed), dot, eta), common.ANSIGray})

	return joinSegments(segs, p.width(), p.color)
}

// runningList returns "15: a, b, c +12".
func runningList(running []string) string {
	shown := running
	if len(shown) > 3 {
		shown = shown[:3]
	}
	text := fmt.Sprintf("%d: %s", len(running), strings.Join(shown, ", "))
	if len(running) > 3 {
		text += fmt.Sprintf(" +%d", len(running)-3)
	}
	return text
}

// clock formats a duration as mm:ss (or hh:mm:ss).
func clock(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// Spinner is shown during the discovery stage, when the total is not known yet.
type Spinner struct {
	mu      sync.Mutex
	baseDir string
	depth   int
	dirs    int
	repos   int
	frame   int
	color   bool
	width   func() int
}

// NewSpinner returns the discovery spinner.
func NewSpinner(baseDir string, depth int, color bool, width func() int) *Spinner {
	if width == nil {
		width = func() int { return 80 }
	}
	return &Spinner{baseDir: baseDir, depth: depth, color: color, width: width}
}

// Update sets the partial counters (called by git.DiscoverRepositories).
func (s *Spinner) Update(visitedDirs, reposFound int) {
	s.mu.Lock()
	s.dirs, s.repos = visitedDirs, reposFound
	s.mu.Unlock()
}

// Line returns "⠹ Scanning /home/user/git (levels 1-2) · 14 directories · 61 repositories found...".
func (s *Spinner) Line() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frame++
	frame := spinnerASCII[s.frame%len(spinnerASCII)]
	dot := " - "
	if s.color {
		frame = spinnerUnicode[s.frame%len(spinnerUnicode)]
		dot = " · "
	}
	return joinSegments([]segment{
		{frame + " ", common.ANSICyan},
		{fmt.Sprintf("Scanning %s (levels 1-%d)%s%d directories%s%d repositories found...", s.baseDir, s.depth, dot, s.dirs, dot, s.repos), common.ANSIGray},
	}, s.width(), s.color)
}
