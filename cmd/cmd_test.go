package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newFlags returns a fresh set of the global flags parsed from args.
func newFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	c := &cobra.Command{Use: "test"}
	defineFlags(c)
	if err := c.PersistentFlags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return c.PersistentFlags()
}

// isolate runs the test in an empty directory, without CLI_* variables.
func isolate(t *testing.T) string {
	t.Helper()
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CLI_") {
			name := strings.SplitN(kv, "=", 2)[0]
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func writeConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigPrecedence(t *testing.T) {
	dir := isolate(t)
	file := writeConfig(t, dir, "git:\n  timeout: 111\n  max_concurrent: 11\nlog:\n  level: warning\n")

	// default
	cfg, used, err := loadConfig(newFlags(t))
	if err != nil || used != "" || cfg.Git.Timeout != 300 || cfg.Git.MaxConcurrent != 10 || cfg.Log.Level != "info" {
		t.Fatalf("defaults: %+v used=%q err=%v", cfg.Git, used, err)
	}

	// file > default
	cfg, used, err = loadConfig(newFlags(t, "-C", file))
	if err != nil || used != file || cfg.Git.Timeout != 111 || cfg.Git.MaxConcurrent != 11 || cfg.Log.Level != "warning" {
		t.Fatalf("file: %+v %+v used=%q err=%v", cfg.Git, cfg.Log, used, err)
	}

	// env > file
	t.Setenv("CLI_GIT_TIMEOUT", "222")
	t.Setenv("CLI_LOG_LEVEL", "error")
	cfg, _, _ = loadConfig(newFlags(t, "-C", file))
	if cfg.Git.Timeout != 222 || cfg.Log.Level != "error" || cfg.Git.MaxConcurrent != 11 {
		t.Fatalf("env: %+v %+v", cfg.Git, cfg.Log)
	}

	// flag > env > file
	cfg, _, _ = loadConfig(newFlags(t, "-C", file, "-T", "333", "--log-level", "debug", "-J", "33"))
	if cfg.Git.Timeout != 333 || cfg.Log.Level != "debug" || cfg.Git.MaxConcurrent != 33 {
		t.Fatalf("flag: %+v %+v", cfg.Git, cfg.Log)
	}
}

func TestConfigEnvironmentVariables(t *testing.T) {
	isolate(t)
	env := map[string]string{
		"CLI_DEBUG":                 "true",
		"CLI_GIT_BASE_DIR":          "/repos",
		"CLI_GIT_PARALLEL_ENABLED":  "false",
		"CLI_GIT_MAX_DEPTH":         "3",
		"CLI_GIT_EXCLUDE_DIRS":      "a,b",
		"CLI_GIT_SKIP_DIRTY":        "false",
		"CLI_BACKUP_ENABLED":        "true",
		"CLI_BACKUP_DIRECTORY":      "/bkp",
		"CLI_BACKUP_STRATEGY":       "stash",
		"CLI_BACKUP_MAX_CONCURRENT": "7",
		"CLI_FILTER_SKIP_REPOS":     "old-project,experimental-stuff",
		"CLI_LOG_COLOR":             "never",
		"CLI_LOG_PROGRESS":          "always",
		"CLI_LOG_FILE_ENABLED":      "true",
		"CLI_LOG_FILE_DIRECTORY":    "/var/log/x",
		"CLI_LOG_FILE_LEVEL":        "debug",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg, _, err := loadConfig(newFlags(t))
	if err != nil {
		t.Fatal(err)
	}
	ok := cfg.Debug && cfg.Git.BaseDir == "/repos" && !cfg.Git.Parallel && cfg.Git.MaxDepth == 3 &&
		strings.Join(cfg.Git.ExcludeDirs, ",") == "a,b" && !cfg.Git.SkipDirty && cfg.Backup.Enabled && cfg.Backup.Directory == "/bkp" &&
		cfg.Backup.Strategy == "stash" && cfg.Backup.MaxConcurrent == 7 &&
		strings.Join(cfg.Filter.SkipRepos, ",") == "old-project,experimental-stuff" &&
		cfg.Log.Color == "never" && cfg.Log.Progress == "always" && cfg.Log.File.Enabled &&
		cfg.Log.File.Directory == "/var/log/x" && cfg.Log.File.Level == "debug"
	if !ok {
		t.Errorf("environment variables not applied: %+v", cfg)
	}
}

func TestConfigFileFromEnvAndFallback(t *testing.T) {
	dir := isolate(t)
	file := writeConfig(t, dir, "git:\n  max_depth: 3\n")
	t.Setenv("CLI_CONFIG_FILE", file)
	cfg, used, err := loadConfig(newFlags(t))
	if err != nil || used != file || cfg.Git.MaxDepth != 3 || cfg.DefaultConfigFile != file {
		t.Fatalf("CLI_CONFIG_FILE: depth=%d used=%q err=%v", cfg.Git.MaxDepth, used, err)
	}

	// Missing file => fallback to .updateGit.yaml in the current directory
	t.Setenv("CLI_CONFIG_FILE", filepath.Join(dir, "missing.yaml"))
	if err := os.WriteFile(filepath.Join(dir, ".updateGit.yaml"), []byte("git:\n  max_depth: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, used, err = loadConfig(newFlags(t))
	if err != nil || cfg.Git.MaxDepth != 1 || !strings.HasSuffix(used, ".updateGit.yaml") {
		t.Fatalf("fallback: depth=%d used=%q err=%v", cfg.Git.MaxDepth, used, err)
	}
}

func TestConfigErrors(t *testing.T) {
	dir := isolate(t)

	invalid := writeConfig(t, dir, "git: [unclosed\n")
	if _, _, err := loadConfig(newFlags(t, "-C", invalid)); err == nil {
		t.Error("invalid YAML should fail")
	}

	_, _, err := loadConfig(newFlags(t, "-J", "0", "--log-level", "verbose"))
	if err == nil || !strings.Contains(err.Error(), "Configuration validation failed") ||
		!strings.Contains(err.Error(), "Config.Git.MaxConcurrent") || !strings.Contains(err.Error(), "Config.Log.Level") {
		t.Errorf("validation error = %v", err)
	}

	if _, _, err := loadConfig(newFlags(t, "-R", "7")); err == nil {
		t.Error("max depth 7 should fail")
	}
}

func TestSkipDirtyDefaultAndFlag(t *testing.T) {
	isolate(t)
	cfg, _, err := loadConfig(newFlags(t))
	if err != nil || !cfg.Git.SkipDirty {
		t.Fatalf("skip_dirty must be enabled by default: %v, %v", cfg.Git.SkipDirty, err)
	}
	cfg, _, _ = loadConfig(newFlags(t, "--git-skip-dirty=false"))
	if cfg.Git.SkipDirty {
		t.Error("--git-skip-dirty=false must disable it")
	}
}

func TestBackupDisabledByDefaultAndFlag(t *testing.T) {
	dir := isolate(t)
	cfg, _, err := loadConfig(newFlags(t))
	if err != nil || cfg.Backup.Enabled {
		t.Fatalf("backup must be disabled by default: %v, %v", cfg.Backup.Enabled, err)
	}
	cfg, _, _ = loadConfig(newFlags(t, "-B"))
	if !cfg.Backup.Enabled {
		t.Error("-B must enable the backup")
	}

	// The flag disables the backup even when the config file and the env enable it.
	t.Setenv("CLI_CONFIG_FILE", writeConfig(t, dir, "backup:\n  enabled: true\n"))
	cfg, _, _ = loadConfig(newFlags(t))
	if !cfg.Backup.Enabled {
		t.Fatal("backup.enabled: true in the config file must enable the backup")
	}
	t.Setenv("CLI_BACKUP_ENABLED", "true")
	for _, args := range [][]string{{"--backup-enabled=false"}, {"-B=false"}} {
		cfg, _, _ = loadConfig(newFlags(t, args...))
		if cfg.Backup.Enabled {
			t.Errorf("%v must disable the backup", args)
		}
	}
}

func TestPullSkipsDirtyRepositories(t *testing.T) {
	reposDir, _ := setupRepos(t, 2)
	if err := os.WriteFile(filepath.Join(reposDir, "repo-a", "a"), []byte("local work"), 0644); err != nil {
		t.Fatal(err)
	}
	useConfig(t, nil)
	if code := runUpdate(context.Background(), reposDir); code != exitOK {
		t.Errorf("skipped repositories are not failures: exit code %d", code)
	}
	content, _ := os.ReadFile(filepath.Join(reposDir, "repo-a", "a"))
	if string(content) != "local work" {
		t.Errorf("local work was changed: %q", content)
	}

	config.Properties.Git.SkipDirty = false
	if code := runUpdate(context.Background(), reposDir); code != exitOK {
		t.Errorf("--git-skip-dirty=false: exit code %d", code)
	}
}

func TestDebugFlagOverridesLogLevel(t *testing.T) {
	isolate(t)
	cfg, _, err := loadConfig(newFlags(t, "-D", "--log-level", "error"))
	if err != nil {
		t.Fatal(err)
	}
	if effectiveLogLevel(cfg) != "debug" {
		t.Errorf("effective level = %q, want debug", effectiveLogLevel(cfg))
	}
	cfg.Debug = false
	if effectiveLogLevel(cfg) != "error" {
		t.Errorf("effective level = %q, want error", effectiveLogLevel(cfg))
	}
}

// setupRepos creates base/repos with n clones of a local origin.
func setupRepos(t *testing.T, n int) (reposDir, origin string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Clear the variables git exports to hooks (e.g. GIT_INDEX_FILE during
	// "git commit"), so the tests also pass when run from the pre-commit hook.
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_PREFIX"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatalf("unset %s: %v", k, err)
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	base := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		c := exec.CommandContext(t.Context(), "git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	run(source, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(source, "a"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	run(source, "add", ".")
	run(source, "commit", "-q", "-m", "init")
	origin = filepath.Join(base, "origin.git")
	run(base, "clone", "-q", "--bare", source, origin)
	reposDir = filepath.Join(base, "repos")
	for i := range n {
		run(base, "clone", "-q", origin, filepath.Join(reposDir, "repo-"+string(rune('a'+i))))
	}
	return reposDir, origin
}

// useConfig sets config.Properties for the test (quiet output, no progress bar).
func useConfig(t *testing.T, change func(*config.Config)) {
	t.Helper()
	original := config.Properties
	cfg := config.DefaultConfig()
	cfg.Log.Progress = "never"
	cfg.Log.Color = "never"
	if change != nil {
		change(&cfg)
	}
	config.Properties = cfg
	if err := common.InitLogger(common.LogOptions{Level: "error", Color: "never", Console: &strings.Builder{}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		config.Properties = original
		_ = common.InitLogger(common.LogOptions{Level: "info"})
	})
}

func TestPullExitCode(t *testing.T) {
	reposDir, _ := setupRepos(t, 3)
	useConfig(t, func(c *config.Config) { c.Git.MaxConcurrent = 3 })

	if code := runUpdate(context.Background(), reposDir); code != exitOK {
		t.Errorf("success: exit code %d, want 0", code)
	}

	// A repository with an invalid remote => exit code 1
	c := exec.CommandContext(t.Context(), "git", "remote", "set-url", "origin", filepath.Join(reposDir, "missing.git"))
	c.Dir = filepath.Join(reposDir, "repo-b")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if code := runUpdate(context.Background(), reposDir); code != exitFailures {
		t.Errorf("failure: exit code %d, want 1", code)
	}

	// Filtered out => only the working repositories => exit code 0
	config.Properties.Filter.SkipRepos = []string{"repo-b"}
	if code := runUpdate(context.Background(), reposDir); code != exitOK {
		t.Errorf("filtered: exit code %d, want 0", code)
	}

	// Ctrl+C => exit code 130
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := runUpdate(ctx, reposDir); code != exitInterrupted {
		t.Errorf("interrupted: exit code %d, want 130", code)
	}

	// No repositories => exit code 0
	if code := runUpdate(context.Background(), t.TempDir()); code != exitOK {
		t.Errorf("empty: exit code %d, want 0", code)
	}
}

func TestPullWithBackupAndLogFile(t *testing.T) {
	reposDir, _ := setupRepos(t, 2)
	logDir := filepath.Join(t.TempDir(), "logs")
	backupDir := filepath.Join(t.TempDir(), "backups")
	useConfig(t, func(c *config.Config) {
		c.Git.Parallel = false
		c.Backup.Enabled = true
		c.Backup.Directory = backupDir
		c.Backup.Strategy = "stash"
		c.Log.File.Enabled = true
		c.Log.File.Directory = logDir
		c.Log.File.Level = "debug"
	})

	if err := openLogFile(); err != nil {
		t.Fatal(err)
	}
	path := logFilePath
	code := runUpdate(context.Background(), reposDir)
	closeLogFile(code)
	if code != exitOK {
		t.Fatalf("exit code %d", code)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"===== updateGit " + config.CLIVersion, "Configuration:", "Updating sequentially",
		"[1/2] repo-a", "[2/2] repo-b", "Summary", "exit code 0"} {
		if !strings.Contains(text, want) {
			t.Errorf("log file does not contain %q:\n%s", want, text)
		}
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Error("log file contains control sequences")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permission %v", info.Mode().Perm())
	}
	if logFile != nil {
		t.Error("closeLogFile must close the file")
	}
	closeLogFile(0) // no-op when closed
}

func TestOpenLogFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	useConfig(t, func(c *config.Config) {
		c.Log.File.Enabled = true
		c.Log.File.Directory = locked
	})
	if err := openLogFile(); err == nil {
		t.Error("an unwritable directory must fail before the update")
	}
}

func TestExitError(t *testing.T) {
	if (&ExitError{Code: 130}).Error() != "exit code 130" {
		t.Error("ExitError message")
	}
}
