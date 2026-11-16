package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:                   "completion <zsh|bash|fish>",
		Short:                 "Generate a shell completion script",
		Args:                  cobra.ExactArgs(1),
		DisableFlagsInUseLine: true,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return matchingCompletions([]string{"zsh", "bash", "fish"}, toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch args[0] {
			case "zsh":
				return root.GenZshCompletion(out)
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "fish":
				return root.GenFishCompletion(out, true)
			default:
				return fmt.Errorf("unknown shell: %s", args[0])
			}
		},
	}
}

// Completion reads Git directly: loading project config can prompt, change cwd,
// or scaffold .wk.toml, and session discovery is unnecessary for suggestions.
func completeBranches(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out, err := runGit("for-each-ref", "--format=%(refname)\t%(symref)", "refs/heads/", "refs/remotes/origin/")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var candidates []string
	for _, line := range strings.Split(out, "\n") {
		ref, symref, _ := strings.Cut(line, "\t")
		if symref != "" {
			continue
		}
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			candidates = append(candidates, strings.TrimPrefix(ref, "refs/heads/"))
		case strings.HasPrefix(ref, "refs/remotes/origin/"):
			branch := strings.TrimPrefix(ref, "refs/remotes/origin/")
			candidates = append(candidates, branch, "origin/"+branch)
		}
	}
	return matchingCompletions(candidates, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func completeWorktrees(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	entries, err := listWorktrees()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	candidates := []string{"."}
	for _, wt := range entries {
		candidates = append(candidates, wt.Dir)
		if wt.Branch != "" {
			candidates = append(candidates, wt.Branch)
		}
	}
	return matchingCompletions(candidates, toComplete), cobra.ShellCompDirectiveNoFileComp
}

func matchingCompletions(candidates []string, prefix string) []string {
	seen := make(map[string]bool)
	var matches []string
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, prefix) && !seen[candidate] {
			seen[candidate] = true
			matches = append(matches, candidate)
		}
	}
	sort.Strings(matches)
	return matches
}
