package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// These tests deliberately execute the compiled binary. Git is real; tmux,
// herdr, and gh are small PATH fakes because their servers and GitHub are not
// part of wk's contract and must not be required by CI.
var e2eBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wk-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	e2eBinary = filepath.Join(dir, "wk")
	build := exec.Command("go", "build", "-o", e2eBinary, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

type e2eResult struct {
	output string
	err    error
}

type e2eProject struct {
	t         *testing.T
	root      string
	home      string
	fakeState string
	env       []string
}

func newE2EProject(t *testing.T, repoConfig string) *e2eProject {
	t.Helper()
	root := t.TempDir()
	p := &e2eProject{
		t:         t,
		root:      filepath.Join(root, "project"),
		home:      filepath.Join(root, "home"),
		fakeState: filepath.Join(root, "fake-state"),
	}
	mustMkdirAll(t, p.root)
	mustMkdirAll(t, p.home)
	mustMkdirAll(t, p.fakeState)
	fakeBin := filepath.Join(root, "fake-bin")
	mustMkdirAll(t, fakeBin)
	writeFakes(t, fakeBin)

	remote := filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	testGit(t, root, "init", "--bare", remote)
	testGit(t, root, "init", seed)
	testGit(t, seed, "config", "user.name", "wk e2e")
	testGit(t, seed, "config", "user.email", "wk-e2e@example.invalid")
	mustWriteFile(t, filepath.Join(seed, "README"), "main\n")
	testGit(t, seed, "add", "README")
	testGit(t, seed, "commit", "-m", "main")
	testGit(t, seed, "branch", "-M", "main")
	testGit(t, seed, "remote", "add", "origin", remote)
	testGit(t, seed, "push", "origin", "main")

	testGit(t, seed, "checkout", "-b", "release")
	mustWriteFile(t, filepath.Join(seed, "RELEASE"), "release\n")
	testGit(t, seed, "add", "RELEASE")
	testGit(t, seed, "commit", "-m", "release")
	testGit(t, seed, "push", "origin", "release")
	testGit(t, seed, "checkout", "main")

	for _, branch := range []string{"remote-branch", "explicit-remote"} {
		testGit(t, seed, "checkout", "-b", branch)
		mustWriteFile(t, filepath.Join(seed, branch), branch+"\n")
		testGit(t, seed, "add", branch)
		testGit(t, seed, "commit", "-m", branch)
		testGit(t, seed, "push", "origin", branch)
		testGit(t, seed, "checkout", "main")
	}

	testGit(t, root, "clone", "--bare", remote, filepath.Join(p.root, ".bare"))
	mustWriteFile(t, filepath.Join(p.root, ".git"), "gitdir: ./.bare\n")
	mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), repoConfig)
	testGit(t, p.root, "fetch", "origin", "+refs/heads/*:refs/remotes/origin/*")
	// git clone --bare stores remote heads as local refs. Create the normal
	// remote-tracking refs explicitly too; wk's default base_ref is origin/main.
	for _, branch := range []string{"main", "release", "remote-branch", "explicit-remote"} {
		testGit(t, p.root, "update-ref", "refs/remotes/origin/"+branch, "refs/heads/"+branch)
	}
	testGit(t, p.root, "worktree", "add", "main", "main")

	p.env = setEnv(os.Environ(), map[string]string{
		"HOME":                p.home,
		"XDG_CONFIG_HOME":     filepath.Join(p.home, ".config"),
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
		"PATH":                fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"WK_FAKE_STATE":       p.fakeState,
	})
	return p
}

func (p *e2eProject) run(input string, args ...string) e2eResult {
	return runE2E(p.root, p.env, input, args...)
}

func (p *e2eProject) runAt(dir, input string, args ...string) e2eResult {
	return runE2E(dir, p.env, input, args...)
}

func (p *e2eProject) writeGlobal(content string) {
	p.t.Helper()
	path := filepath.Join(p.home, ".config", "wk", "config.toml")
	mustMkdirAll(p.t, filepath.Dir(path))
	mustWriteFile(p.t, path, content)
}

