package cmd

import (
	"path/filepath"
	"time"

	"github.com/aeciopires/updateGit/internal/backup"
	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/filter"
	"github.com/aeciopires/updateGit/internal/git"
	"github.com/spf13/cobra"
)

var (
	// runUpdateCmd is the command to run the update process)
	runUpdateCmd = &cobra.Command{
		Use:   "pull",
		Short: "Update git repositories",
		Long:  "Update all git repositories in the specified base directory with optional parallel processing and backup.",
		RunE: func(cmd *cobra.Command, args []string) error {
			baseDir := config.Properties.Git.BaseDir

			if baseDir == "" {
				baseDir = "./git_repos"
			}

			return runUpdate(baseDir)
		},
	}
)

// init initializes the update command and its flags
func init() {
	// Add the update command to the root command
	rootCmd.AddCommand(runUpdateCmd)
}

// runUpdate executes the main update logic with all enhanced features
func runUpdate(baseDir string) error {
	common.Logger("info", "Starting enhanced git repositories update. baseDir=%s parallel=%t max_concurrent=%d backup_enabled=%t backup_dir=%s skip_repos=%s",
		baseDir,
		config.Properties.Git.Parallel,
		config.Properties.Git.MaxConcurrent,
		config.Properties.Backup.Enabled,
		config.Properties.Backup.Directory,
		config.Properties.Filter.SkipRepos,
	)

	if !common.DirExists(baseDir) {
		common.Logger("fatal", "Directory validation failed: directory does not exist: %s", baseDir)
	}

	// Get absolute path
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		common.Logger("fatal", "Failed to get absolute path: %w", err)
	}

	common.Logger("debug", "Using absolute path: %s", absBaseDir)

	// Initialize repository filter
	repoFilter, err := initializeFilter()
	if err != nil {
		common.Logger("fatal", "Failed to initialize filter: %w", err)
	}

	// Initialize backup manager
	backupManager, err := initializeBackupManager()
	if err != nil {
		common.Logger("fatal", "Failed to initialize backup manager: %w", err)
	}

	// Create update configuration
	updateConfig := git.UpdateConfig{
		BaseDir: absBaseDir,
		Parallel: git.ParallelUpdateConfig{
			Enabled:       config.Properties.Git.Parallel,
			MaxConcurrent: config.Properties.Git.MaxConcurrent,
			Timeout:       time.Duration(config.Timeout) * time.Second,
		},
		BackupEnabled: config.Properties.Backup.Enabled,
		BackupManager: backupManager,
		Filter:        repoFilter,
	}

	// Set default timeout if not configured
	if updateConfig.Parallel.Timeout == 0 {
		updateConfig.Parallel.Timeout = 5 * time.Minute
	}

	var filterStats any
	if repoFilter != nil {
		filterStats = repoFilter.GetStats()
	}
	common.Logger("info", "Update configuration prepared. parallel=%t max_concurrent=%d timeout=%v backup=%t filter_stats=%v",
		updateConfig.Parallel.Enabled,
		updateConfig.Parallel.MaxConcurrent,
		updateConfig.Parallel.Timeout,
		updateConfig.BackupEnabled,
		filterStats,
	)

	// Execute repository updates with backup/filter support
	return git.UpdateRepositoriesWithConfig(updateConfig)
}

// initializeFilter creates and configures the repository filter
func initializeFilter() (*filter.Filter, error) {
	skipRepos := config.Properties.Filter.SkipRepos

	// Create filter
	repoFilter, err := filter.NewFilter(skipRepos)
	if err != nil {
		common.Logger("fatal", "Failed to create repository filter: %w", err)
	}

	common.Logger("info", "Repository filter initialized. filter_stats=%v", repoFilter.GetStats())

	return repoFilter, nil
}

// initializeBackupManager creates and configures the backup manager
func initializeBackupManager() (*backup.BackupManager, error) {
	if !config.Properties.Backup.Enabled {
		common.Logger("debug", "Backup disabled, skipping backup manager initialization")
		return nil, nil
	}

	backupDir := config.Properties.Backup.Directory
	if backupDir == "" {
		backupDir = "./backups"
	}

	// For now, default to copy strategy
	strategy := backup.StrategyCopy
	if config.Properties.Backup.Strategy == "stash" {
		strategy = backup.StrategyStash
	}

	backupManager := backup.NewBackupManager(backupDir, strategy)

	common.Logger("info", "Backup manager initialized. backup_stats=%v", backupManager.GetBackupStats())

	return backupManager, nil
}
