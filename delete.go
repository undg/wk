package main

import (
	"fmt"
	"os"
	"strings"
)

// worktreeDirForBranch finds the worktree directory whose checked-out branch
// matches branch, tolerating a leading "./" or "/" on either side.
func worktreeDirForBranch(branch string) (string, bool) {
	entries, err := listWorktrees()
	if err != nil {
		return "", false
	}
	for _, wt := range entries {
		if wt.Branch == branch {
			return wt.Dir, true
		}
	}
	return "", false
}

func runDelete(rawDir, backendOverride string) error {
	var worktreeDir string
	if strings.HasPrefix(rawDir, "/") {
		worktreeDir = rawDir
	} else {
		worktreeDir = "./" + rawDir
	}

	logInfo("delete mode")

	if info, err := os.Stat(worktreeDir); err != nil || !info.IsDir() {
		// rawDir didn't match a directory directly; maybe it's a branch
		// name (e.g. copied from `git branch` or a session label) rather
		// than the sanitized worktree dir. Look it up by branch instead.
		if resolved, ok := worktreeDirForBranch(rawDir); ok {
			worktreeDir = resolved
		} else {
			return fmt.Errorf("worktree directory not found: %s", worktreeDir)
		}
	}
	logInfo("target worktree: %s", worktreeDir)

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

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

	// Killing the session you're currently attached to tears down your own
	// pty mid-command; the "survive the session's death" flow is a later
	// step, so for now this matches the POC and refuses.
	if backend.IsCurrent(sess) {
		return fmt.Errorf("cannot delete the current %s session (run this command from another session or outside %s)", backend.Kind(), backend.Kind())
	}

	if len(cfg.Teardown) > 0 {
		logStep("running teardown steps")
		if err := runShellSteps(worktreeDir, cfg.Teardown); err != nil {
			return err
		}
	}

	if backend.Has(sess) {
		logStep("killing %s session: %s", backend.Kind(), sess.Name)
		if err := backend.Kill(sess); err != nil {
			return fmt.Errorf("failed to kill %s session: %w", backend.Kind(), err)
		}
	} else {
		logInfo("no %s session found: %s", backend.Kind(), sess.Name)
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

	logOK("delete complete")
	return nil
}
