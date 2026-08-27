package main

import (
	"os"
	"os/exec"
	"strings"
)

func tmuxSessionName(template, branch, project string) string {
	name := strings.ReplaceAll(template, "{branch}", branch)
	name = strings.ReplaceAll(name, "{project}", project)
	return name
}

func tmuxHasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", "="+name)
	return cmd.Run() == nil
}

func tmuxCurrentSession() (string, bool) {
	// Without a $TMUX client, `display-message` has nothing to query and
	// silently falls back to the server's most-recently-used session, which
	// would wrongly look like "you're attached to it". Only trust it when
	// we're actually inside a tmux client.
	if os.Getenv("TMUX") == "" {
		return "", false
	}
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
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
