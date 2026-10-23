# Working on wk

`wk` manages Git worktrees and their Herdr workspaces or tmux sessions. Git owns the worktree state; terminal backends host sessions for existing checkouts.

## Worktree identity — important

- Identify the target worktree by its Git branch name or directory/path first. `.` means the caller's current worktree, even if configuration loading later changes cwd.
- Resolve session details from that worktree, not the other way around. A session name, workspace label, or badge is not the worktree's identity.
- Herdr: match the worktree's absolute path against `worktree.checkout_path`, then use the matching workspace ID. Labels are mutable, may repeat, and may come from another tool. Never use labels to select a workspace. Multiple workspaces on the same path are an error; do not guess.
- tmux: derive the session name from the worktree's branch, project name, and configured `session_template`. Unlike Herdr, tmux currently addresses sessions by this name.
- Badges are Herdr display metadata only. They must not change branch names, directory names, upstreams, or worktree lookup/deletion. Omitting a badge preserves an existing workspace label.
- `delete.go` currently also accepts a rendered session-template name as a compatibility fallback. Preserve this behavior unless explicitly asked to change it, but do not make session labels the primary lookup key.

## Code map

- `git.go`: Git operations and registered worktrees.
- `create.go`, `delete.go`, `clean.go`, `list.go`: worktree workflows.
- `session.go`: backend interface and session-name rendering.
- `herdr.go`, `tmux.go`: backend-specific session handling.
- `config.go`: project/global configuration and project-root discovery.
- `README.md`: installation, configuration, workflow, and known limitations.
- `wk.1`: command-line manual. Update it when documented behavior changes; update its `.TH` date for substantive manual changes.

## Build and verification

Run `make help` to discover all available build, test, installation, and verification commands. See `README.md` for usage details.

Keep changes small. Preserve unrelated work. Add behavior-focused regression coverage when changing worktree resolution or backend matching; selecting the wrong worktree or session can destroy user work.
