package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// herdrBackend hosts each worktree in a herdr workspace. Note the mapping:
// a herdr *workspace* is the analogue of a tmux session, while a herdr
// *session* is closer to a tmux server.
//
// Workspaces are found by the worktree they are checked out in, not by
// label. herdr labels are mutable, non-unique (two "main" workspaces from
// two projects), and workspaces created by other tools carry labels that
// don't match wk's template — whereas the checkout path is exactly what wk
// already knows. Keying on it also means wk structurally cannot touch a
// workspace that isn't a git worktree.
type herdrBackend struct{}

var errNoWorkspace = errors.New("no herdr workspace for that worktree")

func (herdrBackend) Kind() string { return backendHerdr }

func (herdrBackend) Has(s session) bool {
	// Mirrors tmuxHasSession: anything we can't resolve — herdr not
	// running, socket error — reads as "no session", not a hard failure.
	_, err := herdrWorkspaceFor(s.Dir)
	return err == nil
}

// IsCurrent guards against tearing down the workspace wk is running inside.
// herdr injects the caller's workspace id, so this needs no socket call to
// know where it is — only to learn what that workspace points at.
//
// It deliberately accepts a label match as well as a path match: a false
// positive only refuses a delete, while a false negative kills the pty
// running the delete.
func (herdrBackend) IsCurrent(s session) bool {
	if !insideHerdr() {
		return false
	}
	id := os.Getenv("HERDR_WORKSPACE_ID")
	if id == "" {
		return false
	}

	workspaces, err := herdrWorkspaces()
	if err != nil {
		return false
	}
	dir, err := filepath.Abs(s.Dir)
	if err != nil {
		return false
	}

	for _, ws := range workspaces {
		if ws.WorkspaceID != id {
			continue
		}
		return ws.Worktree.CheckoutPath == dir || ws.Label == s.Name
	}
	return false
}

// Create registers the worktree wk already created (via git.go) as a herdr
// workspace. It deliberately uses `worktree open`, not `worktree create` or
// plain `workspace create --cwd`: `create` would have herdr run its own git
// worktree checkout, duplicating what wk just did; a plain workspace has no
// worktree metadata at all, so herdr can't group it under the project and
// wk's own path-based lookup (below) can never find it again. `open` is the
// one primitive that registers existing-worktree metadata without touching
// git itself.
func (herdrBackend) Create(s session) error {
	// The workspace is created by the herdr server over a socket, so a
	// relative dir would resolve against the server's cwd, not ours.
	dir, err := filepath.Abs(s.Dir)
	if err != nil {
		return fmt.Errorf("resolving worktree path %s: %w", s.Dir, err)
	}
	_, err = runHerdr("worktree", "open", "--path", dir, "--label", s.Name, "--no-focus")
	return err
}

func (herdrBackend) Kill(s session) error {
	ws, err := herdrWorkspaceFor(s.Dir)
	if err != nil {
		return err
	}
	_, err = runHerdr("workspace", "close", ws.WorkspaceID)
	return err
}

// AttachOrSwitch focuses the workspace, then launches the TUI when wk was
// run from outside herdr. Bare `herdr` attaches to whatever is focused, so
// the focus call has to come first in both paths.
func (herdrBackend) AttachOrSwitch(s session) error {
	ws, err := herdrWorkspaceFor(s.Dir)
	if err != nil {
		return err
	}
	if _, err := runHerdr("workspace", "focus", ws.WorkspaceID); err != nil {
		return err
	}
	if insideHerdr() {
		return nil
	}

	tui := exec.Command("herdr")
	tui.Stdin = os.Stdin
	tui.Stdout = os.Stdout
	tui.Stderr = os.Stderr
	return tui.Run()
}

func insideHerdr() bool {
	return os.Getenv("HERDR_ENV") == "1"
}

type herdrWorkspace struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	// Absent for workspaces that aren't a git checkout, leaving
	// CheckoutPath empty so it never matches a resolved worktree path.
	Worktree struct {
		CheckoutPath string `json:"checkout_path"`
	} `json:"worktree"`
}

// herdrWorkspaceFor finds the single workspace checked out at dir.
func herdrWorkspaceFor(dir string) (herdrWorkspace, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return herdrWorkspace{}, fmt.Errorf("resolving worktree path %s: %w", dir, err)
	}

	workspaces, err := herdrWorkspaces()
	if err != nil {
		return herdrWorkspace{}, err
	}
	return findWorkspaceForPath(workspaces, abs)
}

// findWorkspaceForPath matches on the checkout path, so labels being
// mutable, duplicated across projects, or written by another tool doesn't
// matter. More than one match is an error rather than a guess: closing the
// wrong workspace would take a user's real work with it.
func findWorkspaceForPath(workspaces []herdrWorkspace, abs string) (herdrWorkspace, error) {
	var matches []herdrWorkspace
	for _, ws := range workspaces {
		// Workspaces that aren't a git checkout report no path at all, and
		// must stay unmatchable rather than collide on the empty string.
		if ws.Worktree.CheckoutPath == "" {
			continue
		}
		if ws.Worktree.CheckoutPath == abs {
			matches = append(matches, ws)
		}
	}

	switch len(matches) {
	case 0:
		return herdrWorkspace{}, fmt.Errorf("%w: %s", errNoWorkspace, abs)
	case 1:
		return matches[0], nil
	default:
		ids := make([]string, 0, len(matches))
		for _, ws := range matches {
			ids = append(ids, ws.WorkspaceID)
		}
		return herdrWorkspace{}, fmt.Errorf("%d herdr workspaces share the worktree %s (%s) — close one, wk will not guess", len(matches), abs, strings.Join(ids, ", "))
	}
}

func herdrWorkspaces() ([]herdrWorkspace, error) {
	out, err := runHerdr("workspace", "list")
	if err != nil {
		return nil, err
	}
	return parseHerdrWorkspaces(out)
}

func parseHerdrWorkspaces(out []byte) ([]herdrWorkspace, error) {
	var payload struct {
		Result struct {
			Workspaces []herdrWorkspace `json:"workspaces"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("parsing herdr workspace list: %w", err)
	}
	return payload.Result.Workspaces, nil
}

// runHerdr runs a socket-API command, capturing the JSON it replies with so
// it never lands in the user's terminal. herdr reports server errors as JSON
// on stderr, which stays inherited so those surface as-is.
func runHerdr(args ...string) ([]byte, error) {
	cmd := exec.Command("herdr", args...)
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("herdr %s failed: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
