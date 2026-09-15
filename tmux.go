package main

import (
	"os"
	"os/exec"
)

// tmuxBackend adapts the tmux helpers below to sessionBackend. tmux
// addresses sessions by name, so session.Dir only matters when creating one.
type tmuxBackend struct{}

func (tmuxBackend) Kind() string { return backendTmux }

func (tmuxBackend) Has(s session) bool { return tmuxHasSession(s.Name) }

func (tmuxBackend) Create(s session) error { return tmuxNewDetachedSession(s.Name, s.Dir) }

func (tmuxBackend) Kill(s session) error { return tmuxKillSession(s.Name) }

func (tmuxBackend) AttachOrSwitch(s session) error { return tmuxAttachOrSwitch(s.Name) }

func tmuxHasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", "="+name)
	return cmd.Run() == nil
}

func tmuxKillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tmuxNewDetachedSession(name, dir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tmuxAttachOrSwitch(name string) error {
	var cmd *exec.Cmd
	if os.Getenv("TMUX") != "" {
		cmd = exec.Command("tmux", "switch-client", "-t", name)
	} else {
		cmd = exec.Command("tmux", "attach-session", "-t", name)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
