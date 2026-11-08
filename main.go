// updateGit discovers local git repositories and updates them with "git pull".
package main

import (
	"github.com/aeciopires/updateGit/cmd"
	"github.com/aeciopires/updateGit/internal/common"
	"github.com/aeciopires/updateGit/internal/config"
	"github.com/aeciopires/updateGit/internal/getinfo"
)

func main() {
	getinfo.CheckOperatingSystem()
	common.CheckCommandsAvailable(config.CommandsToCheck)
	cmd.Execute()
}
