// Package output renders the result of the update (one block per repository and
// the final summary), the live progress bar and manages the terminal shared by the
// logger, the blocks and the progress bar.
package output

import (
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// clearLine moves the cursor to the beginning of the line and erases it.
const clearLine = "\r\033[2K"

// Terminal is the only path to the standard output. Every write erases the progress
// bar (if any), writes the content and draws the bar again, so the logs, the blocks
// and the bar never mix. It is safe for concurrent use.
type Terminal struct {
	mu    sync.Mutex
	out   io.Writer
	bar   func() string
	shown bool
}

// NewTerminal returns a Terminal writing to out.
func NewTerminal(out io.Writer) *Terminal {
	return &Terminal{out: out}
}

// Write writes p (complete lines) keeping the progress bar on the last line.
func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearLocked()
	n, err := t.out.Write(p)
	t.drawLocked()
	return n, err
}

// SetBar starts drawing the line returned by bar on the last line of the terminal.
func (t *Terminal) SetBar(bar func() string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearLocked()
	t.bar = bar
	t.drawLocked()
}

// StopBar erases the progress bar and stops drawing it.
func (t *Terminal) StopBar() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearLocked()
	t.bar = nil
}

// Redraw draws the progress bar again (called periodically to animate it).
func (t *Terminal) Redraw() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.drawLocked()
}

// Fd returns the file descriptor of the output (used to detect a terminal).
func (t *Terminal) Fd() uintptr {
	if f, ok := t.out.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

func (t *Terminal) clearLocked() {
	if t.shown {
		_, _ = io.WriteString(t.out, clearLine)
		t.shown = false
	}
}

func (t *Terminal) drawLocked() {
	if t.bar == nil {
		return
	}
	_, _ = io.WriteString(t.out, clearLine+t.bar())
	t.shown = true
}

// StartTicker redraws the terminal every interval until the returned function is called.
func StartTicker(t *Terminal, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				t.Redraw()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}

// TerminalWidth returns the width of the terminal of f (80 when unknown).
func TerminalWidth(f *os.File) func() int {
	return func() int {
		width, _, err := term.GetSize(int(f.Fd()))
		if err != nil || width <= 0 {
			return 80
		}
		return width
	}
}

// ProgressEnabled tells if the progress bar must be shown: "always" => true,
// "never" => false, "auto" => only in a terminal and with more than one worker
// (in sequential mode git may ask for passwords in the same terminal).
func ProgressEnabled(mode string, isTerminal bool, workers int) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		return isTerminal && workers > 1
	}
}

// SpinnerEnabled tells if the discovery spinner must be shown.
func SpinnerEnabled(mode string, isTerminal bool) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default:
		return isTerminal
	}
}
