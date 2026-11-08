# Changelog

<!-- TOC -->

- [Changelog](#changelog)
- [0.2.0](#020)
  - [Added](#added)
  - [Changed](#changed)
  - [Fixed](#fixed)
- [0.1.1](#011)
- [0.1.0](#010)

<!-- TOC -->

# 0.2.0

## Added

- Parallel update of repositories: ``--git-max-concurrent``/``-J`` (1-100, default 10) now really runs N ``git pull`` at the same time.
- ``--git-timeout``/``-T`` (``git.timeout``, ``CLI_GIT_TIMEOUT``): timeout in seconds for each ``git pull`` (default 300).
- ``--log-level``/``-L`` (``log.level``, ``CLI_LOG_LEVEL``): minimum log level (debug, info, warning, error).
- ``--color`` (``log.color``, ``CLI_LOG_COLOR``): colored output (auto, always, never); respects ``NO_COLOR``.
- Output per repository showing the current branch (upstream, ahead/behind), the other local branches and the pull result, with hints for the common failures (authentication, local changes, diverged branches, connection).
- Final summary with updated, failed, timed out, skipped and canceled repositories.
- Exit codes of ``updateGit pull``: 0 (success), 1 (failures or timeouts) and 130 (interrupted with Ctrl+C, with a partial summary).
- ``--log-file-enabled``/``-O``, ``--log-file-dir`` (default ``/tmp``) and ``--log-file-level``: save the log to ``updateGit_YYYY-MM-DD_HH-MM-SS.log`` (permission 0600, without colors, with header, blocks, summary and footer).
- Credentials in URLs (``https://user:token@host``) are masked in the logs.
- Live progress bar (``--progress``: auto, always, never) with counters, running repositories, backups and ETA, and a spinner during the discovery.
- Search for repositories up to 6 directory levels (``--git-max-depth``/``-R``: 1 to 6, default 2) without walking inside repositories, and ``--git-exclude-dirs`` (default ``node_modules,vendor,.terraform,.venv``); git worktrees are detected. The discovery finishes before any update starts, so the total is known for the progress and the logs.
- ``--skip-repos`` accepts the directory name or the path relative to the base directory.
- **Repositories with files not committed (modified, staged, untracked or conflicted) in the current branch are skipped by default** to avoid losing work: ``--git-skip-dirty`` (``git.skip_dirty``, ``CLI_GIT_SKIP_DIRTY``, default true). The block shows the files found (e.g. ``2 modified, 1 untracked``).
- ``--backup-max-concurrent`` (``backup.max_concurrent``, default 4): limit of simultaneous backups, independent of ``--git-max-concurrent``.
- GitHub Actions workflow (``.github/workflows/release.yml``) that builds the binaries (linux/darwin, amd64/arm64), SBOM and checksums and publishes a GitHub release (title = tag, default message pointing to this file) when a semantic version tag is pushed. Pre-releases (``-rc.N``) are supported.
- GitHub Actions CI for pull requests and ``main`` (Linux and macOS) and Dependabot for actions and Go modules.
- ``updateGit -V`` shows the commit and build date.
- Development checks in the ``Makefile``: ``lint``/``lint-fix`` (golangci-lint, ``.golangci.yml``), ``mod-check``, ``test-short``, ``coverage-check``, ``vuln`` (govulncheck), ``security`` (trivy), ``lint-workflows`` (actionlint), ``verify-artifacts``, ``check-all`` and ``install-hooks`` (pre-commit and pre-push hooks, ``.pre-commit-config.yaml``).
- Development tools pinned in ``mise.toml``: golangci-lint, govulncheck, actionlint, shellcheck, pre-commit (and Python for pre-commit).
- ``make lint-diagrams``: validates the Mermaid diagrams of the Markdown files with the Docker image ``minlag/mermaid-cli`` (part of ``check-all``).
- ``SECURITY.md`` with the security policy and how to report vulnerabilities.
- ``docs/`` folder explaining, for non-technical readers and with Mermaid diagrams, how updateGit works and its architecture.
- Unit tests for the parallel runner, discovery, branch information, logger, log file, output, progress bar, configuration precedence and exit codes (total coverage 95%; ``coverage-check`` requires 90%).

## Changed

- **Behavior change:** the backup is disabled by default (``backup.enabled: false``, also in the sample ``.updateGit.yaml``) to avoid filling the disk. Enable it with ``-B``/``--backup-enabled`` and disable it with ``--backup-enabled=false`` (priority over the config file and ``CLI_BACKUP_ENABLED``).
- **Behavior change:** ``updateGit pull`` runs in parallel by default (``git.parallel_enabled: true``, 10 workers). Use ``--git-parallel-enabled=false`` or ``-J 1`` for the previous sequential behavior.
- **Behavior change:** repositories in the second directory level are found by default (``--git-max-depth=2``). Use ``-R 1`` for the previous behavior.
- **Behavior change:** the precedence of the configuration is now flags > environment variables > config file > defaults (before, the config file and the environment variables overrode the flags).
- In parallel mode git runs non-interactively (no password prompts). The sequential mode keeps the prompts.
- Repositories whose current branch has no upstream, whose upstream was deleted or in detached HEAD are skipped instead of failing.
- Repository names are shown as the path relative to the base directory (same as before for first level repositories).
- A permission error while scanning a subdirectory is now a warning instead of a fatal error.
- Log messages are colored by level, have a fixed width label and the stack trace is shown only in debug level. The logger was rewritten without ``zerolog`` (removed dependency).
- The ``git pull`` output is captured and shown in the block of the repository (completely in debug level or on failures).
- ``make check`` now also runs ``mod-check`` and ``lint``; ``check-deps`` also checks the development tools; ``build`` cleans ``bin/`` before building and supports ``shasum`` on macOS.
- Release process: binaries are no longer uploaded manually (see CONTRIBUTING.md).
- ``updateGit update`` is safer: HTTPS-only downloads from GitHub hosts (also for redirects), timeouts, a size limit, ``User-Agent``/GitHub API headers, validation of the SHA256 format, and it only installs **newer** versions (semantic version comparison, pre-releases included). Errors are returned to the command (exit code 1) instead of ending the process inside the library.
- Stricter linters (``noctx``, ``nilerr``, ``nolintlint``, ``errname``, ``durationcheck``, ``usestdlibvars``): every git command and HTTP request has a context with timeout, including the read-only git commands of each repository and the backup commands.
- The GitHub Actions checkout no longer persists the token in ``.git/config``.

## Fixed

- ``--git-parallel-enabled`` and ``--git-max-concurrent`` were ignored.
- The timeout of git operations was ignored.
- ``CLI_DEBUG`` and the ``debug`` key of the config file had no effect.
- ``updateGit update`` replaced the binary by the latest release even when it was older than the installed version.
- Data race in the logger when called from multiple goroutines.
- Build ``LDFLAGS`` referencing nonexistent variables.
- Unchecked errors reported by the linter (restore of the old binary in ``updateGit update``, help output and others).

# 0.1.1

- Replaced ``asdf`` (``.tool-versions``) with ``mise`` (``mise.toml``) to manage tool versions.
- Updated Go to 1.27.3 (``go.mod`` now requires Go 1.27+) and Trivy to 0.76.2.
- Updated Go module dependencies (``cobra`` v1.10.2, ``viper`` v1.21.0, ``zerolog`` v1.35.1, ``golang.org/x/*`` and others).
- Fixed error messages printed as ``%!w(...)`` in fatal logs of the ``pull`` and ``update`` commands.
- Renamed ``AGENTS.md`` to ``CLAUDE.md``.
- Added the ``help`` target to the ``Makefile`` (default target) listing the available targets, documented by ``## `` comments.
- Added the ``check-deps`` target to the ``Makefile`` to check if the software dependencies (git, go, trivy, mise) are installed, showing an error when any is missing and a warning when the version differs from ``mise.toml``. ``requirements`` is now an alias to ``check-deps``.
- Added the ``install-deps`` target to the ``Makefile`` to install go and trivy with mise and download the Go modules.
- Removed ``wget`` from the required software (it was not used).
- Added unit tests for the packages ``backup``, ``common``, ``config``, ``filter``, ``git`` and ``update``.
- Added the targets ``test``, ``test-verbose``, ``coverage``, ``coverage-html``, ``vet``, ``fmt``, ``fmt-check`` and ``check`` to the ``Makefile``. ``clean`` also removes the coverage reports.

# 0.1.0

- First version of the ``updateGit``.
- Initial features:
  - [Bash script](https://gist.githubusercontent.com/aeciopires/2457cccebb9f30fe66ba1d67ae617ee9/raw/8d088c6fadb8a4397b5ff2c7d6a36980f46d40ae/updateGit.sh) completely rewrited in Go programming language
  - Core functionality for updating multiple git repositories:
    - Automatic discovery of git repositories in specified directory
    - Batch execution of `git pull` command on all discovered repositories
    - Git repository metadata detection:
      - Current branch identification
      - Repository validation
  - Repository skip, filtering and exclusion options
