package main

import (
	"fmt"
	"os"
	"strings"
)

func runCreate(rawBranchArg, backendOverride string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	// loadProjectConfig may have chdir'd from a worktree checkout up to the
	// project root, so this has to run after it: --is-bare-repository
	// reports false from inside a linked worktree regardless of the repo's
	// actual core.bare, but true from the root's .bare redirect.
	if !isBareRepository() {
		return fmt.Errorf("run this from a bare repository")
	}

	backend, err := newSessionBackend(cfg.SessionBackend, backendOverride)
	if err != nil {
		return err
	}

	rawBranch := rawBranchArg
	baseRef := cfg.BaseRef
	trackRemote := false

	if strings.HasPrefix(rawBranch, "origin/") {
		baseRef = rawBranch
		rawBranch = strings.TrimPrefix(rawBranch, "origin/")
		trackRemote = true
	} else if !branchExists(rawBranch) && refExists("origin/"+rawBranch) {
		// Shell completion offers remote branch names with the origin/
		// prefix stripped, so a completed name is usually a branch that
		// already exists on the remote — check it out instead of starting
		// unrelated work off base_ref.
		baseRef = "origin/" + rawBranch
		trackRemote = true
	}

	slug := SanitizeBranch(rawBranch)
	if slug == "" {
		return fmt.Errorf("branch name became empty after sanitization")
	}
	worktreeDir := "./" + slug
	sess := session{
		Name: sessionNameFromTemplate(cfg.SessionTemplate, rawBranch, cfg.ProjectName),
		Dir:  worktreeDir,
	}

	logInfo("create mode")
	logInfo("branch: %s", rawBranch)
	logInfo("base: %s", baseRef)
	logInfo("worktree: %s", worktreeDir)
	logInfo("%s session: %s", backend.Kind(), sess.Name)

	if !refExists(baseRef) {
		fetchMissingBaseRef(baseRef)
	}
	if !refExists(baseRef) {
		return fmt.Errorf("base reference not found: %s (tip: pass main, origin/main, or origin/someone-branch)", baseRef)
	}

	if _, err := os.Stat(worktreeDir); err == nil {
		return fmt.Errorf("worktree path already exists: %s", worktreeDir)
	}

	if backend.Has(sess) {
		return fmt.Errorf("%s session already exists: %s", backend.Kind(), sess.Name)
	}

	logStep("creating worktree")
	if branchExists(rawBranch) {
		logInfo("branch already exists, reusing: %s", rawBranch)
		err = worktreeAddExisting(worktreeDir, rawBranch)
	} else {
		err = worktreeAddNew(worktreeDir, rawBranch, baseRef, trackRemote)
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

	logStep("starting %s session: %s", backend.Kind(), sess.Name)
	if err := backend.Create(sess); err != nil {
		return fmt.Errorf("creating %s session failed: %w", backend.Kind(), err)
	}

	return backend.AttachOrSwitch(sess)
}
