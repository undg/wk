package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runLs shows worktrees for the current project alongside their session
// status (detected the same way as add/delete/clean, via .wk.toml).
// With porcelain=true, output is 3-line blocks (branch, dir, session name)
// separated by a blank line, meant for scripts rather than humans.
func runLs(porcelain bool, backendOverride string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	backend, err := newSessionBackend(cfg.SessionBackend, backendOverride)
	if err != nil {
		return err
	}

	entries, err := listWorktrees()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		if !porcelain {
			logInfo("no worktrees found")
		}
		return nil
	}

	type row struct {
		branch, dir, session string
	}
	var rows []row
	branchWidth, dirWidth := len("BRANCH"), len("WORKTREE")

	for _, wt := range entries {
		sessionName := ""
		if wt.Branch != "" {
			candidate := session{
				Name: sessionNameFromTemplate(cfg.SessionTemplate, wt.Branch, cfg.ProjectName),
				Dir:  wt.Dir,
			}
			if backend.Has(candidate) {
				sessionName = candidate.Name
			}
		}

		if porcelain {
			fmt.Printf("%s\n%s\n%s\n\n", wt.Branch, wt.Dir, sessionName)
			continue
		}

		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}

		// Worktree dirs are siblings of the project root, so this is
		// usually just the dir's basename — far more readable than the
		// absolute path git hands back.
		dir := wt.Dir
		if rel, err := filepath.Rel(root, wt.Dir); err == nil && !strings.HasPrefix(rel, "..") {
			dir = rel
		}

		rows = append(rows, row{branch: branch, dir: dir, session: sessionName})
		branchWidth = max(branchWidth, len(branch))
		dirWidth = max(dirWidth, len(dir))
	}

	if porcelain {
		return nil
	}

	fmt.Printf("%s%-*s  %-*s  %s%s\n", colorBlue, branchWidth, "BRANCH", dirWidth, "WORKTREE", "SESSION", colorReset)
	for _, r := range rows {
		session := colorRed + "no " + backend.Kind() + " session" + colorReset
		if r.session != "" {
			session = colorGreen + backend.Kind() + ": " + r.session + colorReset
		}
		fmt.Printf("%-*s  %-*s  %s\n", branchWidth, r.branch, dirWidth, r.dir, session)
	}
	return nil
}
