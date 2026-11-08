package common

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// setupLogger configures the logger to write to buffers with a fixed clock.
func setupLogger(t *testing.T, opts LogOptions) (console, file *bytes.Buffer) {
	t.Helper()
	console = &bytes.Buffer{}
	opts.Console = NewSyncWriter(console)
	if opts.File != nil {
		file = &bytes.Buffer{}
		opts.File = NewSyncWriter(file)
	}
	if err := InitLogger(opts); err != nil {
		t.Fatal(err)
	}
	originalNow, originalExit := nowFunc, exitFunc
	nowFunc = func() time.Time { return time.Date(2026, 11, 8, 10, 15, 2, 0, time.UTC) }
	t.Cleanup(func() {
		nowFunc, exitFunc = originalNow, originalExit
		logMu.Lock()
		log = nil
		exitHook = nil
		logMu.Unlock()
	})
	return console, file
}

func TestLoggerLevels(t *testing.T) {
	levels := []string{"debug", "info", "warning", "error"}
	for i, min := range levels {
		t.Run(min, func(t *testing.T) {
			console, _ := setupLogger(t, LogOptions{Level: min, Color: ColorNever})
			for _, level := range levels {
				Logger(level, "message %s", level)
			}
			out := console.String()
			for j, level := range levels {
				shown := strings.Contains(out, "message "+level)
				if shown != (j >= i) {
					t.Errorf("min=%s: message %s shown=%v\n%s", min, level, shown, out)
				}
			}
		})
	}
}

func TestLoggerFormat(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	Logger("info", "Hello %s", "world")
	Logger("warn", "Careful")
	want := "10:15:02 INFO  Hello world\n10:15:02 WARN  Careful\n"
	if console.String() != want {
		t.Errorf("got %q, want %q", console.String(), want)
	}

	// Debug: complete timestamp and file:line in errors
	console, _ = setupLogger(t, LogOptions{Level: "debug", Color: ColorNever})
	Logger("error", "Broken")
	if !strings.HasPrefix(console.String(), "2026-11-08 10:15:02 ERROR Broken (common/logger_test.go:") {
		t.Errorf("debug error line = %q", console.String())
	}
}

func TestLoggerNoStackTraceOutsideDebug(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	Logger("error", "Something failed: %v", fmt.Errorf("boom"))
	out := console.String()
	if strings.Contains(out, "stack") || strings.Contains(out, ".go:") || out != "10:15:02 ERROR Something failed: boom\n" {
		t.Errorf("error line outside debug = %q", out)
	}
}

func TestLoggerColorModes(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	Logger("error", "x")
	if ansiRegex.MatchString(console.String()) {
		t.Error("never: ANSI codes found")
	}

	console, _ = setupLogger(t, LogOptions{Level: "info", Color: ColorAlways})
	Logger("error", "x")
	if !ansiRegex.MatchString(console.String()) {
		t.Error("always: ANSI codes not found")
	}

	t.Setenv("NO_COLOR", "")
	_ = os.Unsetenv("NO_COLOR") // restored by t.Setenv
	if !ColorEnabled(ColorAuto, true) || ColorEnabled(ColorAuto, false) {
		t.Error("auto: colors only in a terminal")
	}
	t.Setenv("NO_COLOR", "1")
	if ColorEnabled(ColorAuto, true) {
		t.Error("auto: NO_COLOR must disable the colors")
	}
	if !ColorEnabled(ColorAlways, false) || ColorEnabled(ColorNever, true) {
		t.Error("always/never must ignore the terminal")
	}
}

func TestLoggerLevelColors(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "debug", Color: ColorAlways})
	tests := map[string]string{
		"debug":   ANSIGray + "DEBUG" + ANSIReset,
		"info":    ANSICyan + "INFO " + ANSIReset,
		"warning": ANSIBoldYel + "WARN " + ANSIReset,
		"error":   ANSIBoldRed + "ERROR" + ANSIReset,
	}
	for level, label := range tests {
		console.Reset()
		Logger(level, "x")
		if !strings.Contains(console.String(), label) {
			t.Errorf("%s: label %q not found in %q", level, label, console.String())
		}
		// Fixed width: the message always starts at the same column
		plain := ansiRegex.ReplaceAllString(console.String(), "")
		if !regexp.MustCompile(`^2026-11-08 10:15:02 (DEBUG|INFO |WARN |ERROR) x( \(common/logger_test\.go:\d+\))?\n$`).MatchString(plain) {
			t.Errorf("%s: unexpected line %q", level, plain)
		}
	}
}

func TestLoggerBeforeInit(t *testing.T) {
	logMu.Lock()
	log = nil
	logMu.Unlock()
	t.Cleanup(func() {
		logMu.Lock()
		log = nil
		logMu.Unlock()
	})
	// Must not panic and must use the default state (info)
	Logger("debug", "hidden")
	if ConsoleLevel() != LevelInfo || FileLevel() != LevelInfo {
		t.Errorf("default levels = %v/%v", ConsoleLevel(), FileLevel())
	}
}

func TestLoggerConcurrentUse(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			for j := range 10 {
				Logger("info", "goroutine %03d line %d", i, j)
			}
		})
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(console.String()), "\n")
	if len(lines) != 1000 {
		t.Fatalf("got %d lines, want 1000", len(lines))
	}
	lineRegex := regexp.MustCompile(`^10:15:02 INFO  goroutine \d{3} line \d$`)
	for _, line := range lines {
		if !lineRegex.MatchString(line) {
			t.Fatalf("broken line: %q", line)
		}
	}
}

