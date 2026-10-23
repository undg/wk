package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteListPorcelainPreservesGitValues(t *testing.T) {
	rows := []listRow{
		{branch: "feat/review", dir: "/project/.worktrees/feat-review", session: "[rev] custom label"},
		{branch: "main", dir: "/project"},
		{dir: "/outside/detached"},
	}
	var out bytes.Buffer
	writeListPorcelain(&out, rows)
	mustEqual(t, out.String(), "feat/review\n/project/.worktrees/feat-review\n[rev] custom label\n\nmain\n/project\n\n\n\n/outside/detached\n\n\n")

	out.Reset()
	writeListPorcelain(&out, nil)
	mustEqual(t, out.String(), "")
}

func TestWriteListTablePreservesLayoutAndColors(t *testing.T) {
	for _, backend := range []string{backendHerdr, backendTmux} {
		t.Run(backend, func(t *testing.T) {
			rows := []listRow{
				{branch: "main", dir: "/project", session: "[rev] real label"},
				{branch: "feature-long", dir: "/project/wt"},
				{dir: "/outside"},
			}
			var out bytes.Buffer
			writeListTable(&out, rows, "/project", backend)
			want := colorBlue + "BRANCH        WORKTREE  SESSION" + colorReset + "\n" +
				"main          .         " + colorGreen + backend + ": [rev] real label" + colorReset + "\n" +
				"feature-long  wt        " + colorRed + "no " + backend + " session" + colorReset + "\n" +
				"(detached)    /outside  " + colorRed + "no " + backend + " session" + colorReset + "\n"
			mustEqual(t, out.String(), want)

			// A human rendering must not turn later porcelain output into display values.
			out.Reset()
			writeListPorcelain(&out, rows)
			mustEqual(t, out.String(), "main\n/project\n[rev] real label\n\nfeature-long\n/project/wt\n\n\n\n/outside\n\n\n")
		})
	}
}

func TestWriteListTableExpandsWorktreeColumn(t *testing.T) {
	var out bytes.Buffer
	writeListTable(&out, []listRow{
		{branch: "main", dir: "/project"},
		{branch: "feat/x", dir: "/project/.worktrees/feat-x", session: "custom"},
	}, "/project", backendTmux)
	mustEqual(t, stripANSI(out.String()), "BRANCH  WORKTREE           SESSION\nmain    .                  no tmux session\nfeat/x  .worktrees/feat-x  tmux: custom\n")
}

func TestRunLsDiscoversSessionsWithoutQueryingDetachedWorktrees(t *testing.T) {
	for _, backend := range []string{backendHerdr, backendTmux} {
		t.Run(backend, func(t *testing.T) {
			p := newTestProject(t, "name = \"cave\"\n", false)
			p.writeGlobal("session_template = \"S-{project}-{branch}\"\n")
			enterTestProject(t, p, p.root)
			root := mustRealPath(t, p.root)
			feature := filepath.Join(root, ".worktrees", "feat-review")
			detached := filepath.Join(filepath.Dir(root), "detached")
			testGit(t, root, "worktree", "add", "-b", "feat/review", feature, "HEAD")
			testGit(t, root, "worktree", "add", "--detach", detached, "HEAD")
			label := "S-cave-feat/review"
			if backend == backendTmux {
				mustWriteFile(t, filepath.Join(p.fakeState, "tmux-sessions"), label+"\n")
			} else {
				label = "[rev] renamed workspace"
				// A matching template label at another path is not this worktree's session.
				mustWriteFile(t, filepath.Join(p.fakeState, "herdr-workspaces"),
					"decoy\t/unrelated\tS-cave-main\n"+
						"feature\t"+feature+"\t"+label+"\n"+
						"detached\t"+detached+"\tignored detached label\n")
			}

			out, err := captureWorkflowOutput(t, func() error { return runLs(true, backend) })
			if err != nil {
				t.Fatal(err)
			}
			mustEqual(t, out, "main\n"+root+"\n\n\n\n"+detached+"\n\n\nfeat/review\n"+feature+"\n"+label+"\n\n")
			log := mustReadFile(t, filepath.Join(p.fakeState, backend+".log"))
			if backend == backendHerdr {
				mustEqual(t, log, "|workspace list\n|workspace list\n")
			} else {
				mustEqual(t, log, "has-session -t =S-cave-main\nhas-session -t =S-cave-feat/review\n")
			}

			human, err := captureWorkflowOutput(t, func() error { return runLs(false, backend) })
			if err != nil {
				t.Fatal(err)
			}
			mustContain(t, human, ".worktrees/feat-review")
			mustContain(t, human, backend+": "+label)
			mustContain(t, human, "no "+backend+" session")
			mustContain(t, human, "(detached)")
			mustContain(t, human, detached)
			mustNotContain(t, human, feature)
		})
	}
}

func TestRunLsTreatsAmbiguousHerdrPathsAsMissingSession(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	root := mustRealPath(t, p.root)
	mustWriteFile(t, filepath.Join(p.fakeState, "herdr-workspaces"),
		"one\t"+root+"\tfirst\n"+"two\t"+root+"\tsecond\n")
	out, err := captureWorkflowOutput(t, func() error { return runLs(true, backendHerdr) })
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, out, "main\n"+root+"\n\n\n")
}

func TestRunLsEmptyList(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	// Real repositories always list at least the main/bare entry. Intercept
	// only discovery to exercise the existing empty-list contract.
	bin := t.TempDir()
	mustWriteExecutable(t, filepath.Join(bin, "git"), "#!/bin/sh\n"+
		"if [ \"$1\" = worktree ] && [ \"$2\" = list ]; then exit 0; fi\n"+
		"exec \""+git+"\" \"$@\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, porcelain := range []bool{false, true} {
		out, err := captureWorkflowOutput(t, func() error { return runLs(porcelain, backendTmux) })
		if err != nil {
			t.Fatal(err)
		}
		want := "[wk] no worktrees found\n"
		if porcelain {
			want = ""
		}
		mustEqual(t, out, want)
	}
	mustEqual(t, readIfExists(t, filepath.Join(p.fakeState, "tmux.log")), "")
}

func TestRunLsRejectsInvalidBackendBeforeQueryingSessions(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	out, err := captureWorkflowOutput(t, func() error { return runLs(true, "invalid") })
	if err == nil || !strings.Contains(err.Error(), "unknown session backend: invalid") {
		t.Fatalf("expected invalid backend error, got %v", err)
	}
	mustEqual(t, out, "")
	mustEqual(t, readIfExists(t, filepath.Join(p.fakeState, "herdr.log")), "")
	mustEqual(t, readIfExists(t, filepath.Join(p.fakeState, "tmux.log")), "")
}
