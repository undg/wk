package main

import (
	"fmt"
	"os"
	"path/filepath"
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

	reusingWorktree, err := reusableWorktree(worktreeDir, rawBranch)
	if err != nil {
		return err
	}

	if reusingWorktree {
		logInfo("resuming existing worktree: %s", worktreeDir)
	} else {
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
		if err := markSetupPending(worktreeDir); err != nil {
			return err
		}
	}

	// A bare clone commonly has remote branch names as local refs already.
	// In that case worktreeAddExisting is the only valid checkout operation,
	// but it does not set an upstream itself. Explicit remote checkouts must
	// still track their requested origin branch. It is harmless to repeat when
	// resuming after a later failure, and lets a retry repair this step too.
	if trackRemote {
		if err := setBranchUpstream(rawBranch, baseRef); err != nil {
			return fmt.Errorf("setting branch upstream failed: %w", err)
		}
	}

	sessionExists := backend.Has(sess)
	if len(cfg.Setup) > 0 && !sessionExists {
		setupState, err := readSetupState(worktreeDir)
		if err != nil {
			return err
		}
		switch setupState {
		case setupPending:
			logStep("running setup steps")
			if err := runShellSteps(worktreeDir, cfg.Setup); err != nil {
				return err
			}
			if err := markSetupComplete(worktreeDir); err != nil {
				return err
			}
		case setupUnknown:
			// Worktrees created before resumable adds have no checkpoint. Their
			// setup may already have completed, and arbitrary setup is often not
			// idempotent (for example, ln -s), so the safe migration is to skip it.
			logInfo("existing worktree has no setup checkpoint, not rerunning setup")
		}
	}

	if sessionExists {
		logInfo("%s session already exists, attaching: %s", backend.Kind(), sess.Name)
		return backend.AttachOrSwitch(sess)
	}

	logStep("starting %s session: %s", backend.Kind(), sess.Name)
	if err := backend.Create(sess); err != nil {
		return fmt.Errorf("creating %s session failed: %w", backend.Kind(), err)
	}

	return backend.AttachOrSwitch(sess)
}

type setupCheckpoint string

const (
	// setupUnknown is for worktrees made before resumable adds. It is unsafe to
	// assume their setup can be repeated.
	setupUnknown  setupCheckpoint = ""
	setupPending  setupCheckpoint = "pending"
	setupComplete setupCheckpoint = "complete"
)

// Setup checkpoints live in Git's per-worktree metadata, rather than adding a
// wk file to the user's checkout. New worktrees are marked pending before setup
// starts, so a failed setup retries. Unmarked legacy worktrees are left alone.
func readSetupState(dir string) (setupCheckpoint, error) {
	path, err := setupCheckpointPath(dir)
	if err != nil {
		return setupUnknown, err
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return setupUnknown, nil
	}
	if err != nil {
		return setupUnknown, fmt.Errorf("reading setup checkpoint: %w", err)
	}
	return setupCheckpoint(strings.TrimSpace(string(content))), nil
}

func markSetupPending(dir string) error { return writeSetupCheckpoint(dir, setupPending) }

func markSetupComplete(dir string) error { return writeSetupCheckpoint(dir, setupComplete) }

func writeSetupCheckpoint(dir string, state setupCheckpoint) error {
	path, err := setupCheckpointPath(dir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(state+"\n"), 0o600); err != nil {
		return fmt.Errorf("writing setup checkpoint: %w", err)
	}
	return nil
}

func setupCheckpointPath(dir string) (string, error) {
	gitDir, err := runGit("-C", dir, "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("finding git metadata for worktree %s: %w", dir, err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	return filepath.Join(gitDir, "wk-setup-complete"), nil
}

// reusableWorktree reports whether dir is the worktree wk would have created
// for branch. A plain existing directory, or a worktree for another branch,
// is unsafe to adopt and remains an error.
func reusableWorktree(dir, branch string) (bool, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking worktree path %s: %w", dir, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("worktree path is not a directory: %s", dir)
	}

	want, err := filepath.Abs(dir)
	if err != nil {
		return false, fmt.Errorf("resolving worktree path %s: %w", dir, err)
	}
	entries, err := listWorktrees()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		got, err := filepath.Abs(entry.Dir)
		if err != nil {
			return false, fmt.Errorf("resolving registered worktree path %s: %w", entry.Dir, err)
		}
		if got != want {
			continue
		}
		if entry.Branch != branch {
			return false, fmt.Errorf("worktree path already belongs to branch %s: %s", entry.Branch, dir)
		}
		return true, nil
	}
	return false, fmt.Errorf("worktree path already exists but is not a registered git worktree: %s", dir)
}
