// Package cmd implements the CLI commands (Cobra) and loads the configuration (Viper).
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/getinfo"
	"github.com/aeciopires/updateGit/internal/output"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	longVersion  *bool
	shortVersion *bool

	// startTime is the start of the execution (used in the log file name)
	startTime = time.Now()

	// terminal is the only path to the standard output (logger, blocks and progress bar)
	terminal = output.NewTerminal(os.Stdout)

	// exitFunc ends the process (replaced in the tests)
	exitFunc = os.Exit

	// Log file of the execution (--log-file-enabled)
	logFile       *os.File
	logFileWriter io.Writer
	logFilePath   string
)

// ExitError ends the program with a specific exit code, without printing an error
// (the reason was already reported).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }

// flagKeys maps each flag to its configuration key (viper/.updateGit.yaml).
// Environment variables: CLI_ + key in uppercase with "." replaced by "_".
var flagKeys = map[string]string{
	"debug":                 "debug",
	"git-base-dir":          "git.base_dir",
	"git-parallel-enabled":  "git.parallel_enabled",
	"git-max-concurrent":    "git.max_concurrent",
	"git-timeout":           "git.timeout",
	"git-max-depth":         "git.max_depth",
	"git-exclude-dirs":      "git.exclude_dirs",
	"git-skip-dirty":        "git.skip_dirty",
	"backup-enabled":        "backup.enabled",
	"backup-dir":            "backup.directory",
	"backup-strategy":       "backup.strategy",
	"backup-max-concurrent": "backup.max_concurrent",
	"skip-repos":            "filter.skip_repos",
	"log-level":             "log.level",
	"color":                 "log.color",
	"progress":              "log.progress",
	"log-file-enabled":      "log.file.enabled",
	"log-file-dir":          "log.file.directory",
	"log-file-level":        "log.file.level",
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "updateGit",
	Short: "Update local git repositories with advanced features",
	Long: `A modern CLI tool to update all git repositories in a specified directory.

This tool scans a base directory for git repositories and runs 'git pull'
on each one to keep them up to date.`,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if cmd == cmd.Root() || !config.Properties.Log.File.Enabled {
			return nil
		}
		return openLogFile()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// If the user ran the command without providing any arguments and without setting any flags,
		// it assumes the user needs help and displays the command's help text.
		if len(args) == 0 && cmd.Flags().NFlag() == 0 {
			return cmd.Help()
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	code := 0
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.Code
		} else {
			common.Logger("error", "%v", err)
			code = 1
		}
	}

	if code == 0 {
		// Show longVersion: version, commit, build date, system and arch
		if *longVersion {
			getinfo.PrintLongVersion()
			getinfo.ShowOperatingSystem()
			getinfo.ShowSystemArch()
		}
		// Show shortVersion
		if *shortVersion {
			getinfo.PrintShortVersion()
		}
	}

	closeLogFile(code)
	if code != 0 {
		exitFunc(code)
	}
}

func init() {
	config.SetDefaultConfig()
	cobra.OnInitialize(initConfig)
	defineFlags(rootCmd)
	longVersion = rootCmd.Flags().BoolP("long-version", "V", false, "Show long version")
	shortVersion = rootCmd.Flags().BoolP("version", "v", false, "Show short version")
}

