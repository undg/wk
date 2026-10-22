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

func currentHerdrFocus() (herdrFocus, error) {
	// Focus checking is optional UI behavior, not a reason to delay an add
	// indefinitely or fail one whose worktree/workspace are already ready.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "herdr", "api", "snapshot")
	cmd.Env = filterEnv(os.Environ(), "HERDR_WORKSPACE_ID")
	cmd.WaitDelay = 100 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return herdrFocus{}, err
	}
	var reply struct {
		Result struct {
			Snapshot herdrFocus `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return herdrFocus{}, err
	}
	focus := reply.Result.Snapshot
	if focus.Workspace == "" || focus.Tab == "" || focus.Pane == "" {
		return herdrFocus{}, fmt.Errorf("herdr snapshot has no complete focus context")
	}
	return focus, nil
}
