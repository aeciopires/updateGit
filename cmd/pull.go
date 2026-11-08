package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/aeciopires/updateGit/internal/backup"
	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/filter"
	"github.com/aeciopires/updateGit/internal/git"
	"github.com/aeciopires/updateGit/internal/output"
	"github.com/spf13/cobra"
)

// Exit codes of the pull command
const (
	exitOK          = 0
	exitFailures    = 1
	exitInterrupted = 130
)

// tickerInterval is the refresh interval of the progress bar
const tickerInterval = 100 * time.Millisecond

// runUpdateCmd is the command to run the update process
var runUpdateCmd = &cobra.Command{
	Use:   "pull",
	Short: "Update git repositories",
	Long: `Update all git repositories in the specified base directory.

The repositories are discovered first (up to --git-max-depth levels, never inside
other repositories) and then updated with up to --git-max-concurrent simultaneous
'git pull', with optional backup and filtering.

Exit codes: 0 = success, 1 = at least one repository failed or timed out,
130 = interrupted (Ctrl+C).`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.SilenceUsage = true
		baseDir := config.Properties.Git.BaseDir
		if baseDir == "" {
			baseDir = "./git_repos"
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		code := runUpdate(ctx, baseDir)
		if code != exitOK {
			return &ExitError{Code: code}
		}
		return nil
	},
}

// init initializes the update command and its flags
func init() {
	// Add the update command to the root command
	rootCmd.AddCommand(runUpdateCmd)
}

