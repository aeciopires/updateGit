# Security Policy

<!-- TOC -->

- [Security Policy](#security-policy)
  - [Supported versions](#supported-versions)
  - [Reporting a vulnerability](#reporting-a-vulnerability)
  - [Security measures](#security-measures)

<!-- TOC -->

## Supported versions

Only the latest release of ``updateGit`` receives security fixes. Update with ``updateGit update`` or download the latest version from the [releases page](https://github.com/aeciopires/updateGit/releases).

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately with the GitHub [private vulnerability reporting](https://github.com/aeciopires/updateGit/security/advisories/new) of this repository, including:

- the affected version (``updateGit -V``) and operating system;
- the steps to reproduce the problem and its impact;
- a suggested fix, if you have one.

You will receive an answer as soon as possible. After the fix is published, the advisory is disclosed with the credits to the reporter (if desired).

## Security measures

- **Releases**: built by GitHub Actions from a tag, with ``checksums.txt`` (SHA256) and an SBOM (``updateGit.sbom.json``). ``updateGit update`` only downloads from GitHub over HTTPS (including redirects), limits the download size, verifies the SHA256 checksum before replacing the binary and never installs an older version.
- **Your work**: repositories with files not committed are skipped by default; backups are available before the update.
- **Credentials**: ``updateGit`` never asks for or stores passwords. In parallel mode git runs non-interactively. Credentials embedded in URLs are masked in the screen and in the log file, which is created with permission ``0600``.
- **Limits**: every git operation has a timeout; the search of repositories does not follow symbolic links and never goes inside repositories.
- **Development**: every change runs ``golangci-lint`` (including ``gosec``), tests with the race detector, ``govulncheck`` and ``trivy`` (see [CONTRIBUTING.md](CONTRIBUTING.md#checks-during-development)). GitHub Actions are pinned by commit SHA, use the minimum permissions and do not persist credentials; Dependabot keeps actions and Go modules up to date.
