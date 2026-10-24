# Development metrics

[Back to README](../README.md#development)

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

## Source selection

Both metrics use the same source selection:

1. `FILES` selects exact Go implementation files and overrides `BASE` and `-all` targets. Separate multiple paths with spaces; wrap paths containing spaces in double quotes **inside** the value: `make crap FILES='"folder with space/file.go" session.go'`. These values are data, never evaluated as shell commands. Missing paths, globs, directories, symlink source files, outside-repository paths, tests, and generated files are rejected.
2. `BASE` must name the actual feature base; no branch is guessed or fetched. Selection compares `git merge-base HEAD <base>` against the current working tree, including feature commits, staged/unstaged changes, rename destinations, and nonignored untracked files. Deleted files, tests, generated source, hidden directories, vendor/testdata, and build output are excluded. A valid empty or test-only change prints `SKIP` and succeeds without producing reports; it never becomes a full audit.
3. `LIST=1` previews either metric's selection without installing/running metric tools. Unscoped commands fail. Full-project scope requires the explicit `-all` target. Clear `FILES` and `BASE` when requesting a genuinely full audit.

For repeatable exact arguments without Make's list syntax:

```sh
go run ./tools/metrics crap -file session.go -file sanitize.go -list
go run ./tools/metrics mutation -file session.go -dry-run
```

## Cost, isolation, and reports

Scope restricts mutated files or reported CRAP functions, **not the test suite**. Both commands run the full suite in a minimal source copy, including compiled-binary E2E tests with fake terminal backends. Current tests generate their fixtures; only Go sources and `go.mod`/`go.sum` are copied. If tests later need non-Go assets, update the copy allowlist and verify the unmutated copy first. Credentials, user settings, Git metadata, binaries, and unrelated workspace files are not copied. HOME, temporary state, Git discovery, Herdr/tmux context, and workspace configuration are isolated; Go's build/module caches are reused. Test and Go-build temporary state lives in a separate system-temp directory, outside the source snapshot, so upward `.wk.toml` discovery cannot reach the caller's checkout or project ancestors. That test-owned temporary directory is removed on exit; the source snapshot remains available for diagnosis.

Mutation testing first verifies the unmutated copy, then uses Gremlins integration mode with one worker and test-result caching disabled. Existing `GOFLAGS` are retained, with `-count=1`, a two-minute package-test timeout, and `-buildvcs=false` appended. Gremlins uses a per-mutant timeout of three times its coverage baseline; the whole metric run has a 30-minute budget. Nonzero `--test-cpu` is deliberately disabled: v0.6.0 formats that argument incorrectly. `DRY_RUN=1` discovers mutants only and is **not** a mutation score. Even one file can take minutes; full audits can be much more expensive. Gremlins skips uncovered mutants. Inspect killed, lived, uncovered, timed-out, not-viable, and skipped statuses separately; timeouts and unrelated build/test failures are not proof of assertion quality.

CRAP always collects fresh native Go coverage before running go-crap with an explicit sandbox module path, then filters its structured results to exact selected files. JSON retains function names, paths/lines, complexity, coverage, and CRAP; the console prints a compact table. The tool's full-project summary is intentionally omitted. No matching functions or unavailable coverage is an error rather than a misleading empty/zero report. **go-crap v0.5.1 calculates coverage as the fraction of covered basic blocks**, whereas native Go's coverage summary/HTML is statement-weighted. CRAP is `complexity² × (1 − coverage_fraction)³ + complexity`; around 30 is an investigation signal, not a universal gate.

Native coverage here does **not** instrument the separate `wk` binary built by E2E tests. Those tests still run and can kill covered mutants, but subprocess-only paths appear uncovered and are skipped by Gremlins. Treat these reports as in-process coverage, not whole-CLI coverage. This setup supports this repository's single Go module on macOS/Linux; additional modules, special build tags, or static test assets need corresponding runner support.

Reports live under the repository root, with UTC timestamps and short HEAD SHAs:

- `.reports/crap/yyyy-mm-dd_hh-mm_commitsha.json`, `.log`, `.meta.json`, `.coverage.out`, and `.coverage.html`.
- `.reports/mutation/yyyy-mm-dd_hh-mm_commitsha.json`, `.log`, and `.meta.json`.

Same-minute collisions get a numeric suffix; existing reports are never overwritten. Metadata records scope/base/merge-base, HEAD, dirty status, tool/toolchain versions, start/finish times, and outcome. Only `completed` reports are completed measurements; `dry-run`, failed, interrupted, and invalid runs are not scores. Always inspect metadata rather than reusing an older artifact after a skip/failure. Ignored `.cache/metrics/run-*` scratch copies are retained for diagnosis.

## Concurrent runs and lock errors

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
