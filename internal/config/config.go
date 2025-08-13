// Package config set global variables and constants
package config

import (
	"os"
	"regexp"

	"github.com/go-playground/validator/v10"
)

//// ConfigStruct is a struct defined in the global context of the CLI
//// to group all the properties that can be used/changed in different contexts
//// and that can have custom values ​​according to the arguments of each subcommand
type Config struct {
	DefaultConfigFile string `mapstructure:"cli_config_file" validate:"omitempty"`

	Git struct {
		BaseDir       string `mapstructure:"base_dir" validate:"omitempty"`
		Parallel      bool `mapstructure:"parallel_enabled" validate:"omitempty,boolean"`
		MaxConcurrent int  `mapstructure:"max_concurrent" validate:"omitempty,number"`
	} `mapstructure:"git"`

	Backup struct {
		Enabled         bool     `mapstructure:"enabled" validate:"omitempty,boolean"`
		Directory       string   `mapstructure:"directory" validate:"omitempty"`
		Strategy        string   `mapstructure:"strategy" validate:"omitempty,alpha,lowercase,oneof=copy stash"`
	} `mapstructure:"backup"`

	Filter struct {
		IncludePatterns []string `mapstructure:"include_patterns" validate:"omitempty"`
		ExcludePatterns []string `mapstructure:"exclude_patterns" validate:"omitempty"`
		SkipRepos       []string `mapstructure:"skip_repos" validate:"omitempty"`
	} `mapstructure:"filter"`
}

// Global variables
var (
	// Version is set during build time
	// Given a version number MAJOR.MINOR.PATCH, increment the:
	// MAJOR version when you make incompatible changes, like: API, arguments or big code refactory
	// MINOR version when you add functionality in a backward compatible manner
	// PATCH version when you make backward compatible bug fixes
	// Reference: https://semver.org/
	CLIVersion = "0.1.0"
	CLIName    = "updateGit"

	// CommandsToCheck is a list of commands to check if they are installed
	// and available in the PATH environment variable.
	// Separated by comma.
	CommandsToCheck = []string{"git"}

	// Properties is a global variable of PropertiesStruct type
	Properties Config

	// Log configurations
	Debug *bool

	//----------------------------
	// Git configurations
	//----------------------------
  Timeout int = 30 // Default timeout for git operations in seconds

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
)

// SetDefaultConfig set default values to Properties variable
func SetDefaultConfig() {
	Properties.DefaultConfigFile = ".updateGit.yaml"
	Properties.Git.BaseDir = "./git_repos"
	Properties.Git.Parallel = true
	Properties.Git.MaxConcurrent = 10
	Properties.Backup.Enabled = false
	// Attention!!! The validator do not support ˜, $HOME or file globbing in values.
	Properties.Backup.Directory = "./backups"
	Properties.Backup.Strategy = "copy"
	Properties.Filter.IncludePatterns = []string{}
	Properties.Filter.ExcludePatterns = []string{}
	Properties.Filter.SkipRepos = []string{}
}

// NoUnderscores is a custom validator to reject string with underscore '_'
func NoUnderscores(fl validator.FieldLevel) bool {
	matched, _ := regexp.MatchString(`_`, fl.Field().String())
	return !matched
}
