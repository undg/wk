package main

import "testing"

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