// runUpdate executes the update in two stages: discovery (finished before any
// update, so the total is known) and update (parallel workers). It returns the exit code.
func runUpdate(ctx context.Context, baseDir string) int {
	cfg := config.Properties
	start := time.Now()

	if !common.DirExists(baseDir) {
		common.Logger("fatal", "Directory validation failed: directory does not exist: %s", baseDir)
	}
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		common.Logger("fatal", "Failed to get absolute path: %v", err)
	}

	isTerminal := common.IsTerminal(os.Stdout)
	color := common.ColorOutput()
	width := output.TerminalWidth(os.Stdout)

	// ---- Stage 1: discovery ----
	depth := cfg.Git.MaxDepth
	common.Logger("info", "Scanning %s (levels 1-%d)", absBaseDir, depth)
	var spinner *output.Spinner
	stopTicker := func() {}
	if output.SpinnerEnabled(cfg.Log.Progress, isTerminal) {
		spinner = output.NewSpinner(absBaseDir, depth, color, width)
		terminal.SetBar(spinner.Line)
		stopTicker = output.StartTicker(terminal, tickerInterval)
	}
	discovery, err := git.DiscoverRepositories(git.DiscoveryOptions{
		BaseDir:     absBaseDir,
		MaxDepth:    depth,
		ExcludeDirs: cfg.Git.ExcludeDirs,
		OnProgress: func(dirs, repos int) {
			if spinner != nil {
				spinner.Update(dirs, repos)
			}
		},
	})
	stopTicker()
	terminal.StopBar()
	if err != nil {
		common.Logger("fatal", "Failed to find repositories: %v", err)
	}
	common.Logger("info", "Scan finished in %s: %d repositories in %d directories (levels 1-%d)",
		output.FormatDuration(discovery.Duration), len(discovery.Repos), discovery.VisitedDirs, discovery.MaxDepth)

	// ---- Filter ----
	repoFilter, err := filter.NewFilter(cfg.Filter.SkipRepos)
	if err != nil {
		common.Logger("fatal", "Failed to create repository filter: %v", err)
	}
	var toUpdate []git.Repository
	var filtered []string
	for _, repo := range discovery.Repos {
		if repoFilter.ShouldProcess(repo.Name) {
			toUpdate = append(toUpdate, repo)
		} else {
			filtered = append(filtered, repo.Name)
		}
	}
	ignored := "none"
	if len(discovery.IgnoredDirs) > 0 {
		ignored = strings.Join(discovery.IgnoredDirs, ", ")
	}
	common.Logger("info", "Skipped by filter: %d · Ignored directories: %s · To update: %d", len(filtered), ignored, len(toUpdate))

	if len(toUpdate) == 0 {
		common.Logger("warning", "No git repositories to update. baseDir=%s", absBaseDir)
		return exitOK
	}

	// ---- Stage 2: update ----
	workers := git.EffectiveWorkers(cfg.Git.Parallel, cfg.Git.MaxConcurrent, len(toUpdate))
	timeout := time.Duration(cfg.Git.Timeout) * time.Second
	if workers > 1 {
		common.Logger("info", "Updating in parallel with %d workers (timeout %s per repository)", workers, timeout)
	} else {
		common.Logger("info", "Updating sequentially (timeout %s per repository). If necessary, enter login/password when prompted.", timeout)
	}

	runCfg := git.RunConfig{Workers: workers, Timeout: timeout, SkipDirty: cfg.Git.SkipDirty}
	if !cfg.Git.SkipDirty {
		common.Logger("warning", "Repositories with files not committed will be updated (--git-skip-dirty=false).")
	}
	backupLimit := 0
	if cfg.Backup.Enabled {
		runCfg.Backup = initializeBackupManager(cfg)
		runCfg.BackupMaxConcurrent = cfg.Backup.MaxConcurrent
		backupLimit = git.EffectiveBackupLimit(cfg.Backup.MaxConcurrent, workers)
		common.Logger("info", "Backup enabled: strategy=%s directory=%s max_concurrent=%d", cfg.Backup.Strategy, cfg.Backup.Directory, backupLimit)
	} else {
		common.Logger("debug", "Backup disabled (enable it with --backup-enabled).")
	}

	var progress *output.Progress
	if output.ProgressEnabled(cfg.Log.Progress, isTerminal, workers) {
		progress = output.NewProgress(len(toUpdate), backupLimit, color, width)
		terminal.SetBar(progress.Line)
		stopTicker = output.StartTicker(terminal, tickerInterval)
	}

	consoleLevel := common.ConsoleLevel()
	dests := []output.Destination{{
		Writer:   terminal,
		Opts:     output.Options{Color: color, Debug: consoleLevel == common.LevelDebug},
		MinLevel: consoleLevel,
	}}
	if logFileWriter != nil {
		fileLevel := common.FileLevel()
		dests = append(dests, output.Destination{
			Writer:   logFileWriter,
			Opts:     output.Options{Color: false, Debug: fileLevel == common.LevelDebug},
			MinLevel: fileLevel,
		})
	}
	printer := output.NewPrinter(len(toUpdate), progress, dests...)
	if consoleLevel <= common.LevelInfo {
		_, _ = terminal.Write([]byte("\n"))
	}

	results := git.Run(ctx, toUpdate, runCfg, printer)

	stopTicker()
	terminal.StopBar()

	summary := output.Summary{
		Results:  results,
		Total:    len(toUpdate),
		Filtered: filtered,
		Duration: time.Since(start),
	}
	printer.PrintSummary(summary)

	switch {
	case ctx.Err() != nil:
		common.Logger("warning", "Interrupted. %d of %d repositories were processed.", len(results), len(toUpdate))
		return exitInterrupted
	case summary.HasFailures():
		return exitFailures
	default:
		return exitOK
	}
}

// initializeBackupManager creates and configures the backup manager
func initializeBackupManager(cfg config.Config) *backup.BackupManager {
	backupDir := cfg.Backup.Directory
	if backupDir == "" {
		backupDir = "./backups"
	}

	strategy := backup.StrategyCopy
	if cfg.Backup.Strategy == "stash" {
		strategy = backup.StrategyStash
	}

	backupManager := backup.NewBackupManager(backupDir, strategy)
	common.Logger("debug", "Backup manager initialized. backup_stats=%v", backupManager.GetBackupStats())
	return backupManager
}
