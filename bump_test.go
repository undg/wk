package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newBumpProject(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	script, err := os.ReadFile(filepath.Join("scripts", "bump"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, "scripts", "bump")
	if err := os.WriteFile(path, script, 0755); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "init", "-q")
	testGit(t, repo, "config", "user.name", "Test")
	testGit(t, repo, "config", "user.email", "test@example.com")
	testGit(t, repo, "config", "tag.gpgSign", "false")
	testGit(t, repo, "commit", "--allow-empty", "-q", "-m", "initial")
	return repo
}

func bumpGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	cmd.Env = setEnv(os.Environ(), map[string]string{
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
	})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), repo, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runBump(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(filepath.Join(repo, "scripts", "bump"), args...)
	cmd.Dir = repo
	cmd.Env = setEnv(os.Environ(), map[string]string{
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0",
	})
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func bumpExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	return exitErr.ExitCode()
}

func TestBumpStartsFromZero(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"patch", "0.0.1"},
		{"minor", "0.1.0"},
		{"major", "1.0.0"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			repo := newBumpProject(t)
			out, err := runBump(t, repo, tc.mode)
			if err != nil {
				t.Fatalf("bump failed: %v\n%s", err, out)
			}
			mustEqual(t, out, "Created tag "+tc.want+"\n")
			mustEqual(t, bumpGit(t, repo, "tag", "--list"), tc.want)
			mustEqual(t, bumpGit(t, repo, "cat-file", "-t", tc.want), "tag")
		})
	}
}

func TestBumpUsesLatestSemverTag(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"patch", "0.0.12"},
		{"minor", "0.1.0"},
		{"major", "1.0.0"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			repo := newBumpProject(t)
			testGit(t, repo, "tag", "-a", "0.0.11", "-m", "previous")
			testGit(t, repo, "commit", "--allow-empty", "-q", "-m", "next")
			out, err := runBump(t, repo, tc.mode)
			if err != nil {
				t.Fatalf("bump failed: %v\n%s", err, out)
			}
			mustContain(t, bumpGit(t, repo, "tag", "--list"), tc.want)
		})
	}
}

func TestBumpUsesHighestMergedSemverTag(t *testing.T) {
	repo := newBumpProject(t)
	testGit(t, repo, "tag", "-a", "0.9.0", "-m", "old")
	testGit(t, repo, "commit", "--allow-empty", "-q", "-m", "newer")
	testGit(t, repo, "tag", "-a", "0.10.0", "-m", "newest")
	testGit(t, repo, "commit", "--allow-empty", "-q", "-m", "next")

	out, err := runBump(t, repo, "patch")
	if err != nil {
		t.Fatalf("bump failed: %v\n%s", err, out)
	}
	mustContain(t, bumpGit(t, repo, "tag", "--list"), "0.10.1")
}

func TestBumpRejectsDirtyTrackedTreeAndAllowsUntrackedFiles(t *testing.T) {
	repo := newBumpProject(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("before\n"), 0644); err != nil {
		t.Fatal(err)
	}
	testGit(t, repo, "add", "tracked")
	testGit(t, repo, "commit", "-q", "-m", "tracked")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("after\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out, err := runBump(t, repo, "patch")
	if err == nil || !strings.Contains(out, "Tracked changes") {
		t.Fatalf("dirty tree bump = %v, output = %s", err, out)
	}
	mustEqual(t, bumpGit(t, repo, "tag", "--list"), "")

	testGit(t, repo, "checkout", "--", "tracked")
	if err := os.WriteFile(filepath.Join(repo, "local.toml"), []byte("local\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out, err = runBump(t, repo, "patch")
	if err != nil {
		t.Fatalf("bump with untracked file failed: %v\n%s", err, out)
	}
	mustEqual(t, bumpGit(t, repo, "tag", "--list"), "0.0.1")
}

func TestBumpRejectsAlreadyTaggedHead(t *testing.T) {
	repo := newBumpProject(t)
	testGit(t, repo, "tag", "-a", "0.1.0", "-m", "release")

	out, err := runBump(t, repo, "patch")
	if err == nil || !strings.Contains(out, "already tagged 0.1.0") {
		t.Fatalf("tagged HEAD bump = %v, output = %s", err, out)
	}
	mustEqual(t, bumpGit(t, repo, "tag", "--list"), "0.1.0")
}

func TestBumpHelpAndInvalidArgumentsDoNotTag(t *testing.T) {
	repo := newBumpProject(t)
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{"-h"}, 0},
		{[]string{"--help"}, 0},
		{nil, 1},
		{[]string{"release"}, 1},
		{[]string{"patch", "minor"}, 1},
		{[]string{"--unknown"}, 1},
	} {
		out, err := runBump(t, repo, tc.args...)
		if got := bumpExitCode(err); got != tc.want {
			t.Fatalf("bump %v exit = %d, want %d\n%s", tc.args, got, tc.want, out)
		}
		mustEqual(t, bumpGit(t, repo, "tag", "--list"), "")
		if tc.want == 0 {
			mustContain(t, out, "Usage:")
		}
	}
}
