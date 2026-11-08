package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const worktreesDirectory = ".wk"

type repositoryContext struct {
	Root         string
	MainCheckout string // Empty for a bare repository.
}

func gitAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// resolveRepository uses the common Git directory, not the invoking checkout's
// core.bare value or its copy of .wk.toml, to identify the owning project.
func resolveRepository(start string) (repositoryContext, error) {
	common, err := gitAt(start, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return repositoryContext{}, fmt.Errorf("finding repository from %s: %w", start, err)
	}
	bare, err := gitAt(start, "--git-dir="+common, "rev-parse", "--is-bare-repository")
	if err != nil {
		return repositoryContext{}, fmt.Errorf("detecting repository layout: %w", err)
	}
	if bare == "true" {
		root := common
		// A wrapper's .git redirect points at the common Git directory.
		// Verify the redirect rather than depending on the name .bare.
		parent := filepath.Dir(common)
		if info, err := os.Stat(filepath.Join(parent, ".git")); err == nil && !info.IsDir() {
			redirect, err := gitAt(parent, "rev-parse", "--path-format=absolute", "--git-common-dir")
			if err == nil && redirect == common {
				root = parent
			}
		}
		return repositoryContext{Root: root}, nil
	}
	out, err := gitAt(start, "--git-dir="+common, "worktree", "list", "--porcelain")
	if err != nil {
		return repositoryContext{}, fmt.Errorf("finding main checkout: %w", err)
	}
	entries := parseWorktreePorcelain(out)
	if len(entries) == 0 {
		return repositoryContext{}, fmt.Errorf("repository has no main checkout")
	}
	// Git always lists the main checkout first, even from a linked checkout.
	root := entries[0].Dir
	return repositoryContext{Root: root, MainCheckout: root}, nil
}
