---
title: wk README
status: draft
tags: [tools, git, cli, go]
created: 2026-08-26
---

# wk

`wk` automates a specific workflow: one bare git repo per project, one worktree per branch, one tmux session per worktree. It is **not** a generic git-worktree tool — it encodes opinions about layout, config, and naming. This README documents those opinions.

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
name = "pgm-be"                  # optional; tmux sessions use this instead of the folder name

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
| `name` | no | Overrides the project-root folder name as the `{project}` used in tmux session names. |
| `base_ref` | no | Overrides the global `default_base_ref` for this repo's new branches. |
| `setup` | no | Shell steps run inside a freshly created worktree, in order, stopping at the first failure. |
| `teardown` | no | Shell steps run inside a worktree right before it's deleted — for anything `git worktree remove` won't clean up on its own (stopping a docker stack, killing background processes, etc.). |

## 3. Global config

`~/.config/wk/config.toml` — applies to every project, holds nothing project-specific:

```toml
default_base_ref = "origin/main"
tmux_session_template = "{branch} [{project}]"
```

| Field | Meaning |
|---|---|
| `default_base_ref` | Base ref for new branches when a repo's `.wk.toml` doesn't set its own `base_ref`. |
| `tmux_session_template` | How tmux sessions are named. `{project}` resolves to `.wk.toml`'s `name`, or the project-root folder name if unset. |

## 4. Day-to-day usage

```sh
wk add feat/123/add-btn      # create a worktree + branch off base_ref, start a tmux session
wk add origin/someone-branch # adopt someone else's pushed branch instead of branching off base_ref
gup                          # once ready to push: git push -u <remote> <branch>, re-points tracking
wk delete <dir>              # tear down a worktree (alias: wk rm <dir>)
wk clean                     # sweep worktrees whose branch's PR is merged, offering to delete each
wk ls                        # show worktrees + tmux session status for the current project
wk ls --porcelain            # same, but parse-friendly: 3-line blocks (branch, dir, session), no labels
wk init                      # scaffold a starter .wk.toml in the current directory
```

`wk add` is the only way to create a worktree — a bare `wk <branch>` with no recognized subcommand is an error, not an implicit create (guards against typos accidentally creating worktrees).

**Run every `wk` command from the project root** (next to `.bare/`), same as the bash POC requires today — `wk` does not walk up from subdirectories or worktrees to find `.wk.toml`. If it's missing in the current directory, `wk` errors out and offers to run `wk init` for you (`[y/N]`, defaults to no).

New branches created off `base_ref` (e.g. `origin/main`) are meant to track `base_ref` immediately, so `git pull --rebase` works before you've ever pushed — but `wk` doesn't yet set this explicitly (see [[spec]]'s plan-of-action step 6+); today it only happens if your global `branch.autoSetupMerge` git config already does it, same as the old pgm-fe POC relied on. Check `git status`/`git branch -vv` after your first `wk add` in a repo to confirm tracking landed on `base_ref` before assuming it. Once you're ready to push, run `gup` — it re-points tracking from `base_ref` to the branch's own remote counterpart.

## 5. Known sharp edges

- **Deleting your current session is refused, not survived**: `wk delete`/`wk rm` on the worktree whose tmux session you're currently attached to errors out instead of tearing it down — killing that session mid-command would kill the very process running the deletion. Run the delete from another session (or outside tmux) instead. A "survive the session's death" flow (switch to a fallback session, finish teardown in the background) is designed in [[spec]] but not yet built.
- **Teardown runs before removal, not after**: a repo's `teardown` steps run while the worktree still exists, right before the tmux session is killed and the worktree is removed — not as post-removal cleanup.
- **`core.hooksPath` in `setup` is redundant-but-harmless after the first run**: it's stored in the repo's shared git config (not per-worktree), so every `wk add` re-sets the same value. Safe to leave in `setup` for a fresh clone's first worktree; it just no-ops on later ones.
- **No project detection**: `wk` only ever looks for `.wk.toml` in the current directory — there's no registry and no walk-up. Running it from inside a worktree or any other subdirectory won't find your project's config; `cd` back to the project root first.
