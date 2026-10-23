# Automatic repository layout support

## Goal and scope

Implementation authorized. User approved `.worktrees/` and automatic `/.worktrees/` insertion in the main checkout's `.gitignore`. Run CRAP and mutation metrics before committing; multiple small commits are authorized.

Support both existing bare-repository projects and traditional clones without a layout flag. Preserve existing branch sanitization, session naming, setup/retry behavior, and backend path-based matching.

Requested behavior:
- Bare projects continue creating worktrees directly under the project root.
- Traditional clones create worktrees under a hidden, ignored directory at the main checkout root, using the existing sanitized directory-name convention.
- Detect the repository layout automatically.

## Evidence

- `create.go`: `runCreate` currently rejects non-bare repositories and uses `./<slug>`.
- `config.go`: project discovery walks upward for `.wk.toml` and changes cwd to that directory. Configuration location currently doubles as repository/layout identity.
- `git.go`: `isBareRepository` already queries Git, but a linked checkout reports non-bare even when its common repository is bare.
- `delete.go`: deletion supports paths, branches, and rendered session-template names. Its fallback after a failed Git removal calls `os.RemoveAll`; supporting normal clones requires protecting their main checkout before teardown or removal.
- `clean.go`: Git's registered worktree list includes a normal clone's main checkout. It must not become a cleanup candidate.
- `list.go`: listing already uses registered Git worktrees and can render nested relative paths.
- `init.go`: initialization currently writes `.wk.toml` in cwd, with setup examples assuming worktrees are one directory below the project root.

## Agreed decisions

- Use `.worktrees/` as the fixed directory for traditional clones. No configurable layout or directory option initially.
- Automatically ensure the anchored `/.worktrees/` entry exists in the main checkout's `.gitignore` before creating the first nested worktree. Preserve existing content and avoid duplicate entries. Fail clearly if the ignore update cannot be made; do not silently create unignored checkouts.
- Keep `.wk.toml` at the project root: the wrapper/bare root for bare layouts, the main checkout root for traditional clones. Keep configuration required.
- Resolve repository identity and layout using Git metadata, including the common Git directory, rather than directory names or session labels. Resolve layout consistently from the main checkout, linked worktrees, and their subdirectories.
- In a traditional clone, linked worktrees use the main checkout's project configuration even if they contain a tracked copy of `.wk.toml`. Do not let the nearest copied config turn a linked checkout into a separate project with nested `.worktrees` directories.
- Keep the main checkout visible in `wk ls`, but refuse its deletion and exclude it from `wk clean`. Identify it by Git metadata/path, not by branch name such as `main`.
- Safety finding during implementation: remove the recursive filesystem fallback when Git refuses worktree removal. Respect locked-worktree refusal instead of bypassing Git. This also makes registered-target validation effective against unsafe removal paths.
- Verify Git actually ignores the normal-clone container before creating it. Conflicting existing negations cause a clear error rather than silently creating unignored worktrees.
- Preserve existing worktree support outside the new container: registered paths remain authoritative for listing and branch/path deletion. Do not relocate existing worktrees.
- Relative setup and teardown commands still run inside the target worktree. Document that reaching a shared project-root file from `.worktrees/<slug>` requires `../../`, not `../`; do not rewrite user commands.

## Implementation tasks after agreement

