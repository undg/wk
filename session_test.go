package main

import (
	"reflect"
	"testing"
)

func TestExtractBackendFlag(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		override string
		rest     []string
	}{
		{"no flag", []string{"add", "feat/x"}, "", []string{"add", "feat/x"}},
		{"shorthand before subcommand", []string{"--tmux", "add", "feat/x"}, "tmux", []string{"add", "feat/x"}},
		{"shorthand after subcommand", []string{"add", "--herdr", "feat/x"}, "herdr", []string{"add", "feat/x"}},
		{"long form with equals", []string{"--backend=tmux", "ls"}, "tmux", []string{"ls"}},
		{"long form with space consumes value", []string{"--backend", "tmux", "ls"}, "tmux", []string{"ls"}},
		{"keeps other flags", []string{"--tmux", "ls", "--porcelain"}, "tmux", []string{"ls", "--porcelain"}},
		{"last flag wins", []string{"--tmux", "--herdr", "clean"}, "herdr", []string{"clean"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			override, rest, err := extractBackendFlag(c.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if override != c.override {
				t.Errorf("override = %q, want %q", override, c.override)
			}
			if !reflect.DeepEqual(rest, c.rest) {
				t.Errorf("rest = %q, want %q", rest, c.rest)
			}
		})
	}
}

func TestExtractBackendFlagBadValue(t *testing.T) {
	for _, args := range [][]string{
		{"add", "feat/x", "--backend"},
		{"--backend="},
		{"--backend=screen", "ls"},
		{"--backend", "screen", "ls"},
	} {
		if _, _, err := extractBackendFlag(args); err == nil {
			t.Errorf("extractBackendFlag(%q) = nil error, want one", args)
		}
	}
}

func TestNewSessionBackend(t *testing.T) {
	cases := []struct {
		configured string
		override   string
		want       string
	}{
		{"", "", backendHerdr},
		{backendHerdr, "", backendHerdr},
		{backendTmux, "", backendTmux},
		{backendTmux, backendHerdr, backendHerdr},
		{backendHerdr, backendTmux, backendTmux},
	}

	for _, c := range cases {
		backend, err := newSessionBackend(c.configured, c.override)
		if err != nil {
			t.Fatalf("newSessionBackend(%q, %q): %v", c.configured, c.override, err)
		}
		if got := backend.Kind(); got != c.want {
			t.Errorf("newSessionBackend(%q, %q).Kind() = %q, want %q", c.configured, c.override, got, c.want)
		}
	}
}

func TestNewSessionBackendUnknown(t *testing.T) {
	if _, err := newSessionBackend("screen", ""); err == nil {
		t.Error("configured screen: nil error, want one")
	}
	if _, err := newSessionBackend("", "screen"); err == nil {
		t.Error("override screen: nil error, want one")
	}
}
