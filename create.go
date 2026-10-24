package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runCreate(rawBranchArg, badge, backendOverride string) error {
	// Config loading changes cwd to the project root. Preserve the caller's
	// checkout so a linked worktree can be the starting point for new work.
	callerDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	backend, err := newSessionBackend(cfg.SessionBackend, backendOverride)
	if err != nil {
		return err
	}

	if badge != "" && backend.Kind() != backendHerdr {
		return fmt.Errorf("workspace badges require the herdr backend")
	}

	attachOrSwitch := backend.AttachOrSwitch
	if backend.Kind() == backendHerdr && insideHerdr() {
		attachOrSwitch = herdrAttachIfFocusUnchanged()
	}

	finishProgress := startHerdrProgress(cfg.HerdrProgress)
	defer finishProgress()

	branch, err := resolveCreateBranch(rawBranchArg, callerDir, cfg)
	if err != nil {
		return err
	}
	rawBranch, baseRef := branch.Name, branch.BaseRef

	slug := SanitizeBranch(rawBranch)
	if slug == "" {
		return fmt.Errorf("branch name became empty after sanitization")
	}
	worktreeDir := filepath.Join(cfg.Repository.Root, worktreesDirectory, slug)
	sess := session{
		Name: sessionNameFromTemplate(cfg.SessionTemplate, rawBranch, cfg.ProjectName),
		Dir:  worktreeDir,
	}

	if badge != "" {
		sess.Name = "[" + badge + "] " + sess.Name
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
		if cfg.Repository.MainCheckout != "" {
			if err := ensureWorktreesIgnored(cfg.Repository.Root); err != nil {
				return err
			}
			if _, err := runGit("check-ignore", "--quiet", "--", worktreesDirectory+"/"); err != nil {
				return fmt.Errorf("%s/ is not ignored; check conflicting .gitignore rules", worktreesDirectory)
			}
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
		if err := markSetupPending(worktreeDir); err != nil {
			return err
		}
	}

	// A bare clone commonly has remote branch names as local refs already.
	// In that case worktreeAddExisting is the only valid checkout operation,
	// but it does not set an upstream itself. Explicit remote checkouts must
	// still track their requested origin branch. It is harmless to repeat when
	// resuming after a later failure, and lets a retry repair this step too.
	if branch.TrackRemote {
		if err := setBranchUpstream(rawBranch, baseRef); err != nil {
			return fmt.Errorf("setting branch upstream failed: %w", err)
		}
	}

	sessionExists := backend.Has(sess)
	if err := runWorktreeSetup(worktreeDir, cfg.Setup, sessionExists); err != nil {
		return err
	}

	if sessionExists {
		if badge != "" {
			if err := (herdrBackend{}).Rename(sess); err != nil {
				return fmt.Errorf("renaming herdr workspace failed: %w", err)
			}
		}
		logInfo("%s session already exists: %s", backend.Kind(), sess.Name)
		finishProgress()
		return attachOrSwitch(sess)
	}

	logStep("starting %s session: %s", backend.Kind(), sess.Name)
	if err := backend.Create(sess); err != nil {
		if backend.Kind() == backendHerdr && insideHerdr() {
			logInfo("tip: the worktree and setup are ready — rerun wk add %s to retry workspace creation", rawBranchArg)
		}
		return fmt.Errorf("creating %s session failed: %w", backend.Kind(), err)
	}

	finishProgress()
	return attachOrSwitch(sess)
}

type createBranch struct {
	Name        string
	BaseRef     string
	TrackRemote bool
}

// resolveCreateBranch preserves existing branch checkout precedence, then
// chooses the caller's commit for normal clones or the configured bare base.
// Git ref lookups run at the project root after configuration has loaded.
func resolveCreateBranch(rawBranch, callerDir string, cfg ProjectConfig) (createBranch, error) {
	if strings.HasPrefix(rawBranch, "origin/") {
		return createBranch{
			Name: strings.TrimPrefix(rawBranch, "origin/"), BaseRef: rawBranch, TrackRemote: true,
		}, nil
	}

	exists := branchExists(rawBranch)
	if !exists && refExists("origin/"+rawBranch) {
		// Completion offers remote branches without the origin/ prefix.
		return createBranch{
			Name: rawBranch, BaseRef: "origin/" + rawBranch, TrackRemote: true,
		}, nil
	}
	if cfg.Repository.MainCheckout == "" {
		return createBranch{Name: rawBranch, BaseRef: cfg.BaseRef}, nil
	}
	if exists {
		return createBranch{Name: rawBranch, BaseRef: rawBranch}, nil
	}

	// Pin the caller's commit, not the main checkout's HEAD. Detached
	// checkouts are valid starting points too.
	baseRef, err := gitAt(callerDir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return createBranch{}, fmt.Errorf("resolving current checkout commit: %w", err)
	}
	return createBranch{Name: rawBranch, BaseRef: baseRef}, nil
}

// runWorktreeSetup retries pending setup before a session is created. Existing
// sessions and empty setup lists leave the checkpoint untouched.
func runWorktreeSetup(dir string, steps []string, sessionExists bool) error {
	if len(steps) == 0 || sessionExists {
		return nil
	}
	state, err := readSetupState(dir)
	if err != nil {
		return err
	}
	switch state {
	case setupPending:
		logStep("running setup steps")
		if err := runShellSteps(dir, steps); err != nil {
			return err
		}
		return markSetupComplete(dir)
	case setupUnknown:
		// Legacy setup may already have completed. Arbitrary setup is often
		// not idempotent (for example, ln -s), so skip it rather than guessing.
		logInfo("existing worktree has no setup checkpoint, not rerunning setup")
	}
	return nil
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
