# wk

Manage Git worktrees with one [Herdr](https://herdr.dev) workspace or tmux session per branch. Works with bare repositories and traditional clones; all new worktrees live under `.wk/`.

## Install

Requires Go 1.24.5+ to build, Git, and Herdr or tmux at runtime. `wk clean` also requires the GitHub CLI (`gh`). Supports macOS and Linux.

```sh
sudo make install                   # binary and man page in /usr/local
make install PREFIX="$HOME/.local"   # alternatively, install without sudo
man wk
```

Uninstall with `make uninstall` using the same prefix and permissions. Package staging supports `DESTDIR` alongside `PREFIX`.

## Get started

### Traditional clone

```sh
git clone <remote-url> project
cd project
wk init
# Edit .wk.toml before adding worktrees if you need setup steps.
wk add feat/login
```

WK adds `/.wk/` to the main checkout's `.gitignore`; commit that rule to share it. Creation stops if the ignore file cannot be updated or conflicting rules leave the container unignored. The main checkout appears in `wk ls` but cannot be deleted or cleaned.

### Bare repository

```sh
mkdir project && cd project
git clone --bare <remote-url> .bare
echo "gitdir: ./.bare" > .git
wk init
# Edit .wk.toml and add any shared files before creating the first worktree.
wk add main
```

Shared files such as `AGENTS.md` and local env files belong at the project root, alongside `.bare/` and `.wk.toml`. Bare projects do not need a project-root `.gitignore`.

Both flows create `<project-root>/.wk/<sanitized-branch>/`, such as `.wk/feat-login/`. Existing worktrees are not moved. Commands work from the project root, linked worktrees, or their subdirectories; Git metadata identifies the owning project.

## Configure

### Project: `.wk.toml`

Run `wk init` to scaffold the configuration at the owning project root (or cwd outside Git). A linked worktree's tracked copy does not override the root's configuration.

```toml
name = "project"
base_ref = "origin/main"         # starting ref for new branches in bare repos

setup = [
  "ln -s ../../AGENTS.md .",
  "ln -s ../../.env.custom .",
  "pnpm i --frozen-lockfile",
]

teardown = [
  "docker compose down -v",
]
```

All fields are optional:

| Field | Meaning |
|---|---|
| `name` | Project name in session labels; defaults to the project-root folder name. |
| `base_ref` | Starting ref for new branches in bare repositories; overrides `default_base_ref`. |
| `setup` | Shell steps run after creating a worktree, in order, stopping at the first failure. |
| `teardown` | Shell steps run before removing a worktree, while its files still exist. |
| `herdr_progress` | Override the global busy indicator setting for this project. |

Setup and teardown run inside the target worktree. Shared files at the project root are reached through `../../` in either repository layout. Only symlink files that are not already tracked in the checkout.

For shared Git hooks, configure an absolute path once from the project root:

```sh
git config core.hooksPath "$PWD/hooks"
```

### Global: `~/.config/wk/config.toml`

Defaults:

```toml
default_base_ref = "origin/main"
session_template = "{branch} [{project}]"
session_backend = "herdr"
herdr_progress = true
```

`default_base_ref` applies to bare repositories. The session template accepts `{branch}` and `{project}`. Choose `herdr` or `tmux`; override per command with `--herdr`, `--tmux`, or `--backend <kind>`. `herdr_progress` marks the invoking Herdr pane busy during add/delete; reporting failures do not fail commands.

## Daily use

```sh
wk add feat/login           # create a worktree and open its session
wk add origin/review        # check out and track an origin branch
wk add origin/review rev    # Herdr label: [rev] review [project]
wk --tmux add feat/login    # override the session backend
wk ls                       # branches, paths, and session status
wk ls --porcelain           # three-line blocks: branch, absolute path, session
wk delete feat/login        # remove the worktree, branch, and session
wk delete .                 # remove the caller's worktree; also works from subdirectories
wk clean                    # offer to delete worktrees with merged PRs
```

`wk add <name>` reuses a local branch first, then checks for `origin/<name>`. Otherwise it creates a new branch from the caller's current `HEAD` in a traditional clone, or from `base_ref` in a bare repository. Only committed changes are included. For new bare-repository branches, check `git branch -vv` before assuming upstream tracking; it depends on Git configuration. Explicit origin checkouts set their upstream.

An optional badge changes only the Herdr label, not branch names or paths. A new badge replaces the label; omitting it preserves an existing label. Supply the badge again when recreating a closed workspace. Tmux does not support badges.

If `wk add` fails, rerun the same command. Failed setup is retried; completed setup is not repeated after session creation fails.

`wk delete` (alias `wk rm`) accepts a branch, sanitized directory name, project-relative path such as `.wk/feat-login`, absolute registered path, or `.`. Rendered session-template names are also accepted. Unregistered directories cannot be deleted.

### Session backends

WK owns Git operations; Herdr and tmux host the sessions. Herdr workspaces are matched by checkout path, never by mutable labels. Multiple workspaces for one path are an error. Tmux sessions are addressed by the configured session name.

Inside Herdr, WK focuses a new workspace only if you are still in the launching workspace, tab, and pane when it finishes. Switching elsewhere leaves focus alone.

See [backend details](docs/session-backends.md) for command mapping, progress reporting, setup checkpoints, and deletion order.

### Shell completion

`wk completion <zsh|bash|fish>` prints a completion script. Add completion suggests remote branches; delete completion suggests registered worktree paths.

For zsh, save the script in a directory already in your `fpath`, before `compinit` runs:

```sh
mkdir -p ~/.zsh/completions
wk completion zsh > ~/.zsh/completions/_wk
```

For Bash or fish:

```sh
eval "$(wk completion bash)"   # ~/.bashrc
wk completion fish | source    # ~/.config/fish/config.fish
```

## Troubleshooting

1. **Setup holds the launching shell busy.** Keep that pane open until setup finishes; you can switch elsewhere while it runs.
2. **Locked worktrees cannot be removed.** Unlock them explicitly before retrying. Teardown runs before Git attempts removal.
3. **Session closure can fail after deletion.** The worktree and branch are already gone; close the leftover session manually with `herdr workspace close <id>` or `tmux kill-session -t <name>`.

## Development

```sh
make help
make test       # race-enabled suite
make e2e        # compiled-binary end-to-end tests
```

For scoped mutation testing, coverage, and CRAP reports, see [metrics tooling](docs/metrics.md).
