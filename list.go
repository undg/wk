package main

import "fmt"

// runLs shows worktrees for the current project alongside their session
// status (detected the same way as add/delete/clean, via .wk.toml).
// With porcelain=true, output is 3-line blocks (branch, dir, session name)
// separated by a blank line, meant for scripts rather than humans.
func runLs(porcelain bool, backendOverride string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
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
		session := "no " + backend.Kind() + " session"
		if sessionName != "" {
			session = backend.Kind() + ": " + sessionName
		}
		fmt.Printf("%-30s %-40s %s\n", branch, wt.Dir, session)
	}
	return nil
}
