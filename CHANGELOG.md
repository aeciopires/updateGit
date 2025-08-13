# Changelog

<!-- TOC -->

- [Changelog](#changelog)
- [0.1.0](#010)

<!-- TOC -->

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