func (p *e2eProject) writeGH(branch, state string) {
	p.t.Helper()
	mustWriteFile(p.t, filepath.Join(p.fakeState, "gh-"+branch), state+"\n")
}

func runE2E(dir string, env []string, input string, args ...string) e2eResult {
	cmd := exec.Command(e2eBinary, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return e2eResult{output: stripANSI(out.String()), err: err}
}

func TestE2ECLIInterfaceAndCompletion(t *testing.T) {
	dir := t.TempDir()
	env := setEnv(os.Environ(), map[string]string{
		"HOME":                filepath.Join(dir, "home"),
		"XDG_CONFIG_HOME":     filepath.Join(dir, "home", ".config"),
		"GIT_CONFIG_NOSYSTEM": "1",
	})

	for _, arg := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		got := runE2E(dir, env, "", arg...)
		mustSucceed(t, got)
		mustContain(t, got.output, "USAGE")
	}
	for _, arg := range [][]string{{"version"}, {"-v"}, {"--version"}} {
		got := runE2E(dir, env, "", arg...)
		mustSucceed(t, got)
		mustContain(t, got.output, "wk ")
	}

	for shell, marker := range map[string]string{
		"zsh":  "#compdef wk",
		"bash": "complete -F _wk_completions wk",
		"fish": "complete -c wk -l backend",
	} {
		got := runE2E(dir, env, "", "completion", shell)
		mustSucceed(t, got)
		mustContain(t, got.output, marker)
		mustContain(t, got.output, "--porcelain")
		mustContain(t, got.output, "delete")
		mustContain(t, got.output, "origin/")
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "missing argument"},
		{[]string{"feat/no-implicit-create"}, "did you mean 'wk add"},
		{[]string{"completion"}, "expected exactly one shell argument"},
		{[]string{"completion", "powershell"}, "unknown shell"},
		{[]string{"--backend"}, "--backend needs herdr or tmux"},
		{[]string{"--backend="}, "--backend needs herdr or tmux"},
		{[]string{"--backend=screen", "help"}, "--backend needs herdr or tmux"},
		{[]string{"--backend", "screen", "help"}, "--backend needs herdr or tmux"},
		{[]string{"init", "extra"}, "init takes no arguments"},
	} {
		got := runE2E(dir, env, "", c.args...)
		mustFail(t, got)
		mustContain(t, got.output, c.want)
	}

	got := runE2E(dir, env, "", "init")
	mustSucceed(t, got)
	initConfig := filepath.Join(dir, ".wk.toml")
	content := mustReadFile(t, initConfig)
	mustContain(t, content, fmt.Sprintf("name = %q", filepath.Base(dir)))
	mustContain(t, content, "base_ref = \"origin/main\"")
	mustContain(t, content, "setup = [")
	mustContain(t, content, "teardown = [")
	got = runE2E(dir, env, "", "init")
	mustFail(t, got)
	mustContain(t, got.output, "already exists")

	noConfig := t.TempDir()
	got = runE2E(noConfig, env, "n\n", "ls")
	mustFail(t, got)
	mustContain(t, got.output, "Run 'wk init' here?")
	if _, err := os.Stat(filepath.Join(noConfig, ".wk.toml")); !os.IsNotExist(err) {
		t.Fatalf("no answer unexpectedly wrote .wk.toml: %v", err)
	}
	got = runE2E(noConfig, env, "YES\n", "ls")
	mustFail(t, got) // ls still needs a Git repository after scaffolding.
	if _, err := os.Stat(filepath.Join(noConfig, ".wk.toml")); err != nil {
		t.Fatalf("yes answer did not scaffold .wk.toml: %v", err)
	}
}