// defineFlags defines the global flags. The defaults come from config.DefaultConfig.
func defineFlags(cmd *cobra.Command) {
	d := config.DefaultConfig()
	f := cmd.PersistentFlags()

	f.StringP("config-file", "C", d.DefaultConfigFile, "Config file path (env CLI_CONFIG_FILE)")
	f.BoolP("debug", "D", d.Debug, "Enable debug mode (same as --log-level=debug)")

	// Git flags
	f.StringP("git-base-dir", "G", d.Git.BaseDir, "Base directory for git repositories")
	f.BoolP("git-parallel-enabled", "P", d.Git.Parallel, "Enable parallel git repository updates (disable with --git-parallel-enabled=false)")
	f.IntP("git-max-concurrent", "J", d.Git.MaxConcurrent, "Maximum number of concurrent git repositories updates (1-100)")
	f.IntP("git-timeout", "T", d.Git.Timeout, "Timeout in seconds for each 'git pull'")
	f.IntP("git-max-depth", "R", d.Git.MaxDepth, "Directory levels searched for repositories (1-6)")
	f.StringSlice("git-exclude-dirs", d.Git.ExcludeDirs, "Directory names ignored by the search of repositories")
	f.Bool("git-skip-dirty", d.Git.SkipDirty, "Skip repositories with files not committed, to avoid losing work (disable with --git-skip-dirty=false)")

	// Backup flags
	f.BoolP("backup-enabled", "B", d.Backup.Enabled, "Create backup before updating")
	f.StringP("backup-dir", "Z", d.Backup.Directory, "Directory to store backups")
	f.StringP("backup-strategy", "Y", d.Backup.Strategy, "Backup strategy (e.g. 'copy', 'stash')")
	f.Int("backup-max-concurrent", d.Backup.MaxConcurrent, "Maximum number of simultaneous backups (1-100)")

	// Filtering flags
	f.StringSliceP("skip-repos", "S", d.Filter.SkipRepos, "List of repository names (or paths relative to the base directory) to skip")

	// Log and output flags
	f.StringP("log-level", "L", d.Log.Level, "Minimum log level: debug, info, warning, error")
	f.String("color", d.Log.Color, "Colored output: auto, always, never")
	f.String("progress", d.Log.Progress, "Live progress bar: auto, always, never")
	f.BoolP("log-file-enabled", "O", d.Log.File.Enabled, "Save the log to <log-file-dir>/updateGit_YYYY-MM-DD_HH-MM-SS.log")
	f.String("log-file-dir", d.Log.File.Directory, "Directory of the log file")
	f.String("log-file-level", d.Log.File.Level, "Minimum log level saved in the log file (default: same as --log-level)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	flags := rootCmd.PersistentFlags()

	// Preliminary logger: honors -D for the messages of the configuration loading
	if debug, _ := flags.GetBool("debug"); debug {
		_ = common.InitLogger(common.LogOptions{Level: "debug", Color: common.ColorAuto, Console: terminal, ConsoleIsTerminal: common.IsTerminal(os.Stdout)})
	}

	cfg, used, err := loadConfig(flags)
	if err != nil {
		common.Logger("fatal", "%v", err)
	}
	config.Properties = cfg

	if err := common.InitLogger(common.LogOptions{
		Level:             effectiveLogLevel(cfg),
		Color:             cfg.Log.Color,
		Console:           terminal,
		ConsoleIsTerminal: common.IsTerminal(os.Stdout),
	}); err != nil {
		common.Logger("fatal", "%v", err)
	}

	if used != "" {
		common.Logger("debug", "Using config file: %s", used)
	} else {
		common.Logger("debug", "No config file found. Using defaults, environment variables and flags.")
	}
	finalConfigBytes, _ := yaml.Marshal(config.Properties)
	common.Logger("debug", "Final Configuration Loaded:\n%s", string(finalConfigBytes))
}

// effectiveLogLevel returns the console log level: -D/--debug has priority over --log-level.
func effectiveLogLevel(cfg config.Config) string {
	if cfg.Debug {
		return "debug"
	}
	return cfg.Log.Level
}

// loadConfig merges the configuration with the precedence:
// flags > environment variables (CLI_*) > config file > defaults.
// It returns the configuration and the config file used ("" if none).
func loadConfig(flags *pflag.FlagSet) (config.Config, string, error) {
	v := viper.New()
	v.SetEnvPrefix("cli")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.SetConfigType("yaml")

	setDefaults(v, config.DefaultConfig())
	for flagName, key := range flagKeys {
		if flag := flags.Lookup(flagName); flag != nil {
			if err := v.BindPFlag(key, flag); err != nil {
				return config.Config{}, "", fmt.Errorf("cannot bind flag --%s: %w", flagName, err)
			}
		}
	}
	v.AutomaticEnv()

	configFile, used, err := readConfigFile(v, flags)
	if err != nil {
		return config.Config{}, "", err
	}

	var cfg config.Config
	if err := v.Unmarshal(&cfg); err != nil {
		return config.Config{}, "", fmt.Errorf("error unmarshaling config: %w", err)
	}
	cfg.DefaultConfigFile = configFile

	if err := validateConfig(cfg); err != nil {
		return config.Config{}, "", err
	}
	return cfg, used, nil
}

// setDefaults registers all keys with their default values (required by AutomaticEnv and Unmarshal).
func setDefaults(v *viper.Viper, d config.Config) {
	v.SetDefault("debug", d.Debug)
	v.SetDefault("git.base_dir", d.Git.BaseDir)
	v.SetDefault("git.parallel_enabled", d.Git.Parallel)
	v.SetDefault("git.max_concurrent", d.Git.MaxConcurrent)
	v.SetDefault("git.timeout", d.Git.Timeout)
	v.SetDefault("git.max_depth", d.Git.MaxDepth)
	v.SetDefault("git.exclude_dirs", d.Git.ExcludeDirs)
	v.SetDefault("git.skip_dirty", d.Git.SkipDirty)
	v.SetDefault("backup.enabled", d.Backup.Enabled)
	v.SetDefault("backup.directory", d.Backup.Directory)
	v.SetDefault("backup.strategy", d.Backup.Strategy)
	v.SetDefault("backup.max_concurrent", d.Backup.MaxConcurrent)
	v.SetDefault("filter.skip_repos", d.Filter.SkipRepos)
	v.SetDefault("log.level", d.Log.Level)
	v.SetDefault("log.color", d.Log.Color)
	v.SetDefault("log.progress", d.Log.Progress)
	v.SetDefault("log.file.enabled", d.Log.File.Enabled)
	v.SetDefault("log.file.directory", d.Log.File.Directory)
	v.SetDefault("log.file.level", d.Log.File.Level)
}

// readConfigFile reads the config file: --config-file/-C, then CLI_CONFIG_FILE, then
// ".updateGit.yaml" in the current directory or in /app. A missing file is not an error.
func readConfigFile(v *viper.Viper, flags *pflag.FlagSet) (configFile, used string, err error) {
	configFile = config.DefaultConfig().DefaultConfigFile
	if flag := flags.Lookup("config-file"); flag != nil && flag.Changed {
		configFile = flag.Value.String()
	} else if env, ok := os.LookupEnv("CLI_CONFIG_FILE"); ok && env != "" {
		configFile = env
	}

	common.Logger("debug", "Attempting to read specific config file: %s", configFile)
	v.SetConfigFile(configFile)
	err = v.ReadInConfig()
	if err == nil {
		return configFile, v.ConfigFileUsed(), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return configFile, "", fmt.Errorf("error reading config file %s (check permissions and format): %w", configFile, err)
		}
	}

	common.Logger("debug", "Config file '%s' not found. Searching '.updateGit.yaml' in '.' and '/app'.", configFile)
	v.SetConfigFile("")
	v.SetConfigName(".updateGit")
	v.AddConfigPath(".")
	v.AddConfigPath("/app")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return configFile, "", nil
		}
		return configFile, "", fmt.Errorf("error reading config file (check permissions and format): %w", err)
	}
	return configFile, v.ConfigFileUsed(), nil
}

