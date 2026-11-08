# CLAUDE.md

This file guides Claude Code (and other AI agents) when working in this repository.

## Purpose

`updateGit` is a Go CLI tool that discovers local Git repositories under a base directory and updates them with `git pull` in parallel, with optional backup, filtering, log file and a live progress bar.

## Key files

- `main.go` - entry point, validates prerequisites and executes the CLI.
- `cmd/root.go` - Cobra root command, global flags, configuration loading (`loadConfig`: Viper with flags bound by `flagKeys`), log file (`openLogFile`/`closeLogFile`) and exit codes (`ExitError`).
- `cmd/pull.go` - `pull` subcommand: discovery -> filter -> parallel update (`git.Run`) -> summary. Exit codes 0/1/130.
- `cmd/update.go` - `update` subcommand for self-update/version checks.
- `internal/config/config.go` - `Config` (mapstructure/yaml keys = config keys), `DefaultConfig`, `CLIVersion`, `CLICommit`/`CLIBuildDate` (LDFLAGS), `SemverRegex`.
- `internal/common/logger.go` - leveled, colored logger (`InitLogger`, `Logger`, `MaskCredentials`, `SyncWriter`).
- `internal/common/logfile.go` - log file name and creation (`/tmp/updateGit_YYYY-MM-DD_HH-MM-SS.log`, permission 0600).
- `internal/git/git.go` - `Repository`, `IsGitRepository` (`.git` dir or `gitdir:` file), legacy helpers.
- `internal/git/discovery.go` - `DiscoverRepositories`: up to 6 levels (`MaxSearchDepth`), never inside repositories.
- `internal/git/branches.go` - `GetRepoBranches` (single `git for-each-ref`).
- `internal/git/worktree.go` - `GetWorkingTreeStatus` (files not committed: staged, modified, untracked, conflicted).
- `internal/git/runner.go` - `Run`: worker pool, timeout, backup semaphore, `RepoResult`, `Event`, `Sink`, `SummarizePullOutput`, failure hints.
- `internal/output/render.go` - blocks per repository, summary and `Printer` (implements `git.Sink`).
- `internal/output/progress.go` / `terminal.go` - progress bar, discovery spinner and the shared `Terminal` writer.
- `internal/filter/filter.go` - skip list (directory name or relative path).
- `internal/backup/backup.go` - copy/stash backups.
- `internal/update/update.go` - `LatestRelease`, `ApplyUpdate`, `CompareVersions`/`IsNewerVersion`, `AssetName`, checksum validation and binary replacement (returns errors; HTTPS + host allowlist, timeouts, size limit).
- `internal/getinfo/get_info.go` - version/OS info; only Linux and macOS are supported.
- `mise.toml` - pinned tool versions (Go, Trivy, golangci-lint, govulncheck, actionlint, shellcheck, pre-commit, Python for pre-commit).
- `Makefile` - dependencies, checks, tests, build and release targets (`make help`).
- `.golangci.yml`, `.pre-commit-config.yaml` - linter and git hooks configuration.
- `.github/workflows/ci.yml` - CI for PRs/main (`make check-all` on Linux, `make check` on macOS).
- `.github/workflows/release.yml` - builds and publishes the release on semver tags.
- `plans/` - implementation plans (e.g. `plans/parallel-pull-and-output.md`).
- `docs/` - non-technical explanation of how updateGit works and its architecture, with Mermaid diagrams (`docs/README.md` is the index). Keep it updated when the flow or the components change.
- `README.md`, `CONTRIBUTING.md` and `SECURITY.md` - usage, development, release, contribution and security guidance.

## Development workflow

- Tool versions are managed with **mise** (not asdf), pinned in `mise.toml`. Go 1.27.3 (`go.mod` requires Go 1.27+).
- `make install-deps` (mise install + go mod download) and `make install-hooks` (pre-commit and pre-push hooks).
- **Run `make check` before considering any change done**, and `make check-all` before opening a PR.
- `make help` (or just `make`) lists the targets. Every new Makefile target must have a `## Description` comment after its name (use `##@ Section` lines to group targets).

### Makefile targets

