package main

import (
	"fmt"
	"os"
)

func usage(w *os.File) {
	fmt.Fprintf(w, "wk\n\n")
	fmt.Fprintf(w, "USAGE\n")
	fmt.Fprintf(w, "  wk add <branch-name|origin/branch-name>\n")
	fmt.Fprintf(w, "  wk delete <worktree-dir>\n")
	fmt.Fprintf(w, "  wk rm <worktree-dir>\n")
	fmt.Fprintf(w, "  wk clean\n")
	fmt.Fprintf(w, "  wk ls [--porcelain]\n")
	fmt.Fprintf(w, "  wk init\n")
	fmt.Fprintf(w, "  wk completion <zsh|bash|fish>\n")
	fmt.Fprintf(w, "  wk help|-h|--help\n\n")
	fmt.Fprintf(w, "BEHAVIOR\n")
	fmt.Fprintf(w, "  add branch-name          creates <sanitized-branch> from origin/main\n")
	fmt.Fprintf(w, "  add origin/branch-name   creates local branch-name from origin/branch-name\n")
	fmt.Fprintf(w, "  delete/rm <dir>          removes worktree, deletes its branch, prunes\n")
	fmt.Fprintf(w, "  clean                    delete worktrees whose branch PR has merged\n")
	fmt.Fprintf(w, "  ls [--porcelain]         show worktrees + tmux session status\n")
	fmt.Fprintf(w, "                           --porcelain: parse-friendly 3-line blocks (branch, dir, session)\n")
	fmt.Fprintf(w, "  init                     scaffold a starter .wk.toml in the current directory\n")
	fmt.Fprintf(w, "  completion <shell>       print a completion script for zsh, bash, or fish\n\n")
	fmt.Fprintf(w, "EXAMPLES\n")
	fmt.Fprintf(w, "  wk add feat/my-branch\n")
	fmt.Fprintf(w, "  wk add origin/someone-branch\n")
	fmt.Fprintf(w, "  wk delete feat-my-branch\n")
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		logError("missing argument")
		usage(os.Stderr)
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return
	case "add":
		if len(args) != 2 {
			logError("expected exactly one branch argument")
			usage(os.Stderr)
			os.Exit(1)
		}
		err = runCreate(args[1])
	case "delete", "rm":
		if len(args) != 2 {
			logError("expected exactly one worktree-dir argument")
			usage(os.Stderr)
			os.Exit(1)
		}
		err = runDelete(args[1])
	case "clean":
		if len(args) != 1 {
			logError("clean takes no arguments")
			os.Exit(1)
		}
		err = runClean()
	case "ls":
		porcelain := false
		switch len(args) {
		case 1:
		case 2:
			if args[1] != "--porcelain" {
				logError("unknown flag for ls: %s", args[1])
				usage(os.Stderr)
				os.Exit(1)
			}
			porcelain = true
		default:
			logError("ls takes at most one flag (--porcelain)")
			usage(os.Stderr)
			os.Exit(1)
		}
		err = runLs(porcelain)
	case "init":
		if len(args) != 1 {
			logError("init takes no arguments")
			os.Exit(1)
		}
		err = runInit()
	case "completion":
		err = runCompletion(args[1:])
	default:
		// No implicit create: a bare branch name here is either a typo of a
		// subcommand or a stray argument, not a create request.
		logError("unknown command: %s (did you mean 'wk add %s'?)", args[0], args[0])
		usage(os.Stderr)
		os.Exit(1)
	}

	if err != nil {
		logError("%s", err.Error())
		os.Exit(1)
	}
}