func TestE2EAddConfigurationBackendFlagsAndLs(t *testing.T) {
	p := newE2EProject(t, `name = "cave"
base_ref = "origin/release"
setup = ["printf setup > setup.marker"]
teardown = ["test -f setup.marker && printf teardown > ../teardown.marker"]
`)
	p.writeGlobal(`default_base_ref = "origin/main"
session_template = "S-{project}-{branch}"
session_backend = "tmux"
`)
	// A bare clone represents remote branches as local refs. Remove this one
	// so the unprefixed remote-branch case takes wk's remote-tracking path.
	testGit(t, p.root, "branch", "-D", "remote-branch")
	testGit(t, p.root, "branch", "local", "origin/main")

	// Every accepted backend spelling is exercised around a real add. Flags are
	// accepted before or after command operands because main extracts them first.
	for _, c := range []struct {
		args    []string
		branch  string
		backend string
	}{
		{[]string{"add", "--tmux", "feat/one"}, "feat/one", "tmux"},
		{[]string{"--backend=tmux", "add", "origin/explicit-remote"}, "explicit-remote", "tmux"},
		{[]string{"add", "remote-branch", "--backend", "tmux"}, "remote-branch", "tmux"},
		{[]string{"--backend", "herdr", "add", "feat/herdr"}, "feat/herdr", "herdr"},
		{[]string{"add", "feat/herdr-short", "--herdr"}, "feat/herdr-short", "herdr"},
		{[]string{"--tmux", "--herdr", "add", "feat/last-wins"}, "feat/last-wins", "herdr"},
	} {
		got := p.run("", c.args...)
		mustSucceed(t, got)
		dir := filepath.Join(p.root, SanitizeBranch(c.branch))
		mustDir(t, dir)
		mustEqual(t, gitOutput(t, p, p.root, "-C", dir, "rev-parse", "--abbrev-ref", "HEAD"), c.branch)
		if c.branch == "feat/one" {
			mustEqual(t, gitOutput(t, p, p.root, "rev-parse", "feat/one"), gitOutput(t, p, p.root, "rev-parse", "origin/release"))
			mustEqual(t, mustReadFile(t, filepath.Join(dir, "setup.marker")), "setup")
		}
		if c.backend == "tmux" {
			mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux-sessions")), "S-cave-"+c.branch)
		} else {
			mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "herdr-workspaces")), "S-cave-"+c.branch)
		}
	}
	mustEqual(t, gitOutput(t, p, p.root, "-C", filepath.Join(p.root, "explicit-remote"), "rev-parse", "--abbrev-ref", "@{upstream}"), "origin/explicit-remote")
	mustEqual(t, gitOutput(t, p, p.root, "-C", filepath.Join(p.root, "remote-branch"), "rev-parse", "--abbrev-ref", "@{upstream}"), "origin/remote-branch")

	got := p.run("", "ls", "--porcelain", "--tmux")
	mustSucceed(t, got)
	mustContain(t, got.output, "feat/one\n"+mustRealPath(t, filepath.Join(p.root, "feat-one"))+"\nS-cave-feat/one\n")
	mustNotContain(t, got.output, "\x1b[")
	got = p.run("", "ls", "--tmux")
	mustSucceed(t, got)
	mustContain(t, got.output, "BRANCH")
	mustContain(t, got.output, "WORKTREE")
	mustContain(t, got.output, "SESSION")
	mustContain(t, got.output, "no tmux session")

	for _, args := range [][]string{{"ls", "--bad"}, {"ls", "--porcelain", "again"}, {"add"}, {"delete"}} {
		got := p.run("", args...)
		mustFail(t, got)
	}

	// Commands invoked from a nested worktree find the project config above it.
	nested := filepath.Join(p.root, "main", "nested")
	mustMkdirAll(t, nested)
	got = p.runAt(nested, "", "ls", "--porcelain", "--tmux")
	mustSucceed(t, got)
	mustContain(t, got.output, "feat/one")

	// Missing global options use the documented defaults, including herdr.
	defaults := newE2EProject(t, "")
	got = defaults.run("", "add", "feat/default")
	mustSucceed(t, got)
	projectName := filepath.Base(defaults.root)
	mustContain(t, mustReadFile(t, filepath.Join(defaults.fakeState, "herdr-workspaces")), "feat/default ["+projectName+"]")
	mustEqual(t, gitOutput(t, defaults, defaults.root, "rev-parse", "feat/default"), gitOutput(t, defaults, defaults.root, "rev-parse", "origin/main"))
}

