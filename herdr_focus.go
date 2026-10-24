package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// herdrFocus is UI context only. Worktrees are still resolved by Git/path.
type herdrFocus struct {
	Workspace string `json:"focused_workspace_id"`
	Tab       string `json:"focused_tab_id"`
	Pane      string `json:"focused_pane_id"`
}

// herdrAttachIfFocusUnchanged captures focus before setup can take time. A
// missing snapshot or caller identity must never authorize a focus change.
func herdrAttachIfFocusUnchanged() func(session) error {
	initial, err := currentHerdrFocus()
	canSwitch := err == nil && initial.Pane == os.Getenv("HERDR_PANE_ID")

	return func(s session) error {
		if canSwitch {
			current, err := currentHerdrFocus()
			if err == nil && current == initial {
				return (herdrBackend{}).AttachOrSwitch(s)
			}
		}
		logOK("herdr workspace ready: %s (leaving focus unchanged)", s.Name)
		return nil
	}
}

// herdrFocusParentBeforeClose is best-effort UI behavior. A missing or
// ambiguous parent must never prevent closing the workspace.
func herdrFocusParentBeforeClose(target herdrWorkspace) {
	if !insideHerdr() {
		return
	}
	snapshot, err := currentHerdrSnapshot()
	if err != nil || snapshot.Workspace != target.WorkspaceID {
		return
	}
	// Use one snapshot for focus and group membership. The deleted checkout
	// no longer exists on disk, but Herdr retains its workspace metadata.
	var child herdrWorkspace
	for _, ws := range snapshot.Workspaces {
		if ws.WorkspaceID == target.WorkspaceID {
			child = ws
			break
		}
	}
	if child.Worktree.RepoKey == "" || child.Worktree.IsLinkedWorktree == nil || !*child.Worktree.IsLinkedWorktree {
		return
	}
	var parent string
	for _, ws := range snapshot.Workspaces {
		if ws.Worktree.RepoKey != child.Worktree.RepoKey || ws.Worktree.IsLinkedWorktree == nil || *ws.Worktree.IsLinkedWorktree {
			continue
		}
		if parent != "" || ws.WorkspaceID == "" {
			return
		}
		parent = ws.WorkspaceID
	}
	if parent != "" {
		if _, err := runHerdr("workspace", "focus", parent); err != nil {
			logInfo("could not focus parent workspace; using herdr's default close behavior: %v", err)
		}
	}
}

type herdrSnapshot struct {
	herdrFocus
	Workspaces []herdrWorkspace `json:"workspaces"`
}

func currentHerdrFocus() (herdrFocus, error) {
	snapshot, err := currentHerdrSnapshot()
	if err != nil {
		return herdrFocus{}, err
	}
	focus := snapshot.herdrFocus
	if focus.Workspace == "" || focus.Tab == "" || focus.Pane == "" {
		return herdrFocus{}, fmt.Errorf("herdr snapshot has no complete focus context")
	}
	return focus, nil
}

func currentHerdrSnapshot() (herdrSnapshot, error) {
	// Focus checking is optional UI behavior, not a reason to delay or fail
	// an add/delete whose Git operations are already complete.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", "api", "snapshot")
	cmd.Env = filterEnv(os.Environ(), "HERDR_WORKSPACE_ID")
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return herdrSnapshot{}, err
	}
	var reply struct {
		Result struct {
			Snapshot herdrSnapshot `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return herdrSnapshot{}, err
	}
	return reply.Result.Snapshot, nil
}
