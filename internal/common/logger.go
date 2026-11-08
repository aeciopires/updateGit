package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Level is the severity of a log message.
type Level int

// Log levels, from the most to the least verbose.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarning
	LevelError
	LevelFatal
	LevelPanic
)

// Color modes accepted by --color / log.color.
const (
	ColorAuto   = "auto"
	ColorAlways = "always"
	ColorNever  = "never"
)

// ANSI escape codes used by the logger and the output package.
const (
	ANSIReset     = "\033[0m"
	ANSIBold      = "\033[1m"
	ANSIGray      = "\033[90m"
	ANSIRed       = "\033[31m"
	ANSIGreen     = "\033[32m"
	ANSIYellow    = "\033[33m"
	ANSICyan      = "\033[36m"
	ANSIBoldRed   = "\033[1;31m"
	ANSIBoldGreen = "\033[1;32m"
	ANSIBoldYel   = "\033[1;33m"
	ANSIFatal     = "\033[1;37;41m"
)

// String returns the label of the level (fixed width of 5 characters).
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO "
	case LevelWarning:
		return "WARN "
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	case LevelPanic:
		return "PANIC"
	default:
		return "INFO "
	}
}

// labelColor returns the ANSI color of the level label.
func (l Level) labelColor() string {
	switch l {
	case LevelDebug:
		return ANSIGray
	case LevelInfo:
		return ANSICyan
	case LevelWarning:
		return ANSIBoldYel
	case LevelError:
		return ANSIBoldRed
	default:
		return ANSIFatal
	}
}

// messageColor returns the ANSI color of the message ("" = terminal default).
func (l Level) messageColor() string {
	switch l {
	case LevelDebug:
		return ANSIGray
	case LevelWarning:
		return ANSIYellow
	case LevelError, LevelFatal, LevelPanic:
		return ANSIRed
	default:
		return ""
	}
}

// ParseLevel converts a level name (debug, info, warning/warn, error, fatal, panic)
// to Level. Unknown names return LevelInfo and an error.
func ParseLevel(name string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return LevelDebug, nil
	case "info", "":
		return LevelInfo, nil
	case "warning", "warn":
		return LevelWarning, nil
	case "error":
		return LevelError, nil
	case "fatal":
		return LevelFatal, nil
	case "panic":
		return LevelPanic, nil
	default:
		return LevelInfo, fmt.Errorf("invalid log level %q (valid: debug, info, warning, error)", name)
	}
}

// LogOptions configures the logger (see InitLogger).
type LogOptions struct {
	// Level is the minimum level shown in the console (debug, info, warning, error).
	Level string
	// Color is the color mode of the console: auto, always or never.
	Color string
	// Console is where the messages are written (default os.Stdout).
	Console io.Writer
	// ConsoleIsTerminal tells if Console is an interactive terminal (used by Color=auto).
	ConsoleIsTerminal bool
	// File is an optional second destination (log file). It never receives colors.
	File io.Writer
	// FileLevel is the minimum level saved in File (empty = same as Level).
	FileLevel string
}

type logState struct {
	mu           sync.Mutex
	console      io.Writer
	consoleLevel Level
	color        bool
	file         io.Writer
	fileLevel    Level
}

var (
	logMu     sync.RWMutex
	log       *logState
	exitHook  func(code int)
	exitFunc  = os.Exit
	nowFunc   = time.Now
	credRegex = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://)[^/@\s]+@`)
)

// defaultLogState is used when Logger is called before InitLogger.
func defaultLogState() *logState {
	return &logState{
		console:      os.Stdout,
		consoleLevel: LevelInfo,
		color:        ColorEnabled(ColorAuto, IsTerminal(os.Stdout)),
		fileLevel:    LevelInfo,
	}
}

func currentLog() *logState {
	logMu.RLock()
	s := log
	logMu.RUnlock()
	if s != nil {
		return s
	}
	logMu.Lock()
	defer logMu.Unlock()
	if log == nil {
		log = defaultLogState()
	}
	return log
}

// InitLogger configures the logger once with the final configuration
// (flags + environment variables + config file). It is safe to call it again
// (e.g. to add the log file), and safe for concurrent use with Logger.
func InitLogger(opts LogOptions) error {
	consoleLevel, err := ParseLevel(opts.Level)
	if err != nil {
		return err
	}
	fileLevel := consoleLevel
	if opts.FileLevel != "" {
		if fileLevel, err = ParseLevel(opts.FileLevel); err != nil {
			return err
		}
	}
	console := opts.Console
	if console == nil {
		console = os.Stdout
	}
	color := opts.Color
	if color == "" {
		color = ColorAuto
	}

	logMu.Lock()
	log = &logState{
		console:      console,
		consoleLevel: consoleLevel,
		color:        ColorEnabled(color, opts.ConsoleIsTerminal),
		file:         opts.File,
		fileLevel:    fileLevel,
	}
	logMu.Unlock()
	return nil
}

