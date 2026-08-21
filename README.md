<p align="center">
  <source media="(prefers-color-scheme: dark)" srcset="images/lnk-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="images/lnk-light.png">
  <img alt="Project Logo" src="images/lnk-dark.png" width="128">
</p>

# lnk

[![Go Version](https://img.shields.io/github/go-mod/go-version/polymorcodeus/lnk)](https://go.dev/) [![License](https://img.shields.io/github/license/polymorcodeus/lnk)](./LICENSE) [![Build Status](https://img.shields.io/github/actions/workflow/status/polymorcodeus/lnk/ci.yml?branch=main)](https://github.com/polymorcodeus/lnk/actions)

**Lightweight git-native dotfiles management.**

Track dotfiles across machines with one command. Lnk moves files into a Git repo (defaults to `~/.config/lnk`; override with `--repo` flag or `LNK_HOME` / `LNK_REPO`), symlinks them back, and stays out of your way. Setup multiple host/scopes in addition to a common scope to separate work and personal settings.

## Quick Demo

```bash
lnk init                                       # create a local repo
lnk clone git@github.com:you/dotfiles.git      # clone a remote repo
lnk add ~/.vimrc ~/.bashrc ~/.gitconfig        # track files
lnk add --host work ~/.ssh/config              # per-machine config
lnk push                                       # push to remote
lnk update                                     # pull and restore symlinks
```

## Getting Started

### Install

```bash
curl -sSL https://raw.githubusercontent.com/polymorcodeus/lnk/main/install.sh | bash
```

Or grab a binary from [releases](https://github.com/polymorcodeus/lnk/releases), or build from source:

```bash
go install github.com/polymorcodeus/lnk@latest
```

*NOTE:*

Installing with Homebrew is not yet supported.

### Quick Start

1. **Initialize** on a new machine:

   ```bash
   lnk clone git@github.com:you/dotfiles.git --bootstrap
   lnk update
   lnk update --host $(hostname)
   ```

   That's it. Bootstrap runs automatically (with flag), symlinks get restored, you're working.

2. **Add** dotfiles on your daily machine:

   ```bash
   lnk add ~/.vimrc ~/.bashrc ~/.gitconfig
   ```

3. **Sync** changes:

   ```bash
   lnk push
   ```

## How it works

```bash
Before: ~/.vimrc (regular file)
After:  ~/.vimrc → ~/.config/lnk/.vimrc (symlink into git repo)
```

Common files live at the repo root (v1) or under `common.lnk/` (v2). Host-specific files go in `<hostname>.lnk/` subdirectories. A plain text `.lnk` file tracks what's managed — one path per line, no special format.

```bash
~/.config/lnk/
├── .lnkrepo               # version marker (v2)
├── .lnk.common            # tracked common files (v2)
├── .lnk.work              # tracked work-specific files
├── common.lnk/            # v2: common storage directory
│   ├── .vimrc
│   └── .gitconfig
└── work.lnk/              # host-specific storage
    └── .ssh/config
```

### Legacy Format

```bash
~/.config/lnk/
├── .lnk                   # tracked common files (v1)
├── .lnk.work              # tracked work-specific files          
├── .vimrc                 # v1: common files and directories in repo root.
├── .gitconfig
├── .config/               
│   └── ghostty
└── work.lnk/              # host-specific storage
    └── .ssh/config
```

## Features

### Add files

```bash
lnk add ~/.vimrc ~/.bashrc                # multiple at once
lnk add --host laptop ~/.ssh/config       # host-specific
```

### Move between scopes

```bash
lnk move ~/.ssh/config --to-common        # move to common scope
lnk move ~/.vimrc --to-host work          # move to host scope
```

### Sync

```bash
lnk status                                # full git status output
lnk status --color                        # colorized git status
lnk diff                                  # uncommitted changes
lnk diff --color                          # colorized diff
lnk commit -m "updated vim config"        # stage and commit - not needed after `lnk add`
lnk commit                                # commit with default message - not needed after `lnk add`
lnk push                                  # push existing commits
lnk pull                                  # pull repo changes
lnk restore                               # restore symlinks (no pull)
lnk update                                # pull + restore symlinks
lnk update --host work                    # include host-specific files
lnk restore --dry-run                     # preview what would be restored
```

`status` shows the full `git status` output. If no remote is configured, it prints `Remote not set` at the top. Use `--color` to enable colorized output (default is plain text).

### Remove

```bash
lnk remove ~/.vimrc                       # stop managing, restore file locally
lnk forget ~/.bashrc                      # stop tracking, keep stored repo copy
```

`remove` restores the file to its original location and removes it from the repo. `forget` keeps the stored copy in the repo but removes the symlink and tracking entry — useful for temporarily stopping management of a path.

### List

```bash
lnk list                                  # common files
lnk list --host work                      # host-specific
lnk list --all                            # all scopes
```

### Health checks

```bash
lnk doctor                                # audit repo and profile health
lnk doctor --fix                          # apply safe automatic fixes
lnk doctor --fix --prune-empty            # also remove empty host scopes
lnk doctor --all                          # check all scopes
```

When restoring symlinks, if a real file exists at the target location (not a symlink), it will be renamed to `<path>.lnk-backup` to preserve your data before the symlink is created. Check for `.lnk-backup` files after running `restore`, `update`, or `doctor` if you expect them.

### Format migration

```bash
lnk format                                # show current repo format
lnk format --v2                           # migrate to v2 format, i.e. common files under lnk.common
lnk format --v1                           # migrate back to v1 format
```

v2 aggregates common dotfiles under `common.lnk/` for cleaner repo organization. New repos are initialized as v2 by default. After the format, run a `lnk doctor --fix` as all your common symlinks will be broken.

### Bootstrap

Drop a `bootstrap.sh` in your dotfiles repo. Lnk runs it automatically on `lnk clone <url> --bootstrap`.

```bash
lnk clone <url>                           # clones repo locally, no bootstrap.sh
lnk clone <url> --bootstrap               # runs bootstrap.sh after clone
lnk bootstrap                             # run manually
```

### Project scope

Track project-local configuration files without committing them to the project's own git repository. Useful for `.crush/crush.json`, `.vscode/settings.json`, repo-specific shell aliases, or any file you want backed up in your dotfiles repo but not pushed upstream.

Project scope uses a `.lnkinclude` file inside the project root. Patterns follow `.gitignore` syntax, but a match means "include". Global patterns can live in your lnk repo root; local patterns are project-specific.

```bash
# inside a git repository
lnk project init                          # create an empty .lnkinclude
lnk project add .crush/**                 # track all files under .crush/
lnk project add .vscode/settings.json     # track a single file
lnk project list                          # show effective global + local patterns
lnk project push                          # move matches to lnk storage and symlink back
lnk project restore                       # recreate symlinks from storage
lnk project restore --dry-run             # preview what would be restored
lnk project pull                          # pull lnk repo and restore
lnk project untrack .crush/**             # remove a local pattern
```

Matched files are stored under `projects/<normalized-origin>/<path>/` in your lnk repo (derived from the project's origin remote) and symlinked back into the project. Existing files at symlink locations are backed up to `<path>.lnk-backup` during restore, just like host/common scope restores.

## Man pages

Man pages are generated from the Cobra command tree and ship with release archives.

```bash
make man                                  # generate pages in man/
man man/lnk-project-push.1                # read a generated page
```

## Commands

| Command | What it does |
| --- | --- |
| `init` | Create or adopt a local lnk repo |
| `clone <url> [--bootstrap]` | Clone a remote lnk repo |
| `add [--host H] <path...>` | Track files (move to repo + symlink) |
| `move <path> (--to-common \| --to-host H)` | Move a tracked path between scopes |
| `remove [--host H] <path>` | Stop managing, restore file locally |
| `forget [--host H] <path>` | Stop tracking, keep stored repo copy |
| `list [--host H \| --all]` | Show tracked files by scope |
| `status [--color]` | Show full git status output |
| `diff [--color]` | Show uncommitted changes |
| `commit [-m message]` | Stage all changes and commit |
| `push` | Push existing commits |
| `pull` | Pull repo changes |
| `restore [--host H] [--dry-run]` | Restore symlinks without pulling |
| `update [--host H]` | Pull and restore the effective profile |
| `doctor [--host H \| --all] [--fix] [--prune-empty]` | Audit and fix repo health |
| `format [--v1 \| --v2]` | Migrate repo format |
| `bootstrap` | Run bootstrap.sh explicitly |
| `project init` | Activate project scope in the current git repo |
| `project add <pattern...>` | Add patterns to the project's `.lnkinclude` |
| `project list` | Show effective project patterns |
| `project untrack <pattern>` | Remove a pattern from the project's `.lnkinclude` |
| `project push` | Move matching project files to lnk storage |
| `project restore [--dry-run]` | Recreate project symlinks from storage |
| `project pull` | Pull lnk repo and restore project symlinks |

## Global Options

Available with all commands:

| Option | Default | What it does |
| --- | --- | --- |
| `--repo <path>` | `~/.config/lnk` | Path to the lnk repository |

## Acknowledgements

This originally started off as a fork of [yarlson/lnk](https://github.com/yarlson/lnk) with a number of features that I wanted. It has since turned into a standalone version after I saw the plan to rewrite a v2 in Rust. I've cleaned up the legacy code and added some opinionated fixes along the way. This should™ be fully compatible with the original repos from yarlson's tool, but now stands alone. I can't guarantee backwards or cross compatibility going forward so use both at your own peril.

The idea of a project scope was born out of seeing [claytercek/offstage](https://github.com/claytercek/offstage). It felt like a good extension of what was already built out here, but I wanted to streamline it and have it fit with the intent I've curated here, namely a targeted working snapshot of my different machine profiles.

## Contributing

```bash
git clone https://github.com/polymorcodeus/lnk.git
cd lnk
make check    # fmt, vet, lint, test
```

## Contributors

<a href="https://github.com/polymorcodeus/lnk/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=polymorcodeus/lnk" />
</a>

## License

[MIT](LICENSE)
