# AI Agent Instructions for updateGit

## Purpose
This repository is a Go CLI tool named `updateGit` that discovers local Git repositories and updates them using `git pull`.

## Key files
- `main.go` - entry point, validates prerequisites and executes CLI.
- `cmd/root.go` - Cobra root command and global flags, config initialization.
- `cmd/pull.go` - `pull` subcommand implementation for repository updates.
- `cmd/update.go` - `update` subcommand for self-update/version checks.
- `internal/config/config.go` - default config values, CLI flags, environment variable mapping, semantic version guidance.
- `internal/git/git.go` - repository discovery, branch detection, `git pull` execution, filter/backup logic.
- `Makefile` - prepare, build, and SBOM generation commands.
- `README.md` and `CONTRIBUTING.md` - project usage, development, release, and contribution guidance.

## Development workflow
- Install Go 1.25+ and Git.
- Run `make prepare` to validate required commands and download Go modules.
- Run `go test -race -coverprofile=coverage.out ./...` for tests.
- Build with `make build` or `go build -o updateGit`.

## CLI behavior
- `updateGit pull` updates repositories under `--git-base-dir` / `-G`.
- `updateGit update` checks GitHub releases and can self-update the binary.
- Global flags include `-D/--debug`, `-V/--long-version`, `-v/--version`, and config flags for git, backup, and filter behavior.

## Configuration conventions
- Default config file: `.updateGit.yaml`.
- Environment variables use prefix `CLI_` and map dashes/periods to underscores.
- Example env vars: `CLI_GIT_BASE_DIR`, `CLI_BACKUP_ENABLED`, `CLI_FILTER_SKIP_REPOS`.
- Flag names map directly to config keys in `cmd/root.go`.

## Release and patch conventions
- Update `CHANGELOG.md` when making user-visible changes.
- Bump `config.CLIVersion` in `internal/config/config.go` for releases.
- Use semantic versioning (`MAJOR.MINOR.PATCH`).
- Build artifacts are produced in `bin/`.

## Notes for agents
- Preserve the existing CLI and config semantics.
- Prefer linking to `README.md` / `CONTRIBUTING.md` instead of duplicating large docs.
- Do not assume Windows support; code explicitly checks only Linux and macOS in `internal/getinfo/get_info.go`.
- Keep changes aligned with the current Cobra-based command structure.
