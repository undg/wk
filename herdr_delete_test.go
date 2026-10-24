package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EHerdrDeleteFocusesProjectRoot(t *testing.T) {
	for _, bare := range []bool{false, true} {
		t.Run(fmt.Sprintf("bare=%t", bare), func(t *testing.T) {
			// A focus snapshot captured before Git removal must not be used:
			// teardown supplies the current state immediately before deletion.
			p := newTestProject(t, `teardown = ['cp "$WK_FAKE_STATE/next-focus.json" "$WK_FAKE_STATE/herdr-focus.json"']
`, bare)
			mustSucceed(t, p.run("", "add", "feature"))
			root := mustRealPath(t, p.root)
			dir := filepath.Join(root, ".wk", "feature")
			key := filepath.Join(root, ".git")
			if bare {
				key = filepath.Join(root, ".bare")
			}
			// Labels deliberately collide. A linked checkout named main and a
			// root from another repository must not become the destination.
			snapshot := fmt.Sprintf(`{"result":{"snapshot":{"focused_workspace_id":"w0","workspaces":[
{"workspace_id":"other-root","label":"project","worktree":{"repo_key":"other","is_linked_worktree":false}},
{"workspace_id":"main-child","label":"main","worktree":{"repo_key":%q,"is_linked_worktree":true}},
{"workspace_id":"root","label":"main","worktree":{"repo_key":%q,"checkout_path":%q,"is_linked_worktree":false}},
{"workspace_id":"w0","label":"main","worktree":{"repo_key":%q,"checkout_path":%q,"is_linked_worktree":true}}
]}}}`, key, key, root, key, dir)
			mustWriteFile(t, filepath.Join(p.fakeState, "next-focus.json"), snapshot)
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus.json"), "unavailable")
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr.log"), "")
			p.env = setEnv(p.env, map[string]string{"HERDR_ENV": "1"})

			mustSucceed(t, p.runAt(dir, "", "rm", "."))

			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			mustContain(t, log, "|api snapshot\n|workspace focus root\n|workspace close w0\n")
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("deleted checkout still exists: %v", err)
			}
			mustEqual(t, gitOutput(t, p, root, "branch", "--list", "feature"), "")
			mustNotContain(t, mustReadFile(t, filepath.Join(p.fakeState, "herdr-workspaces")), dir)
		})
	}
}

// Run deletion in-process as well as through the CLI above: native coverage
// and mutation tools cannot observe the separately built E2E binary.
func TestHerdrDeleteFocusSafety(t *testing.T) {
	child := `{"workspace_id":"w0","worktree":{"repo_key":"repo","is_linked_worktree":true}}`
	parent := `{"workspace_id":"root","worktree":{"repo_key":"repo","is_linked_worktree":false}}`
	for _, c := range []struct {
		name       string
		focused    string
		workspaces string
		snapshot   string
		outside    bool
		focusFails bool
		wantFocus  bool
	}{
		{name: "focused child returns to parent", focused: "w0", workspaces: child + "," + parent, wantFocus: true},
		{name: "background deletion", focused: "elsewhere", workspaces: child + "," + parent},
		{name: "no parent", focused: "w0", workspaces: child},
		{name: "ambiguous parent", focused: "w0", workspaces: child + "," + parent + `,{"workspace_id":"second-root","worktree":{"repo_key":"repo","is_linked_worktree":false}}`},
		{name: "parent missing workspace ID", focused: "w0", workspaces: child + `,{"worktree":{"repo_key":"repo","is_linked_worktree":false}}`},
		{name: "invalid parent before valid parent", focused: "w0", workspaces: child + `,{"worktree":{"repo_key":"repo","is_linked_worktree":false}},` + parent},
		{name: "linked sibling is not parent", focused: "w0", workspaces: child + `,{"workspace_id":"main","worktree":{"repo_key":"repo","is_linked_worktree":true}},` + parent, wantFocus: true},
		{name: "different repository", focused: "w0", workspaces: child + `,{"workspace_id":"root","worktree":{"repo_key":"other","is_linked_worktree":false}}`},
		{name: "parent missing linked flag", focused: "w0", workspaces: child + `,{"workspace_id":"root","worktree":{"repo_key":"repo"}}`},
		{name: "parent null linked flag", focused: "w0", workspaces: child + `,{"workspace_id":"root","worktree":{"repo_key":"repo","is_linked_worktree":null}}`},
		{name: "child missing linked flag", focused: "w0", workspaces: `{"workspace_id":"w0","worktree":{"repo_key":"repo"}},` + parent},
		{name: "child missing repository key", focused: "w0", workspaces: `{"workspace_id":"w0","worktree":{"is_linked_worktree":true}},` + parent},
		{name: "no worktree metadata", focused: "w0", workspaces: `{"workspace_id":"w0"},` + parent},
		{name: "target is root", focused: "w0", workspaces: `{"workspace_id":"w0","worktree":{"repo_key":"repo","is_linked_worktree":false}},` + parent},
		{name: "missing child", focused: "w0", workspaces: parent},
		{name: "no focus", workspaces: child + "," + parent},
		{name: "snapshot unavailable", snapshot: "unavailable"},
		{name: "snapshot malformed", snapshot: "not json"},
		{name: "outside Herdr", focused: "w0", workspaces: child + "," + parent, outside: true},
		{name: "focus failure still closes", focused: "w0", workspaces: child + "," + parent, focusFails: true, wantFocus: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := newE2EProject(t, `teardown = ['cp "$WK_FAKE_STATE/next-focus.json" "$WK_FAKE_STATE/herdr-focus.json"']
`)
			mustSucceed(t, p.run("", "add", "feature"))
			snapshot := c.snapshot
			if snapshot == "" {
				snapshot = fmt.Sprintf(`{"result":{"snapshot":{"focused_workspace_id":%q,"workspaces":[%s]}}}`, c.focused, c.workspaces)
			}
			mustWriteFile(t, filepath.Join(p.fakeState, "next-focus.json"), snapshot)
			if c.focusFails {
				mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus-fail"), "")
			}
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr.log"), "")
			if !c.outside {
				p.env = setEnv(p.env, map[string]string{"HERDR_ENV": "1"})
			}

			enterTestProject(t, p, p.root)
			output, err := captureWorkflowOutput(t, func() error { return runDelete("feature", backendHerdr) })
			if err != nil {
				t.Fatal(err)
			}
			if c.focusFails {
				mustContain(t, output, "could not focus parent workspace")
			} else {
				mustNotContain(t, output, "could not focus parent workspace")
			}

			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			mustContain(t, log, "|workspace close w0\n")
			if strings.Contains(log, "|workspace focus ") != c.wantFocus {
				t.Fatalf("unexpected focus behavior:\n%s", log)
			}
			if c.wantFocus {
				mustContain(t, log, "|workspace focus root\n|workspace close w0\n")
			}
			if c.outside {
				mustNotContain(t, log, "|api snapshot")
			}
			mustEqual(t, gitOutput(t, p, p.root, "branch", "--list", "feature"), "")
		})
	}
}

