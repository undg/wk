package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveWorktreeTarget accepts only registered checkouts. A path or branch
// identifies the worktree; a rendered session name remains a legacy fallback.
func resolveWorktreeTarget(raw, start string, cfg ProjectConfig) (string, error) {
	entries, err := listWorktrees()
	if err != nil {
		return "", err
	}
	paths := []string{filepath.Join(cfg.Repository.Root, raw), filepath.Join(cfg.Repository.Root, worktreesDirectory, raw)}
	switch {
	case raw == ".":
		current, err := gitAt(start, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", fmt.Errorf("finding current worktree: %w", err)
		}
		paths = []string{current}
	case filepath.IsAbs(raw):
		paths = []string{raw}
	}
	return matchRegisteredWorktree(raw, paths, entries, cfg)
}

// matchRegisteredWorktree prefers checkout paths over branches, and branches
// over rendered session names. Discovery order must not change that precedence.
func matchRegisteredWorktree(raw string, paths []string, entries []worktreeEntry, cfg ProjectConfig) (string, error) {
	for _, path := range paths {
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		for _, wt := range entries {
			if canonical == wt.Dir {
				return wt.Dir, nil
			}
		}
	}
	for _, wt := range entries {
		if wt.Branch != "" && wt.Branch == raw {
			return wt.Dir, nil
		}
	}
	for _, wt := range entries {
		if wt.Branch != "" && sessionNameFromTemplate(cfg.SessionTemplate, wt.Branch, cfg.ProjectName) == raw {
			return wt.Dir, nil
		}
	}
	return "", fmt.Errorf("worktree directory not found or not a registered checkout: %s", raw)
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

	worktreeDir, err := resolveWorktreeTarget(rawDir, startDir, cfg)
	if err != nil {
		return err
	}
	if worktreeDir == cfg.Repository.MainCheckout {
		return fmt.Errorf("refusing to delete the main checkout: %s", worktreeDir)
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

	finishProgress := startHerdrProgress(cfg.HerdrProgress)
	defer finishProgress()

	if len(cfg.Teardown) > 0 {
		logStep("running teardown steps")
		if err := runShellSteps(worktreeDir, cfg.Teardown); err != nil {
			return err
		}
	}

	// Configuration loading moved cwd to the owning root. That root is
	// never a removable linked checkout, including for `delete .`.
	logStep("removing worktree")
	if err := worktreeRemove(worktreeDir); err != nil {
		return fmt.Errorf("removing worktree: %w", err)
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
	finishProgress()
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