// ColorEnabled tells if the colors must be used: "always" => true, "never" => false,
// "auto" => only when the output is a terminal and NO_COLOR is not defined (https://no-color.org).
func ColorEnabled(mode string, isTerminal bool) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
			return false
		}
		return isTerminal
	}
}

// IsTerminal tells if w is an interactive terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ConsoleLevel returns the minimum level shown in the console.
func ConsoleLevel() Level { return currentLog().consoleLevel }

// FileLevel returns the minimum level saved in the log file.
func FileLevel() Level { return currentLog().fileLevel }

// ColorOutput tells if the console uses colors.
func ColorOutput() bool { return currentLog().color }

// SetExitHook registers a function called before the process exits because of a
// fatal message (e.g. to write the footer and close the log file).
func SetExitHook(hook func(code int)) {
	logMu.Lock()
	exitHook = hook
	logMu.Unlock()
}

// MaskCredentials hides credentials embedded in URLs (https://user:token@host => https://***@host).
func MaskCredentials(s string) string {
	return credRegex.ReplaceAllString(s, "${1}***@")
}

// Logger prints a message according to the level. The message works like fmt.Sprintf.
// Levels: debug, info, warning (or warn), error, fatal (exits with code 1) and panic.
//
// Examples:
//
//	common.Logger("info", "Hello world!")
//	common.Logger("warning", "Repository %s has no upstream", name)
//	common.Logger("fatal", "Directory does not exist: %s", dir)
//
// Console output (colors by level when enabled):
//
//	10:15:02 INFO  Hello world!
//	10:15:02 WARN  Repository scratch has no upstream
//	10:15:02 FATAL Directory does not exist: /tmp/x
//
// With the debug level the timestamp is complete and errors show the file and line.
func Logger(level string, message string, args ...any) {
	lvl, err := ParseLevel(level)
	if err != nil {
		lvl = LevelInfo
	}

	s := currentLog()
	formatted := MaskCredentials(fmt.Sprintf(message, args...))
	formatted = strings.TrimRight(formatted, "\n")

	caller := ""
	if lvl >= LevelError && (s.consoleLevel == LevelDebug || s.fileLevel == LevelDebug) {
		if _, file, line, ok := runtime.Caller(1); ok {
			caller = fmt.Sprintf(" (%s:%d)", shortPath(file), line)
		}
	}

	now := nowFunc()
	s.mu.Lock()
	if lvl >= s.consoleLevel || lvl >= LevelFatal {
		withCaller := ""
		if s.consoleLevel == LevelDebug {
			withCaller = caller
		}
		_, _ = io.WriteString(s.console, formatLine(now, lvl, formatted+withCaller, s.color, s.consoleLevel == LevelDebug))
	}
	if s.file != nil && (lvl >= s.fileLevel || lvl >= LevelFatal) {
		withCaller := ""
		if s.fileLevel == LevelDebug {
			withCaller = caller
		}
		_, _ = io.WriteString(s.file, formatLine(now, lvl, formatted+withCaller, false, true))
	}
	if lvl >= LevelFatal && s.consoleLevel == LevelDebug {
		_, _ = s.console.Write(debug.Stack())
	}
	s.mu.Unlock()

	switch lvl {
	case LevelFatal:
		logMu.RLock()
		hook := exitHook
		logMu.RUnlock()
		if hook != nil {
			hook(1)
		}
		exitFunc(1)
	case LevelPanic:
		panic(formatted)
	}
}

// formatLine builds one log line: "<timestamp> <LEVEL> <message>\n".
func formatLine(now time.Time, lvl Level, msg string, color, fullTimestamp bool) string {
	ts := now.Format("15:04:05")
	if fullTimestamp {
		ts = now.Format("2006-01-02 15:04:05")
	}
	if !color {
		return fmt.Sprintf("%s %s %s\n", ts, lvl.String(), msg)
	}
	msgColor := lvl.messageColor()
	if msgColor != "" {
		msg = msgColor + msg + ANSIReset
	}
	return fmt.Sprintf("%s%s%s %s%s%s %s\n", ANSIGray, ts, ANSIReset, lvl.labelColor(), lvl.String(), ANSIReset, msg)
}

// shortPath returns the last two elements of a file path (e.g. git/git.go).
func shortPath(file string) string {
	dir, name := filepath.Split(file)
	return filepath.Join(filepath.Base(dir), name)
}

// SyncWriter serializes the writes of several goroutines to the same writer.
type SyncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewSyncWriter returns a writer safe for concurrent use.
func NewSyncWriter(w io.Writer) *SyncWriter { return &SyncWriter{w: w} }

// Write writes p atomically.
func (s *SyncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
