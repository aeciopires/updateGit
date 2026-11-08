<!-- TOC -->

- [Requirements](#requirements)
- [Learning Golang](#learning-golang)
- [Contributing](#contributing)
- [Checks during development](#checks-during-development)
- [Security](#security)
- [Releases](#releases)
- [About Visual Code (VSCode)](#about-visual-code-vscode)

<!-- TOC -->

# Requirements

- Configure authentication on your Github account to use the SSH protocol instead of HTTP. Watch this tutorial to learn how to set up: https://help.github.com/en/github/authenticating-to-github/adding-a-new-ssh-key-to-your-github-account
- Install the commands ``mise``, ``make`` and ``git`` following the tutorials:
  - [mise](https://mise.jdx.dev/getting-started.html)
  - [MacOS](https://github.com/aeciopires/adsoft/blob/master/softwares-macos.md)
  - [Ubuntu](https://github.com/aeciopires/adsoft/blob/master/softwares-ubuntu.md)
- Install ``golang``, ``trivy`` and the development tools (``golangci-lint``, ``govulncheck``, ``actionlint``, ``shellcheck``, ``pre-commit``) with the versions pinned in [mise.toml](mise.toml), check if all software dependencies are installed and install the git hooks:

```bash
cd updateGit
make install-deps
make check-deps
make install-hooks
```

> Run ``make help`` (or just ``make``) to list all available targets.

# Learning Golang

See [LEARNING_GOLANG.md](LEARNING_GOLANG.md) file.

# Contributing

- Create a fork this repository.
- Clone the forked repository to your local system:

```bash
git clone FORKED_REPOSITORY
```

- Add the address for the remote original repository:

```bash
git remote -v
git remote add upstream git@github.com:aeciopires/updateGit.git
git remote -v
```

- Configure your name and email used during commits:

> Attention!!! Change your name and surname in commands bellow

```bash
cd updateGit

git config --local user.name "Your_name Your_surname"

git config --local user.email "your_email"
```

- Install dependencies and the git hooks.

```bash
cd updateGit
make install-deps
make install-hooks
```

- Create a branch. Example:

```bash
git checkout -b BRANCH_NAME
```

> - **TIP!!!** Try use conventionnal names for branch and commits explained in follow pages:
>   - https://medium.com/@shinjithkanhangad/git-good-best-practices-for-branch-naming-and-commit-messages-a903b9f08d68
>   - https://www.conventionalcommits.org/en/v1.0.0/
>   - https://dev.to/varbsan/a-simplified-convention-for-naming-branches-and-commits-in-git-il4
>   - https://gist.github.com/qoomon/5dfcdf8eec66a051ecd85625518cfd13

- Make sure you are on the correct branch using the following command. The branch in use contains the '*' before the name.

```bash
git branch
```

- Make your changes and tests. **Every change must come with unit tests** (``*_test.go`` files next to the code). Run the checks before committing (see [Checks during development](#checks-during-development)):

```bash
make check       # fast: gofmt, go.mod, go vet, golangci-lint and unit tests
make check-all   # complete: also coverage, vulnerabilities, workflows, build and artifacts
# Optional: coverage report
make coverage
make coverage-html
```

> Attention!!!
>
> 1. Change the ``CHANGELOG.md`` when you make changes.
> 2. Tool versions (Go, Trivy and the development tools) are pinned in ``mise.toml``. Go module dependencies are managed in ``go.mod``/``go.sum`` (``go get -u ./... && go mod tidy``).
> 3. New targets in ``Makefile`` must have a ``## Description`` comment after the target name to be listed by ``make help``.
> 4. Change the value of ``CLIVersion`` variable in ``internal/config/config.go`` when you make changes to your Golang application. Follow the Semantic Version (https://semver.org/).

- Commit the changes to the branch.
- Push files to repository remote with command:

```bash
git push --set-upstream origin BRANCH_NAME
```

- Create Pull Request (PR) to the `main` branch. The [CI workflow](.github/workflows/ci.yml) runs ``make check-all`` (Linux) and ``make check`` (macOS) and must pass before the merge. See this [tutorial](https://help.github.com/en/github/collaborating-with-issues-and-pull-requests/creating-a-pull-request-from-a-fork)
- Update the content with the suggestions of the reviewer (if necessary).
- After your pull request is merged to the `main` branch, update your local clone:

```bash
git checkout main
git pull upstream main
```

- Clean up after your pull request is merged with command:

```bash
git branch -d BRANCH_NAME
```

- Then you can update the ``main`` branch in your forked repository.

```bash
git push origin main
```

- And push the deletion of the feature branch to your GitHub repository with command:

```bash
git push --delete origin BRANCH_NAME
```

- To publish a new release, see [Releases](#releases).

- To keep your fork in sync with the original repository, use these commands:

```bash
git pull upstream main
git pull upstream main --tags

git push origin main
git push origin main --tags
```

References:

- https://blog.scottlowe.org/2015/01/27/using-fork-branch-git-workflow/

# Checks during development

The same checks run locally, in the git hooks and in the CI, all through the ``Makefile``:

| Command | What it checks | When |
|---|---|---|
| ``make fmt-check`` | Go code formatted with ``gofmt`` (fix: ``make fmt``) | pre-commit hook |
| ``make lint`` | ``golangci-lint`` ([.golangci.yml](.golangci.yml)): unchecked errors, ``%w`` in ``common.Logger``, security, misspelling... (fix: ``make lint-fix``) | pre-commit hook |
| ``make test-short`` | Fast unit tests (skips the load tests) | pre-commit hook |
| ``make check`` | ``fmt-check`` + ``mod-check`` (``go mod tidy -diff``, ``go mod verify``) + ``vet`` + ``lint`` + ``test`` (race detector) | before each commit |
| ``make check-all`` | ``check`` + ``coverage-check`` (minimum 90%) + ``vuln`` (govulncheck) + ``security`` (trivy) + ``lint-workflows`` (actionlint) + ``lint-diagrams`` (Mermaid, Docker) + ``build`` + ``verify-artifacts`` | pre-push hook and CI |

Common failures:

- ``The following files are not formatted``: run ``make fmt``.
- ``go.mod/go.sum are not tidy``: run ``go mod tidy``.
- ``Lint failed``: read the message, try ``make lint-fix``. Exclusions (``//nolint:<linter> // reason``) must be specific and justified.
- ``Total coverage ... is below the minimum``: add tests (``make coverage`` shows the coverage per function).
- The golden files of ``internal/output`` changed on purpose: run ``go test ./internal/output -update`` and review the diff in ``internal/output/testdata``.
- ``Invalid Mermaid diagram``: fix the diagram in the Markdown file (``make lint-diagrams`` uses the Docker image ``minlag/mermaid-cli``; without Docker the check is skipped).
- Emergency: ``git push --no-verify`` skips the pre-push hook (the CI still runs the checks).

# Security

See [SECURITY.md](SECURITY.md) to report vulnerabilities. Code rules: return errors instead of exiting in the ``internal`` packages, use a ``context`` with timeout in every external command or HTTP request, never log credentials and justify every ``//nolint`` (enforced by ``golangci-lint``).

# Releases

Releases are built and published by the [Release workflow](.github/workflows/release.yml) when a tag is pushed:

1. In a PR to the ``main`` branch, update ``CHANGELOG.md`` and the ``CLIVersion`` variable in ``internal/config/config.go`` (semantic version **without** the ``v`` prefix, e.g. ``0.2.0``).
2. After the merge, create and push the tag (it must be equal to ``CLIVersion``):

```bash
git checkout main && git pull
export CLI_VERSION="$(go run . -v)"
echo "$CLI_VERSION"
git tag -a "$CLI_VERSION" -m "Release $CLI_VERSION"
git push origin "$CLI_VERSION"
```

3. Follow the **Release** workflow in the *Actions* tab. It validates the tag, runs ``make check-all`` and creates the release with:
   - title equal to the tag;
   - the message ``See changes in [CHANGELOG.md](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md) file``;
   - the assets ``updateGit-{linux,darwin}-{amd64,arm64}``, ``checksums.txt`` and ``updateGit.sbom.json``.

Rules:

- The tag must be a semantic version without ``v`` (``0.2.0``, not ``v0.2.0``): ``updateGit update`` compares the tag with ``CLIVersion``.
- Pre-releases (``0.3.0-rc.1``) are published as *pre-release* and are not offered by ``updateGit update``.
- The asset names and the ``bin/`` prefix in ``checksums.txt`` are a contract with ``updateGit update``: do not change them.
- Wrong tag? Delete it and create it again: ``git push --delete origin <tag> && git tag -d <tag>``. Running the workflow again for the same tag replaces the assets.
- The binaries are no longer uploaded manually.

# About Visual Code (VSCode)

Use a IDE (Integrated Development Environment) or text editor of your choice. By default, the use of VSCode is recommended.

VSCode (https://code.visualstudio.com), combined with the following plugins, helps the editing/review process, mainly allowing the preview of the content before the commit, analyzing the Markdown syntax and generating the automatic summary, as the section titles are created/changed.

Plugins to Visual Code:

- gitlens: https://marketplace.visualstudio.com/items?itemName=eamodio.gitlens (require git package)
- golang: https://marketplace.visualstudio.com/items?itemName=golang.go
- gotemplate-syntax: https://marketplace.visualstudio.com/items?itemName=casualjim.gotemplate
- Markdown-all-in-one: https://marketplace.visualstudio.com/items?itemName=yzhang.markdown-all-in-one
- Markdown-lint: https://marketplace.visualstudio.com/items?itemName=DavidAnson.vscode-markdownlint
- Markdown-toc: https://marketplace.visualstudio.com/items?itemName=AlanWalk.markdown-toc
- YAML: https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml

Theme for VSCode:

- https://code.visualstudio.com/docs/getstarted/themes
- https://dev.to/thegeoffstevens/50-vs-code-themes-for-2020-45cc
- https://vscodethemes.com/
