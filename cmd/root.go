package cmd

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/getinfo"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	longVersion  *bool
	shortVersion *bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "updateGit",
	Short: "Update local git repositories with advanced features",
	Long: `A modern CLI tool to update all git repositories in a specified directory.

This tool scans a base directory for git repositories and runs 'git pull'
on each one to keep them up to date.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If the user ran the command without providing any arguments and without setting any flags.
		// If both of those conditions are met, it assumes the user needs help and displays the command's help text.
		if len(args) == 0 && cmd.Flags().NFlag() == 0 {
			cmd.Help()
			return
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}

	// Show longVersion. *longVersion contains the pointer address. If the content is true print longVersion, system and arch
	if *longVersion {
		getinfo.PrintLongVersion()
		getinfo.ShowOperatingSystem()
		getinfo.ShowSystemArch()
	}

	// Show shortVersion. *shortVersion contains the pointer address. If the content is true print shortVersion, system and arch
	if *shortVersion {
		getinfo.PrintShortVersion()
	}

	// Debug message is displayed if -D option was passed
	common.Logger("debug", "====> Values loaded in cmd/root.go")
	auxValue := reflect.ValueOf(config.Properties)
	auxType := reflect.TypeOf(config.Properties)

	// Interate over the fields of the struct
	for i := 0; i < auxValue.NumField(); i++ {
		fieldName := auxType.Field(i).Name
		fieldValue := auxValue.Field(i).Interface()
		common.Logger("debug", "Field: %s, Value: %v", fieldName, fieldValue)
	}
}

func init() {
	config.SetDefaultConfig()
	cobra.OnInitialize(initConfig)

	// Global flags
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVarP(&config.Properties.DefaultConfigFile, "config-file", "C", config.Properties.DefaultConfigFile, "Config file path")

	config.Debug = rootCmd.PersistentFlags().BoolP("debug", "D", false, "Enable debug mode.")
	longVersion = rootCmd.Flags().BoolP("long-version", "V", false, "Show long version")
	shortVersion = rootCmd.Flags().BoolP("version", "v", false, "Show short version")

	// Git flags
	rootCmd.PersistentFlags().StringVarP(&config.Properties.Git.BaseDir, "git-base-dir", "G", config.Properties.Git.BaseDir, "Base directory for git repositories")
	rootCmd.PersistentFlags().BoolVarP(&config.Properties.Git.Parallel, "git-parallel-enabled", "P", config.Properties.Git.Parallel, "Enable parallel git repository updates")
	rootCmd.PersistentFlags().IntVarP(&config.Properties.Git.MaxConcurrent, "git-max-concurrent", "J", config.Properties.Git.MaxConcurrent, "Maximum number of concurrent git repositories updates")

	// Backup flags
	rootCmd.PersistentFlags().BoolVarP(&config.Properties.Backup.Enabled, "backup-enabled", "B", config.Properties.Backup.Enabled, "Create backup before updating")
	rootCmd.PersistentFlags().StringVarP(&config.Properties.Backup.Directory, "backup-dir", "Z", config.Properties.Backup.Directory, "Directory to store backups")
	rootCmd.PersistentFlags().StringVarP(&config.Properties.Backup.Strategy, "backup-strategy", "Y", config.Properties.Backup.Strategy, "Backup strategy (e.g. 'copy', 'stash')")

	// Filtering flags
	rootCmd.PersistentFlags().StringSliceVarP(&config.Properties.Filter.IncludePatterns, "include-patterns", "I", config.Properties.Filter.IncludePatterns, "Include repositories matching pattern (regex)")
	rootCmd.PersistentFlags().StringSliceVarP(&config.Properties.Filter.ExcludePatterns, "exclude-patterns", "E", config.Properties.Filter.ExcludePatterns, "Exclude repositories matching pattern (regex)")
	rootCmd.PersistentFlags().StringSliceVarP(&config.Properties.Filter.SkipRepos, "skip-repos", "S", config.Properties.Filter.SkipRepos, "List of repository names to skip")
}

// initConfig reads in config file and ENV variables if set.
// This function is performaded in cmd/root.go and cmd/subcommand.go
func initConfig() {
	// Environment variables expect with prefix CLI_ . This helps avoid conflicts.
	viper.SetEnvPrefix("cli")
	// Type file
	viper.SetConfigType("yaml")
	// Environment variables can't have dashes in them, so bind them to their equivalent
	// keys with underscores, e.g. --backup-enabled to CLI_BACKUP_ENABLED
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

  // Bind nested keys to ENV vars
	bindEnvs(
		"debug",
		"git.base_dir",
		"git.parallel_enabled",
		"git.max_concurrent",
		"backup.enabled",
		"backup.directory",
		"backup.strategy",
		"filter.include_patterns",
		"filter.exclude_patterns",
		"filter.skip_repos",
	)

	// Attempt to read the SPECIFIC config file (passed by default value or -c option)
	common.Logger("debug", "Attempting to read specific config file: %s", config.Properties.DefaultConfigFile)
	// Tell Viper the exact file path
	viper.SetConfigFile(config.Properties.DefaultConfigFile)
	// Attempt to read the specific file
	err := viper.ReadInConfig()
	// Handle outcome of reading the specific file
	if err == nil {
		// SUCCESS reading specific file
		common.Logger("debug", "Using config file: %v", viper.ConfigFileUsed())
	} else {
		// FAILURE reading specific file - Log details and attempt fallback
		common.Logger("debug", "Could not read specific config file '%s': %v\n", viper.ConfigFileUsed(), err)
		// Check if the error was specifically "file not found"
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			common.Logger("debug", "Specific config file not found. Falling back to search for '.updateGit.yaml' file.")
		} else {
			// A different error occurred (permissions, format, etc.)
			common.Logger("debug", "Error occurred while reading specific config file '%s'.: %v\n", viper.ConfigFileUsed(), err)
			common.Logger("debug", "Check %v file permissions and format.", viper.ConfigFileUsed())
		}

		// Configure and attempt fallback search for ".updateGit.yaml"
		common.Logger("debug", "Setting up fallback search for '.updateGit.yaml' in paths: '.', '/app'")
		viper.SetConfigName(".updateGit") // Target filename for fallback
		viper.SetConfigType("yaml")       // Expected format for fallback
		viper.AddConfigPath(".")          // Search current directory
		viper.AddConfigPath("/app")       // Search /app directory

		// Attempt to read AGAIN, performing the search defined above
		if fallbackErr := viper.ReadInConfig(); fallbackErr == nil {
			// SUCCESS reading fallback .updateGit.yaml file
			common.Logger("debug", "Using fallback config file: %v", viper.ConfigFileUsed())
		} else {
			// FAILURE reading fallback .updateGit.yaml file
			if errors.As(fallbackErr, &configFileNotFoundError) {
				// This is expected if no .updateGit.yaml file exists in the search paths
				common.Logger("debug", "No '.updateGit.yaml' config file found in search paths either. Using defaults and environment variables.")
			} else {
				// An error occurred reading the fallback .updateGit.yaml file (permissions, format?)
				common.Logger("debug", "Error reading fallback '.updateGit.yaml' file: %v\n", fallbackErr)
				common.Logger("debug", "Check %v file permissions and format.", viper.ConfigFileUsed())
			}
		}
	}

	// Read in environment variables that match Viper keys or have the CLI_ prefix
	// Read environment variables *now*. They might be overridden by config file.
	viper.AutomaticEnv()

	// Unmarshal the final configuration
	// Viper now contains the merged view: Defaults overridden by Env Vars overridden by (potentially) a loaded Config File.
	common.Logger("debug", "Unmarshaling final configuration into struct.")
	if err := viper.Unmarshal(&config.Properties); err != nil {
		common.Logger("fatal", "Error unmarshaling config: %s", err)
	}

	// Validate the populated struct
	common.Logger("debug", "Validating final configuration...")
	// Create a new validator instance
	validate := validator.New(validator.WithRequiredStructEnabled())
	// Register custom validators
	validate.RegisterValidation("noUnderscore", config.NoUnderscores)

	// Validate the Properties struct (pass by reference)
	if err := validate.Struct(&config.Properties); err != nil {
		// Check if the error is specifically validation errors
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			// Build a user-friendly error message
			errorMsg := "Configuration validation failed:\n"
			for _, fieldErr := range validationErrors {
				errorMsg += fmt.Sprintf("  - Field '%s': Failed on validation rule '%s'. Value: '%v'\n",
					fieldErr.StructNamespace(), // e.g., PropertiesStruct.DefaultEnvironment
					fieldErr.Tag(),             // e.g., "required", "oneof"
					fieldErr.Value(),           // The actual invalid value
				)
			}
			// Log as fatal error and exit
			common.Logger("fatal", "%s", errorMsg)
		} else {
			// Handle other potential errors during validation itself (less common)
			common.Logger("fatal", "An unexpected error occurred during configuration validation: %s", err)
		}
	}

	// Optional: Log the final loaded configuration for verification
	finalConfigBytes, _ := yaml.Marshal(config.Properties) // Or use json.MarshalIndent
	common.Logger("debug", "Final Configuration Loaded:\n%s\n", string(finalConfigBytes))

}

// helper to bind nested keys to ENV vars
func bindEnvs(keys ...string) {
	for _, key := range keys {
		if err := viper.BindEnv(key); err != nil {
			common.Logger("debug", "Could not bind env for key %s: %v", key, err)
		}
	}
}
