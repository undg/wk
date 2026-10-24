package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCurrentHerdrFocusRequiresCompleteContext(t *testing.T) {
	p := newE2EProject(t, "")
	enterTestProject(t, p, p.root)
	for _, c := range []struct {
		name                 string
		workspace, tab, pane string
		snapshot             string
	}{
		{name: "complete", workspace: "w0", tab: "t0", pane: "p0"},
		{name: "missing workspace", tab: "t0", pane: "p0"},
		{name: "missing tab", workspace: "w0", pane: "p0"},
		{name: "missing pane", workspace: "w0", tab: "t0"},
		{name: "unavailable", snapshot: "unavailable"},
		{name: "malformed", snapshot: "not json"},
	} {
		t.Run(c.name, func(t *testing.T) {
			snapshot := c.snapshot
			if snapshot == "" {
				snapshot = fmt.Sprintf(`{"result":{"snapshot":{"focused_workspace_id":%q,"focused_tab_id":%q,"focused_pane_id":%q}}}`, c.workspace, c.tab, c.pane)
			}
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr-focus.json"), snapshot)
			got, err := currentHerdrFocus()
			if c.name != "complete" {
				if err == nil || got != (herdrFocus{}) {
					t.Fatalf("incomplete context accepted: %+v, %v", got, err)
				}
				return
			}
			want := herdrFocus{Workspace: c.workspace, Tab: c.tab, Pane: c.pane}
			if err != nil || got != want {
				t.Fatalf("focus = %+v, %v; want %+v", got, err, want)
			}
		})
	}
}

func TestCurrentHerdrSnapshotDoesNotWaitForInheritedPipes(t *testing.T) {
	bin := t.TempDir()
	// The command exits, but its background child still holds stdout open.
	mustWriteExecutable(t, filepath.Join(bin, "herdr"), "#!/bin/sh\nsleep 2 &\nprintf '{}'\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := currentHerdrSnapshot(); !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("snapshot error = %v, want inherited-pipe wait limit", err)
	}
}

func TestCurrentHerdrSnapshotTimeout(t *testing.T) {
	bin := t.TempDir()
	mustWriteExecutable(t, filepath.Join(bin, "herdr"), "#!/bin/sh\nexec sleep 10\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	start := time.Now()
	if _, err := currentHerdrSnapshot(); err == nil {
		t.Fatal("expected snapshot timeout")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("snapshot exceeded its timeout")
	}
}
