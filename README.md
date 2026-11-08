# updateGit

[![CI](https://github.com/aeciopires/updateGit/actions/workflows/ci.yml/badge.svg)](https://github.com/aeciopires/updateGit/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://opensource.org/license/gpl-3-0)
[![Downloads](https://img.shields.io/github/downloads/aeciopires/updateGit/total?label=Downloads
)](https://somsubhra.github.io/github-release-stats/?username=aeciopires&repository=updateGit&page=1&per_page=500)
[![Releases ChangeLog](https://img.shields.io/badge/Changelog-8A2BE2
)](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md)

<!-- TOC -->

- [updateGit](#updategit)
  - [About](#about)
  - [Documentation](#documentation)
  - [Features](#features)
  - [Installation](#installation)
    - [Downloading CLI](#downloading-cli)
    - [From Source](#from-source)
    - [Using Go Install](#using-go-install)
  - [Usage](#usage)
    - [Basic Usage](#basic-usage)
    - [Output](#output)
    - [Files not committed](#files-not-committed)
    - [Parallel mode and authentication](#parallel-mode-and-authentication)
    - [Recursive search](#recursive-search)
    - [Backup](#backup)
    - [Log levels and colors](#log-levels-and-colors)
    - [Progress bar](#progress-bar)
    - [Log file](#log-file)
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

## Documentation

New to updateGit? The [docs](docs/README.md) folder explains, in plain language and with diagrams, [how it works](docs/how-it-works.md) and [how it is organized](docs/architecture.md).

## Features

- 🔍 **Auto-discovery**: finds the git repositories up to 6 directory levels (default 2) below a base directory
  - The whole discovery finishes before any update, so the total is known for the progress and the logs
  - Never walks inside a repository; ignores hidden directories, ``node_modules``, ``vendor``, etc.
  - Detects git worktrees
- 🔄 **Parallel updates**: runs up to N ``git pull`` at the same time (e.g. 15, 20 or 50) with a timeout per repository
- 🌿 **Branch information**: current branch, upstream, ahead/behind and the other local branches of each repository
- 🛟 **Your work first**: repositories with files not committed are skipped by default
- 🛡️ **Backup** (copy or git stash) before the update, with its own limit of simultaneous backups
- 🎨 **Readable output**: colors by message type, one block per repository, live progress bar and final summary
- 📝 **Log levels and log file**: minimum log level and an optional log file (``/tmp`` by default)
- ⚙️ **Configuration**: flags, environment variables and config file
- 🔁 **Self-update**: ``updateGit update`` downloads (HTTPS, GitHub only) and verifies the checksum of the new version
- 🔒 **Security**: see [SECURITY.md](SECURITY.md)

## Installation

### Downloading CLI

Get the latest version of ``updateGit`` from the [releases page](https://github.com/aeciopires/updateGit/releases) according your operating system and architecture (``updateGit-linux-amd64``, ``updateGit-linux-arm64``, ``updateGit-darwin-amd64`` or ``updateGit-darwin-arm64``).

Each release also has ``checksums.txt`` (SHA256 of the files) and ``updateGit.sbom.json`` (SBOM). The releases are built and published by GitHub Actions when a new version tag is pushed.

```bash
VERSION=0.2.0   # see https://github.com/aeciopires/updateGit/releases
PLATFORM=linux-amd64

mkdir -p $HOME/updateGit/bin && cd $HOME/updateGit
curl -fsSL -o bin/updateGit-$PLATFORM https://github.com/aeciopires/updateGit/releases/download/$VERSION/updateGit-$PLATFORM
curl -fsSL -o checksums.txt https://github.com/aeciopires/updateGit/releases/download/$VERSION/checksums.txt

# Verify the checksum (macOS: shasum -a 256 -c)
grep "bin/updateGit-$PLATFORM" checksums.txt | sha256sum -c

chmod +x bin/updateGit-$PLATFORM
mv bin/updateGit-$PLATFORM $HOME/updateGit/updateGit
```

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
updateGit -V # Show long version, with commit, build date, architeture and operating system

# Pull many git repositories using config file
updateGit pull -C $HOME/.updateGit.yaml

# Pull many git repositories in parallel (10 by default)
updateGit pull -G $HOME/git/

# Pull many git repositories processing 15, 20 or 50 repositories in parallel
updateGit pull -G $HOME/git/ -J 15
updateGit pull -G $HOME/git/ -J 20 -T 600   # timeout of 10 minutes per repository
updateGit pull -G $HOME/git/ -J 50

# Pull many git repositories in sequence (allows to type login/password)
updateGit pull -G $HOME/git/ --git-parallel-enabled=false   # or -J 1

# Also update repositories with files not committed (not recommended, see "Files not committed")
updateGit pull -G $HOME/git/ --git-skip-dirty=false -B -Y stash

# Search repositories up to 3 directory levels (maximum 6)
updateGit pull -G $HOME/git/ -R 3

# Making backup (copy) of repositories before of pull, with up to 4 simultaneous backups
updateGit pull -G $HOME/git/ -J 15 -B -Y copy -Z /tmp/git_backup --backup-max-concurrent 4

# Making backup (stash) of repositories before of pull
updateGit pull -G $HOME/git/ -J 15 -B -Y stash -Z /tmp/git_backup

# Pull many git repositories (except the filter)
updateGit pull -G $HOME/git/ -J 15 -S "old-project,experimental-stuff,github/broken-repo"

# Only warnings, errors and the summary (good for cron) and save the log in /tmp
updateGit pull -G $HOME/git/ -J 20 -L warning -O

# Debug mode
updateGit pull -D -G $HOME/git/

# Update binary
updateGit update
```

Enable debug mode using the ``-D`` (same as ``--log-level=debug``) for ``updateGit`` in any position.

Exit codes of ``updateGit pull``: ``0`` success, ``1`` at least one repository failed or timed out, ``130`` interrupted (Ctrl+C).

### Output

The repositories are discovered first. Then each repository is shown in a block when its update finishes:

```text
10:15:02 INFO  Scanning /home/user/git (levels 1-2)
10:15:02 INFO  Scan finished in 0.3s: 52 repositories in 14 directories (levels 1-2)
10:15:02 INFO  Skipped by filter: 2 · Ignored directories: node_modules · To update: 50
10:15:02 INFO  Updating in parallel with 15 workers (timeout 5m0s per repository)

✔ [1/50] updateGit                                            0.8s
   ● main → origin/main · up to date
   ○ develop, feature/parallel-pull (ahead 1)
   ↓ Already up to date.

✘ [3/50] private-repo                                         0.4s
   ● develop → origin/develop · behind 3
   ○ main
   ↓ fatal: could not read Username for 'https://github.com': terminal prompts disabled
   ⚠ Authentication required. Run sequentially (--git-parallel-enabled=false) or configure ssh-agent / a git credential helper.

════════════════════════════ Summary ════════════════════════════
  Total 50   ✔ Updated 46   ✘ Failed 1   ⏱ Timeout 1   ⏭ Skipped 2   ⌛ 41.7s
  Failed:
    ✘ private-repo    authentication required
    ⏱ huge-monorepo   timeout after 5m0s
  Skipped (no upstream): scratch, tmp-experiment
  Filtered out (--skip-repos): old-project, broken-repo
═════════════════════════════════════════════════════════════════
```

How to read a block:

- ``✔`` updated, ``✘`` failed, ``⏱`` timeout, ``⏭`` skipped, ``⊘`` canceled. ``[n/N]`` is the completion order.
- ``●`` current branch, its upstream and the state: ``up to date``, ``ahead N`` (local commits not pushed), ``behind N``, ``no upstream``, ``upstream gone`` or ``detached HEAD``.
- ``○`` other local branches (up to 8, the complete list in debug level). Branches with commits not pushed are highlighted with ``(ahead N)``.
- ``↓`` result of ``git pull``; ``⚠`` hint to fix a failure; ``▣`` backup created.
- Repositories without upstream (or in detached HEAD) are skipped instead of failing.
- Repositories with files not committed are skipped by default (see [Files not committed](#files-not-committed)).

Without colors (``--color=never``, ``NO_COLOR`` or output redirected to a file/pipe) the symbols are replaced by ``[OK]``, ``[FAIL]``, ``[TIMEOUT]``, ``[SKIP]``, ``[CANCEL]``, ``*``, ``-``, ``>`` and ``!``.

### Files not committed

To avoid losing work, **repositories whose current branch has files not committed are skipped by default**: modified, staged (``git add``), untracked (new files not ignored by ``.gitignore``) or conflicted files. The check runs before the backup and the update, and the block shows what was found:

```text
⏭ [3/50] github/wip                                          0.1s
   ● feature/login → origin/feature/login · ahead 1
   ↓ Skipped pull: files not committed in branch feature/login (2 modified, 1 untracked).
   ⚠ Commit or stash the changes, or use --git-skip-dirty=false (with --backup-enabled --backup-strategy stash to keep them).
```

- The summary lists them as ``Skipped (uncommitted changes)``. Skipped repositories are not failures (exit code ``0``).
- ``--git-skip-dirty=false`` (``git.skip_dirty: false``, ``CLI_GIT_SKIP_DIRTY=false``) restores the previous behavior: the update runs anyway. Use it with ``-B -Y stash`` to put the local changes aside first.

### Parallel mode and authentication

- By default ``updateGit pull`` runs up to 10 ``git pull`` at the same time (``-J``/``--git-max-concurrent``, 1 to 100).
- Each ``git pull`` has a timeout (``-T``/``--git-timeout``, in seconds, default 300).
- In parallel mode git runs **non-interactively**: it never asks for login/password (``GIT_TERMINAL_PROMPT=0`` and ``ssh -o BatchMode=yes``, unless you defined ``GIT_SSH_COMMAND``). Repositories that need a password fail with a hint. Options:
  - Use ``ssh-agent`` (``ssh-add``) or a [git credential helper](https://git-scm.com/docs/gitcredentials).
  - Run sequentially with ``--git-parallel-enabled=false`` or ``-J 1``: git can ask for passwords in the terminal.
- Many simultaneous SSH connections to the same host may be refused (e.g. ``Connection closed by remote host``). Reduce ``-J`` or reuse connections with SSH ControlMaster:

```text
# ~/.ssh/config
Host github.com
  ControlMaster auto
  ControlPath ~/.ssh/cm-%r@%h:%p
  ControlPersist 5m
```

### Recursive search

- The search looks up to ``-R``/``--git-max-depth`` directory levels below the base directory: from ``1`` to ``6`` (default ``2``). ``-R 1`` is the behavior of the versions before 0.2.0. Higher values search more directories: as the search never goes inside repositories, it stays fast, but avoid large base directories like ``$HOME`` with ``-R 6``.
- The search **never goes inside a directory that is already a git repository**, ignores hidden directories, the directories in ``--git-exclude-dirs`` (default ``node_modules,vendor,.terraform,.venv``) and does not follow symbolic links.
- The whole search finishes before any update starts.
- Repositories are identified by the path relative to the base directory (e.g. ``github/helm-charts``). ``--skip-repos`` accepts the directory name (``helm-charts``) or the relative path (``github/helm-charts``).

```text
~/git                         <- base directory
├── updateGit/  (.git)        <- level 1: found; the search does NOT go inside it
├── github/                   <- level 1: normal directory, the search goes inside it
│   ├── helm-charts/ (.git)   <- level 2: found
│   └── org-x/                <- level 2: normal directory, only with -R 3
│       └── api/ (.git)       <- level 3: found only with -R 3
└── node_modules/             <- ignored (--git-exclude-dirs)
```

### Backup

- **The backup is disabled by default**, so the updates do not fill the disk. Enable it with ``-B``/``--backup-enabled`` (``backup.enabled: true``, ``CLI_BACKUP_ENABLED=true``) and disable it with ``--backup-enabled=false`` (or ``-B=false``), which has priority over the config file and the environment variables.
- ``-B``/``--backup-enabled`` creates a backup before the update of each repository: ``-Y copy`` (copy of the files, except ``.git``, to ``<backup-dir>/<timestamp>/<repository>``) or ``-Y stash`` (``git stash push -u``, only when there are local changes).
- ``--backup-max-concurrent`` (default 4) limits the simultaneous backups, independent of ``-J``. For ``copy`` keep it low (disk I/O); ``stash`` is cheap and can use a higher value.
- A backup failure is reported in the block and the update of the repository continues.

### Log levels and colors

- ``-L``/``--log-level``: minimum level shown: ``debug``, ``info`` (default), ``warning`` or ``error``. With ``warning`` or ``error`` only the failures and the summary are shown.
- ``-D``/``--debug`` is the same as ``--log-level=debug`` and has priority over ``--log-level``. In debug the timestamp is complete, the errors show the file and line, and the blocks show the complete ``git pull`` output and all branches.
- ``--color``: ``auto`` (default: colors only in a terminal and when the [NO_COLOR](https://no-color.org) variable is not defined), ``always`` or ``never``.

```text
10:15:02 DEBUG Command 'git' found in system PATH.          (gray)
10:15:02 INFO  Found 52 repositories                        (cyan label)
10:15:02 WARN  Skipping directory: permission denied        (yellow)
10:15:09 ERROR Failed to create backup: disk full           (red)
10:15:09 FATAL Directory does not exist: /home/user/gti     (white on red)
```

### Progress bar

In a terminal and with more than one worker, a live progress bar is shown on the last line:

```text
⠼ [████████████░░░░░░░░░░░░] 21/50  42%  ✔ 19 ✘ 1 ⏱ 0 ⏭ 1 │ running 15: infra-live, docs-site, api +12 │ backup 2/4 │ 00:12 · ETA ~00:17
```

- ``--progress``: ``auto`` (default), ``always`` or ``never``.
- It is disabled in sequential mode (git may ask for passwords in the same terminal) and when the output is not a terminal (cron, pipe, file). It is never written to the log file.
- During the discovery a spinner shows the directories visited and the repositories found.

### Log file

- ``-O``/``--log-file-enabled`` saves the log to ``<log-file-dir>/updateGit_YYYY-MM-DD_HH-MM-SS.log`` (start time of the execution), e.g. ``/tmp/updateGit_2026-11-08_10-15-02.log``.
- ``--log-file-dir`` (default ``/tmp``) and ``--log-file-level`` (default: same as ``--log-level``; e.g. ``debug`` in the file and ``info`` in the terminal).
- The file is created with permission ``0600``, without colors, with a header (version, command, configuration) and a footer (duration and exit code), and contains the blocks and the summary. The path is shown at the end.
- Credentials in URLs (``https://user:token@host``) are masked as ``https://***@host``.
- There is no automatic rotation: ``/tmp`` is cleaned by the system. Use a persistent directory if necessary (e.g. ``--log-file-dir ~/.local/state/updateGit``).

Example with cron (every day at 8:00):

```text
0 8 * * * $HOME/updateGit/updateGit pull -G $HOME/git -J 20 -L warning -O --log-file-dir $HOME/.local/state/updateGit
```

## Configuration

### Precedence order

> ATTENTION!!! Order of precedence (since 0.2.0):
>
> 1) CLI options (flags) have priority over everything.
>
> 2) Environment variables (starting with ``CLI_``) have priority over the configuration file.
>
> 3) The configuration file (``-C``/``--config-file`` or ``CLI_CONFIG_FILE``; if it does not exist, ``.updateGit.yaml`` in the current directory or in ``/app``) has priority over the defaults.
>
> 4) The default values of ``updateGit`` are defined in the ``internal/config/config.go`` file.

### Configuration File

Create a configuration file at `~/.updateGit.yaml` (see the complete example in [.updateGit.yaml](.updateGit.yaml)):

```yaml
# Git settings
git:
  # Base directory for git repositories
  base_dir: "./git_repos"
  # Enable parallel processing of git repositories
  parallel_enabled: true
  # Maximum number of concurrent git repository updates (1-100)
  max_concurrent: 10
  # Timeout (in seconds) for each "git pull"
  timeout: 300
  # Directory levels searched for repositories: 1 to 6 (default 2)
  max_depth: 2
  # Directory names ignored by the search
  exclude_dirs:
    - "node_modules"
    - "vendor"
    - ".terraform"
    - ".venv"
  # Skip repositories with files not committed (avoids losing work)
  skip_dirty: true

# Backup settings
backup:
  # Enable backup before updates (default false: backups use disk space)
  enabled: false
  # Backup directory (relative or absolute path)
  directory: "./git_backups"
  # Backup strategy: "copy" or "stash"
  strategy: "copy"
  # Maximum number of simultaneous backups (independent of git.max_concurrent)
  max_concurrent: 4

# Repository filtering
filter:
  # Repositories to skip: directory name or path relative to git.base_dir
  skip_repos:
    - "old-project"
    - "experimental-stuff"
    - "broken-repo"

# Log and output settings
log:
  # Minimum log level: debug, info, warning, error
  level: "info"
  # Colored output: auto, always, never
  color: "auto"
  # Live progress bar: auto, always, never
  progress: "auto"
  file:
    # Save the log to <directory>/updateGit_YYYY-MM-DD_HH-MM-SS.log
    enabled: false
    directory: "/tmp"
    # Empty = same as log.level
    level: ""
```

### Environment Variables

You can also configure the tool using environment variables (prefix ``CLI_`` and the configuration key in uppercase with ``.`` replaced by ``_``):

```bash
# Examples of environment variable overrides default options:
# Pay attention to precendence order explained in before section
export CLI_DEBUG=true;
export CLI_GIT_BASE_DIR="./git_repos2";
export CLI_GIT_PARALLEL_ENABLED=false;
export CLI_GIT_MAX_CONCURRENT=11;
export CLI_GIT_TIMEOUT=600;
export CLI_GIT_MAX_DEPTH=3;
export CLI_GIT_EXCLUDE_DIRS="node_modules,vendor";
export CLI_GIT_SKIP_DIRTY=false;
export CLI_BACKUP_ENABLED=false;
export CLI_BACKUP_DIRECTORY="/path/to/backup";
export CLI_BACKUP_STRATEGY="copy";
export CLI_BACKUP_MAX_CONCURRENT=4;
export CLI_FILTER_SKIP_REPOS="old-project,experimental-stuff,broken-repo";
export CLI_LOG_LEVEL="warning";
export CLI_LOG_COLOR="never";
export CLI_LOG_PROGRESS="never";
export CLI_LOG_FILE_ENABLED=true;
export CLI_LOG_FILE_DIRECTORY="/tmp";
export CLI_LOG_FILE_LEVEL="debug";
export CLI_CONFIG_FILE=".updateGit.yaml";

# Unset environement variables
unset CLI_DEBUG CLI_GIT_BASE_DIR CLI_GIT_PARALLEL_ENABLED CLI_GIT_MAX_CONCURRENT CLI_GIT_TIMEOUT \
  CLI_GIT_MAX_DEPTH CLI_GIT_EXCLUDE_DIRS CLI_GIT_SKIP_DIRTY CLI_BACKUP_ENABLED CLI_BACKUP_DIRECTORY CLI_BACKUP_STRATEGY \
  CLI_BACKUP_MAX_CONCURRENT CLI_FILTER_SKIP_REPOS CLI_LOG_LEVEL CLI_LOG_COLOR CLI_LOG_PROGRESS \
  CLI_LOG_FILE_ENABLED CLI_LOG_FILE_DIRECTORY CLI_LOG_FILE_LEVEL CLI_CONFIG_FILE;
```

## Development

### Prerequisites

- Go 1.27 or later (version pinned in [mise.toml](mise.toml))
- [mise](https://mise.jdx.dev) (recommended, installs Go, Trivy and the development tools with the pinned versions)
- Git command-line tool
- Make (optional, for using Makefile)
- A C compiler (optional, ``cc``/``gcc``, used by the Go race detector in ``make test``)

### Building from Source

```bash
# Clone the repository
git clone https://github.com/aeciopires/updateGit.git
cd updateGit

# Check if the software dependencies are installed
make check-deps

# Install the pinned tool versions declared in mise.toml and download the Go modules
make install-deps

# Install the git hooks (pre-commit and pre-push)
make install-hooks

# Build the binary
go build -o updateGit

# Run with debug mode
./updateGit -h
```

### Development Commands

Run ``make help`` (or just ``make``) to list all available targets.

```bash
# Check / install the software dependencies and the git hooks
make check-deps
make install-deps
make install-hooks

# Fast checks used during development: gofmt, go.mod, go vet, golangci-lint and unit tests (with race detector)
make check

# All checks (used by the pre-push hook and the CI): check + coverage, govulncheck, trivy, actionlint, build and artifacts
make check-all

# Individual checks
make lint          # golangci-lint (make lint-fix applies the automatic fixes)
make mod-check     # go.mod/go.sum tidy and verified
make vuln          # govulncheck
make security      # trivy (dependencies, secrets and misconfigurations)
make lint-workflows
make lint-diagrams # Mermaid diagrams of the Markdown files (Docker image minlag/mermaid-cli)

# Run the unit tests
make test          # with race detector and coverage
make test-short    # fast tests only (skips the load tests)
make test-verbose

# Show the test coverage per function, generate the HTML report or check the minimum (COVERAGE_MIN=90)
make coverage
make coverage-html
make coverage-check

# Format the code
make fmt

# Build for all platforms (binaries, SBOM and checksums in bin/) and verify the artifacts
make build
make verify-artifacts

# Clean build artifacts and coverage reports
make clean

# To see documentation of packages and files
go doc -C internal/getinfo/ -all
go doc -C internal/config/ -all
go doc -C internal/common/ -all
go doc -C internal/backup/ -all
go doc -C internal/git/ -all
go doc -C internal/filter/ -all
go doc -C internal/output/ -all
```

## Initial mainteners

- [Aécio Pires](https://www.linkedin.com/in/aeciopires/?locale=en_US)

## Learning Golang

Why Golang?

Because it is easier to distribute a binary compatible with different operating systems containing all dependencies.
Golang is one of the many languages used by SRE and DevOps teams.

See [LEARNING_GOLANG.md](LEARNING_GOLANG.md) file.
