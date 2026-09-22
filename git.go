package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func runGitInherit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func isBareRepository() bool {
	out, err := runGit("rev-parse", "--is-bare-repository")
	return err == nil && out == "true"
}

func refExists(ref string) bool {
	_, err := runGit("rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func branchExists(branch string) bool {
	_, err := runGit("show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// fetchMissingBaseRef mirrors the POC: if baseRef looks like <remote>/<branch>
// and that remote is configured, fetch it into refs/remotes/<remote>/<branch>.
func fetchMissingBaseRef(baseRef string) {
	if !strings.Contains(baseRef, "/") {
		return
	}
	parts := strings.SplitN(baseRef, "/", 2)
	remote, remoteBranch := parts[0], parts[1]
	if _, err := runGit("remote", "get-url", remote); err != nil {
		return
	}
	logStep("fetching missing base ref: %s", baseRef)
	refspec := fmt.Sprintf("%s:refs/remotes/%s/%s", remoteBranch, remote, remoteBranch)
	_ = runGitInherit("fetch", remote, refspec)
}

// worktreeAddNew creates a branch at base in a new worktree. Tracking is
// configured afterwards: `git worktree add --track -b` rejects the remote
// branch layout produced by `git clone --bare`, which is wk's documented
// repository setup.
func worktreeAddNew(dir, branch, base string) error {
	return runGitInherit("worktree", "add", "-b", branch, dir, base)
}

func worktreeAddExisting(dir, branch string) error {
	return runGitInherit("worktree", "add", dir, branch)
}

func setBranchUpstream(branch, upstream string) error {
	if _, err := runGit("branch", "--set-upstream-to="+upstream, branch); err == nil {
		return nil
	}

	// `git clone --bare` maps a remote's branches straight into local heads,
	// rather than configuring refs/remotes/<remote>/* as normal clones do.
	// Materialize the equivalent remote-tracking ref from the just-created
	// local branch, then configure the usual fetch mapping so Git recognizes
	// it as a valid upstream.
	remote, remoteBranch, ok := strings.Cut(upstream, "/")
	if !ok || remote == "" || remoteBranch == "" {
		return fmt.Errorf("invalid remote upstream: %s", upstream)
	}
	if err := runGitInherit("config", "remote."+remote+".fetch", "+refs/heads/*:refs/remotes/"+remote+"/*"); err != nil {
		return err
	}
	if err := runGitInherit("update-ref", "refs/remotes/"+remote+"/"+remoteBranch, "refs/heads/"+branch); err != nil {
		return err
	}
	return runGitInherit("branch", "--set-upstream-to="+upstream, branch)
}

func worktreeRemove(dir string) error {
	return runGitInherit("worktree", "remove", "--force", dir)
}

func worktreePrune() error {
	return runGitInherit("worktree", "prune")
}

func deleteBranch(branch string) error {
	return runGitInherit("branch", "-D", branch)
}

func branchOfWorktree(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