// validateConfig validates the configuration and builds a user-friendly message.
func validateConfig(cfg config.Config) error {
	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.RegisterValidation("noUnderscore", config.NoUnderscores); err != nil {
		return fmt.Errorf("cannot register validation: %w", err)
	}

	err := validate.Struct(&cfg)
	if err == nil {
		return nil
	}
	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return fmt.Errorf("an unexpected error occurred during configuration validation: %w", err)
	}
	var b strings.Builder
	b.WriteString("Configuration validation failed:\n")
	for _, fieldErr := range validationErrors {
		fmt.Fprintf(&b, "  - Field '%s': Failed on validation rule '%s'. Value: '%v'\n",
			fieldErr.StructNamespace(), // e.g., Config.Git.MaxConcurrent
			fieldErr.Tag(),             // e.g., "min", "oneof"
			fieldErr.Value(),           // The actual invalid value
		)
	}
	return errors.New(strings.TrimRight(b.String(), "\n"))
}

// openLogFile creates the log file, adds it to the logger and writes the header.
func openLogFile() error {
	cfg := config.Properties
	file, path, err := common.OpenLogFile(cfg.Log.File.Directory, startTime)
	if err != nil {
		return err
	}
	logFile, logFilePath = file, path
	logFileWriter = common.NewSyncWriter(file)

	if err := common.InitLogger(common.LogOptions{
		Level:             effectiveLogLevel(cfg),
		Color:             cfg.Log.Color,
		Console:           terminal,
		ConsoleIsTerminal: common.IsTerminal(os.Stdout),
		File:              logFileWriter,
		FileLevel:         cfg.Log.File.Level,
	}); err != nil {
		return err
	}
	common.SetExitHook(closeLogFile)

	configBytes, _ := yaml.Marshal(cfg)
	_, _ = fmt.Fprintf(logFileWriter, "===== %s %s · started %s · pid %d =====\nCommand: %s\nConfiguration:\n%s\n",
		config.CLIName, config.CLIVersion, startTime.Format("2006-01-02 15:04:05"), os.Getpid(),
		common.MaskCredentials(strings.Join(os.Args, " ")), common.MaskCredentials(string(configBytes)))
	return nil
}

// closeLogFile writes the footer and closes the log file (if open).
func closeLogFile(code int) {
	if logFile == nil {
		return
	}
	common.Logger("info", "Log saved to %s", logFilePath)
	_, _ = fmt.Fprintf(logFileWriter, "===== finished %s · duration %s · exit code %d =====\n",
		time.Now().Format("2006-01-02 15:04:05"), output.FormatDuration(time.Since(startTime)), code)
	_ = logFile.Close()
	logFile = nil
	// Stop writing to the closed file
	_ = common.InitLogger(common.LogOptions{
		Level:             effectiveLogLevel(config.Properties),
		Color:             config.Properties.Log.Color,
		Console:           terminal,
		ConsoleIsTerminal: common.IsTerminal(os.Stdout),
	})
}
