package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/output"
	"github.com/aeciopires/updateGit/internal/update"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlags restores the default value of every flag (cobra keeps the values
// between executions in the same process).
func resetFlags(t *testing.T) {
	t.Helper()
	reset := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				var values []string
				if def := strings.Trim(f.DefValue, "[]"); def != "" {
					values = strings.Split(def, ",")
				}
				_ = sv.Replace(values)
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
	reset(rootCmd.PersistentFlags())
	reset(rootCmd.Flags())
	for _, c := range rootCmd.Commands() {
		reset(c.Flags())
	}
}

// runCLI runs Execute with args in the current process and returns the exit code
// and the output written to the terminal.
func runCLI(t *testing.T, stdin string, args ...string) (int, string) {
	t.Helper()
	resetFlags(t)

	var out bytes.Buffer
	originalTerminal, originalExit, originalProperties := terminal, exitFunc, config.Properties
	terminal = output.NewTerminal(common.NewSyncWriter(&out))
	code := 0
	exitFunc = func(c int) { code = c }
	t.Cleanup(func() {
		terminal, exitFunc, config.Properties = originalTerminal, originalExit, originalProperties
		rootCmd.SetArgs(nil)
		rootCmd.SetIn(nil)
		_ = common.InitLogger(common.LogOptions{Level: "info"})
	})

	rootCmd.SetIn(strings.NewReader(stdin))
	rootCmd.SetArgs(args)
	Execute()
	return code, out.String()
}

func TestExecuteVersion(t *testing.T) {
	isolate(t)
	if code, _ := runCLI(t, "", "-v"); code != 0 {
		t.Errorf("-v: exit code %d", code)
	}
	if code, _ := runCLI(t, "", "-V"); code != 0 {
		t.Errorf("-V: exit code %d", code)
	}
	if code, _ := runCLI(t, ""); code != 0 {
		t.Errorf("help: exit code %d", code)
	}
}

func TestExecutePull(t *testing.T) {
	reposDir, _ := setupRepos(t, 2)
	dir := isolate(t)
	logDir := filepath.Join(dir, "logs")

	code, out := runCLI(t, "", "pull", "-G", reposDir, "--progress", "never", "--color", "never", "-D", "-O", "--log-file-dir", logDir)
	if code != 0 {
		t.Fatalf("exit code %d\n%s", code, out)
	}
	for _, want := range []string{"Scan finished", "[1/2]", "Summary", "Log saved to", "Final Configuration Loaded"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q", want)
		}
	}
	files, _ := filepath.Glob(filepath.Join(logDir, "updateGit_*.log"))
	if len(files) != 1 {
		t.Fatalf("log files = %v", files)
	}
	content, _ := os.ReadFile(files[0])
	if !strings.Contains(string(content), "exit code 0") {
		t.Errorf("log footer not found:\n%s", content)
	}
}

func TestExecutePullFailureExitCode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	isolate(t)
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	// The log file cannot be created: error before the update => exit code 1
	code, out := runCLI(t, "", "pull", "-G", t.TempDir(), "-O", "--log-file-dir", filepath.Join(locked, "sub"))
	if code != 1 || !strings.Contains(out, "cannot create log directory") {
		t.Errorf("exit code %d\n%s", code, out)
	}
}

func TestExecuteUnknownFlag(t *testing.T) {
	isolate(t)
	if code, _ := runCLI(t, "", "pull", "--no-such-flag"); code != 1 {
		t.Errorf("unknown flag: exit code %d, want 1", code)
	}
}

// fakeUpdate replaces the functions used by the update command.
func fakeUpdate(t *testing.T, tag string, latestErr, applyErr error) *int {
	t.Helper()
	applied := 0
	originalLatest, originalApply, originalExe := latestRelease, applyUpdate, executablePath
	latestRelease = func(context.Context, string) (*update.GitHubRelease, error) {
		if latestErr != nil {
			return nil, latestErr
		}
		return &update.GitHubRelease{TagName: tag}, nil
	}
	applyUpdate = func(context.Context, *update.GitHubRelease, string) (string, error) {
		applied++
		return "/tmp/updateGit.old", applyErr
	}
	executablePath = func() (string, error) { return "/tmp/updateGit", nil }
	t.Cleanup(func() { latestRelease, applyUpdate, executablePath = originalLatest, originalApply, originalExe })
	return &applied
}

func TestUpdateCommand(t *testing.T) {
	isolate(t)

	t.Run("already the latest version", func(t *testing.T) {
		applied := fakeUpdate(t, config.CLIVersion, nil, nil)
		if code, out := runCLI(t, "", "update"); code != 0 || *applied != 0 || !strings.Contains(out, "already on the latest version") {
			t.Errorf("exit code %d, applied %d\n%s", code, *applied, out)
		}
	})
	t.Run("older release is never installed", func(t *testing.T) {
		applied := fakeUpdate(t, "0.0.1", nil, nil)
		if code, _ := runCLI(t, "", "update"); code != 0 || *applied != 0 {
			t.Errorf("exit code %d, applied %d", code, *applied)
		}
	})
	t.Run("newer version confirmed", func(t *testing.T) {
		applied := fakeUpdate(t, "99.0.0", nil, nil)
		if code, out := runCLI(t, "y\n", "update"); code != 0 || *applied != 1 || !strings.Contains(out, "Update successful") {
			t.Errorf("exit code %d, applied %d\n%s", code, *applied, out)
		}
	})
	t.Run("newer version canceled", func(t *testing.T) {
		applied := fakeUpdate(t, "99.0.0", nil, nil)
		if code, out := runCLI(t, "n\n", "update"); code != 1 || *applied != 0 || !strings.Contains(out, "Update canceled") {
			t.Errorf("exit code %d, applied %d\n%s", code, *applied, out)
		}
	})
	t.Run("errors", func(t *testing.T) {
		fakeUpdate(t, "", errors.New("network down"), nil)
		if code, out := runCLI(t, "", "update"); code != 1 || !strings.Contains(out, "network down") {
			t.Errorf("exit code %d\n%s", code, out)
		}
		fakeUpdate(t, "not-semver", nil, nil)
		if code, out := runCLI(t, "", "update"); code != 1 || !strings.Contains(out, "cannot compare the versions") {
			t.Errorf("exit code %d\n%s", code, out)
		}
		fakeUpdate(t, "99.0.0", nil, errors.New("checksum mismatch"))
		if code, out := runCLI(t, "y\n", "update"); code != 1 || !strings.Contains(out, "checksum mismatch") {
			t.Errorf("exit code %d\n%s", code, out)
		}
		fakeUpdate(t, "99.0.0", nil, nil)
		executablePath = func() (string, error) { return "", errors.New("no executable") }
		if code, _ := runCLI(t, "y\n", "update"); code != 1 {
			t.Errorf("exit code %d", code)
		}
	})
}

func TestRunSelfUpdateWithoutContext(t *testing.T) {
	isolate(t)
	fakeUpdate(t, config.CLIVersion, nil, nil)
	useConfig(t, nil)
	if err := runSelfUpdate(&cobra.Command{}, nil); err != nil {
		t.Errorf("runSelfUpdate = %v", err)
	}
}