func TestHerdrKillMissingWorkspaceLeavesFocusUnchanged(t *testing.T) {
	p := newE2EProject(t, "")
	enterTestProject(t, p, p.root)
	t.Setenv("HERDR_ENV", "1")
	if err := (herdrBackend{}).Kill(session{Dir: p.root}); err == nil {
		t.Fatal("closing a missing workspace succeeded")
	}
	log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
	mustNotContain(t, log, "|api snapshot")
	mustNotContain(t, log, "|workspace focus ")
	mustNotContain(t, log, "|workspace close ")
}

func TestHerdrAttachStopsAfterFocusFailure(t *testing.T) {
	p := newE2EProject(t, "")
	mustSucceed(t, p.run("", "add", "feature"))
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus-fail"), "")
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr.log"), "")
	enterTestProject(t, p, p.root)
	if err := (herdrBackend{}).AttachOrSwitch(session{Dir: filepath.Join(p.root, ".wk", "feature")}); err == nil {
		t.Fatal("attachment ignored the failed focus request")
	}
	log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
	mustContain(t, log, "|workspace focus w0\n")
	mustNotContain(t, log, "|\n") // No bare herdr TUI launch after failure.
}

func TestE2EHerdrFailedDeleteLeavesFocusUnchanged(t *testing.T) {
	p := newE2EProject(t, "")
	mustSucceed(t, p.run("", "add", "feature"))
	dir := filepath.Join(mustRealPath(t, p.root), ".wk", "feature")
	testGit(t, p.root, "worktree", "lock", dir)
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus.json"), `{"result":{"snapshot":{"focused_workspace_id":"w0","workspaces":[
{"workspace_id":"w0","worktree":{"repo_key":"repo","is_linked_worktree":true}},
{"workspace_id":"root","worktree":{"repo_key":"repo","is_linked_worktree":false}}
]}}}`)
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr.log"), "")
	p.env = setEnv(p.env, map[string]string{"HERDR_ENV": "1"})

	got := p.runAt(dir, "", "delete", ".")

	mustFail(t, got)
	mustContain(t, got.output, "removing worktree")
	log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
	mustNotContain(t, log, "|api snapshot")
	mustNotContain(t, log, "|workspace focus ")
	mustNotContain(t, log, "|workspace close ")
	mustDir(t, dir)
	mustEqual(t, gitOutput(t, p, dir, "branch", "--show-current"), "feature")
}

func TestE2EDeleteFocusLeavesTmuxUnchanged(t *testing.T) {
	p := newE2EProject(t, "")
	p.env = setEnv(p.env, map[string]string{"HERDR_ENV": "1"})
	mustSucceed(t, p.run("", "add", "feature", "--tmux"))
	mustSucceed(t, p.run("", "delete", "feature", "--tmux"))
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux.log")), "kill-session -t feature [project]")
	if _, err := os.Stat(filepath.Join(p.fakeState, "herdr.log")); !os.IsNotExist(err) {
		t.Fatalf("tmux deletion invoked herdr: %v", err)
	}
}
