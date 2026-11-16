package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ECompletionUsesGitWithoutConfigOrSessions(t *testing.T) {
	p := newTestProject(t, "", false)
	testGit(t, p.root, "branch", "local-only")
	testGit(t, p.root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	testGit(t, p.root, "branch", "feat/linked")
	linked := filepath.Join(p.root, ".wk", "path with spaces")
	testGit(t, p.root, "worktree", "add", linked, "feat/linked")
	subdir := filepath.Join(linked, "subdir")
	mustMkdirAll(t, subdir)
	if err := os.Remove(filepath.Join(p.root, ".wk.toml")); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		args []string
		want []string
	}{
		{"local branch", []string{"add", "local-"}, []string{"local-only"}},
		{"origin branch", []string{"add", "origin/remote-"}, []string{"origin/remote-branch"}},
		{"origin branch shorthand", []string{"add", "remote-"}, []string{"remote-branch"}},
		{"deduplicated branch", []string{"add", "main"}, []string{"main"}},
		{"worktree branch", []string{"delete", "feat/"}, []string{"feat/linked"}},
		{"worktree path with spaces and alias", []string{"rm", filepath.Join(p.root, ".wk")}, []string{linked}},
		{"caller worktree", []string{"delete", "."}, []string{"."}},
		{"backend before command", []string{"--backend", "t"}, []string{"tmux"}},
		{"backend after command", []string{"add", "--backend", "h"}, []string{"herdr"}},
		{"command flag", []string{"ls", "--por"}, []string{"--porcelain"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"__completeNoDesc"}, c.args...)
			got := p.runAt(subdir, "YES\n", args...)
			mustSucceed(t, got)
			for _, want := range c.want {
				if count := strings.Count("\n"+got.output, "\n"+want+"\n"); count != 1 {
					t.Fatalf("wanted completion %q exactly once, got:\n%s", want, got.output)
				}
			}
			mustContain(t, got.output, ":4\n") // Disable unrelated filesystem completion.
			mustNotContain(t, got.output, "Run 'wk init'")
		})
	}

	for _, args := range [][]string{
		{"add", "local-only", ""}, // Badges are free text, not branches or files.
		{"delete", "feat/linked", ""},
		{"add", "no-such-branch"},
		{"add", "origin/HEAD"}, // Remote HEAD is symbolic, not a branch to create.
		{"add", "HEAD"},
		{"clean", ""},
		{"ls", ""},
		{"init", ""},
		{"version", ""},
		{"completion", "bash", ""},
	} {
		got := p.runAt(subdir, "YES\n", append([]string{"__complete"}, args...)...)
		mustSucceed(t, got)
		if !strings.HasPrefix(got.output, ":4\n") {
			t.Fatalf("expected no suggestions for %q, got:\n%s", args, got.output)
		}
	}

	for _, path := range []string{
		filepath.Join(p.root, ".wk.toml"),
		filepath.Join(linked, ".wk.toml"),
		filepath.Join(subdir, ".wk.toml"),
		filepath.Join(p.fakeState, "herdr.log"),
		filepath.Join(p.fakeState, "tmux.log"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("completion unexpectedly created %s: %v", path, err)
		}
	}
	mustEqual(t, gitOutput(t, p, linked, "branch", "--show-current"), "feat/linked")
}

func TestE2ECompletionInBareRepository(t *testing.T) {
	p := newE2EProject(t, "")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"add", "origin/remote-"}, "origin/remote-branch"},
		{[]string{"delete", "ma"}, "main"},
		{[]string{"rm", filepath.Join(p.root, ".wk")}, filepath.Join(p.root, ".wk", "main")},
	} {
		got := p.run("", append([]string{"__completeNoDesc"}, c.args...)...)
		mustSucceed(t, got)
		mustContain(t, "\n"+got.output, "\n"+c.want+"\n")
		mustContain(t, got.output, ":4\n")
	}
}

func TestE2ECompletionIncludesCommandsFlagsAndShells(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"ver"}, "version"},
		{[]string{"--back"}, "--backend"},
		{[]string{"completion", "f"}, "fish"},
		{[]string{"--tmux", "ls", "--por"}, "--porcelain"},
	} {
		got := runE2E(dir, os.Environ(), "", append([]string{"__completeNoDesc"}, c.args...)...)
		mustSucceed(t, got)
		mustContain(t, "\n"+got.output, "\n"+c.want+"\n")
	}
}

func TestE2ECompletionOutsideGitDoesNotScaffold(t *testing.T) {
	dir := t.TempDir()
	for _, command := range []string{"add", "delete", "rm"} {
		got := runE2E(dir, os.Environ(), "YES\n", "__complete", command, "")
		mustSucceed(t, got)
		if !strings.HasPrefix(got.output, ":4\n") {
			t.Fatalf("expected no suggestions outside Git, got:\n%s", got.output)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("completion created files outside Git: %v", entries)
	}
}