func TestLoggerFileIndependentLevel(t *testing.T) {
	console, file := setupLogger(t, LogOptions{Level: "warning", Color: ColorAlways, File: &bytes.Buffer{}, FileLevel: "debug"})
	Logger("debug", "debug message")
	Logger("info", "info message")
	Logger("warning", "warning message")

	if strings.Contains(console.String(), "info message") || !strings.Contains(console.String(), "warning message") {
		t.Errorf("console = %q", console.String())
	}
	out := file.String()
	for _, msg := range []string{"debug message", "info message", "warning message"} {
		if !strings.Contains(out, msg) {
			t.Errorf("file does not contain %q: %q", msg, out)
		}
	}
	if ansiRegex.MatchString(out) {
		t.Error("the log file must not contain ANSI codes")
	}
	if !strings.Contains(out, "2026-11-08 10:15:02 INFO  info message") {
		t.Errorf("file must use the complete timestamp: %q", out)
	}
}

func TestLoggerFatalCallsHookAndExits(t *testing.T) {
	console, _ := setupLogger(t, LogOptions{Level: "error", Color: ColorNever})
	var hookCode, exitCode int
	SetExitHook(func(code int) { hookCode = code })
	exitFunc = func(code int) { exitCode = code }
	Logger("fatal", "stop")
	if hookCode != 1 || exitCode != 1 || !strings.Contains(console.String(), "FATAL stop") {
		t.Errorf("hook=%d exit=%d console=%q", hookCode, exitCode, console.String())
	}
}

func TestLoggerPanic(t *testing.T) {
	setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	defer func() {
		if r := recover(); r != "boom" {
			t.Errorf("recover() = %v", r)
		}
	}()
	Logger("panic", "boom")
}

func TestParseLevel(t *testing.T) {
	tests := map[string]Level{"debug": LevelDebug, "INFO": LevelInfo, "": LevelInfo, "warn": LevelWarning,
		"warning": LevelWarning, "error": LevelError, "fatal": LevelFatal, "panic": LevelPanic}
	for name, want := range tests {
		if got, err := ParseLevel(name); err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", name, got, err)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel(verbose) should fail")
	}
	if err := InitLogger(LogOptions{Level: "verbose"}); err == nil {
		t.Error("InitLogger with an invalid level should fail")
	}
	if err := InitLogger(LogOptions{Level: "info", FileLevel: "verbose"}); err == nil {
		t.Error("InitLogger with an invalid file level should fail")
	}
}

func TestMaskCredentials(t *testing.T) {
	tests := map[string]string{
		"https://user:token@github.com/org/repo.git": "https://***@github.com/org/repo.git",
		"https://token@github.com/x":                 "https://***@github.com/x",
		"ssh://git@host:22/x.git":                    "ssh://***@host:22/x.git",
		"git@github.com:org/repo.git":                "git@github.com:org/repo.git",
		"no url here":                                "no url here",
	}
	for in, want := range tests {
		if got := MaskCredentials(in); got != want {
			t.Errorf("MaskCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if IsTerminal(&bytes.Buffer{}) {
		t.Error("a buffer is not a terminal")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
}

func TestLogFileName(t *testing.T) {
	start := time.Date(2026, 11, 8, 10, 15, 2, 0, time.UTC)
	if got := LogFileName(start); got != "updateGit_2026-11-08_10-15-02.log" {
		t.Errorf("LogFileName = %q", got)
	}
}

func TestLogFilePermissionsAndDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "logs")
	start := time.Date(2026, 11, 8, 10, 15, 2, 0, time.UTC)
	f, path, err := OpenLogFile(dir, start)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if path != filepath.Join(dir, "updateGit_2026-11-08_10-15-02.log") {
		t.Errorf("path = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("permission = %v, %v; want 0600", info.Mode().Perm(), err)
	}
}

func TestLogFileDirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenLogFile(locked, time.Now()); err == nil || !strings.Contains(err.Error(), "cannot write log file") {
		t.Errorf("err = %v, want 'cannot write log file'", err)
	}
	if _, _, err := OpenLogFile(filepath.Join(locked, "sub"), time.Now()); err == nil || !strings.Contains(err.Error(), "cannot create log directory") {
		t.Errorf("err = %v, want 'cannot create log directory'", err)
	}
}

func TestLevelString(t *testing.T) {
	for l := LevelDebug; l <= LevelPanic; l++ {
		if len(l.String()) != 5 {
			t.Errorf("label %q is not 5 characters", l.String())
		}
	}
	if Level(99).String() != "INFO " {
		t.Error("unknown level should be INFO")
	}
}

func TestColorOutputAndLabels(t *testing.T) {
	setupLogger(t, LogOptions{Level: "info", Color: ColorAlways})
	if !ColorOutput() {
		t.Error("ColorOutput must be true with always")
	}
	setupLogger(t, LogOptions{Level: "info", Color: ColorNever})
	if ColorOutput() {
		t.Error("ColorOutput must be false with never")
	}
	if LevelFatal.labelColor() != ANSIFatal || LevelPanic.labelColor() != ANSIFatal || LevelInfo.messageColor() != "" {
		t.Error("colors of fatal/panic/info")
	}
	// Defaults of InitLogger: console os.Stdout and color auto
	if err := InitLogger(LogOptions{Level: "info"}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLogFileDefaultDir(t *testing.T) {
	f, path, err := OpenLogFile("", time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC))
	if err != nil {
		t.Skipf("/tmp is not writable: %v", err)
	}
	defer os.Remove(path)
	defer f.Close()
	if filepath.Dir(path) != "/tmp" {
		t.Errorf("default directory = %s, want /tmp", filepath.Dir(path))
	}
}
