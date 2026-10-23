package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalCloneAddBranchesFromCurrentCheckout(t *testing.T) {
	for _, checkout := range []string{"main", "linked", "detached"} {
		t.Run(checkout, func(t *testing.T) {
			p := newTestProject(t, "base_ref = \"origin/missing\"\n", false)
			caller := p.root
			if checkout == "main" {
				testGit(t, caller, "switch", "-c", "current-feature")
			} else {
				caller = filepath.Join(p.root, ".worktrees", "source")
				testGit(t, p.root, "worktree", "add", "-b", "current-feature", caller)
			}
			testGit(t, caller, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "caller-only commit")
			if checkout == "detached" {
				testGit(t, caller, "switch", "--detach")
			}
			want := gitOutput(t, p, caller, "rev-parse", "HEAD")
			nested := filepath.Join(caller, "src")
			mustMkdirAll(t, nested)
			enterTestProject(t, p, nested)

			if err := runCreate("feat/child", "", backendTmux); err != nil {
				t.Fatal(err)
			}

			dir := filepath.Join(p.root, ".worktrees", "feat-child")
			mustEqual(t, gitOutput(t, p, dir, "rev-parse", "HEAD"), want)
			mustEqual(t, gitOutput(t, p, dir, "branch", "--show-current"), "feat/child")
			// A retry from the main checkout must reuse the child branch,
			// without resolving the missing configured base or resetting it.
			if err := runCreate("feat/child", "", backendTmux); err != nil {
				t.Fatal(err)
			}
			mustEqual(t, gitOutput(t, p, dir, "rev-parse", "HEAD"), want)
		})
	}
}

func TestNormalCloneAddPreservesExistingBranchPrecedence(t *testing.T) {
	for _, branch := range []string{"remote-branch", "origin/remote-branch", "local"} {
		t.Run(branch, func(t *testing.T) {
			p := newTestProject(t, "base_ref = \"origin/missing\"\n", false)
			// Give local and origin/local different commits. The local branch
			// must win for an unprefixed name, without changing its upstream.
			testGit(t, p.root, "branch", "local", "origin/release")
			testGit(t, p.root, "update-ref", "refs/remotes/origin/local", "origin/remote-branch")
			testGit(t, p.root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "caller-only commit")
			enterTestProject(t, p, p.root)

			if err := runCreate(branch, "", backendTmux); err != nil {
				t.Fatal(err)
			}

			name, wantRef := "remote-branch", "origin/remote-branch"
			if branch == "local" {
				name, wantRef = "local", "origin/release"
			}
			dir := filepath.Join(p.root, ".worktrees", name)
			mustEqual(t, gitOutput(t, p, dir, "rev-parse", "HEAD"), gitOutput(t, p, p.root, "rev-parse", wantRef))
			mustEqual(t, gitOutput(t, p, dir, "rev-parse", "--abbrev-ref", "@{upstream}"), wantRef)
		})
	}
}

func TestNormalCloneAddRejectsUnbornCurrentBranch(t *testing.T) {
	p := newTestProject(t, "", false)
	testGit(t, p.root, "symbolic-ref", "HEAD", "refs/heads/unborn")
	enterTestProject(t, p, p.root)

	err := runCreate("feat/child", "", backendTmux)
	if err == nil {
		t.Fatal("created a branch without a current commit")
	}
	mustContain(t, err.Error(), "resolving current checkout commit")
	if branchExists("feat/child") {
		t.Fatal("created branch despite invalid starting point")
	}
	if _, err := os.Stat(filepath.Join(p.root, ".worktrees")); !os.IsNotExist(err) {
		t.Fatalf("created worktree container despite invalid starting point: %v", err)
	}
}
