package main

import (
	"fmt"
	"strings"
)

const (
	backendHerdr = "herdr"
	backendTmux  = "tmux"
)

// session is wk's unit of "one terminal session per worktree". Both fields
// identify it, because the backends disagree about which one is the handle:
// tmux addresses sessions by name, while herdr tracks the worktree a
// workspace is checked out in and treats the name as a mutable label.
type session struct {
	Name string
	Dir  string
}

// sessionBackend is the terminal half of wk's workflow. wk owns the git side
// itself (see git.go) and only asks the backend to host a session at a
// directory that already exists.
type sessionBackend interface {
	Kind() string
	Has(s session) bool
	IsCurrent(s session) bool
	Create(s session) error
	Kill(s session) error
	AttachOrSwitch(s session) error
}

// sessionNameFromTemplate renders the configured session template into a
// tmux session name or a herdr workspace label. The default template is
// valid as both, so neither backend needs its own rendering.
func sessionNameFromTemplate(template, branch, project string) string {
	name := strings.ReplaceAll(template, "{branch}", branch)
	return strings.ReplaceAll(name, "{project}", project)
}

func validBackendKind(kind string) bool {
	return kind == backendHerdr || kind == backendTmux
}

// newSessionBackend picks the backend for this run. An explicit flag beats
// the configured value, and herdr is the default when neither says anything.
func newSessionBackend(configured, override string) (sessionBackend, error) {
	kind := configured
	if override != "" {
		kind = override
	}

	switch kind {
	case "", backendHerdr:
		return herdrBackend{}, nil
	case backendTmux:
		return tmuxBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown session backend: %s (want %s or %s)", kind, backendHerdr, backendTmux)
	}
}
