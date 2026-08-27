package main

import (
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

	if err := cleanWorktrees(entries, lister, confirm, deleteFn); err != nil {
		t.Fatalf("cleanWorktrees() error = %v", err)
	}

	if len(confirmed) != 1 {
		t.Errorf("expected exactly one confirm prompt, got %v", confirmed)
	}
	if !reflect.DeepEqual(deleted, []string{"/repo/merged"}) {
		t.Errorf("deleted = %v, want [/repo/merged]", deleted)
	}
}
