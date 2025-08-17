# updateGit

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://opensource.org/license/gpl-3-0)
[![Downloads](https://img.shields.io/github/downloads/aeciopires/updateGit/total?label=Downloads
)](https://somsubhra.github.io/github-release-stats/?username=aeciopires&repository=updateGit&page=1&per_page=500)
[![Releases ChangeLog](https://img.shields.io/badge/Changelog-8A2BE2
)](https://github.com/aeciopires/updateGit/blob/0.1.0/app/CHANGELOG.md)

<!-- TOC -->

- [updateGit](#updategit)
  - [About](#about)
  - [Features](#features)
  - [Installation](#installation)
    - [Downloading CLI](#downloading-cli)
    - [From Source](#from-source)
    - [Using Go Install](#using-go-install)
  - [Usage](#usage)
    - [Basic Usage](#basic-usage)
  - [Configuration](#configuration)
    - [Precedence order](#precedence-order)
    - [Configuration File](#configuration-file)
    - [Environment Variables](#environment-variables)
  - [Development](#development)
    - [Prerequisites](#prerequisites)
    - [Building from Source](#building-from-source)
    - [Development Commands](#development-commands)
  - [Initial mainteners](#initial-mainteners)
  - [Learning Golang](#learning-golang)

<!-- TOC -->

## About

**updateGit** is a Golang CLI tool that automatically discovers and updates all git repositories within a specified base directory.

## Features

- 🔍 **Auto-discovery**: Automatically finds all git repositories in a directory
  - Core functionality for updating multiple git repositories:
    - Automatic discovery of git repositories in specified directory
    - Batch execution of `git pull` command on all discovered repositories
    - Git repository metadata detection:
      - Current branch identification
      - Repository validation
    - Repository filtering and exclusion options
- 🔄 **Batch updates**: Updates multiple repositories with a single command
  - Parallel repository processing
- ⚙️ **Configuration**: Support for config files and environment variables
- 🛡️ **Error handling**: Comprehensive error handling with detailed messages

## Installation

### Downloading CLI

Get the latest version of ``updateGit`` from https://github.com/aeciopires/updateGit/releases according your operating system and architecture.
Save the binary in ``$HOME/updateGit/`` directory (create it if necessary) and add permission to execute.

See [README.md#usage](README.md#usage) section to more informations.

### From Source

```bash
# Clone the repository
git clone https://github.com/aeciopires/updateGit.git
cd updateGit

# Build the binary
go build -o updateGit

# Optional: Install to system PATH
sudo mv updateGit /usr/local/bin/
```

### Using Go Install

```bash
go install github.com/aeciopires/updateGit@latest
```

## Usage

### Basic Usage

```bash
# Get help of command
updateGit -h # Show global help
updateGit pull -h # show help about pull command
updateGit update -h # show help about update command
updateGit -v # Show short version
updateGit -V # Show long version, with architeture and operating system

# Pull many git repositories using config file without debug mode
updateGit pull -C $HOME/.updateGit.yaml

# Pull many git repositories in sequence using debug mode
updateGit pull -D -G $HOME/git/

# Pull many git repositories processing 15 repositories in parallel using debug mode
updateGit pull -D -G $HOME/git/ -P -J 15

# Making backup (copy) of repositories before of pull many git repositories processing 15 repositories in parallel using debug mode
updateGit pull -D -G $HOME/git/ -J 15 -P -B -Y copy -Z /tmp/git_backup

# Making backup (stash) of repositories before of pull many git repositories processing 15 repositories in parallel using debug mode
updateGit pull -D -G $HOME/git/ -J 15 -P -B -Y stash -Z /tmp/git_backup

# Pull many git repositories (except the filter)
updateGit pull -D -G $HOME/git/ -P -J 15 -S "old-project,experimental-stuff,broken-repo"

# Update binary without debug mode
updateGit update
```

Enable debug mode using the ``-D`` for ``updateGit`` in any position.

## Configuration

### Precedence order

> ATTENTION!!! Order of precedence:
>
> 1) Configuration files have priority over environment variables and CLI options.
>
> 2) If no custom path with customization file is passed, the ``.updateGit.yaml`` or ``/app/.updaGit.yaml`` file will be considered and will have priority over CLI options.
>
> 3) If the ``.updateGit.yaml`` or ``/app/.updaGit.yaml`` file does not exist, environment variables (starting with ``CLI_``) will be given priority over CLI options.
>
> 4) If environment variables (starting with ``CLI_``) do not exist, CLI options will be considered.
>
> 5) If no CLI options are passed and there is no error message related to this, the default values ​​of ``updateGit`` defined in the ``internal/config/config.go`` file will be considered.

### Configuration File

Create a configuration file at `~/.updateGit.yaml`:

```yaml
# Git settings
git:
  # Base directory for git repositories
  base_dir: "./git_repos"
  # Enable parallel processing of git repositories
  parallel_enabled: true
  # Maximum number of concurrent git repository updates
  max_concurrent: 5

# Backup settings
backup:
  # Enable backup before updates
  enabled: true
  # Backup directory (relative or absolute path)
  directory: "./git_backups"
  # Backup strategy: "copy" or "stash"
  strategy: "copy"

# Repository filtering
filter:
  # Specific repositories to skip (exact names)
  skip_repos:
    - "old-project"
    - "experimental-stuff"
    - "broken-repo"
```

### Environment Variables

You can also configure the tool using environment variables:

```bash
# Examples of environment variable overrides default options:
# Pay attention to precendence order explained in before section
export CLI_DEBUG=true;
export CLI_GIT_BASE_DIR="./git_repos2";
export CLI_GIT_PARALLEL_ENABLED=false;
export CLI_GIT_MAX_CONCURRENT=11;
export CLI_BACKUP_ENABLED=true;
export CLI_BACKUP_DIRECTORY="/path/to/backup";
export CLI_BACKUP_STRATEGY="copy";
export CLI_FILTER_SKIP_REPOS="old-project,experimental-stuff,broken-repo";
export CLI_CONFIG_FILE=".updateGit.yaml";

# Unset environement variables
unset CLI_DEBUG;
unset CLI_GIT_BASE_DIR;
unset CLI_GIT_PARALLEL_ENABLED;
unset CLI_GIT_MAX_CONCURRENT;
unset CLI_BACKUP_ENABLED;
unset CLI_BACKUP_DIRECTORY;
unset CLI_BACKUP_STRATEGY;
unset CLI_FILTER_SKIP_REPOS;
unset CLI_CONFIG_FILE;
```

## Development

### Prerequisites

- Go 1.25 or later
- Git command-line tool
- Make (optional, for using Makefile)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/aeciopires/updateGit.git
cd updateGit

# Download dependencies
go mod download

# Build the binary
go build -o updateGit

# Run with debug mode
./updateGit -h
```

### Development Commands

```bash
# Run tests with coverage
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Build
make build

# To see documentation of packages and files
go doc -C internal/getinfo/ -all
go doc -C internal/config/ -all
go doc -C internal/common/ -all
go doc -C internal/backup/ -all
go doc -C internal/git/ -all
go doc -C internal/filter/ -all
```

## Initial mainteners

- [Aécio Pires](https://www.linkedin.com/in/aeciopires/?locale=en_US)

## Learning Golang

Why Golang?

Because it is easier to distribute a binary compatible with different operating systems containing all dependencies.
Golang is one of the many languages used by SRE and DevOps teams.

See [LEARNING_GOLANG.md](LEARNING_GOLANG.md) file.
