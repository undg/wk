package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "wk",
		Short:         "Manage Git worktrees and their development sessions",
		Version:       resolveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("missing argument: use 'wk help' for available commands")
		},
	}
	root.SetVersionTemplate("wk {{.Version}}\n")

	// All backend flags update the same value as they are parsed, preserving
	// last-flag-wins behavior even when different spellings are mixed.
	backendOverride := ""
	root.PersistentFlags().Func("backend", "Session backend (herdr or tmux)", func(value string) error {
		if !validBackendKind(value) {
			return fmt.Errorf("--backend needs herdr or tmux, got %q", value)
		}
		backendOverride = value
		return nil
	})
	for _, kind := range []string{backendHerdr, backendTmux} {
		root.PersistentFlags().BoolFunc(kind, "Use the "+kind+" session backend", func(value string) error {
			enabled, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			if enabled {
				backendOverride = kind
			}
			return nil
		})
	}
	// The flag was just registered, so a registration error is a programming bug.
	if err := root.RegisterFlagCompletionFunc("backend", cobra.FixedCompletions([]string{backendHerdr, backendTmux}, cobra.ShellCompDirectiveNoFileComp)); err != nil {
		panic(err)
	}

	add := &cobra.Command{
		Use:   "add <branch-name|origin/branch-name> [badge]",
		Short: "Create a worktree and open its session",
		Long: "Reuse a local branch, check out an origin branch, or create a new branch.\n" +
			"New branches start at the caller's HEAD (bare repositories: base_ref).\n" +
			"An optional badge changes only the Herdr workspace label.",
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: completeBranches,
		RunE: func(cmd *cobra.Command, args []string) error {
			badge := ""
			if len(args) == 2 {
				badge = args[1]
			}
			return runCreate(args[0], badge, backendOverride)
		},
	}
	deleteCmd := &cobra.Command{
		Use:               "delete <worktree-dir|branch|.>",
		Aliases:           []string{"rm"},
		Short:             "Remove a worktree, its session, and its branch",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeWorktrees,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(args[0], backendOverride)
		},
	}
	clean := &cobra.Command{
		Use:               "clean",
		Short:             "Offer to delete worktrees whose pull requests have merged",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClean(backendOverride)
		},
	}
	porcelain := false
	list := &cobra.Command{
		Use:               "ls",
		Short:             "Show worktrees and session status",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(porcelain, backendOverride)
		},
	}
	list.Flags().BoolVar(&porcelain, "porcelain", false, "Print three-line blocks: branch, absolute path, session")
	initCmd := &cobra.Command{
		Use:               "init",
		Short:             "Scaffold .wk.toml at the owning project root (cwd outside Git)",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit()
		},
	}
	versionCmd := &cobra.Command{
		Use:               "version",
		Short:             "Print the wk version",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "wk %s\n", cmd.Root().Version)
			return err
		},
	}
	root.AddCommand(add, deleteCmd, clean, list, initCmd, versionCmd, newCompletionCommand())
	return root
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		logError("%s", err)
		os.Exit(1)
	}
}
