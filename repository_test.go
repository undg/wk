package main

import (
	"path/filepath"
	"testing"
)

func TestRepositoryLayoutFromRootAndLinkedCheckout(t *testing.T) {
	for _, bare := range []bool{true, false} {
		name := "normal"
		if bare {
			name = "bare-wrapper"
		}
		t.Run(name, func(t *testing.T) {
			p := newTestProject(t, "", bare)
			linked := filepath.Join(p.root, ".wk", "feature")
			testGit(t, p.root, "worktree", "add", "-b", "feature", linked, "origin/main")
			nested := filepath.Join(linked, "src")
			mustMkdirAll(t, nested)
			for _, start := range []string{p.root, linked, nested} {
				got, err := resolveRepository(start)
				if err != nil {
					t.Fatal(err)
				}
				root := mustRealPath(t, p.root)
				mustEqual(t, got.Root, root)
				if bare {
					mustEqual(t, got.MainCheckout, "")
				} else {
					mustEqual(t, got.MainCheckout, root)
				}
			}
		})
	}
}

func TestRepositoryLayoutLegacyBareRoot(t *testing.T) {
	root := t.TempDir()
	testGit(t, root, "init", "--bare", ".")
	got, err := resolveRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, got.Root, mustRealPath(t, root))
	mustEqual(t, got.MainCheckout, "")
}

func TestRepositoryLayoutOutsideGitFails(t *testing.T) {
	if _, err := resolveRepository(t.TempDir()); err == nil {
		t.Fatal("accepted a directory outside Git")
	}
}

func TestWorktreesIgnoreHint(t *testing.T) {
	for _, c := range []struct {
		name, file, content string
	}{
		{"unignored", "", ""},
		{"gitignore", ".gitignore", "/.wk/\n"},
		{"info exclude", ".git/info/exclude", "/.wk/\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			testGit(t, root, "init", ".")
			if c.file != "" {
				mustWriteFile(t, filepath.Join(root, c.file), c.content)
			}
			hint := worktreesIgnoreHint(root)
			if c.file != "" {
				mustEqual(t, hint, "")
				return
			}
			mustContain(t, hint, ".gitignore")
			mustContain(t, hint, ".git/info/exclude")
		})
	}
}
