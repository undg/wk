# Session backends

[Back to README](../README.md#session-backends)

WK owns Git worktree creation and removal. Backends host sessions for existing checkouts: a Herdr **workspace** corresponds to a tmux **session**; a Herdr **session** is closer to a tmux **server**.

| Operation | Herdr | tmux |
|---|---|---|
| Create | `herdr worktree open --cwd <project-root> --path <dir> --label <name> --no-focus` | `tmux new-session -d -s <name> -c <dir>` |
| Attach or switch | `herdr workspace focus <id>`, launching the TUI when outside Herdr | `tmux switch-client` inside `$TMUX`, otherwise `tmux attach-session` |
| Close | `herdr workspace close <id>` | `tmux kill-session -t <name>` |
| Detect context | `$HERDR_ENV` and `$HERDR_WORKSPACE_ID` | `$TMUX` and `tmux display-message` |

## Herdr registration and identity

`herdr worktree open` registers an existing checkout without performing Git operations. WK does not use `worktree create` or `remove`, which would duplicate its Git operations. A plain `workspace create --cwd` lacks the worktree metadata needed for project grouping and path-based lookup.

WK supplies `--cwd <project-root>` and strips `HERDR_WORKSPACE_ID` when opening a worktree, so source selection does not depend on the workspace currently displayed.

Herdr workspace IDs are opaque; labels are mutable and may repeat or come from other tools. WK resolves the workspace ID by matching the worktree's absolute path against **`worktree.checkout_path`**, never by label. This also supports workspaces created by other tools. Multiple workspaces matching one path are an error; WK does not guess which to close.

Tmux sessions are addressed by the name rendered from `session_template`. Unlike Herdr labels, these names are session identifiers.

## Focus behavior

Inside Herdr, `wk add` focuses its destination only if the launching workspace, tab, and pane are still active when creation finishes. Switching elsewhere leaves focus unchanged and prints that the workspace is ready. Missing caller identity or unavailable/incomplete focus snapshots also leave focus alone.

When deletion closes the currently focused linked-worktree workspace inside Herdr, WK first focuses its project's top-level workspace. The parent shares the child's nonempty `worktree.repo_key` and explicitly reports `worktree.is_linked_worktree = false`; exactly one parent must match. This works for normal checkouts and bare repository roots, independent of labels or branch names. A linked worktree named `main` is still a child.

Focus is checked after teardown and Git deletion, immediately before closing the workspace. Deleting a background workspace or switching elsewhere during teardown leaves focus unchanged. Missing metadata, an unavailable snapshot, an ambiguous or missing parent, or a failed focus request leaves Herdr's default close behavior in place. Root-workspace closure is unchanged. Herdr's focus command affects all attached clients; focus and close are separate API requests.

Outside Herdr, WK launches the TUI and attaches on add; deletion does not explicitly change focus. Tmux switches clients inside tmux and attaches otherwise; its deletion behavior is unchanged.

## Progress indicator

With `herdr_progress = true`, WK uses Herdr's inherited pane, binary, and socket environment to mark the invoking workspace busy during add/delete, including setup/teardown. It does nothing outside Herdr and skips panes that already have an agent.

Reporting failures do not fail the command. The temporary agent is released on completion or error, before attaching to or closing a workspace.

## Setup retries and deletion order

New worktrees have a setup checkpoint in Git's per-worktree metadata. Failed setup is retried; a later backend failure retries only session creation. Worktrees without a checkpoint skip setup to avoid repeating potentially non-idempotent commands. WK checks that a reused directory is the expected registered worktree and branch.

Deletion runs teardown, removes the worktree, deletes its branch, and prunes Git metadata before closing the backend session. Closing the current session therefore cannot interrupt those Git steps. Git removal errors stop deletion; WK never bypasses them with recursive filesystem removal.

If session closure fails, the worktree and branch are already gone. Retrying deletion cannot find that session through Git registration; close it manually with `herdr workspace close <id>` or `tmux kill-session -t <name>`.
