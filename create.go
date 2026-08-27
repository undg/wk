package main

import (
	"fmt"
	"os"
	"strings"
)

func runCreate(rawBranchArg string) error {
	if !isBareRepository() {
		return fmt.Errorf("run this from a bare repository")
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	rawBranch := rawBranchArg
	baseRef := cfg.BaseRef

	if strings.HasPrefix(rawBranch, "origin/") {
		baseRef = rawBranch
		rawBranch = strings.TrimPrefix(rawBranch, "origin/")
	}

	slug := SanitizeBranch(rawBranch)
	if slug == "" {
		return fmt.Errorf("branch name became empty after sanitization")
	}
	worktreeDir := "./" + slug
	sessionName := tmuxSessionName(cfg.TmuxTemplate, rawBranch, cfg.ProjectName)

	logInfo("create mode")
	logInfo("branch: %s", rawBranch)
	logInfo("base: %s", baseRef)
	logInfo("worktree: %s", worktreeDir)
	logInfo("tmux session: %s", sessionName)

	if !refExists(baseRef) {
		fetchMissingBaseRef(baseRef)
	}
	if !refExists(baseRef) {
		return fmt.Errorf("base reference not found: %s (tip: pass main, origin/main, or origin/someone-branch)", baseRef)
	}

	if _, err := os.Stat(worktreeDir); err == nil {
		return fmt.Errorf("worktree path already exists: %s", worktreeDir)
	}

	if tmuxHasSession(sessionName) {
		return fmt.Errorf("tmux session already exists: %s", sessionName)
	}

	logStep("creating worktree")
	if branchExists(rawBranch) {
		logInfo("branch already exists, reusing: %s", rawBranch)
		err = worktreeAddExisting(worktreeDir, rawBranch)
	} else {
		err = worktreeAddNew(worktreeDir, rawBranch, baseRef)
	}
	if err != nil {
		return fmt.Errorf("git worktree add failed: %w", err)
	}

	if len(cfg.Setup) > 0 {
		logStep("running setup steps")
		if err := runShellSteps(worktreeDir, cfg.Setup); err != nil {
			return err
		}
	}

	logStep("starting tmux session: %s", sessionName)
	if err := tmuxNewDetachedSession(sessionName, worktreeDir); err != nil {
		return fmt.Errorf("tmux new-session failed: %w", err)
	}

	return tmuxAttachOrSwitch(sessionName)
}