- `make check-deps` - required tools (git, go, trivy) fail with `[MISSING]`/`[ERROR]`; development tools (golangci-lint, govulncheck, actionlint, shellcheck, pre-commit) only warn. Versions are compared with `mise.toml`. `make requirements` is an alias.
- `make check` - `fmt-check`, `mod-check`, `vet`, `lint`, `test` (race detector when `cc` exists; `make test RACE_FLAG=` disables it).
- `make check-all` - `check` + `coverage-check` (`COVERAGE_MIN`, default 90) + `vuln` (govulncheck) + `security` (trivy, HIGH/CRITICAL) + `lint-workflows` (actionlint) + `lint-diagrams` (Mermaid via Docker image `minlag/mermaid-cli`, pinned in `MERMAID_IMAGE`; skipped without Docker) + `build` + `verify-artifacts`.
- `make test-short` - skips the load tests (`testing.Short()`); used by the pre-commit hook.
- `make lint` / `make lint-fix`, `make mod-check`, `make vuln`, `make security`, `make fmt`, `make fmt-check`.
- `make coverage`, `make coverage-html`, `make coverage-check`.
- `make build` - binaries for linux/darwin amd64/arm64, SBOM and `checksums.txt` in `bin/` (git-ignored). `make verify-artifacts` checks them.
- `make clean` - removes `bin/` and coverage reports.

## Code and tests conventions

