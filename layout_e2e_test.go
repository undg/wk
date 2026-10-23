package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestE2ENormalCloneListAndCleanFromLinkedSubdirectory(t *testing.T) {
	p := newTestProject(t, "", false)
	mustSucceed(t, p.run("", "add", "merged", "--tmux"))
	mustSucceed(t, p.run("", "add", "open", "--tmux"))
	root := mustRealPath(t, p.root)
	nested := filepath.Join(root, ".worktrees", "open", "src")
	mustMkdirAll(t, nested)

	got := p.runAt(nested, "", "ls", "--porcelain", "--tmux")
	mustSucceed(t, got)
	mustContain(t, got.output, "main\n"+root+"\n\n")
	mustContain(t, got.output, "merged\n"+filepath.Join(root, ".worktrees", "merged")+"\nmerged [project]\n")
	got = p.runAt(nested, "", "ls", "--tmux")
	mustSucceed(t, got)
	mustContain(t, got.output, ".worktrees/merged")

	// gh's fake keys by directory basename. The main checkout has a merged
	// result too, but must never even be queried by clean.
	p.writeGH("project", "MERGED")
	p.writeGH("merged", "MERGED")
	p.writeGH("open", "OPEN")
	mustSucceed(t, p.runAt(nested, "yes\n", "clean", "--tmux"))
	if _, err := os.Stat(filepath.Join(root, ".worktrees", "merged")); !os.IsNotExist(err) {
		t.Fatalf("merged worktree still exists: %v", err)
	}
	mustDir(t, filepath.Join(root, ".worktrees", "open"))
	mustNotContain(t, mustReadFile(t, filepath.Join(p.fakeState, "gh.log")), root+"|pr view")
	mustEqual(t, gitOutput(t, p, root, "branch", "--show-current"), "main")

	got = p.run("", "delete", ".", "--tmux")
	mustFail(t, got)
	mustContain(t, got.output, "refusing to delete the main checkout")
}

func TestE2EInitPromptFromLinkedCheckoutUsesOwningRoot(t *testing.T) {
	p := newTestProject(t, "", false)
	linked := filepath.Join(p.root, ".worktrees", "linked")
	testGit(t, p.root, "worktree", "add", "-b", "linked", linked)
	if err := os.Remove(filepath.Join(p.root, ".wk.toml")); err != nil {
		t.Fatal(err)
	}
	mustSucceed(t, p.runAt(linked, "yes\n", "add", "after-init", "--tmux"))
	mustDir(t, filepath.Join(p.root, ".worktrees", "after-init"))
	mustContain(t, mustReadFile(t, filepath.Join(p.root, ".wk.toml")), "name = \"project\"")
	if _, err := os.Stat(filepath.Join(linked, ".wk.toml")); !os.IsNotExist(err) {
		t.Fatalf("scaffolded a linked-checkout config: %v", err)
	}
}

func TestBareLayoutsStillCreateDirectChildren(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "wrapper"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			p := newE2EProject(t, "")
			if legacy {
				// The Git directory itself is the project root in the old layout.
				p.root = filepath.Join(p.root, ".bare")
				mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), "")
				// Remove the parent's wrapper redirect to make this a true bare root.
				if err := os.Remove(filepath.Join(filepath.Dir(p.root), ".git")); err != nil {
					t.Fatal(err)
				}
			}
			enterTestProject(t, p, p.root)
			if err := runCreate("feat/bare", "", backendTmux); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(p.root, "feat-bare")
			mustDir(t, dir)
			if _, err := os.Stat(filepath.Join(p.root, ".gitignore")); !os.IsNotExist(err) {
				t.Fatalf("bare add changed .gitignore: %v", err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			if err := runCreate("feat/second", "", backendTmux); err != nil {
				t.Fatal(err)
			}
			mustDir(t, filepath.Join(p.root, "feat-second"))
			if err := runCreate("release", "", backendTmux); err != nil {
				t.Fatal(err)
			}
			mustEqual(t, gitOutput(t, p, filepath.Join(p.root, "release"), "rev-parse", "HEAD"), gitOutput(t, p, p.root, "rev-parse", "origin/release"))
			if err := runCreate("origin/explicit-remote", "", backendTmux); err != nil {
				t.Fatal(err)
			}
			mustEqual(t, gitOutput(t, p, filepath.Join(p.root, "explicit-remote"), "rev-parse", "--abbrev-ref", "@{upstream}"), "origin/explicit-remote")
			if err := runDelete("feat-bare", backendTmux); err != nil {
				t.Fatal(err)
			}
		})
	}
}
