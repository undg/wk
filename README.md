---
title: wk README
status: draft
tags: [tools, git, cli, go]
created: 2026-08-26
---

# wk

`wk` automates a specific workflow: one bare git repo per project, one worktree per branch, one session per worktree — a [herdr](https://herdr.dev) workspace by default, or a tmux session. It is **not** a generic git-worktree tool — it encodes opinions about layout, config, and naming. This README documents those opinions.

> **Status**: the Go binary is implemented and replaces the old bash POC (`wk-pgm-fe.sh` + `wk-delete-merged.sh`). See [[spec]] for the full design and remaining plan-of-action items.

## 1. Setting up a new bare repo

New projects use the `.bare/`-wrapper convention (the older bare-root-is-project-root layout, still used by `pgm-fe`, is kept only for backward compatibility — don't use it for new projects):

```sh
mkdir -p ~/Code/<project> && cd ~/Code/<project>

# the actual bare git dir lives inside .bare/, not at the project root
git clone --bare <remote-url> .bare

# redirect plain `git` commands run from the project root to .bare/
echo "gitdir: ./.bare" > .git

# optional: shared hooks across every worktree
git --git-dir=.bare config core.hooksPath ../hooks

# add the first worktree (usually main)
git worktree add main main
```

At this point `~/Code/<project>/` contains `.bare/`, `.git` (the gitfile), `main/` (your first worktree), and nothing else. Any files you want symlinked into *every* worktree (shared env files, `AGENTS.md`, etc.) go here too, as siblings of `.bare/` — `wk`'s `setup` steps (below) `ln -s ../<file> .` them into new worktrees.

Finish setup by adding `.wk.toml` (next section) and running `wk init` from this directory if you'd rather scaffold it than hand-write it.

## 2. `.wk.toml`

Lives at the project root (next to `.bare/`). Declares everything specific to this repo.

```toml
name = "pgm-be"                  # optional; session names use this instead of the folder name

base_ref = "origin/main"         # optional; overrides the global default_base_ref

setup = [                        # run in order, inside the new worktree, after `git worktree add`
  "pnpm i --frozen-lockfile",
  "git config core.hooksPath ../hooks",
  "ln -s ../AGENTS.md .",
  "ln -s ../.env.custom .",
  "ln -s ../.env.custom-local .",
]

teardown = [                     # run in order, inside the worktree, before it's torn down
  "docker compose down -v",
]
```

| Field | Required | Meaning |
|---|---|---|
| `name` | no | Overrides the project-root folder name as the `{project}` used in session names. |
| `base_ref` | no | Overrides the global `default_base_ref` for this repo's new branches. |
| `setup` | no | Shell steps run inside a freshly created worktree, in order, stopping at the first failure. |
| `teardown` | no | Shell steps run inside a worktree right before it's deleted — for anything `git worktree remove` won't clean up on its own (stopping a docker stack, killing background processes, etc.). |

## 3. Global config

`~/.config/wk/config.toml` — applies to every project, holds nothing project-specific:

```toml
default_base_ref = "origin/main"
session_template = "{branch} [{project}]"
session_backend = "herdr"
```

| Field | Meaning |
|---|---|
| `default_base_ref` | Base ref for new branches when a repo's `.wk.toml` doesn't set its own `base_ref`. |
| `session_template` | How sessions are named — a herdr workspace label or a tmux session name. `{project}` resolves to `.wk.toml`'s `name`, or the project-root folder name if unset. |
| `session_backend` | `herdr` (default) or `tmux`. Overridable per run with `--herdr` / `--tmux` / `--backend <kind>`. |

## 4. Session backends

A worktree's session is a **herdr workspace** by default, or a **tmux session** with `session_backend = "tmux"`. Either way `wk` owns the git side itself — it runs `git worktree add`/`remove` directly and only asks the backend to host a session at the directory it just created.

Watch the terminology: a herdr **workspace** is the analogue of a tmux **session**; a herdr **session** is closer to a tmux **server**.

| `wk` does | herdr | tmux |
|---|---|---|
| start a session | `herdr worktree open --path <dir> --label <name> --no-focus` | `tmux new-session -d -s <name> -c <dir>` |
| attach / switch to it | `herdr workspace focus <id>`, plus launching the TUI from outside herdr | `tmux switch-client` inside `$TMUX`, else `tmux attach-session` |
| kill it | `herdr workspace close <id>` | `tmux kill-session -t <name>` |
| detect the current one | `$HERDR_ENV` + `$HERDR_WORKSPACE_ID` | `$TMUX` + `tmux display-message` |

`wk` uses `herdr worktree open`, not `herdr worktree create`/`remove` and not a plain `herdr workspace create --cwd`. `worktree create` would have herdr run its own git checkout on top of the one `wk` just made; a plain workspace carries no worktree metadata at all, so herdr can't group it under the project in the sidebar and `wk` can't find it again by path either (see below). `worktree open` registers an *existing* worktree's metadata without herdr touching git.

### How a worktree is matched to a herdr workspace

herdr addresses workspaces by an opaque id (`w1`, `w1K`) and stores the display name separately as a mutable label, so `wk` has to look the id up on every run (it keeps no state). It looks it up by **`worktree.checkout_path`, not by label**, because the label is the weaker key on all three counts:

- **Labels repeat.** One workspace per project checked out on `main` gives two workspaces labelled `main`.
- **Labels drift.** Renaming a workspace in the TUI, or changing `session_template`, would orphan every existing session.
- **Labels are written by other tools.** Workspaces created by worktrunk (`wt`) carry raw branch names, which never match `wk`'s template.

That last point is what makes the migration off `wt` painless: `wk delete` and `wk clean` find and close workspaces `wt` created, because the checkout path is the same either way. Matching on the path also means a workspace that isn't a git checkout has no path to match, so `wk` structurally cannot close one.

Two workspaces on a single worktree is refused rather than guessed at — closing the wrong one would take real work with it.

## 5. Day-to-day usage

```sh
wk add feat/123/add-btn      # create a worktree + branch off base_ref, start a session
wk add someone-branch        # existing origin/someone-branch? check it out and track it
wk add origin/someone-branch # same, spelled explicitly
gup                          # once ready to push: git push -u <remote> <branch>, re-points tracking
wk delete <dir>              # tear down a worktree + its session (alias: wk rm <dir>)
wk clean                     # sweep worktrees whose branch's PR is merged, offering to delete each
wk ls                        # show worktrees + session status for the current project
wk ls --porcelain            # same, but parse-friendly: 3-line blocks (branch, dir, session), no labels
wk init                      # scaffold a starter .wk.toml in the current directory
wk --tmux add feat/123       # same as above, but on tmux for this run only
```

`wk add <name>` with no `origin/` prefix checks whether `origin/<name>` already exists: if it does, the worktree checks that branch out and tracks it (shell completion offers exactly these names); if it doesn't, `<name>` is a new branch off `base_ref`.

`wk add` is the only way to create a worktree — a bare `wk <branch>` with no recognized subcommand is an error, not an implicit create (guards against typos accidentally creating worktrees).

If a prior `wk add` failed, run the same command again. `wk` verifies that the existing directory is the expected Git worktree and resumes incomplete work. It checkpoints successful `setup` in Git's per-worktree metadata, so a later Herdr/tmux failure retries only the session step; a failed `setup` has no checkpoint and is run again. An existing directory that is not the expected registered worktree, or belongs to another branch, remains an error.

`wk` finds the project root by walking up from cwd looking for `.wk.toml`, so any command works from inside a worktree checkout too, not just from the project root (next to `.bare/`) — `wk delete .` deletes whatever worktree you're standing in. If no `.wk.toml` turns up anywhere above cwd, `wk` errors out and offers to run `wk init` right there (`[y/N]`, defaults to no).

New branches created off `base_ref` (e.g. `origin/main`) are meant to track `base_ref` immediately, so `git pull --rebase` works before you've ever pushed — but `wk` doesn't yet set this explicitly (see [[spec]]'s plan-of-action step 6+); today it only happens if your global `branch.autoSetupMerge` git config already does it, same as the old pgm-fe POC relied on. Check `git status`/`git branch -vv` after your first `wk add` in a repo to confirm tracking landed on `base_ref` before assuming it. Once you're ready to push, run `gup` — it re-points tracking from `base_ref` to the branch's own remote counterpart.

## 6. Shell completion

`wk completion <zsh|bash|fish>` prints a completion script. `delete`/`rm` completion shells out to `wk ls --porcelain` for live worktree-dir suggestions; `add` completion suggests remote branch names.

For zsh, drop the script into a directory that's already in your `fpath` (before `compinit` runs) rather than `eval`-ing it — the `#compdef` marker that registers the completion is only picked up by `compinit` scanning `fpath`, not by `eval`:

```sh
mkdir -p ~/.zsh/completions   # or wherever your fpath already points
wk completion zsh > ~/.zsh/completions/_wk
```

Bash and fish can use the more familiar `eval` form:

```sh
eval "$(wk completion bash)"   # ~/.bashrc
wk completion fish | source    # ~/.config/fish/config.fish
```

## 7. Known sharp edges

- **Deleting your current session works, but session teardown is the last step**: `wk delete .`/`wk delete <dir>` on the worktree whose session you're currently inside runs teardown, removes the worktree, deletes the branch, and prunes *before* touching the session — killing the session you're inside can't leave a half-deleted worktree behind, since by then there's nothing left to lose.
- **Teardown runs before removal, not after**: a repo's `teardown` steps run while the worktree still exists, right after delete starts and before anything is torn down — not as post-removal cleanup.
- **`core.hooksPath` in `setup` is redundant-but-harmless after the first run**: it's stored in the repo's shared git config (not per-worktree), so every `wk add` re-sets the same value. Safe to leave in `setup` for a fresh clone's first worktree; it just no-ops on later ones.
