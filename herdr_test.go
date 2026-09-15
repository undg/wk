package main

import (
	"errors"
	"strings"
	"testing"
)

// Trimmed from real `herdr workspace list` output. Kept verbatim in shape so
// it fails loudly if herdr's contract changes: note the two workspaces both
// labelled "main" in different projects, and the ones with no worktree at
// all (a plain terminal workspace, not a git checkout).
const herdrWorkspaceListFixture = `{"id":"cli:workspace:list","result":{"type":"workspace_list","workspaces":[
{"active_tab_id":"wM:t2","agent_status":"unknown","focused":false,"label":"OBSIDIAN","number":2,"pane_count":2,"tab_count":2,"workspace_id":"wM"},
{"active_tab_id":"w11:t1","agent_status":"unknown","focused":false,"label":"main","number":7,"pane_count":1,"tab_count":1,"workspace_id":"w11","worktree":{"checkout_path":"/Users/x/Code/pgm-be/main","is_linked_worktree":true,"repo_key":"/Users/x/Code/pgm-be/.bare","repo_name":"pgm-be","repo_root":"/Users/x/Code/pgm-be"}},
{"active_tab_id":"w15:t1","agent_status":"unknown","focused":false,"label":"main","number":8,"pane_count":1,"tab_count":1,"workspace_id":"w15","worktree":{"checkout_path":"/Users/x/Code/pgm-fe/main","is_linked_worktree":true,"repo_key":"/Users/x/Code/pgm-fe/.bare","repo_name":"pgm-fe","repo_root":"/Users/x/Code/pgm-fe"}},
{"active_tab_id":"w1J:t1","agent_status":"done","focused":false,"label":"feat/606895/non-root-user","number":11,"pane_count":2,"tab_count":1,"workspace_id":"w1J","worktree":{"checkout_path":"/Users/x/Code/pgm-fe/feat-606895-non-root-user","is_linked_worktree":true,"repo_key":"/Users/x/Code/pgm-fe/.bare","repo_name":"pgm-fe","repo_root":"/Users/x/Code/pgm-fe"}},
{"active_tab_id":"w1K:t1","agent_status":"idle","focused":true,"label":"wk.go","number":12,"pane_count":2,"tab_count":1,"workspace_id":"w1K"}
]}}`

func fixtureWorkspaces(t *testing.T) []herdrWorkspace {
	t.Helper()
	workspaces, err := parseHerdrWorkspaces([]byte(herdrWorkspaceListFixture))
	if err != nil {
		t.Fatalf("parseHerdrWorkspaces: %v", err)
	}
	if len(workspaces) != 5 {
		t.Fatalf("parsed %d workspaces, want 5", len(workspaces))
	}
	return workspaces
}

func TestParseHerdrWorkspaces(t *testing.T) {
	workspaces := fixtureWorkspaces(t)

	if got, want := workspaces[1].WorkspaceID, "w11"; got != want {
		t.Errorf("WorkspaceID = %q, want %q", got, want)
	}
	if got, want := workspaces[1].Label, "main"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
	if got, want := workspaces[1].Worktree.CheckoutPath, "/Users/x/Code/pgm-be/main"; got != want {
		t.Errorf("CheckoutPath = %q, want %q", got, want)
	}

	// A workspace that isn't a git checkout must leave CheckoutPath empty,
	// so it can never match a resolved worktree path.
	if got := workspaces[0].Worktree.CheckoutPath; got != "" {
		t.Errorf("workspace with no worktree: CheckoutPath = %q, want empty", got)
	}
}

// The two "main" workspaces are why wk matches on path: by label they are
// indistinguishable, by path they are not.
func TestFindWorkspaceForPathDisambiguatesSharedLabels(t *testing.T) {
	workspaces := fixtureWorkspaces(t)

	for _, c := range []struct{ path, wantID string }{
		{"/Users/x/Code/pgm-be/main", "w11"},
		{"/Users/x/Code/pgm-fe/main", "w15"},
		{"/Users/x/Code/pgm-fe/feat-606895-non-root-user", "w1J"},
	} {
		ws, err := findWorkspaceForPath(workspaces, c.path)
		if err != nil {
			t.Fatalf("findWorkspaceForPath(%q): %v", c.path, err)
		}
		if ws.WorkspaceID != c.wantID {
			t.Errorf("findWorkspaceForPath(%q) = %q, want %q", c.path, ws.WorkspaceID, c.wantID)
		}
	}
}

func TestFindWorkspaceForPathNoMatch(t *testing.T) {
	workspaces := fixtureWorkspaces(t)

	_, err := findWorkspaceForPath(workspaces, "/Users/x/Code/pgm-be/feat-not-open")
	if !errors.Is(err, errNoWorkspace) {
		t.Errorf("err = %v, want errNoWorkspace", err)
	}

	// An empty path must not match the workspaces that have no worktree.
	if _, err := findWorkspaceForPath(workspaces, ""); !errors.Is(err, errNoWorkspace) {
		t.Errorf("empty path: err = %v, want errNoWorkspace", err)
	}
}

func TestFindWorkspaceForPathRefusesDuplicates(t *testing.T) {
	workspaces := fixtureWorkspaces(t)
	dupe := workspaces[1]
	dupe.WorkspaceID = "w99"
	workspaces = append(workspaces, dupe)

	_, err := findWorkspaceForPath(workspaces, "/Users/x/Code/pgm-be/main")
	if err == nil {
		t.Fatal("two workspaces on one worktree: nil error, want a refusal")
	}
	if errors.Is(err, errNoWorkspace) {
		t.Errorf("err = %v, want a duplicate refusal, not errNoWorkspace", err)
	}
	for _, id := range []string{"w11", "w99"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("err %q does not name the ambiguous workspace %q", err, id)
		}
	}
}
