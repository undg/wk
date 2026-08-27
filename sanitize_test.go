package main

import "testing"

func TestSanitizeBranch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"feat/my-branch", "feat-my-branch"},
		{"feat/My Branch!!  weird", "feat-my-branch-weird"},
		{"already-good-slug", "already-good-slug"},
		{"---leading-trailing---", "leading-trailing"},
		{"a//b   c", "a-b-c"},
		{"UPPER_CASE.value", "upper_case.value"},
	}
	for _, c := range cases {
		if got := SanitizeBranch(c.in); got != c.want {
			t.Errorf("SanitizeBranch(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTmuxSessionName(t *testing.T) {
	got := tmuxSessionName(defaultTmuxSessionTemplate, "feat/123/add-btn", "pgm-be")
	want := "feat/123/add-btn [pgm-be]"
	if got != want {
		t.Errorf("tmuxSessionName() = %q, want %q", got, want)
	}
}
