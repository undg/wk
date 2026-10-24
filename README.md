---
title: wk README
status: draft
tags: [tools, git, cli, go]
created: 2026-08-26
---

# wk

`wk` manages Git worktrees in bare repositories and traditional clones: one worktree per branch, one session per worktree — a [herdr](https://herdr.dev) workspace by default, or a tmux session. It detects the repository layout automatically and keeps opinions about layout, config, and naming. This README documents those opinions.

> **Status**: the Go binary is implemented and replaces the old bash POC (`wk-pgm-fe.sh` + `wk-delete-merged.sh`). See [[spec]] for the full design and remaining plan-of-action items.

## Installation

Building requires Go 1.24.5 or newer. At runtime, `wk` needs Git and the selected session backend: [herdr](https://herdr.dev) by default or tmux. `wk clean` also needs the GitHub CLI (`gh`).

```sh
sudo make install                    # installs to /usr/local by default
make install PREFIX="$HOME/.local"   # user-local install; no sudo
man wk
```

`make install` installs both `wk` and `wk.1`. Use the same variables to remove them:

```sh
sudo make uninstall
make uninstall PREFIX="$HOME/.local"
```

For package staging, set `DESTDIR` while retaining the final `PREFIX`, for example `make install DESTDIR="$pkgdir" PREFIX=/usr`.

The Makefile uses portable POSIX tooling available on macOS and Linux. No `./configure` script is needed. The only usual platform difference is the installation prefix chosen by the system or package manager.

## 1. Repository layouts

### Bare repositories

New bare projects use the `.bare/`-wrapper convention (the older bare-root-is-project-root layout, still used by `pgm-fe`, is kept only for backward compatibility — don't use it for new projects):

```sh
mkdir -p ~/Code/<project> && cd ~/Code/<project>

# the actual bare git dir lives inside .bare/, not at the project root
git clone --bare <remote-url> .bare

# redirect plain `git` commands run from the project root to .bare/
echo "gitdir: ./.bare" > .git

# optional: shared hooks across every worktree
git --git-dir=.bare config core.hooksPath "$PWD/hooks"

# add the first worktree (usually main)
git worktree add .worktrees/main main
```

At this point `~/Code/<project>/` contains `.bare/`, `.git` (the gitfile), and `.worktrees/main/` (your first worktree). Any files you want symlinked into *every* worktree (shared env files, `AGENTS.md`, etc.) go at the project root too, as siblings of `.bare/` — `wk`'s `setup` steps (below) `ln -s ../../<file> .` them into new worktrees. The optional hooks setting uses an absolute path because Git resolves relative hook paths from different directories for different hooks.

Finish setup by adding `.wk.toml` (next section) and running `wk init` from this directory if you'd rather scaffold it than hand-write it.

### Traditional clones

No conversion to a bare repository is needed:

```sh
git clone <remote-url> project
cd project
wk init
wk add feat/login
```

`wk add` creates `.worktrees/feat-login/` inside the main checkout, using the same branch sanitization as bare projects. New branches start at the caller's current `HEAD`, not the configured `base_ref`. This includes invocation from a linked worktree or a subdirectory, and detached checkouts. Only committed changes are included. Before creating the first linked worktree, it adds `/.worktrees/` to the main checkout's `.gitignore`, preserving existing content and avoiding duplicate entries. Commit that ignore rule if you want it shared with collaborators. An ignore-file read or write failure, or a conflicting rule that leaves the container unignored, stops creation.

```text
project/
  .git/
  .gitignore
  .wk.toml
  .worktrees/
    feat-login/
```

Git metadata determines the owning project root, including when invoked from a linked worktree or a subdirectory. Both bare repositories and traditional clones create `<project-root>/.worktrees/<sanitized-branch>/`; for traditional clones, the project root is the main checkout. No layout flag or configuration setting is required. Existing registered worktrees are not moved. Bare projects do not need a project-root `.gitignore`.

The main checkout remains visible in `wk ls`, but `wk delete` refuses to remove it and `wk clean` skips it, regardless of its branch name.

## 2. `.wk.toml`

Lives at the project root: next to `.bare/` for a bare wrapper, at the bare root for the legacy layout, or in the main checkout for a traditional clone. A linked checkout's tracked copy does not override the owning project's config. Declares everything specific to this repo.

```toml
name = "pgm-be"                  # optional; session names use this instead of the folder name

base_ref = "origin/main"         # optional; bare repos only, overrides default_base_ref

setup = [                        # run in order, inside the new worktree, after `git worktree add`
  "pnpm i --frozen-lockfile",
  "ln -s ../../AGENTS.md .",
  "ln -s ../../.env.custom .",
  "ln -s ../../.env.custom-local .",
]

teardown = [                     # run in order, inside the worktree, before it's torn down
  "docker compose down -v",
]
```

| Field | Required | Meaning |
|---|---|---|
| `name` | no | Overrides the project-root folder name as the `{project}` used in session names. |
| `base_ref` | no | Overrides the global `default_base_ref` for new branches in a bare repo. Traditional clones use the caller's current `HEAD`. |
| `herdr_progress` | no | Overrides the global progress setting for this repo (`true` or `false`). |
| `setup` | no | Shell steps run inside a freshly created worktree, in order, stopping at the first failure. |
| `teardown` | no | Shell steps run inside a worktree right before it's deleted — for anything `git worktree remove` won't clean up on its own (stopping a docker stack, killing background processes, etc.). |

Setup and teardown always run inside the target worktree. To reach a shared file at the project root, use `../../<file>` in either repository layout. `wk` does not rewrite your shell commands.

## 3. Global config

`~/.config/wk/config.toml` — applies to every project, holds nothing project-specific:

```toml
default_base_ref = "origin/main"
session_template = "{branch} [{project}]"
session_backend = "herdr"
herdr_progress = true
```

| Field | Meaning |
|---|---|
| `default_base_ref` | Base ref for new branches in bare repos when `.wk.toml` doesn't set `base_ref`. Traditional clones use the caller's current `HEAD`. |
| `session_template` | How sessions are named — a herdr workspace label or a tmux session name. `{project}` resolves to `.wk.toml`'s `name`, or the project-root folder name if unset. |
| `session_backend` | `herdr` (default) or `tmux`. Overridable per run with `--herdr` / `--tmux` / `--backend <kind>`. |
| `herdr_progress` | `true` by default. Report `wk` as working in the invoking Herdr pane during add/delete, including setup/teardown. Set `false` to opt out globally; a repo's `.wk.toml` can override either value. |

Progress uses Herdr's inherited pane, binary, and socket environment variables; outside Herdr it does nothing. The busy indicator appears on the invoking workspace, not the destination worktree. It skips panes that already have an agent, and reporting failures do not fail the command. The temporary agent is released on completion or errors, before attaching or closing a workspace.

## 4. Session backends

A worktree's session is a **herdr workspace** by default, or a **tmux session** with `session_backend = "tmux"`. Either way `wk` owns the git side itself — it runs `git worktree add`/`remove` directly and only asks the backend to host a session at the directory it just created.

Watch the terminology: a herdr **workspace** is the analogue of a tmux **session**; a herdr **session** is closer to a tmux **server**.

| `wk` does | herdr | tmux |
|---|---|---|
| start a session | `herdr worktree open --cwd <project-root> --path <dir> --label <name> --no-focus` | `tmux new-session -d -s <name> -c <dir>` |
| attach / switch to it | `herdr workspace focus <id>`, plus launching the TUI from outside herdr | `tmux switch-client` inside `$TMUX`, else `tmux attach-session` |
| kill it | `herdr workspace close <id>` | `tmux kill-session -t <name>` |
| detect the current one | `$HERDR_ENV` + `$HERDR_WORKSPACE_ID` | `$TMUX` + `tmux display-message` |

`wk` uses `herdr worktree open`, not `herdr worktree create`/`remove` and not a plain `herdr workspace create --cwd`. `worktree create` would have herdr run its own git checkout on top of the one `wk` just made; a plain workspace carries no worktree metadata at all, so herdr can't group it under the project in the sidebar and `wk` can't find it again by path either (see below). `worktree open` registers an *existing* worktree's metadata without herdr touching git. `wk` passes the project root explicitly with `--cwd` and strips `HERDR_WORKSPACE_ID` so source selection does not depend on the pane or workspace you are viewing. Inside Herdr, `wk add` focuses the destination only if you are still in the launching workspace, tab, and pane when it finishes. If you switch elsewhere, it leaves focus alone and prints that the workspace is ready. Missing caller identity or an unavailable/incomplete focus snapshot also leaves focus unchanged. Outside Herdr, attaching works as before; tmux behavior is unchanged.

### How a worktree is matched to a herdr workspace

herdr addresses workspaces by an opaque id (`w1`, `w1K`) and stores the display name separately as a mutable label, so `wk` has to look the id up on every run (it keeps no state). It looks it up by **`worktree.checkout_path`, not by label**, because the label is the weaker key on all three counts:

- **Labels repeat.** One workspace per project checked out on `main` gives two workspaces labelled `main`.
- **Labels drift.** Renaming a workspace in the TUI, or changing `session_template`, would orphan every existing session.
- **Labels are written by other tools.** Workspaces created by worktrunk (`wt`) carry raw branch names, which never match `wk`'s template.

That last point is what makes the migration off `wt` painless: `wk delete` and `wk clean` find and close workspaces `wt` created, because the checkout path is the same either way. Matching on the path also means a workspace that isn't a git checkout has no path to match, so `wk` structurally cannot close one.

Two workspaces on a single worktree is refused rather than guessed at — closing the wrong one would take real work with it.

## 5. Day-to-day usage

```sh
wk add feat/123/add-btn      # new branch from current HEAD (bare repo: base_ref), start session
wk add someone-branch        # existing origin/someone-branch? check it out and track it
wk add origin/someone-branch # same, spelled explicitly
wk add origin/someone-branch rev # Herdr label: [rev] someone-branch [project]
gup                          # once ready to push: git push -u <remote> <branch>, re-points tracking
wk delete <dir>              # tear down a worktree + its session (alias: wk rm <dir>)
wk clean                     # sweep worktrees whose branch's PR is merged, offering to delete each
wk ls                        # show worktrees + session status for the current project
wk ls --porcelain            # same, but parse-friendly: 3-line blocks (branch, dir, session), no labels
wk init                      # scaffold .wk.toml at the owning project root (cwd outside Git)
wk --tmux add feat/123       # same as above, but on tmux for this run only
```

`wk add <name>` with no `origin/` prefix reuses an existing local branch first. Otherwise, if `origin/<name>` exists, the worktree checks that branch out and tracks it (shell completion offers exactly these names). If neither exists, `<name>` is a new branch from the caller's current `HEAD` in a traditional clone, or from `base_ref` in a bare repository.

`wk add <branch> [badge]` accepts an optional display-only badge for Herdr. The badge prefixes the configured session label; the existing project suffix stays intact. Supplying another badge replaces the label with `[badge] <rendered session_template>`, including for an already-open workspace. Omitting the badge leaves an existing label unchanged. Branch names, worktree paths, upstreams, and deletion by branch/path/`.` are unaffected. Badges live only in Herdr's label: to recreate a closed workspace with its badge, supply it again. Badges are not supported by the tmux backend.

`wk add` is the only way to create a worktree — a bare `wk <branch>` with no recognized subcommand is an error, not an implicit create (guards against typos accidentally creating worktrees).

If a prior `wk add` failed, run the same command again. `wk` verifies that the existing directory is the expected Git worktree and resumes incomplete work. New worktrees are checkpointed in Git's per-worktree metadata before `setup` starts: a failed setup is retried, while a later Herdr/tmux failure retries only the session step. Worktrees created before this behavior have no checkpoint, so `wk` safely skips their potentially non-idempotent setup and resumes the session instead. An existing directory that is not the expected registered worktree, or belongs to another branch, remains an error.

`wk` finds the owning project through Git's common repository metadata, so commands work from the root, linked checkouts, or their subdirectories. `wk delete .` targets the caller's current worktree, even from a subdirectory. Deletion accepts a branch, sanitized directory name, project-relative path such as `.worktrees/feat-login`, or absolute registered-worktree path; rendered session-template names remain a compatibility fallback. An unregistered directory cannot be deleted. If the owning root has no `.wk.toml`, `wk` offers to scaffold it there (`[y/N]`, defaults to no). `wk init` also resolves the owning root when invoked from inside a checkout; outside Git it scaffolds in cwd.

In bare repositories, new branches created off `base_ref` (e.g. `origin/main`) are meant to track `base_ref` immediately, so `git pull --rebase` works before you've ever pushed — but `wk` doesn't yet set this explicitly (see [[spec]]'s plan-of-action step 6+); today it only happens if your global `branch.autoSetupMerge` git config already does it, same as the old pgm-fe POC relied on. Check `git status`/`git branch -vv` after your first `wk add` in a repo to confirm tracking landed on `base_ref` before assuming it. Once you're ready to push, run `gup` — it re-points tracking from `base_ref` to the branch's own remote counterpart.

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

## Development metrics

Optional metric tools are separate from `wk`'s runtime dependencies and ordinary tests:

```sh
make test                       # normal race-enabled checks
make tools                      # pinned tools in bin/metrics/<os>-<arch>; requires Go 1.26.2+
make test-mutants FILES="session.go"
make crap FILES="session.go sanitize.go"
make test-mutants BASE=<feature-base> LIST=1  # preview before expensive work
make test-mutants BASE=<feature-base>
make crap BASE=<feature-base>
make test-mutants-all            # explicit full-project audit; potentially very slow
make crap-all
```

`make tools` installs Gremlins v0.6.0 and go-crap v0.5.1 using their pinned module dependencies. Binaries live in `bin/metrics/<os>-<arch>/` so a shared checkout can hold separate Mac and Linux tools without overwriting them; the runner also accepts the original `bin/metrics/` layout as a fallback. Go may download the development toolchain when needed; the application's Go 1.24.5 minimum and `go.mod` are unchanged. Downloads require network access. No tools install automatically during tests or metric runs, and no score gates are imposed.

Both metrics use the same source selection:

1. `FILES` selects exact Go implementation files and overrides `BASE` and `-all` targets. Separate multiple paths with spaces; wrap paths containing spaces in double quotes **inside** the value: `make crap FILES='"folder with space/file.go" session.go'`. These values are data, never evaluated as shell commands. Missing paths, globs, directories, symlink source files, outside-repository paths, tests, and generated files are rejected.
2. `BASE` must name the actual feature base; no branch is guessed or fetched. Selection compares `git merge-base HEAD <base>` against the current working tree, including feature commits, staged/unstaged changes, rename destinations, and nonignored untracked files. Deleted files, tests, generated source, hidden directories, vendor/testdata, and build output are excluded. A valid empty or test-only change prints `SKIP` and succeeds without producing reports; it never becomes a full audit.
3. `LIST=1` previews either metric's selection without installing/running metric tools. Unscoped commands fail. Full-project scope requires the explicit `-all` target. Clear `FILES` and `BASE` when requesting a genuinely full audit.

For repeatable exact arguments without Make's list syntax:

```sh
go run ./tools/metrics crap -file session.go -file sanitize.go -list
go run ./tools/metrics mutation -file session.go -dry-run
```

### Cost, isolation, and reports

Scope restricts mutated files or reported CRAP functions, **not the test suite**. Both commands run the full suite in a minimal source copy, including compiled-binary E2E tests with fake terminal backends. Current tests generate their fixtures; only Go sources and `go.mod`/`go.sum` are copied. If tests later need non-Go assets, update the copy allowlist and verify the unmutated copy first. Credentials, user settings, Git metadata, binaries, and unrelated workspace files are not copied. HOME, temporary state, Git discovery, Herdr/tmux context, and workspace configuration are isolated; Go's build/module caches are reused. Test and Go-build temporary state lives in a separate system-temp directory, outside the source snapshot, so upward `.wk.toml` discovery cannot reach the caller's checkout or project ancestors. That test-owned temporary directory is removed on exit; the source snapshot remains available for diagnosis.

Mutation testing first verifies the unmutated copy, then uses Gremlins integration mode with one worker and test-result caching disabled. Existing `GOFLAGS` are retained, with `-count=1`, a two-minute package-test timeout, and `-buildvcs=false` appended. Gremlins uses a per-mutant timeout of three times its coverage baseline; the whole metric run has a 30-minute budget. Nonzero `--test-cpu` is deliberately disabled: v0.6.0 formats that argument incorrectly. `DRY_RUN=1` discovers mutants only and is **not** a mutation score. Even one file can take minutes; full audits can be much more expensive. Gremlins skips uncovered mutants. Inspect killed, lived, uncovered, timed-out, not-viable, and skipped statuses separately; timeouts and unrelated build/test failures are not proof of assertion quality.

CRAP always collects fresh native Go coverage before running go-crap with an explicit sandbox module path, then filters its structured results to exact selected files. JSON retains function names, paths/lines, complexity, coverage, and CRAP; the console prints a compact table. The tool's full-project summary is intentionally omitted. No matching functions or unavailable coverage is an error rather than a misleading empty/zero report. **go-crap v0.5.1 calculates coverage as the fraction of covered basic blocks**, whereas native Go's coverage summary/HTML is statement-weighted. CRAP is `complexity² × (1 − coverage_fraction)³ + complexity`; around 30 is an investigation signal, not a universal gate.

Native coverage here does **not** instrument the separate `wk` binary built by E2E tests. Those tests still run and can kill covered mutants, but subprocess-only paths appear uncovered and are skipped by Gremlins. Treat these reports as in-process coverage, not whole-CLI coverage. This setup supports this repository's single Go module on macOS/Linux; additional modules, special build tags, or static test assets need corresponding runner support.

Reports live under the repository root, with UTC timestamps and short HEAD SHAs:

- `.reports/crap/yyyy-mm-dd_hh-mm_commitsha.json`, `.log`, `.meta.json`, `.coverage.out`, and `.coverage.html`.
- `.reports/mutation/yyyy-mm-dd_hh-mm_commitsha.json`, `.log`, and `.meta.json`.

Same-minute collisions get a numeric suffix; existing reports are never overwritten. Metadata records scope/base/merge-base, HEAD, dirty status, tool/toolchain versions, start/finish times, and outcome. Only `completed` reports are completed measurements; `dry-run`, failed, interrupted, and invalid runs are not scores. Always inspect metadata rather than reusing an older artifact after a skip/failure. Ignored `.cache/metrics/run-*` scratch copies are retained for diagnosis.

### Concurrent runs and lock errors

**Run only one metric command at a time per checkout.** CRAP and mutation commands share one OS file lock, including scoped and `-all` targets. A second run fails immediately rather than waiting. Scope previews (`LIST=1`) do not acquire the lock.

To run both full audits sequentially, starting mutation testing only if CRAP succeeds:

```sh
make crap-all && make test-mutants-all
```

If a command reports `cannot acquire metrics lock` with `resource temporarily unavailable`, check the lock holder on the machine running the commands:

```sh
lsof .cache/metrics/lock/active.lock
```

Let the existing run finish, or interrupt it in its original terminal before retrying. The OS releases the lock when the process exits, including after a crash. The persistent lock file and directory alone do not indicate an active run. **Do not delete the lock file while metrics may be running**: replacing it could allow two processes to lock different files and run concurrently. Empty directories left by the older directory-lock implementation need no cleanup.

## 7. Known sharp edges

- **Git's refusal to remove a worktree is respected**: deletion stops on a `git worktree remove --force` error rather than recursively deleting the directory itself. Locked worktrees must be explicitly unlocked before retrying. Teardown still runs before Git attempts removal.

- **Deleting your current session works, but session teardown is the last step**: `wk delete .`/`wk delete <dir>` on the worktree whose session you're currently inside runs teardown, removes the worktree, deletes the branch, and prunes *before* touching the session — killing the session you're inside can't leave a half-deleted worktree behind, since by then there's nothing left to lose.
- **A failed session kill on delete leaves an orphan `wk` can't find again**: the flip side of the ordering above. If the kill fails (herdr not running, two workspaces on one path, socket error), the worktree and branch are already gone, so rerunning `wk delete` reports `worktree directory not found` and `wk clean` never sees it — both look sessions up via git worktrees. Close it by hand: `herdr workspace close <id>` or `tmux kill-session -t <name>`. Not made resumable: that would need a session lookup independent of git, which isn't worth it for a rare failure with a one-command fix.
- **Teardown runs before removal, not after**: a repo's `teardown` steps run while the worktree still exists, right after delete starts and before anything is torn down — not as post-removal cleanup.
- **`core.hooksPath` in `setup` is redundant-but-harmless after the first run**: it's stored in the repo's shared git config (not per-worktree), so every `wk add` re-sets the same value. Safe to leave in `setup` for a fresh clone's first worktree; it just no-ops on later ones.
- **Herdr source selection requires explicit context**: older `wk` versions passed only the destination `--path`, allowing Herdr to infer the source from UI context. This could produce `linked_worktree_source` or `worktree_not_found`, including after switching panes during setup. `wk` now passes `--cwd <project-root>` as well. If opening still fails, the worktree and setup are preserved; rerun `wk add <branch>` to retry the workspace step. The explicit-source command is covered by regression tests and has been verified by switching workspaces during setup.
- **`setup` runs in the foreground, in the pane you called `wk` from**: that shell stays busy until setup finishes, and the session is created only afterwards. You can switch to another pane, tab, or workspace while it runs, but keep the launching pane open. Setup keeps its terminal input/output, and failures remain visible there.
