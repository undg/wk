package main

import (
	"fmt"
	"io"
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

	rows := discoverListRows(entries, cfg, backend)
	if porcelain {
		writeListPorcelain(os.Stdout, rows)
	} else {
		writeListTable(os.Stdout, rows, root, backend.Kind())
	}
	return nil
}

// listRow keeps Git's branch and absolute path unchanged until rendering.
type listRow struct {
	branch, dir, session string
}

func discoverListRows(entries []worktreeEntry, cfg ProjectConfig, backend sessionBackend) []listRow {
	var rows []listRow
	for _, wt := range entries {
		rows = append(rows, listRow{branch: wt.Branch, dir: wt.Dir, session: listSessionName(wt, cfg, backend)})
	}
	return rows
}

func listSessionName(wt worktreeEntry, cfg ProjectConfig, backend sessionBackend) string {
	if wt.Branch == "" {
		return ""
	}
	if backend.Kind() == backendHerdr {
		if ws, err := herdrWorkspaceFor(wt.Dir); err == nil {
			return ws.Label
		}
		return ""
	}
	candidate := session{
		Name: sessionNameFromTemplate(cfg.SessionTemplate, wt.Branch, cfg.ProjectName),
		Dir:  wt.Dir,
	}
	if backend.Has(candidate) {
		return candidate.Name
	}
	return ""
}

func writeListPorcelain(w io.Writer, rows []listRow) {
	for _, r := range rows {
		fmt.Fprintf(w, "%s\n%s\n%s\n\n", r.branch, r.dir, r.session)
	}
}

func writeListTable(w io.Writer, rows []listRow, root, backendKind string) {
	// Copy display rows so rendering never changes Git's original values.
	displayRows := make([]listRow, 0, len(rows))
	branchWidth, dirWidth := len("BRANCH"), len("WORKTREE")
	for _, r := range rows {
		if r.branch == "" {
			r.branch = "(detached)"
		}
		// Show paths relative to the project root, including the hidden
		// container in normal clones, rather than Git's absolute paths.
		if rel, err := filepath.Rel(root, r.dir); err == nil && !strings.HasPrefix(rel, "..") {
			r.dir = rel
		}
		displayRows = append(displayRows, r)
		branchWidth = max(branchWidth, len(r.branch))
		dirWidth = max(dirWidth, len(r.dir))
	}

	fmt.Fprintf(w, "%s%-*s  %-*s  %s%s\n", colorBlue, branchWidth, "BRANCH", dirWidth, "WORKTREE", "SESSION", colorReset)
	for _, r := range displayRows {
		session := colorRed + "no " + backendKind + " session" + colorReset
		if r.session != "" {
			session = colorGreen + backendKind + ": " + r.session + colorReset
		}
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", branchWidth, r.branch, dirWidth, r.dir, session)
	}
}