- [x] Add repository-context resolution separating project root, common Git metadata, main checkout, and worktree destination directory. Preserve both the `.bare` wrapper and legacy bare-root layouts; surface Git detection errors rather than treating them as a normal clone.
- [x] Integrate context resolution with configuration discovery. Preserve caller identity for `delete .`; ensure invocation from linked checkouts and subdirectories uses the same owning project and layout. Define init/no-config behavior at the correct root without broadening unrelated initialization behavior.
- [x] Remove the bare-only creation restriction; select `<root>/<slug>` or `<root>/.worktrees/<slug>`. Add agreed ignore handling only for normal-clone creation. Preserve collision checks, checkpoint/retry behavior, branch tracking, and backend source/destination paths.
- [x] Update deletion resolution for nested sanitized directory names while preserving explicit paths, branch lookup, and rendered-session-name compatibility. Validate registered target identity and refuse the main checkout before teardown, filesystem fallback, branch deletion, or backend calls.
- [x] Exclude the main checkout from cleanup; verify listing and completion still expose usable linked-worktree paths without layout-specific guesses.
- [x] Add behavior-focused regression coverage for both bare layouts and normal clones: root/subdirectory/linked invocations, slash-containing branches, copied tracked config, ignore preservation/idempotence, creation retries/collisions, backend paths, deletion by branch/directory/path/`.` and subdirectory, and main-checkout deletion/cleanup protection. Include an ordinary unregistered directory rejection before destructive fallback.
- [x] Update README and `wk.1` (including its `.TH` date) with both layouts, automatic detection, ignore behavior, main-checkout protection, and relative setup-path implications. Review initializer examples for bare-only assumptions.
- [x] Run `make test`, `make e2e`, CRAP, and mutation testing; inspect findings and record actual results and any gaps before committing.

## Verification and quality assessment

- `make test`: passed (race-enabled full suite). `make e2e`: passed. `git diff --check`: passed.
- An initial cold full-suite run hit the existing one-second Herdr focus-query timeout; the focused rerun and subsequent full suites/metric baselines passed without changing that behavior.
- Final assessment scope: all nine implementation files changed against the starting commit `27cc868`, including `main.go` because its help text changed. Both metric runs completed before commits.
- `make crap BASE=27cc868`: completed. Report: `.reports/crap/2026-10-23_12-44_27cc868.json`; adjacent metadata, logs, coverage, and HTML are retained. Native application statement coverage is 67.5%; go-crap's function coverage uses basic blocks, not statement weighting.
- New layout functions: `resolveRepository` CRAP 10.98 (complexity 10, block coverage 78.6%); `ensureWorktreesIgnored` CRAP 8 (complexity 8, block coverage 100%). Target resolution: CRAP 15.22 (complexity 15, block coverage 90%).
- Remaining complexity findings: existing `runCreate` is CRAP 65.74 (complexity 37, block coverage 72.4%); `runDelete` is 30 (complexity 18, block coverage 66.7%). Broad workflow refactoring was deliberately not mixed into layout support. `main` and `resolveVersion` report 0% native coverage / CRAP 462 and 90: those existing paths are exercised only through the separate E2E binary, which native coverage does not instrument. This is a measurement limitation, not a claim that the CLI is untested.
- `make test-mutants BASE=27cc868`: completed. Report: `.reports/mutation/2026-10-23_12-44_27cc868.json`; adjacent metadata and logs are retained. 162 killed, 3 survived, 25 uncovered; 0 timeouts, invalid, or skipped mutants. Tool-reported efficacy 98.18%, mutator coverage 86.84%. All 19 generated mutants in `repository.go` were killed.
- Survivor review: `create.go:120` changes the empty-setup boundary (current tests do not distinguish later setup configuration changes after creating with no steps); `create.go:155` changes the Herdr retry-tip condition; `delete.go:94` changes the empty-teardown boundary (only extra logging for an empty shell-step list). These were not suppressed or counted as kills. Twenty uncovered mutants are in existing CLI/version paths; the other five are unexercised error/compatibility branches in cleanup parsing, config initialization outside Git, existing workspace rename, bare-root `delete .`, and missing-base fetching.
- Strengthened tests after initial findings: main-checkout safety, rejection of foreign/unregistered directories, locked-worktree refusal, nested deletion identities, ignore conflicts and write failures, setup/session retries, copied config precedence, initialization at the owning root, both backends' paths/labels, human and porcelain listing, detached/external worktrees, cleanup refusal/confirmation and deletion-error reporting, and bare upstream compatibility.

## Acceptance criteria

- Existing bare projects keep their paths and behavior without config changes.
- A normal clone creates `.worktrees/<existing-sanitized-slug>` and that container is ignored according to the agreed policy.
- Repeating creation from any linked checkout does not create another nested container or change project identity.
- Branch names, session names/badges, and upstream behavior do not change because of layout selection.
- Main checkout and unrelated directories cannot be removed or have teardown run through the new deletion paths; clean never offers the main checkout.
- Registered linked worktrees remain discoverable and removable by existing supported identities in either layout.
