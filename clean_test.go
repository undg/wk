package main

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"
)

func TestParseWorktreePorcelain(t *testing.T) {
	input := "worktree /repo/main\n" +
		"HEAD abc123\n" +
		"branch refs/heads/main\n" +
		"\n" +
		"worktree /repo/feat-x\n" +
		"HEAD def456\n" +
		"branch refs/heads/feat/x\n" +
		"\n" +
		"worktree /repo/detached\n" +
		"HEAD ghi789\n" +
		"detached\n"

	got := parseWorktreePorcelain(input)
	want := []worktreeEntry{
		{Dir: "/repo/main", Branch: "main"},
		{Dir: "/repo/feat-x", Branch: "feat/x"},
		{Dir: "/repo/detached", Branch: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseWorktreePorcelain() = %+v, want %+v", got, want)
	}
}

func TestCleanWorktreesSkipsMainCheckoutEvenWhenMerged(t *testing.T) {
	entries := []worktreeEntry{
		{Dir: "/repo", Branch: "feature-not-main"},
		{Dir: "/repo/.worktrees/main", Branch: "main"},
		{Dir: "/repo/detached"},
	}
	states := fakePRLister{
		"/repo":                 "MERGED",
		"/repo/.worktrees/main": "MERGED",
		"/repo/detached":        "MERGED",
	}
	var deleted []string
	err := cleanWorktrees(entries, "/repo", states, func(string) bool { return true }, func(dir string) error {
		deleted = append(deleted, dir)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(deleted, []string{"/repo/.worktrees/main"}) {
		t.Fatalf("deleted %v, want only linked checkout on branch main", deleted)
	}
}

func TestCleanReportsFailedDeletionAndContinues(t *testing.T) {
	entries := []worktreeEntry{{Dir: "/repo/locked", Branch: "locked"}, {Dir: "/repo/next", Branch: "next"}}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	stderr := os.Stderr
	os.Stderr = writer
	var attempted []string
	err = cleanWorktrees(entries, "", fakePRLister{"/repo/locked": "MERGED", "/repo/next": "MERGED"}, func(string) bool { return true }, func(dir string) error {
		attempted = append(attempted, dir)
		if dir == "/repo/locked" {
			return fmt.Errorf("worktree is locked")
		}
		return nil
	})
	os.Stderr = stderr
	writer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempted, []string{"/repo/locked", "/repo/next"}) {
		t.Fatalf("did not continue after failure: %v", attempted)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, string(output), "worktree is locked")
	mustNotContain(t, string(output), "/repo/next")
}

type fakePRLister map[string]string

func (f fakePRLister) PRState(dir string) (string, error) {
	return f[dir], nil
}

func TestCleanWorktrees(t *testing.T) {
	entries := []worktreeEntry{
		{Dir: "/repo/main", Branch: "main"},
		{Dir: "/repo/merged", Branch: "feat/merged"},
		{Dir: "/repo/open", Branch: "feat/open"},
		{Dir: "/repo/detached", Branch: ""},
	}
	lister := fakePRLister{
		"/repo/merged": "MERGED",
		"/repo/open":   "OPEN",
	}

	var confirmed []string
	confirm := func(prompt string) bool {
		confirmed = append(confirmed, prompt)
		return true
	}

	var deleted []string
	deleteFn := func(dir string) error {
		deleted = append(deleted, dir)
		return nil
	}

	if err := cleanWorktrees(entries, "", lister, confirm, deleteFn); err != nil {
		t.Fatalf("cleanWorktrees() error = %v", err)
	}

	if len(confirmed) != 1 {
		t.Errorf("expected exactly one confirm prompt, got %v", confirmed)
	}
	if !reflect.DeepEqual(deleted, []string{"/repo/merged"}) {
		t.Errorf("deleted = %v, want [/repo/merged]", deleted)
	}
}
