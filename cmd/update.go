package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/update"
	"github.com/spf13/cobra"
)

// Local variables (the functions are variables so the tests can replace them)
var (
	githubRepo = "aeciopires/updateGit"

	latestRelease  = update.LatestRelease
	applyUpdate    = update.ApplyUpdate
	executablePath = os.Executable

	// updateCmd represents the update command
	updateCmd = &cobra.Command{
		Use:   "update",
		Short: "Check for a new version and update the application.",
		Long: `Checks for the latest release on GitHub. If a newer version is found
for your operating system and architecture, it downloads it, verifies its SHA256
checksum and replaces the current application binary.`,
		RunE: runSelfUpdate,
	}
)

func init() {
	rootCmd.AddCommand(updateCmd) // Add update to parent root command
}

// runSelfUpdate checks the latest release and, after the confirmation of the user,
// replaces the binary. Only newer versions are installed.
func runSelfUpdate(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	common.Logger("info", "Checking for updates...")
	release, err := latestRelease(ctx, githubRepo)
	if err != nil {
		return err
	}
	common.Logger("info", "Current version: %s, Latest version on GitHub: %s", config.CLIVersion, release.TagName)

	newer, err := update.IsNewerVersion(release.TagName, config.CLIVersion)
	if err != nil {
		return fmt.Errorf("cannot compare the versions: %w", err)
	}
	if !newer {
		common.Logger("info", "You are already on the latest version: %s", config.CLIVersion)
		return nil
	}

	common.Logger("info", "A new version is available: %s. Do you want to update? (y/n): ", release.TagName)
	var response string
	if _, err := fmt.Fscanln(cmd.InOrStdin(), &response); err != nil {
		common.Logger("debug", "Could not read the answer: %v", err)
	}
	if response != "y" && response != "Y" {
		common.Logger("warning", "Update canceled.")
		return &ExitError{Code: 1}
	}

	exe, err := executablePath()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}
	common.Logger("info", "Updating to version: %s", release.TagName)
	oldPath, err := applyUpdate(ctx, release, exe)
	if err != nil {
		return err
	}
	common.Logger("info", "Update successful! The old binary is at %s. It can be removed manually.", oldPath)
	common.Logger("info", "Update complete! Please run the CLI again.")
	return nil
}
