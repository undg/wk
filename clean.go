package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PRLister looks up a worktree's PR state (e.g. via `gh pr view`), wrapped
// behind an interface so tests can fake it without a real gh auth/network.
type PRLister interface {
	PRState(dir string) (string, error)
}

type ghPRLister struct{}

func (ghPRLister) PRState(dir string) (string, error) {
	cmd := exec.Command("gh", "pr", "view", "--json", "state", "--jq", ".state")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// No PR for this branch, or gh not authenticated — same as the
		// bash POC's `2>/dev/null`, treat as "no state" rather than fatal.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

type worktreeEntry struct {
	Dir    string
	Branch string
}

// parseWorktreePorcelain parses `git worktree list --porcelain` output.
// Entries with no branch line (detached HEAD) are returned with Branch ==
// "" so callers can skip them, matching the bash POC which only acts on
// lines starting with "branch".
func parseWorktreePorcelain(output string) []worktreeEntry {
	var entries []worktreeEntry
	var current worktreeEntry
	flush := func() {
		if current.Dir != "" {
			entries = append(entries, current)
		}
	}
	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			current = worktreeEntry{Dir: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
			flush()
			current = worktreeEntry{}
		}
	}
	flush()
	return entries
}

func listWorktrees() ([]worktreeEntry, error) {
	out, err := runGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %w", err)
	}
	return parseWorktreePorcelain(out), nil
}

func promptYesNo(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s (y/n) ", prompt)
	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func runClean() error {
	entries, err := listWorktrees()
	if err != nil {
		return err
	}
	return cleanWorktrees(entries, ghPRLister{}, promptYesNo, runDelete)
}

// cleanWorktrees is the testable core of `wk clean`: for each worktree with
// a MERGED PR, prompt and delete via deleteFn — the same internal delete
// path `wk delete` uses, no shelling back out to another script.
func cleanWorktrees(entries []worktreeEntry, prLister PRLister, confirm func(prompt string) bool, deleteFn func(dir string) error) error {
	for _, wt := range entries {
		if wt.Branch == "" {
			continue
		}

		state, err := prLister.PRState(wt.Dir)
		if err != nil {
			return err
		}

		if state != "MERGED" {
			label := state
			if label == "" {
				label = "no PR"
			}
			logInfo("%s: %s", wt.Branch, label)
			continue
		}

		logOK("%s: MERGED", wt.Branch)
		if !confirm(fmt.Sprintf("delete worktree %s?", wt.Dir)) {
			continue
		}
		if err := deleteFn(wt.Dir); err != nil {
			logError("failed to delete %s: %s", wt.Dir, err)
			continue
		}
		logOK("deleted %s", wt.Dir)
	}
	return nil
}
