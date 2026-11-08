// Package config set global variables and constants
package config

import (
	"os"
	"regexp"

	"github.com/go-playground/validator/v10"
)

// Config is a struct defined in the global context of the CLI
// to group all the properties that can be used/changed in different contexts
// and that can have custom values according to the arguments of each subcommand.
//
// The mapstructure keys are also the viper keys. Environment variables use the
// prefix CLI_ and replace "." by "_" (e.g. git.max_concurrent => CLI_GIT_MAX_CONCURRENT).
type Config struct {
	DefaultConfigFile string `mapstructure:"cli_config_file" yaml:"cli_config_file" validate:"omitempty"`

	// Debug is the same as Log.Level = "debug" (kept for compatibility with -D/--debug)
	Debug bool `mapstructure:"debug" yaml:"debug"`

	Git struct {
		BaseDir       string `mapstructure:"base_dir" yaml:"base_dir" validate:"omitempty"`
		Parallel      bool   `mapstructure:"parallel_enabled" yaml:"parallel_enabled"`
		MaxConcurrent int    `mapstructure:"max_concurrent" yaml:"max_concurrent" validate:"min=1,max=100"`
		// Timeout in seconds for each "git pull"
		Timeout int `mapstructure:"timeout" yaml:"timeout" validate:"min=1"`
		// Directory levels searched for repositories (1 = only base_dir children)
		MaxDepth    int      `mapstructure:"max_depth" yaml:"max_depth" validate:"min=1,max=6"`
		ExcludeDirs []string `mapstructure:"exclude_dirs" yaml:"exclude_dirs" validate:"omitempty"`
		// SkipDirty skips repositories with files not committed (avoids losing work)
		SkipDirty bool `mapstructure:"skip_dirty" yaml:"skip_dirty"`
	} `mapstructure:"git" yaml:"git"`

	Backup struct {
		Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
		Directory string `mapstructure:"directory" yaml:"directory" validate:"omitempty"`
		Strategy  string `mapstructure:"strategy" yaml:"strategy" validate:"omitempty,alpha,lowercase,oneof=copy stash"`
		// Maximum number of simultaneous backups (independent of Git.MaxConcurrent)
		MaxConcurrent int `mapstructure:"max_concurrent" yaml:"max_concurrent" validate:"min=1,max=100"`
	} `mapstructure:"backup" yaml:"backup"`

	Filter struct {
		SkipRepos []string `mapstructure:"skip_repos" yaml:"skip_repos" validate:"omitempty"`
	} `mapstructure:"filter" yaml:"filter"`

	Log struct {
		// Minimum log level shown in the terminal
		Level string `mapstructure:"level" yaml:"level" validate:"oneof=debug info warning error"`
		// Colored output
		Color string `mapstructure:"color" yaml:"color" validate:"oneof=auto always never"`
		// Live progress bar
		Progress string `mapstructure:"progress" yaml:"progress" validate:"oneof=auto always never"`
		File     struct {
			Enabled   bool   `mapstructure:"enabled" yaml:"enabled"`
			Directory string `mapstructure:"directory" yaml:"directory" validate:"required_if=Enabled true"`
			// Minimum log level saved in the file (empty = same as Log.Level)
			Level string `mapstructure:"level" yaml:"level" validate:"omitempty,oneof=debug info warning error"`
		} `mapstructure:"file" yaml:"file"`
	} `mapstructure:"log" yaml:"log"`
}

// Global variables
var (
	// Given a version number MAJOR.MINOR.PATCH, increment the:
	// MAJOR version when you make incompatible changes, like: API, arguments or big code refactory
	// MINOR version when you add functionality in a backward compatible manner
	// PATCH version when you make backward compatible bug fixes
	// Reference: https://semver.org/
	// The release tag must be equal to this value (without "v" prefix).
	CLIVersion        = "0.2.0"
	CLIName           = "updateGit"
	CLICheckSumBinDir = "bin/"

	// CLICommit and CLIBuildDate are set during the build (Makefile LDFLAGS)
	CLICommit    = "unknown"
	CLIBuildDate = "unknown"

	// CommandsToCheck is a list of commands to check if they are installed
	// and available in the PATH environment variable.
	// Separated by comma.
	CommandsToCheck = []string{"git"}

	// Properties is a global variable of Config type
	Properties Config

	//----------------------------
	// Linux/Unix configurations
	//----------------------------
	// 0755 => 0 -> selects attributes for the set user ID
	//         7 -> (U)ser/owner can read, can write and can execute.
	//         5 -> (G)roup can read, can't write and can execute.
	//         5 -> (O)thers can read, can't write and can
	// References:
	// https://chmodcommand.com/chmod-0755/
	// https://stackoverflow.com/questions/14249467/os-mkdir-and-os-mkdirall-permissions
	PermissionDir    os.FileMode = 0755
	PermissionBinary os.FileMode = 0755

	// 0644 => 0 -> selects attributes for the set user ID
	//         6 -> (U)ser/owner can read, can write and can't execute.
	//         4 -> (G)roup can read, can't write and can't execute.
	//         4 -> (O)thers can read, can't write and can't execute.
	// References:
	// https://chmodcommand.com/chmod-0644/
	// https://stackoverflow.com/questions/14249467/os-mkdir-and-os-mkdirall-permissions
	PermissionFile os.FileMode = 0644

	// 0600 => only the owner can read and write. Used by the log file, because
	// the default directory (/tmp) is shared with other users.
	PermissionLogFile os.FileMode = 0600
)

// SetDefaultConfig set default values to Properties variable
func SetDefaultConfig() {
	Properties = DefaultConfig()
}

// DefaultConfig returns a Config with the default values
func DefaultConfig() Config {
	var c Config
	c.DefaultConfigFile = ".updateGit.yaml"
	c.Debug = false
	c.Git.BaseDir = "./git_repos"
	c.Git.Parallel = true
	c.Git.MaxConcurrent = 10
	c.Git.Timeout = 300
	c.Git.MaxDepth = 2
	c.Git.ExcludeDirs = []string{"node_modules", "vendor", ".terraform", ".venv"}
	c.Git.SkipDirty = true
	c.Backup.Enabled = false
	// Attention!!! The validator do not support ˜, $HOME or file globbing in values.
	c.Backup.Directory = "./backups"
	c.Backup.Strategy = "copy"
	c.Backup.MaxConcurrent = 4
	c.Filter.SkipRepos = []string{}
	c.Log.Level = "info"
	c.Log.Color = "auto"
	c.Log.Progress = "auto"
	c.Log.File.Enabled = false
	c.Log.File.Directory = "/tmp"
	c.Log.File.Level = ""
	return c
}

// SemverRegex validates a semantic version without "v" prefix (https://semver.org),
// without build metadata. The same expression is used by the release workflow.
var SemverRegex = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?$`)

// NoUnderscores is a custom validator to reject string with underscore '_'
func NoUnderscores(fl validator.FieldLevel) bool {
	matched, _ := regexp.MatchString(`_`, fl.Field().String())
	return !matched
}