func TestE2EDeleteAcceptedTargetsAndTeardown(t *testing.T) {
	p := newE2EProject(t, `name = "cave"
setup = ["printf setup > setup.marker"]
teardown = ["test -f setup.marker && printf teardown > ../teardown.marker"]
`)
	p.writeGlobal("session_template = \"S-{project}-{branch}\"\nsession_backend = \"tmux\"\n")

	add := func(branch string, args ...string) string {
		t.Helper()
		if len(args) == 0 {
			args = []string{"add", branch}
		}
		mustSucceed(t, p.run("", args...))
		return filepath.Join(p.root, SanitizeBranch(branch))
	}
	one := add("feat/one")
	remote := add("remote-branch")
	local := add("local")
	absolute := add("feat/absolute")
	rm := add("feat/rm")
	dot := add("feat/dot")
	herdr := add("feat/herdr", "--herdr", "add", "feat/herdr")

	// Directory, branch, session label, absolute path, rm alias, current
	// worktree, and alternate backend each take the real delete path.
	for _, c := range []struct {
		dir  string
		args []string
	}{
		{one, []string{"delete", "feat-one"}},
		{remote, []string{"delete", "remote-branch"}},
		{local, []string{"delete", "S-cave-local"}},
		{absolute, []string{"delete", absolute}},
		{rm, []string{"rm", "feat-rm"}},
		{dot, []string{"delete", "."}},
		{herdr, []string{"--backend", "herdr", "delete", "feat-herdr"}},
	} {
		var got e2eResult
		if c.dir == dot {
			got = p.runAt(dot, "", c.args...)
		} else {
			got = p.run("", c.args...)
		}
		mustSucceed(t, got)
		if _, err := os.Stat(c.dir); !os.IsNotExist(err) {
			t.Fatalf("worktree still exists after %v: %v", c.args, err)
		}
	}
	mustEqual(t, mustReadFile(t, filepath.Join(p.root, "teardown.marker")), "teardown")
	mustNotContain(t, readIfExists(t, filepath.Join(p.fakeState, "tmux-sessions")), "S-cave-feat/one")
	mustNotContain(t, readIfExists(t, filepath.Join(p.fakeState, "herdr-workspaces")), "feat/herdr")

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"delete", "does-not-exist"}, "worktree directory not found"},
	} {
		got := p.run("", c.args...)
		mustFail(t, got)
		mustContain(t, got.output, c.want)
	}
}

func TestE2ECleanAndConfigurationFailures(t *testing.T) {
	p := newE2EProject(t, "")
	p.writeGlobal("session_backend = \"tmux\"\n")
	mustSucceed(t, p.run("", "add", "merged"))
	mustSucceed(t, p.run("", "add", "open"))
	p.writeGH("merged", "MERGED")
	p.writeGH("open", "OPEN")

	got := p.run("yes\n", "clean", "--tmux")
	mustSucceed(t, got)
	if _, err := os.Stat(filepath.Join(p.root, "merged")); !os.IsNotExist(err) {
		t.Fatalf("merged worktree was not deleted: %v", err)
	}
	mustDir(t, filepath.Join(p.root, "open"))
	mustContain(t, got.output, "open: OPEN")
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "gh.log")), "pr view --json state --jq .state")

	badGlobal := newE2EProject(t, "")
	badGlobal.writeGlobal("session_backend = \"screen\"\n")
	got = badGlobal.run("", "ls")
	mustFail(t, got)
	mustContain(t, got.output, "unknown session backend")
	mustEqual(t, gitOutput(t, badGlobal, badGlobal.root, "worktree", "list", "--porcelain"), gitOutput(t, badGlobal, badGlobal.root, "worktree", "list", "--porcelain"))

	badRepo := newE2EProject(t, "not = [valid")
	got = badRepo.run("", "ls")
	mustFail(t, got)
	mustContain(t, got.output, "parsing repo config")
}

