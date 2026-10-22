package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestE2EHerdrAddOnlySwitchesFromLaunchingPane(t *testing.T) {
	snapshot := func(workspace, tab, pane string) string {
		return fmt.Sprintf(`{"result":{"snapshot":{"focused_workspace_id":%q,"focused_tab_id":%q,"focused_pane_id":%q}}}`, workspace, tab, pane)
	}
	launch := snapshot("w1", "w1:t1", "w1:p1")
	elsewhere := snapshot("w2", "w2:t1", "w2:p1")

	for _, c := range []struct {
		name      string
		before    string
		after     string
		caller    string
		wantFocus bool
	}{
		{"still in launching pane", launch, launch, "w1:p1", true},
		{"different workspace", launch, elsewhere, "w1:p1", false},
		{"different tab", launch, snapshot("w1", "w1:t2", "w1:p2"), "w1:p1", false},
		{"different pane", launch, snapshot("w1", "w1:t1", "w1:p2"), "w1:p1", false},
		{"already elsewhere at launch", elsewhere, elsewhere, "w1:p1", false},
		{"missing caller identity", launch, launch, "", false},
		{"initial query fails", "unavailable", launch, "w1:p1", false},
		{"final query fails", launch, "unavailable", "w1:p1", false},
		{"initial snapshot malformed", "not json", launch, "w1:p1", false},
		{"final snapshot malformed", launch, "not json", "w1:p1", false},
		{"initial snapshot incomplete", snapshot("w1", "", "w1:p1"), launch, "w1:p1", false},
		{"final snapshot incomplete", launch, snapshot("w1", "w1:t1", ""), "w1:p1", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Setup deterministically changes the fake UI focus while wk runs.
			p := newE2EProject(t, `setup = ['cp "$WK_FAKE_STATE/next-focus.json" "$WK_FAKE_STATE/herdr-focus.json"']
`)
			p.env = setEnv(p.env, map[string]string{
				"HERDR_ENV":     "1",
				"HERDR_PANE_ID": c.caller,
			})
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus.json"), c.before)
			mustWriteFile(t, filepath.Join(p.fakeState, "next-focus.json"), c.after)

			got := p.run("", "add", "test-worktree")
			mustSucceed(t, got)
			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			if c.wantFocus {
				mustContain(t, log, "|workspace focus w0\n")
			} else {
				mustNotContain(t, log, "|workspace focus ")
				mustContain(t, got.output, "workspace ready: test-worktree [project] (leaving focus unchanged)")
			}
			mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "herdr-workspaces")), filepath.Join(mustRealPath(t, p.root), "test-worktree"))
			mustEqual(t, gitOutput(t, p, filepath.Join(p.root, "test-worktree"), "branch", "--show-current"), "test-worktree")
		})
	}
}

func TestE2EAddFocusGuardLeavesOtherBackendsUnchanged(t *testing.T) {
	for _, backend := range []string{backendHerdr, backendTmux} {
		t.Run(backend, func(t *testing.T) {
			p := newE2EProject(t, "")
			p.env = setEnv(p.env, map[string]string{"TMUX": "", "HERDR_ENV": ""})
			if backend == backendTmux {
				p.env = setEnv(p.env, map[string]string{"HERDR_ENV": "1"})
			}
			mustSucceed(t, p.run("", "--backend", backend, "add", "test-worktree"))
			if backend == backendHerdr {
				log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
				mustContain(t, log, "|workspace focus w0\n")
				mustNotContain(t, log, "|api snapshot")
			} else {
				log := mustReadFile(t, filepath.Join(p.fakeState, "tmux.log"))
				mustContain(t, log, "attach-session -t test-worktree [project]")
				if _, err := os.Stat(filepath.Join(p.fakeState, "herdr.log")); !os.IsNotExist(err) {
					t.Fatalf("tmux add unexpectedly invoked herdr: %v", err)
				}
			}
		})
	}
}

func TestE2EHerdrExistingWorkspaceDoesNotStealFocus(t *testing.T) {
	p := newE2EProject(t, "")
	mustSucceed(t, p.run("", "add", "test-worktree"))
	if err := os.Remove(filepath.Join(p.fakeState, "herdr.log")); err != nil {
		t.Fatal(err)
	}
	p.env = setEnv(p.env, map[string]string{
		"HERDR_ENV":     "1",
		"HERDR_PANE_ID": "w1:p1",
	})
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus.json"),
		`{"result":{"snapshot":{"focused_workspace_id":"w2","focused_tab_id":"w2:t1","focused_pane_id":"w2:p1"}}}`)

	got := p.run("", "add", "test-worktree", "rev")
	mustSucceed(t, got)
	log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
	mustNotContain(t, log, "|workspace focus ")
	mustNotContain(t, log, "|worktree open ")
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "herdr-workspaces")), "[rev] test-worktree [project]")
	mustContain(t, got.output, "leaving focus unchanged")
}
