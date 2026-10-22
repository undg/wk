package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// startHerdrProgress reports activity in the invoking pane, not the target
// worktree. The returned cleanup is safe to call before attach/close and defer.
func startHerdrProgress(enabled bool) func() {
	noop := func() {}
	bin, pane := os.Getenv("HERDR_BIN_PATH"), os.Getenv("HERDR_PANE_ID")
	if !enabled || !insideHerdr() || bin == "" || pane == "" || os.Getenv("HERDR_SOCKET_PATH") == "" {
		return noop
	}

	// Never take lifecycle authority from an agent that invoked wk as a tool.
	out, err := runHerdrProgress(bin, "pane", "get", pane)
	if err != nil {
		return noop
	}
	var reply struct {
		Result struct {
			Pane *struct {
				ID    string `json:"pane_id"`
				Agent string `json:"agent"`
			} `json:"pane"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &reply) != nil || reply.Result.Pane == nil || reply.Result.Pane.ID != pane || reply.Result.Pane.Agent != "" {
		return noop
	}

	seq := time.Now().UnixNano()
	_, _ = runHerdrProgress(bin, "pane", "report-agent", pane,
		"--source", "wk", "--agent", "wk", "--state", "working", "--seq", strconv.FormatInt(seq, 10))

	// Release even if reporting timed out: the server may have applied it.
	released := false
	return func() {
		if released {
			return
		}
		released = true
		_, _ = runHerdrProgress(bin, "pane", "release-agent", pane,
			"--source", "wk", "--agent", "wk", "--seq", strconv.FormatInt(seq+1, 10))
	}
}

// Progress is decoration: bound its latency and keep failures off the terminal.
func runHerdrProgress(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd.Output()
}