func writeFakes(t *testing.T, dir string) {
	t.Helper()
	mustWriteExecutable(t, filepath.Join(dir, "tmux"), `#!/bin/sh
set -eu
state="$WK_FAKE_STATE/tmux-sessions"
log="$WK_FAKE_STATE/tmux.log"
touch "$state"
printf '%s\n' "$*" >> "$log"
case "$1" in
  has-session) grep -Fqx "${3#=}" "$state" ;;
  new-session) printf '%s\n' "$4" >> "$state" ;;
  kill-session)
    grep -Fvx "$3" "$state" > "$state.next" || :
    mv "$state.next" "$state"
    ;;
  attach-session|switch-client) ;;
  *) echo "unexpected tmux invocation: $*" >&2; exit 2 ;;
esac
`)
	mustWriteExecutable(t, filepath.Join(dir, "herdr"), `#!/bin/sh
set -eu
state="$WK_FAKE_STATE/herdr-workspaces"
log="$WK_FAKE_STATE/herdr.log"
touch "$state"
printf '%s|%s\n' "${HERDR_WORKSPACE_ID-}" "$*" >> "$log"
if [ "$#" -eq 0 ]; then exit 0; fi
if [ "$1" = workspace ] && [ "$2" = list ]; then
  printf '{"result":{"workspaces":['
  first=1
  while IFS='	' read -r id path label; do
    [ -n "$id" ] || continue
    [ "$first" = 1 ] || printf ','
    first=0
    printf '{"workspace_id":"%s","label":"%s","worktree":{"checkout_path":"%s"}}' "$id" "$label" "$path"
  done < "$state"
  printf ']}}\n'
  exit 0
fi
if [ "$1" = worktree ] && [ "$2" = open ]; then
  shift 2
  path= label=
  while [ "$#" -gt 0 ]; do
    case "$1" in --path) path=$2; shift 2;; --label) label=$2; shift 2;; *) shift;; esac
  done
  id="w$(wc -l < "$state" | tr -d ' ')"
  printf '%s\t%s\t%s\n' "$id" "$path" "$label" >> "$state"
  printf '{}'\n
  exit 0
fi
if [ "$1" = workspace ] && [ "$2" = focus ]; then printf '{}'\n; exit 0; fi
if [ "$1" = workspace ] && [ "$2" = close ]; then
  grep -Fv "$3	" "$state" > "$state.next" || :
  mv "$state.next" "$state"
  printf '{}'\n
  exit 0
fi
echo "unexpected herdr invocation: $*" >&2
exit 2
`)
	mustWriteExecutable(t, filepath.Join(dir, "gh"), `#!/bin/sh
set -eu
printf '%s|%s\n' "$PWD" "$*" >> "$WK_FAKE_STATE/gh.log"
state="$WK_FAKE_STATE/gh-$(basename "$PWD")"
[ -f "$state" ] || exit 1
cat "$state"
`)
}

func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = setEnv(os.Environ(), map[string]string{"GIT_CONFIG_NOSYSTEM": "1", "GIT_TERMINAL_PROMPT": "0"})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

func gitOutput(t *testing.T, p *e2eProject, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = p.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func setEnv(base []string, values map[string]string) []string {
	out := make([]string, 0, len(base)+len(values))
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if _, replace := values[key]; !replace {
			out = append(out, kv)
		}
	}
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func mustSucceed(t *testing.T, got e2eResult) {
	t.Helper()
	if got.err != nil {
		t.Fatalf("wk failed: %v\n%s", got.err, got.output)
	}
}

func mustFail(t *testing.T, got e2eResult) {
	t.Helper()
	if got.err == nil {
		t.Fatalf("wk unexpectedly succeeded:\n%s", got.output)
	}
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
}

func mustNotContain(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Fatalf("output unexpectedly contains %q:\n%s", unwanted, got)
	}
}

func mustEqual(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func mustDir(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected directory %s: %v", path, err)
	}
}

func mustRealPath(t *testing.T, path string) string {
	t.Helper()
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolving %s: %v", path, err)
	}
	return realPath
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustWriteExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func readIfExists(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}
