# updateGit architecture

This page shows how updateGit is organized: its building blocks, what each one is responsible for and how they work together. It does not explain the code itself. For the step-by-step journey of an execution, read [How it works](how-it-works.md) first.

<!-- TOC -->

- [updateGit architecture](#updategit-architecture)
  - [Overview](#overview)
  - [The building blocks](#the-building-blocks)
  - [Where each block lives](#where-each-block-lives)
  - [How the blocks talk during a pull](#how-the-blocks-talk-during-a-pull)
  - [Working in parallel without mixing things up](#working-in-parallel-without-mixing-things-up)
  - [Settings](#settings)
  - [Screen and log file](#screen-and-log-file)
  - [Self-update](#self-update)
  - [Quality checks and releases](#quality-checks-and-releases)
  - [Design principles](#design-principles)

<!-- TOC -->

## Overview

updateGit is a single program (one file to download, no installation of other components besides git). Inside it, the work is divided in layers, like a small company: a **reception** that receives your request, a **planning** team that decides what to do, an **operations** team that does the work, and a **communication** team that reports the results.

```mermaid
flowchart TB
    User(["👤 You (terminal, script or cron)"])

    subgraph Reception["Reception"]
        CLI["Commands<br/>pull · update · version"]
        Settings["Settings<br/>options · environment · file · defaults"]
    end

    subgraph Planning["Planning"]
        Discovery["Discovery<br/>finds the repositories"]
        Filter["Filter<br/>removes skipped repositories"]
    end

    subgraph Operations["Operations"]
        Runner["Coordinator<br/>queue + workers + time limits"]
        Backup["Backup<br/>copy or stash, own limit"]
        Git["Git helper<br/>branches + git pull"]
    end

    subgraph Communication["Communication"]
        Reporter["Reporter<br/>blocks + final report"]
        Progress["Progress bar<br/>+ search spinner"]
        Logger["Logger<br/>levels + colors"]
        Terminal["Screen gatekeeper"]
        LogFile["Log file"]
    end

    External[("☁️ Git servers<br/>GitHub, GitLab...")]
    Disk[("💽 Your folders<br/>and backups")]

    User --> CLI
    CLI --> Settings
    CLI --> Discovery --> Filter --> Runner
    Runner --> Backup --> Disk
    Runner --> Git --> External
    Git --> Disk
    Discovery --> Disk
    Runner -->|results and events| Reporter
    Reporter --> Progress
    Reporter --> Terminal
    Reporter --> LogFile
    Logger --> Terminal
    Logger --> LogFile
    Progress --> Terminal
    Terminal --> Screen(["🖥️ Your screen"])
```

## The building blocks

| Block | Responsibility | Does NOT do |
|---|---|---|
| **Commands** | Understand what you asked (`pull`, `update`, `-v`), start the right flow and return the exit code (0, 1 or 130). | Any git work. |
| **Settings** | Combine the options, environment variables, config file and defaults into one validated configuration. | Decide anything about repositories. |
| **Discovery** | Search the base folder (up to 6 levels, default 2) and return the complete list of repositories, never looking inside a repository. | Update anything. It finishes before any update starts. |
| **Filter** | Remove the repositories you asked to skip (by folder name or path). | Search folders. |
| **Coordinator** (runner) | Keep the queue, start the workers, enforce the time limit of each update, limit the simultaneous backups and react to Ctrl+C. | Write on the screen. |
| **Workers** | For one repository at a time: check the branch, skip it if it has files not committed (default), make the backup, run `git pull`, classify the result (OK, failed, timeout, skipped, canceled) and explain failures. | Write on the screen or stop the program. |
| **Backup** | Make a copy of the files or a git stash before the update. | Decide when to run (the coordinator does). |
| **Git helper** | Talk to git: read the branches and their state, run `git pull` in a non-interactive way when in parallel. | Talk to the user. |
| **Reporter** | Turn each result into a readable block, write it to the screen and to the log file, update the progress bar and write the final report. | Any git work. |
| **Progress bar** | Keep the counters (done, running, backups, time) and draw the last line of the screen. | Write to the log file. |
| **Logger** | Write messages with a level (debug, info, warning, error), colors and time, to the screen and to the log file, hiding passwords. | Decide the flow. |
| **Screen gatekeeper** | The only door to the screen: erases the progress bar, writes the message, draws the bar again. | Format messages. |
| **Log file** | Create the file (`/tmp` by default, readable only by you) with a header and a footer. | Anything on the screen. |
| **Self-update** | Look for a new version on GitHub, download it, verify its checksum and replace the program. | Update repositories. |

## Where each block lives

The folders of the project follow the blocks above (you do not need to read them, this is only a map):

```mermaid
flowchart LR
    root["updateGit/"] --> main["main.go<br/>start of the program"]
    root --> cmd["cmd/<br/>Commands + Settings"]
    root --> internal["internal/"]
    internal --> config["config/<br/>defaults and rules of the settings"]
    internal --> gitpkg["git/<br/>Discovery · Coordinator · Workers · Git helper"]
    internal --> filter["filter/<br/>Filter"]
    internal --> backup["backup/<br/>Backup"]
    internal --> output["output/<br/>Reporter · Progress bar · Screen gatekeeper"]
    internal --> common["common/<br/>Logger · Log file · small helpers"]
    internal --> update["update/<br/>Self-update"]
    internal --> getinfo["getinfo/<br/>version and system information"]
    root --> github[".github/workflows/<br/>automatic checks and releases"]
    root --> docs["docs/<br/>this documentation"]
```

## How the blocks talk during a pull

```mermaid
sequenceDiagram
    autonumber
    actor You
    participant CMD as Commands
    participant SET as Settings
    participant DIS as Discovery
    participant FIL as Filter
    participant RUN as Coordinator
    participant WRK as Workers (N)
    participant REP as Reporter
    participant SCR as Screen / Log file

    You->>CMD: updateGit pull -G ~/git -J 15
    CMD->>SET: combine and validate the settings
    SET-->>CMD: final settings (or stop with an error)
    CMD->>DIS: find the repositories
    Note over DIS,SCR: spinner on the screen while searching
    DIS-->>CMD: complete list (e.g. 52 repositories)
    CMD->>FIL: remove the skipped ones
    FIL-->>CMD: 50 to update
    CMD->>RUN: update these 50 with 15 workers
    loop until the queue is empty
        RUN->>WRK: next repository
        WRK->>WRK: branch check, uncommitted files check, backup, git pull (time limit)
        WRK-->>RUN: result (OK / FAILED / TIMEOUT / SKIPPED / CANCELED)
        RUN->>REP: result, one at a time
        REP->>SCR: block of the repository + progress bar
    end
    RUN-->>CMD: all results
    CMD->>REP: final report
    REP->>SCR: summary
    CMD-->>You: exit code 0, 1 or 130
```

## Working in parallel without mixing things up

Many workers run at the same time, but only **one** component writes on the screen. This is the main design decision that keeps the output readable:

```mermaid
flowchart LR
    subgraph Workers["Workers (run at the same time)"]
        W1["Worker 1"]
        W2["Worker 2"]
        WN["Worker N"]
    end
    subgraph BackupRoom["Backup slots (e.g. 4)"]
        S1["slot"]
        S2["slot"]
        S3["slot"]
        S4["slot"]
    end
    W1 -.->|waits for a free slot| BackupRoom
    W2 -.->|waits for a free slot| BackupRoom
    WN -.->|waits for a free slot| BackupRoom
    W1 -->|result| MB[["📨 Results mailbox"]]
    W2 -->|result| MB
    WN -->|result| MB
    MB --> REP["Reporter<br/>(only one)"]
    REP --> GK["Screen gatekeeper<br/>(one at a time)"]
    TICK["⏱️ Timer<br/>every 0.1 s"] -->|redraw the bar| GK
    LOG["Logger messages"] --> GK
    GK --> SCREEN["🖥️ Screen"]
```

- **Queue**: a repository is taken by only one worker, so it is never updated twice.
- **Mailbox**: workers drop their results in a mailbox; the reporter takes them one by one, in the order they finish.
- **Backup slots**: a separate, smaller limit, so heavy disk copies do not overload the computer even with 50 workers.
- **Gatekeeper**: the logger, the reporter and the timer of the progress bar all pass through the same door, one at a time. The bar is erased, the message is written and the bar is drawn again.
- **Ctrl+C**: a single "stop" signal reaches every worker. Running updates are stopped, the queue is closed and the reporter still writes a partial report.

## Settings

All settings have a default value and can be changed by a config file, environment variables or options in the command. The final value follows a fixed priority, and it is validated before anything starts:

```mermaid
flowchart TB
    D["⚙️ Defaults"] --> M{{"Combine<br/>(highest priority wins)"}}
    F["📄 Config file<br/>.updateGit.yaml"] --> M
    E["🌱 Environment variables<br/>CLI_*"] --> M
    O["⌨️ Command options<br/>-J, -T, -R, -L..."] --> M
    M --> V{"Valid?<br/>(limits and allowed values)"}
    V -- No --> X(["Stop with a clear message"])
    V -- Yes --> C(["Final settings used<br/>by all the blocks"])
```

Priority, from highest to lowest: **command options > environment variables > config file > defaults**.

## Screen and log file

The same information can go to two places, each one in its own format:

| | Screen | Log file |
|---|---|---|
| Colors and symbols | Yes, when it is a terminal (and `NO_COLOR` is not set) | Never |
| Progress bar and spinner | Yes, in a terminal with more than one worker | Never |
| Minimum level | `--log-level` | `--log-file-level` (default: same as the screen) |
| Header and footer | No | Version, command, settings, duration, exit code |
| Passwords in addresses | Hidden | Hidden |
| Who can read | You | Only you (file permission) |

## Self-update

`updateGit update` replaces the program by the newest published version, safely:

```mermaid
sequenceDiagram
    actor You
    participant U as updateGit update
    participant GH as GitHub Releases
    You->>U: updateGit update
    U->>GH: what is the latest version?
    GH-->>U: e.g. 0.2.0 (+ list of files)
    alt already the latest version
        U-->>You: nothing to do
    else new version available
        U->>You: update? (y/n)
        You->>U: y
        U->>GH: download the program for your system + checksums
        U->>U: compare the checksum (integrity)
        U->>U: keep the old program as .old and put the new one in place
        U-->>You: done, run it again
    end
```

Pre-release versions (e.g. `0.3.0-rc.1`) are never offered by the self-update.

## Quality checks and releases

The same checks run on the developer's computer, before each commit/push, and on GitHub. A new version is published automatically when a version tag is created:

```mermaid
flowchart LR
    Dev(["👩‍💻 Developer"]) --> Check["make check<br/>format · lint · tests"]
    Check --> Commit["git commit<br/>(quick checks run automatically)"]
    Commit --> Push["git push<br/>(all checks run automatically)"]
    Push --> PR["Pull request"]
    PR --> CI["GitHub CI<br/>Linux + macOS<br/>all checks"]
    CI -->|approved and merged| Main["main branch"]
    Main --> Tag["Version tag<br/>e.g. 0.2.0"]
    Tag --> Rel["Release workflow<br/>validate tag = program version<br/>checks · build · verify"]
    Rel --> Page["📦 GitHub release<br/>title = tag<br/>4 programs + checksums + SBOM"]
    Page --> Users(["👥 Users<br/>download or updateGit update"])
```

The checks include: code formatting, a code analyzer (linter), automatic tests (with at least 90% of the code covered), search for known vulnerabilities, a security scan, validation of the automation files and of the diagrams of this documentation, and the build of the programs for Linux and macOS (Intel and ARM).

## Design principles

- **Find first, update later**: the complete list is known before the first update, so the progress and the report are accurate.
- **Never look inside a repository**: fast search and no "projects inside projects".
- **Isolation**: each repository is independent; one problem never stops the others.
- **One voice on the screen**: workers produce results, a single reporter shows them.
- **Limits everywhere**: number of workers, time per update, simultaneous backups and search depth.
- **Protect the user's work**: repositories with files not committed are skipped by default.
- **No surprises**: no password prompts in parallel mode, clear reasons and hints for failures, exit codes for scripts.
- **Same rules everywhere**: the checks used by developers are the same used by the automatic pipelines.
