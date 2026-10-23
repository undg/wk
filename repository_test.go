package main

import (
	"os"
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
			linked := filepath.Join(p.root, ".worktrees", "feature")
			if bare {
				linked = filepath.Join(p.root, "feature")
			}
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
					mustEqual(t, got.WorktreeRoot, root)
					mustEqual(t, got.MainCheckout, "")
				} else {
					mustEqual(t, got.WorktreeRoot, filepath.Join(root, ".worktrees"))
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
	mustEqual(t, got.WorktreeRoot, mustRealPath(t, root))
	mustEqual(t, got.MainCheckout, "")
}

func TestRepositoryLayoutOutsideGitFails(t *testing.T) {
	if _, err := resolveRepository(t.TempDir()); err == nil {
		t.Fatal("accepted a directory outside Git")
	}
}

func TestIgnoreWorktreesPreservesContentAndIsIdempotent(t *testing.T) {
	for _, c := range []struct {
		name, before, want string
	}{
		{"missing", "", "/.worktrees/\n"},
		{"no newline", "node_modules/", "node_modules/\n/.worktrees/\n"},
		{"newline", "# keep me\n", "# keep me\n/.worktrees/\n"},
		{"existing", "/.worktrees/\nother\n", "/.worktrees/\nother\n"},
		{"CRLF", "# keep me\r\n/.worktrees/\r\n", "# keep me\r\n/.worktrees/\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			testGit(t, root, "init", ".")
			path := filepath.Join(root, ".gitignore")
			if c.before != "" {
				mustWriteFile(t, path, c.before)
			}
			for i := 0; i < 2; i++ {
				if err := ensureWorktreesIgnored(root); err != nil {
					t.Fatal(err)
				}
				mustEqual(t, mustReadFile(t, path), c.want)
			}
			testGit(t, root, "check-ignore", "-q", ".worktrees/feature/README")
		})
	}
}

func TestIgnoreWorktreesReportsReadAndWriteErrors(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, ".gitignore"))
	if err := ensureWorktreesIgnored(root); err == nil {
		t.Fatal("accepted a directory as .gitignore")
	}
	if err := ensureWorktreesIgnored(filepath.Join(root, "missing")); err == nil {
		t.Fatal("ignored failure to write .gitignore")
	}
	// Reading an existing file must preserve its permissions.
	path := filepath.Join(root, "other")
	mustMkdirAll(t, path)
	ignore := filepath.Join(path, ".gitignore")
	mustWriteFile(t, ignore, "keep\n")
	if err := os.Chmod(ignore, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureWorktreesIgnored(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ignore)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("changed existing .gitignore permissions: %v", info.Mode())
	}
}
