package getinfo

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/aeciopires/updateGit/internal/config"
)

// captureStdout returns what fn prints to the standard output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestLongVersionShowsBuildInfo(t *testing.T) {
	commit, date := config.CLICommit, config.CLIBuildDate
	t.Cleanup(func() { config.CLICommit, config.CLIBuildDate = commit, date })

	out := captureStdout(t, PrintLongVersion)
	for _, want := range []string{"Version: " + config.CLIVersion, "Commit: unknown", "Build date: unknown"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q does not contain %q", out, want)
		}
	}

	config.CLICommit, config.CLIBuildDate = "33d34d6", "2026-11-08T10:00:00Z"
	out = captureStdout(t, PrintLongVersion)
	if !strings.Contains(out, "Commit: 33d34d6") || !strings.Contains(out, "Build date: 2026-11-08T10:00:00Z") {
		t.Errorf("output %q", out)
	}
}

func TestShortVersionAndSystem(t *testing.T) {
	if out := captureStdout(t, PrintShortVersion); out != config.CLIVersion+"\n" {
		t.Errorf("PrintShortVersion = %q", out)
	}
	if out := captureStdout(t, ShowSystemArch); !strings.Contains(out, runtime.GOARCH) {
		t.Errorf("ShowSystemArch = %q", out)
	}
	if out := captureStdout(t, ShowOperatingSystem); !strings.Contains(out, runtime.GOOS) {
		t.Errorf("ShowOperatingSystem = %q", out)
	}
	CheckOperatingSystem() // linux/darwin: must not exit
}
