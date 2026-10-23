package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveWorktreeTargetPrecedence(t *testing.T) {
	for _, c := range []struct {
		name       string
		rootPath   bool
		slugPath   bool
		branch     bool
		wantBranch string
	}{
		{"root path before container path and branch", true, true, true, "root-owner"},
		{"container path before branch", false, true, true, "container-owner"},
		{"branch before rendered session", false, false, true, "session-topic"},
		{"rendered session compatibility", false, false, false, "topic"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newTestProject(t, "", false)
			p.writeGlobal(`session_template = "session-{branch}"`)
			enterTestProject(t, p, p.root)
			// Git lists these branches before the path owners. Matching each entry
			// against all identities at once would select the wrong checkout.
			testGit(t, p.root, "worktree", "add", "-b", "topic", filepath.Join(p.root, "a-topic"))
			if c.branch {
				testGit(t, p.root, "worktree", "add", "-b", "session-topic", filepath.Join(p.root, "b-branch"))
			}
			if c.rootPath {
				testGit(t, p.root, "worktree", "add", "-b", "root-owner", filepath.Join(p.root, "session-topic"))
			}
			if c.slugPath {
				testGit(t, p.root, "worktree", "add", "-b", "container-owner", filepath.Join(p.root, ".worktrees", "session-topic"))
			}
			cfg, err := loadProjectConfig()
			if err != nil {
				t.Fatal(err)
			}
			dir, err := resolveWorktreeTarget("session-topic", p.root, cfg)
			if err != nil {
				t.Fatal(err)
			}
			mustEqual(t, gitOutput(t, p, dir, "branch", "--show-current"), c.wantBranch)
		})
	}
}

func TestResolveWorktreeTargetSymlinkPaths(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	dir := filepath.Join(p.root, ".worktrees", "feature")
	testGit(t, p.root, "worktree", "add", "-b", "feature", dir)
	alias := filepath.Join(p.root, "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{alias, "alias", ".worktrees/feature", "feature"} {
		got, err := resolveWorktreeTarget(target, p.root, cfg)
		if err != nil {
			t.Fatalf("resolve %q: %v", target, err)
		}
		mustEqual(t, got, mustRealPath(t, dir))
	}
}

func TestDeleteDotUsesLinkedCheckoutSubdirectoryBeforeConfigWalkup(t *testing.T) {
	p := newTestProject(t, "", false)
	dir := filepath.Join(p.root, ".worktrees", "feature")
	testGit(t, p.root, "worktree", "add", "-b", "feature", dir)
	nested := filepath.Join(dir, "src", "nested")
	mustMkdirAll(t, nested)
	enterTestProject(t, p, nested)
	if err := runDelete(".", backendTmux); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("current linked checkout was not removed: %v", err)
	}
	mustDir(t, p.root)
	if branchExists("feature") {
		t.Fatal("deleted checkout's branch remains")
	}
}

func TestDetachedCheckoutResolvesByPathButDeleteRefusesBeforeTeardown(t *testing.T) {
	p := newTestProject(t, `teardown = ["touch teardown-ran"]`, false)
	enterTestProject(t, p, p.root)
	dir := filepath.Join(p.root, ".worktrees", "detached")
	testGit(t, p.root, "worktree", "add", "--detach", dir, "HEAD")
	cfg, err := loadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolveWorktreeTarget(dir, p.root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, got, mustRealPath(t, dir))
	for _, target := range []string{"HEAD", " [project]"} {
		if _, err := resolveWorktreeTarget(target, p.root, cfg); err == nil {
			t.Fatalf("detached checkout resolved as branch/session %q", target)
		}
	}
	if err := runDelete(dir, backendTmux); err == nil || !strings.Contains(err.Error(), "could not determine branch") {
		t.Fatalf("expected detached checkout refusal, got %v", err)
	}
	mustDir(t, dir)
	if _, err := os.Stat(filepath.Join(dir, "teardown-ran")); !os.IsNotExist(err) {
		t.Fatalf("teardown ran for detached checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.fakeState, "tmux.log")); !os.IsNotExist(err) {
		t.Fatalf("detached checkout deletion contacted backend: %v", err)
	}
}

func TestResolveWorktreeTargetRejectsUnregisteredPaths(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	dir := filepath.Join(p.root, "unregistered")
	mustMkdirAll(t, dir)
	foreign := filepath.Join(p.root, "foreign")
	testGit(t, p.root, "init", foreign)
	alias := filepath.Join(p.root, "alias")
	if err := os.Symlink(foreign, alias); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"unregistered", dir, foreign, alias, "missing"} {
		if _, err := resolveWorktreeTarget(target, p.root, cfg); err == nil {
			t.Fatalf("accepted unregistered target %q", target)
		}
	}
}
