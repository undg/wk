package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, repoConfigFileName), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	worktree := filepath.Join(root, "feat-x")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("found at start", func(t *testing.T) {
		got, err := findProjectRoot(root)
		if err != nil {
			t.Fatalf("findProjectRoot() error = %v", err)
		}
		if got != root {
			t.Errorf("findProjectRoot() = %q, want %q", got, root)
		}
	})

	t.Run("found by walking up", func(t *testing.T) {
		got, err := findProjectRoot(worktree)
		if err != nil {
			t.Fatalf("findProjectRoot() error = %v", err)
		}
		if got != root {
			t.Errorf("findProjectRoot() = %q, want %q", got, root)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := findProjectRoot(t.TempDir()); err == nil {
			t.Error("findProjectRoot() error = nil, want not-found error")
		}
	})
}