- `common.Logger` is printf-like but is **not** `fmt.Errorf`: use `%v` (never `%w`). `go vet`/golangci-lint check it.
- The logger is configured with `common.InitLogger` (once, after the configuration is loaded; again when the log file is opened). Never reconfigure it inside `Logger`.
- Every write to the standard output goes through `cmd.terminal` (`output.Terminal`), which erases and redraws the progress bar. Do not use `fmt.Print` in the pull flow.
- **Workers never write to the terminal nor call `Logger("fatal")`**: `processRepo` returns a `RepoResult`; only the `Sink` (`output.Printer`), called from the goroutine of `git.Run`, prints it. Only that goroutine and the ticker draw the progress bar (both through the `Terminal` mutex).
- `common.Logger("fatal", ...)` exits the process (after the exit hook that closes the log file), so tests only cover non-fatal paths; `CheckForUpdate`/`ApplyUpdate` are not unit tested.
- The git output is parsed: git runs with `LC_ALL=C`. In parallel mode (`workers > 1`) git runs with `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND="ssh -o BatchMode=yes"` (only if the user did not define it). Sequential mode (`workers == 1`) keeps stdin for password prompts and has no progress bar.
- Discovery (`git.DiscoverRepositories`) finishes before any update; `Repository.Name` is the path relative to the base directory (`/` separator); the search never goes inside a repository, ignores hidden dirs and `exclude_dirs`, and does not follow symlinks. Max depth 6 (`git.MaxSearchDepth` and the `max=6` validation in `config.Config` must match), default 2.
- Repositories with files not committed are skipped by default (`git.skip_dirty`, `RunConfig.SkipDirty`; the zero value of `RunConfig` does NOT skip). The check runs after the branch checks and before the backup and the pull.
- Every external command (`exec.CommandContext`) and HTTP request (`http.NewRequestWithContext`) has a context with timeout (enforced by the `noctx` linter); the read-only git commands of a repository share `git.timeout`.
- Packages in `internal/` return errors; only `cmd` decides how to exit (`ExitError`, `exitFunc`). `update` must keep downloads HTTPS-only from GitHub hosts (`allowedDownloadHosts`), with size limit and checksum verification before replacing the binary, and never install an older version.
- Every `//nolint` must name the linter and explain why (`nolintlint`).
- Backups: semaphore of `backup.max_concurrent` acquired only around `CreateBackup`, always with `select` on `ctx.Done()`. A backup failure does not stop the pull.
- Tests: standard library only (no testify), next to the code (`*_test.go`). Git tests isolate the user config (`GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, fixed author) and use `t.TempDir()` repositories with a local `--bare` remote (no network); see `setupGitEnv`, `newOrigin`, `cloneMany` in `internal/git/*_test.go`.
- `cmd` tests run `Execute` in-process (`runCLI` in `cmd/execute_test.go`: replaces `terminal` and `exitFunc` and resets the flags).
- Concurrency tests replace `git.pullFunc` (fake pull with atomic counters) and must pass with `-race`. Load tests (50 repositories) call `t.Skip` when `testing.Short()`.
- Golden files in `internal/output/testdata` are updated with `go test ./internal/output -update` (review the diff).
- Update dependencies with `go get -u ./... && go mod tidy`, then run `make check` and `make build`.

## CLI behavior

- `updateGit pull` updates repositories under `--git-base-dir`/`-G`. Exit codes: 0 success, 1 failures/timeouts, 130 interrupted.
- Parallelism: `-P/--git-parallel-enabled` (default true; disable with `=false`), `-J/--git-max-concurrent` (1-100, default 10), `-T/--git-timeout` (seconds, default 300).
- Search: `-R/--git-max-depth` (1-6, default 2), `--git-exclude-dirs`.
- Safety: `--git-skip-dirty` (default true; disable with `=false`) skips repositories with files not committed.
- Backup: disabled by default (`backup.enabled: false`, saves disk); `-B/--backup-enabled` enables it and `--backup-enabled=false` disables it. `-Z`, `-Y copy|stash`, `--backup-max-concurrent` (1-100, default 4).
- Filter: `-S/--skip-repos` (directory name or relative path).
- Log/output: `-L/--log-level` (debug, info, warning, error), `-D/--debug` (= debug, has priority), `--color` (auto, always, never; respects `NO_COLOR`), `--progress` (auto, always, never), `-O/--log-file-enabled`, `--log-file-dir` (default `/tmp`), `--log-file-level`.
- `updateGit update` checks GitHub releases and, after confirmation, self-updates the binary (only newer versions; exit 1 if canceled or on errors). Tests replace `latestRelease`, `applyUpdate` and `executablePath` in `cmd/update.go`. `-v`/`-V` show the version (`-V` also commit, build date, OS and arch).

## Configuration conventions

- Precedence: **flags > environment variables (`CLI_*`) > config file > defaults** (`loadConfig`; flags are bound to viper with `BindPFlag`).
- Default config file: `.updateGit.yaml` (`-C` or `CLI_CONFIG_FILE`; fallback `.updateGit.yaml` in `.` or `/app`).
- Environment variables use prefix `CLI_` and map `.`/`-` to `_` (e.g. `CLI_GIT_MAX_CONCURRENT`, `CLI_LOG_FILE_DIRECTORY`). Lists accept commas (`CLI_FILTER_SKIP_REPOS="a,b"`).
- To add an option: field with `mapstructure`/`yaml`/`validate` tags in `config.Config`, default in `DefaultConfig`, flag in `defineFlags`, entry in `flagKeys`, `v.SetDefault` in `setDefaults`, docs (README, `.updateGit.yaml`) and tests (`cmd/cmd_test.go`, `internal/config/config_test.go`).

## Release and patch conventions

- Update `CHANGELOG.md` (and its TOC) when making user-visible changes.
- Bump `config.CLIVersion` in `internal/config/config.go` for releases, following semantic versioning (`MAJOR.MINOR.PATCH`).
- **The tag is a semver without `v` and must be equal to `CLIVersion`** (`updateGit update` compares them). Pushing the tag triggers `.github/workflows/release.yml`, which runs `make check-all` and creates the release with title = tag and the message ``See changes in [CHANGELOG.md](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md) file``. Tags with `-` are pre-releases.
- The asset names (`updateGit-<os>-<arch>`, `update.AssetName`) and the `bin/` prefix in `checksums.txt` are a contract with `updateGit update` of installed versions: do not change them.
- When changing tool versions, update `mise.toml` and the Go version references in `README.md` (badge and prerequisites).
- GitHub Actions are pinned by commit SHA (comment with the version); Dependabot updates them.

## Notes for agents

- Preserve the existing CLI and config semantics.
- Prefer linking to `README.md` / `CONTRIBUTING.md` instead of duplicating large docs.
- Do not assume Windows support; code explicitly checks only Linux and macOS.
- Keep changes aligned with the current Cobra-based command structure.
