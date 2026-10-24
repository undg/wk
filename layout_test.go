package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalCloneAddResumeAndBackendPaths(t *testing.T) {
	p := newTestProject(t, `name = "cave"
setup = ["printf setup >> setup.marker"]
`, false)
	enterTestProject(t, p, p.root)
	root := mustRealPath(t, p.root)
	mustWriteFile(t, filepath.Join(p.root, ".gitignore"), "# keep this")

	// Failed backend creation keeps the worktree and completed setup ready.
	failure := filepath.Join(p.fakeState, "herdr-open-fail")
	mustWriteFile(t, failure, "fail")
	if err := runCreate("feat/login", "rev", backendHerdr); err == nil {
		t.Fatal("backend failure unexpectedly succeeded")
	}
	dir := filepath.Join(root, ".wk", "feat-login")
	mustEqual(t, mustReadFile(t, filepath.Join(dir, "setup.marker")), "setup")
	if err := os.Remove(failure); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "src")
	mustMkdirAll(t, nested)
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	if err := runCreate("feat/login", "rev", backendHerdr); err != nil {
		t.Fatal(err)
	}
	mustEqual(t, mustReadFile(t, filepath.Join(dir, "setup.marker")), "setup")
	mustEqual(t, mustReadFile(t, filepath.Join(root, ".gitignore")), "# keep this\n/.wk/\n")
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "herdr.log")), "|worktree open --cwd "+root+" --path "+dir+" --label [rev] feat/login [cave] --no-focus\n")
	mustEqual(t, gitOutput(t, p, dir, "branch", "--show-current"), "feat/login")
	testGit(t, root, "check-ignore", "-q", ".wk/")

	if err := runCreate("remote-branch", "", backendTmux); err != nil {
		t.Fatal(err)
	}
	remoteDir := filepath.Join(root, ".wk", "remote-branch")
	mustEqual(t, gitOutput(t, p, remoteDir, "rev-parse", "--abbrev-ref", "@{upstream}"), "origin/remote-branch")
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux.log")), "new-session -d -s remote-branch [cave] -c "+remoteDir)
	// A different branch with the same sanitized directory is not adopted.
	if err := runCreate("feat-login", "", backendTmux); err == nil {
		t.Fatal("adopted another branch's worktree")
	}
	mustEqual(t, gitOutput(t, p, dir, "branch", "--show-current"), "feat/login")
}

func TestNormalCloneDeletionTargets(t *testing.T) {
	p := newTestProject(t, `name = "cave"
teardown = ["printf teardown >> ../../teardown.marker"]
`, false)
	enterTestProject(t, p, p.root)
	root := mustRealPath(t, p.root)
	for _, c := range []struct {
		branch, target string
	}{
		{"feat/branch", "feat/branch"},
		{"feat/slug", "feat-slug"},
		{"feat/path", ".wk/feat-path"},
		{"feat/absolute", filepath.Join(root, ".wk", "feat-absolute")},
		{"feat/label", "feat/label [cave]"},
		{"feat/dot", "."},
	} {
		if err := runCreate(c.branch, "", backendTmux); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, ".wk", SanitizeBranch(c.branch))
		if c.target == "." {
			nested := filepath.Join(dir, "src")
			mustMkdirAll(t, nested)
			if err := os.Chdir(nested); err != nil {
				t.Fatal(err)
			}
		}
		if err := runDelete(c.target, backendTmux); err != nil {
			t.Fatalf("delete %s: %v", c.target, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("worktree still exists: %s (%v)", dir, err)
		}
		if branchExists(c.branch) {
			t.Fatalf("branch still exists: %s", c.branch)
		}
	}
	mustEqual(t, mustReadFile(t, filepath.Join(root, "teardown.marker")), "teardownteardownteardownteardownteardownteardown")
}

