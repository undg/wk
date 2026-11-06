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

func TestExcludeWorktreesPreservesContentAndIsIdempotent(t *testing.T) {
	for _, c := range []struct {
		name, before, want string
	}{
		{"missing info directory", "", "/.wk/\n"},
		{"no newline", "local-only", "local-only\n/.wk/\n"},
		{"newline", "# keep me\n", "# keep me\n/.wk/\n"},
		{"existing", "/.wk/\nother\n", "/.wk/\nother\n"},
		{"CRLF", "# keep me\r\n/.wk/\r\n", "# keep me\r\n/.wk/\r\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			testGit(t, root, "init", ".")
			path := filepath.Join(root, ".git", "info", "exclude")
			if c.before == "" {
				if err := os.RemoveAll(filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			} else {
				mustWriteFile(t, path, c.before)
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 2; i++ {
				if err := ensureWorktreesExcluded(root); err != nil {
					t.Fatal(err)
				}
				mustEqual(t, mustReadFile(t, path), c.want)
			}
			testGit(t, root, "check-ignore", "-q", ".wk/feature/README")
			if c.before != "" {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Fatalf("changed exclude permissions: %v", info.Mode())
				}
			}
			if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
				t.Fatalf("created .gitignore: %v", err)
			}
		})
	}
}
