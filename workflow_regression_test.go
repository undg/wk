package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func captureWorkflowOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "stdout-")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdout := os.Stdout
	os.Stdout = output
	defer func() { os.Stdout = stdout }()

	runErr := run()
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(output)
	if err != nil {
		t.Fatal(err)
	}
	return stripANSI(string(data)), runErr
}

func TestAddRetryRunsSetupConfiguredAfterWorkspaceFailure(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	failure := filepath.Join(p.fakeState, "herdr-open-fail")
	mustWriteFile(t, failure, "fail")
	_, err := captureWorkflowOutput(t, func() error { return runCreate("retry", "", backendHerdr) })
	if err == nil {
		t.Fatal("workspace failure unexpectedly succeeded")
	}
	mustContain(t, err.Error(), "creating herdr session failed")

	// No setup existed on the first attempt. Configuring it before retrying
	// must still run it, rather than treating the empty setup as completed.
	mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), `setup = ["printf setup >> setup.marker"]`)
	if err := os.Remove(failure); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := runCreate("retry", "", backendHerdr); err != nil {
			t.Fatal(err)
		}
		mustEqual(t, mustReadFile(t, filepath.Join(p.root, ".worktrees", "retry", "setup.marker")), "setup")
	}
}

func TestAddWorkspaceFailureShowsRetryTipOnlyInsideHerdr(t *testing.T) {
	for _, inside := range []bool{false, true} {
		name, env := "outside Herdr", ""
		if inside {
			name, env = "inside Herdr", "1"
		}
		t.Run(name, func(t *testing.T) {
			p := newTestProject(t, `setup = ["printf ready > setup.marker"]`, false)
			enterTestProject(t, p, p.root)
			t.Setenv("HERDR_ENV", env)
			t.Setenv("HERDR_PANE_ID", "w-launch:p1")
			mustWriteFile(t, filepath.Join(p.fakeState, "herdr-open-fail"), "fail")

			output, err := captureWorkflowOutput(t, func() error { return runCreate("feat/retry", "", backendHerdr) })
			if err == nil {
				t.Fatal("workspace failure unexpectedly succeeded")
			}
			mustContain(t, err.Error(), "creating herdr session failed")
			if inside {
				mustContain(t, output, "rerun wk add feat/retry to retry workspace creation")
			} else {
				mustNotContain(t, output, "retry workspace creation")
			}
			mustEqual(t, mustReadFile(t, filepath.Join(p.root, ".worktrees", "feat-retry", "setup.marker")), "ready")
		})
	}
}

func TestDeleteWithoutTeardownDoesNotAnnounceTeardown(t *testing.T) {
	p := newTestProject(t, "teardown = []\n", false)
	enterTestProject(t, p, p.root)
	if err := runCreate("remove", "", backendTmux); err != nil {
		t.Fatal(err)
	}

	output, err := captureWorkflowOutput(t, func() error { return runDelete("remove", backendTmux) })
	if err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, output, "running teardown steps")
	mustContain(t, output, "removing worktree")
	if _, err := os.Stat(filepath.Join(p.root, ".worktrees", "remove")); !os.IsNotExist(err) {
		t.Fatalf("worktree was not removed: %v", err)
	}
	if branchExists("remove") {
		t.Fatal("branch was not removed")
	}
	mustNotContain(t, readIfExists(t, filepath.Join(p.fakeState, "tmux-sessions")), "remove [project]")
}