func TestDeleteProtectsMainCheckoutAndUnregisteredDirectories(t *testing.T) {
	p := newTestProject(t, `name = "cave"
teardown = ["touch teardown-ran"]
`, false)
	enterTestProject(t, p, p.root)
	root := mustRealPath(t, p.root)
	unregistered := filepath.Join(root, "unregistered")
	mustMkdirAll(t, unregistered)
	mustWriteFile(t, filepath.Join(unregistered, "README"), "keep me")
	// An entirely different repository is not one of this project's worktrees.
	foreign := filepath.Join(root, "foreign")
	mustMkdirAll(t, foreign)
	testGit(t, foreign, "init", ".")
	link := filepath.Join(root, "main-alias")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{".", root, "main", "main [cave]", "main-alias", "unregistered", foreign, ".git"} {
		if err := runDelete(target, backendTmux); err == nil {
			t.Fatalf("accepted unsafe target %q", target)
		}
		mustDir(t, root)
		mustEqual(t, gitOutput(t, p, root, "branch", "--show-current"), "main")
		mustEqual(t, mustReadFile(t, filepath.Join(unregistered, "README")), "keep me")
	}
	for _, dir := range []string{root, unregistered, foreign} {
		if _, err := os.Stat(filepath.Join(dir, "teardown-ran")); !os.IsNotExist(err) {
			t.Fatalf("teardown ran for protected target %s: %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(p.fakeState, "tmux.log")); !os.IsNotExist(err) {
		t.Fatalf("unsafe deletion contacted backend: %v", err)
	}
}

func TestDeleteHonorsGitRemovalRefusal(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	if err := runCreate("locked", "", backendTmux); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.root, ".wk", "locked")
	testGit(t, p.root, "worktree", "lock", dir)
	mustWriteFile(t, filepath.Join(dir, "important"), "keep me")
	if err := runDelete("locked", backendTmux); err == nil {
		t.Fatal("deleted locked worktree")
	}
	mustEqual(t, mustReadFile(t, filepath.Join(dir, "important")), "keep me")
	if !branchExists("locked") {
		t.Fatal("deleted locked worktree's branch")
	}
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux-sessions")), "locked [project]")
}

func TestNormalCloneSetupFailureRetriesWithoutRepeatingCompletedSetup(t *testing.T) {
	p := newTestProject(t, `setup = ["test -f ../../allow-setup && printf setup >> setup.marker"]
`, false)
	enterTestProject(t, p, p.root)
	if err := runCreate("retry", "", backendTmux); err == nil {
		t.Fatal("setup failure unexpectedly succeeded")
	}
	dir := filepath.Join(p.root, ".wk", "retry")
	mustDir(t, dir)
	mustEqual(t, readIfExists(t, filepath.Join(p.fakeState, "tmux-sessions")), "")
	mustWriteFile(t, filepath.Join(p.root, "allow-setup"), "")
	for i := 0; i < 2; i++ {
		if err := runCreate("retry", "", backendTmux); err != nil {
			t.Fatal(err)
		}
		mustEqual(t, mustReadFile(t, filepath.Join(dir, "setup.marker")), "setup")
	}
	mustEqual(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux-sessions")), "retry [project]\n")
}

func TestNormalCloneIgnoreFailureDoesNotCreateBranchOrWorktree(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	path := filepath.Join(p.root, ".gitignore")
	mustMkdirAll(t, path)
	if err := runCreate("failure", "", backendTmux); err == nil {
		t.Fatal("created worktree despite unreadable ignore file")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, path, "/.wk/\n!/.wk/\n")
	if err := runCreate("failure", "", backendTmux); err == nil {
		t.Fatal("created worktree despite conflicting ignore rule")
	}
	if branchExists("failure") {
		t.Fatal("created branch before ensuring ignore rule")
	}
	if _, err := os.Stat(filepath.Join(p.root, ".wk")); !os.IsNotExist(err) {
		t.Fatalf("created container despite ignore failure: %v", err)
	}
}

func TestNormalCloneListShowsMainAndNestedPaths(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	if err := runCreate("feat/list", "", backendTmux); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(p.root), "detached")
	testGit(t, p.root, "worktree", "add", "--detach", outside, "origin/main")
	// Small fixture output fits the pipe buffer; read after the command returns.
	capture := func(porcelain bool, backend string) string {
		t.Helper()
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		defer writer.Close()
		stdout := os.Stdout
		os.Stdout = writer
		err = runLs(porcelain, backend)
		os.Stdout = stdout
		writer.Close()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	human := stripANSI(capture(false, backendTmux))
	mustContain(t, human, ".wk/feat-list")
	mustNotContain(t, human, filepath.Join(mustRealPath(t, p.root), ".wk"))
	for _, line := range strings.Split(human, "\n") {
		if strings.Contains(line, mustRealPath(t, outside)) && !strings.HasPrefix(line, "(detached)") {
			t.Fatalf("detached checkout missing its branch marker: %s", line)
		}
	}
	mustContain(t, human, mustRealPath(t, outside))
	mustContain(t, human, "tmux: feat/list [project]")
	mustContain(t, human, "no tmux session")
	out := capture(true, backendTmux)
	root := mustRealPath(t, p.root)
	mustContain(t, out, "main\n"+root+"\n\n")
	mustContain(t, out, "feat/list\n"+filepath.Join(root, ".wk", "feat-list")+"\nfeat/list [project]\n")
	if err := runCreate("feat/list", "rev", backendHerdr); err != nil {
		t.Fatal(err)
	}
	mustContain(t, stripANSI(capture(false, backendHerdr)), "herdr: [rev] feat/list [project]")
}

func TestNormalCloneCleanSkipsMainBeforeQueryingPR(t *testing.T) {
	p := newTestProject(t, "", false)
	enterTestProject(t, p, p.root)
	if err := runCreate("merged", "", backendTmux); err != nil {
		t.Fatal(err)
	}
	p.writeGH("project", "MERGED")
	p.writeGH("merged", "MERGED")
	for _, answer := range []string{"n\n", "y\n"} {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.WriteString(answer); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		stdin := os.Stdin
		os.Stdin = reader
		err = runClean(backendTmux)
		os.Stdin = stdin
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if answer == "n\n" {
			mustDir(t, filepath.Join(p.root, ".wk", "merged"))
		}
	}
	if _, err := os.Stat(filepath.Join(p.root, ".wk", "merged")); !os.IsNotExist(err) {
		t.Fatalf("merged checkout still exists: %v", err)
	}
	root := mustRealPath(t, p.root)
	mustNotContain(t, mustReadFile(t, filepath.Join(p.fakeState, "gh.log")), root+"|pr view")
	mustEqual(t, gitOutput(t, p, root, "branch", "--show-current"), "main")
}

func TestNormalCloneTeardownFailurePreservesWorktreeBranchAndSession(t *testing.T) {
	p := newTestProject(t, `teardown = ["exit 1"]
`, false)
	enterTestProject(t, p, p.root)
	if err := runCreate("keep", "", backendTmux); err != nil {
		t.Fatal(err)
	}
	if err := runDelete("keep", backendTmux); err == nil {
		t.Fatal("ignored teardown failure")
	}
	mustDir(t, filepath.Join(p.root, ".wk", "keep"))
	if !branchExists("keep") {
		t.Fatal("deleted branch after teardown failed")
	}
	mustContain(t, mustReadFile(t, filepath.Join(p.fakeState, "tmux-sessions")), "keep [project]")
}

func TestNormalCloneInitUsesMainRoot(t *testing.T) {
	p := newTestProject(t, "", false)
	linked := filepath.Join(p.root, ".wk", "feature")
	testGit(t, p.root, "worktree", "add", "-b", "feature", linked)
	if err := os.Remove(filepath.Join(p.root, ".wk.toml")); err != nil {
		t.Fatal(err)
	}
	enterTestProject(t, p, linked)
	if err := runInit(); err != nil {
		t.Fatal(err)
	}
	mustContain(t, mustReadFile(t, filepath.Join(p.root, ".wk.toml")), "name = \"project\"")
	if _, err := os.Stat(filepath.Join(linked, ".wk.toml")); !os.IsNotExist(err) {
		t.Fatalf("scaffolded config in linked checkout: %v", err)
	}
	if err := runInit(); err == nil {
		t.Fatal("overwrote owning project config")
	}
}
