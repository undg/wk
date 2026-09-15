package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// worktreeDirForRef finds the worktree directory matching ref, which may be
// the checked-out branch name or the rendered session name/workspace label
// (e.g. "feat/x [project]", copied from a herdr/tmux session listing).
func worktreeDirForRef(ref string, cfg ProjectConfig) (string, bool) {
	entries, err := listWorktrees()
	if err != nil {
		return "", false
	}
	for _, wt := range entries {
		if wt.Branch == "" {
			continue
		}
		if wt.Branch == ref {
			return wt.Dir, true
		}
		if sessionNameFromTemplate(cfg.SessionTemplate, wt.Branch, cfg.ProjectName) == ref {
			return wt.Dir, true
		}
	}
	return "", false
}

func runDelete(rawDir, backendOverride string) error {
	// Captured before loadProjectConfig, which may chdir to the project
	// root: "." needs to mean "here", not wherever that walk-up lands.
	startDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving current directory: %w", err)
	}

	logInfo("delete mode")

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	var worktreeDir string
	switch {
	case rawDir == ".":
		worktreeDir = startDir
	case strings.HasPrefix(rawDir, "/"):
		worktreeDir = rawDir
	default:
		worktreeDir = "./" + rawDir
	}

	if info, err := os.Stat(worktreeDir); err != nil || !info.IsDir() {
		// rawDir didn't match a directory directly; maybe it's a branch
		// name or a rendered session name/label (e.g. copied from `git
		// branch` or a herdr/tmux session listing) rather than the
		// sanitized worktree dir. Look it up by either instead.
		if resolved, ok := worktreeDirForRef(rawDir, cfg); ok {
			worktreeDir = resolved
		} else {
			return fmt.Errorf("worktree directory not found: %s", worktreeDir)
		}
	}
	logInfo("target worktree: %s", worktreeDir)

	backend, err := newSessionBackend(cfg.SessionBackend, backendOverride)
	if err != nil {
		return err
	}

	branch, err := branchOfWorktree(worktreeDir)
	if err != nil || branch == "" || branch == "HEAD" {
		return fmt.Errorf("could not determine branch from worktree: %s", worktreeDir)
	}
	logInfo("detected branch: %s", branch)

	sess := session{
		Name: sessionNameFromTemplate(cfg.SessionTemplate, branch, cfg.ProjectName),
		Dir:  worktreeDir,
	}

	if len(cfg.Teardown) > 0 {
		logStep("running teardown steps")
		if err := runShellSteps(worktreeDir, cfg.Teardown); err != nil {
			return err
		}
	}

	// If the worktree being deleted is also the process's cwd (e.g. `wk
	// delete .`), removing it pulls the rug out from under every git command
	// below: cwd no longer exists once git unlinks it. Hop over to the
	// repo's common git dir first — captured while cwd is still valid — so
	// the rest of the flow keeps running from solid ground.
	if cwd, err := os.Getwd(); err == nil {
		if absTarget, err := filepath.Abs(worktreeDir); err == nil && cwd == absTarget {
			commonDir, err := runGit("rev-parse", "--path-format=absolute", "--git-common-dir")
			if err != nil {
				return fmt.Errorf("resolving repo git dir: %w", err)
			}
			if err := os.Chdir(commonDir); err != nil {
				return fmt.Errorf("leaving worktree before removal: %w", err)
			}
			worktreeDir = absTarget
		}
	}

	logStep("removing worktree")
	if err := worktreeRemove(worktreeDir); err != nil {
		logStep("removing leftover untracked files")
		if rmErr := os.RemoveAll(worktreeDir); rmErr != nil {
			return fmt.Errorf("failed to remove worktree directory: %w", rmErr)
		}
	}

	if _, err := os.Stat(worktreeDir); err == nil {
		return fmt.Errorf("worktree directory could not be removed: %s", worktreeDir)
	}

	if branchExists(branch) {
		logStep("deleting local branch: %s", branch)
		if err := deleteBranch(branch); err != nil {
			return fmt.Errorf("failed to delete branch: %w", err)
		}
	} else {
		logStep("local branch already missing: %s", branch)
	}

	logStep("pruning stale worktree entries")
	if err := worktreePrune(); err != nil {
		return fmt.Errorf("failed to prune worktrees: %w", err)
	}

	// Session/workspace teardown runs last, after the worktree, branch, and
	// prune are all done. That way killing the session you're currently
	// attached to — which some backends do without waiting, potentially
	// tearing down the pty mid-command — can't leave a half-deleted worktree
	// behind: by the time it runs, there's nothing left to lose.
	if backend.Has(sess) {
		logStep("killing %s session: %s", backend.Kind(), sess.Name)
		if err := backend.Kill(sess); err != nil {
			return fmt.Errorf("failed to kill %s session: %w", backend.Kind(), err)
		}
	} else {
		logInfo("no %s session found: %s", backend.Kind(), sess.Name)
	}

	logOK("delete complete")
	return nil
}
