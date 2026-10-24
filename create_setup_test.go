package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateRespectsSetupCheckpoints(t *testing.T) {
	for _, tc := range []struct {
		name, checkpoint string
		sessionExists    bool
		noSteps          bool
		wantSetup        bool
	}{
		{name: "pending setup runs", checkpoint: "pending", wantSetup: true},
		{name: "completed setup stays complete", checkpoint: "complete"},
		{name: "legacy setup is not repeated"},
		{name: "unrecognized checkpoint is preserved", checkpoint: "future"},
		{name: "existing session skips pending setup", checkpoint: "pending", sessionExists: true},
		{name: "existing session does not read checkpoint", checkpoint: "directory", sessionExists: true},
		{name: "empty setup leaves pending checkpoint", checkpoint: "pending", noSteps: true},
		{name: "empty setup does not read checkpoint", checkpoint: "directory", noSteps: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := `setup = ["printf setup >> setup.marker"]`
			if tc.noSteps {
				config = "setup = []"
			}
			p := newTestProject(t, config, false)
			enterTestProject(t, p, p.root)
			dir := filepath.Join(p.root, ".wk", "resume")
			testGit(t, p.root, "worktree", "add", "-b", "resume", dir)
			checkpoint, err := setupCheckpointPath(dir)
			if err != nil {
				t.Fatal(err)
			}
			switch tc.checkpoint {
			case "":
			case "directory":
				mustMkdirAll(t, checkpoint)
			default:
				mustWriteFile(t, checkpoint, tc.checkpoint+"\n")
			}
			if tc.sessionExists {
				mustWriteFile(t, filepath.Join(p.fakeState, "tmux-sessions"), "resume [project]\n")
			}

			if err := runCreate("resume", "", backendTmux); err != nil {
				t.Fatal(err)
			}

			marker := filepath.Join(dir, "setup.marker")
			if tc.wantSetup {
				mustEqual(t, mustReadFile(t, marker), "setup")
				mustEqual(t, mustReadFile(t, checkpoint), "complete\n")
			} else {
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("setup unexpectedly ran: %v", err)
				}
				switch tc.checkpoint {
				case "":
					if _, err := os.Stat(checkpoint); !os.IsNotExist(err) {
						t.Fatalf("legacy checkpoint changed: %v", err)
					}
				case "directory":
					mustDir(t, checkpoint)
				default:
					mustEqual(t, mustReadFile(t, checkpoint), tc.checkpoint+"\n")
				}
			}
			log := mustReadFile(t, filepath.Join(p.fakeState, "tmux.log"))
			mustContain(t, log, "attach-session -t resume [project]")
			if tc.sessionExists {
				mustNotContain(t, log, "new-session")
			} else {
				mustContain(t, log, "new-session")
			}
		})
	}
}

func TestCreateSetupFailureRetriesBeforeSessionAndReleasesProgress(t *testing.T) {
	p := newTestProject(t, `setup = ["test -f ../../allow-setup && printf setup >> setup.marker"]`, false)
	enableE2EHerdrProgress(p)
	enterTestProject(t, p, p.root)
	logPath := filepath.Join(p.fakeState, "herdr.log")

	if err := runCreate("retry", "", backendHerdr); err == nil {
		t.Fatal("setup failure unexpectedly succeeded")
	}
	dir := filepath.Join(p.root, ".wk", "retry")
	state, err := readSetupState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state != setupPending {
		t.Fatalf("failed setup checkpoint = %q, want pending", state)
	}
	log := mustReadFile(t, logPath)
	if strings.Count(log, "pane release-agent") != 1 {
		t.Fatalf("failure must release progress once:\n%s", log)
	}
	mustNotContain(t, log, "worktree open")
	mustNotContain(t, log, "workspace focus")

	mustWriteFile(t, filepath.Join(p.root, "allow-setup"), "")
	for attempt := 0; attempt < 2; attempt++ {
		mustWriteFile(t, logPath, "")
		if err := runCreate("retry", "", backendHerdr); err != nil {
			t.Fatal(err)
		}
		mustEqual(t, mustReadFile(t, filepath.Join(dir, "setup.marker")), "setup")
		state, err := readSetupState(dir)
		if err != nil {
			t.Fatal(err)
		}
		if state != setupComplete {
			t.Fatalf("successful setup checkpoint = %q, want complete", state)
		}
		log = mustReadFile(t, logPath)
		release, focus := strings.Index(log, "pane release-agent"), strings.Index(log, "workspace focus")
		if strings.Count(log, "pane release-agent") != 1 || focus < 0 || release > focus {
			t.Fatalf("must release progress once before attaching:\n%s", log)
		}
		if attempt == 0 {
			open := strings.Index(log, "worktree open")
			if open < 0 || open > release {
				t.Fatalf("must create session before releasing progress:\n%s", log)
			}
		} else {
			mustNotContain(t, log, "worktree open")
		}
	}
}

func TestCreateStopsBeforeSessionWhenSetupCheckpointCannotBeRead(t *testing.T) {
	p := newTestProject(t, `setup = ["touch setup.marker"]`, false)
	enterTestProject(t, p, p.root)
	dir := filepath.Join(p.root, ".wk", "resume")
	testGit(t, p.root, "worktree", "add", "-b", "resume", dir)
	checkpoint, err := setupCheckpointPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustMkdirAll(t, checkpoint)

	err = runCreate("resume", "", backendTmux)
	if err == nil {
		t.Fatal("unreadable checkpoint unexpectedly succeeded")
	}
	mustContain(t, err.Error(), "reading setup checkpoint")
	if _, err := os.Stat(filepath.Join(dir, "setup.marker")); !os.IsNotExist(err) {
		t.Fatalf("setup ran despite unreadable checkpoint: %v", err)
	}
	log := mustReadFile(t, filepath.Join(p.fakeState, "tmux.log"))
	mustNotContain(t, log, "new-session")
	mustNotContain(t, log, "attach-session")
}

func TestWorktreeSetupReportsCompletionCheckpointFailure(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	if err := markSetupPending(p.root); err != nil {
		t.Fatal(err)
	}
	// Setup succeeds but leaves its checkpoint unwritable. The failure must
	// reach the caller rather than allowing session creation to proceed.
	err := runWorktreeSetup(p.root, []string{
		`printf setup >> setup.marker`,
		`checkpoint=$(git rev-parse --git-path wk-setup-complete); rm "$checkpoint"; mkdir "$checkpoint"`,
	}, false)
	if err == nil {
		t.Fatal("completion checkpoint failure unexpectedly succeeded")
	}
	mustContain(t, err.Error(), "writing setup checkpoint")
	mustEqual(t, mustReadFile(t, filepath.Join(p.root, "setup.marker")), "setup")
}
