package main

import "fmt"

func usageCompletion() {
	fmt.Println("usage: wk completion <zsh|bash|fish>")
}

func runCompletion(args []string) error {
	if len(args) != 1 {
		usageCompletion()
		return fmt.Errorf("expected exactly one shell argument")
	}

	switch args[0] {
	case "zsh":
		fmt.Print(zshCompletionScript)
	case "bash":
		fmt.Print(bashCompletionScript)
	case "fish":
		fmt.Print(fishCompletionScript)
	default:
		usageCompletion()
		return fmt.Errorf("unknown shell: %s", args[0])
	}
	return nil
}

// zshCompletionScript is a standalone #compdef function. It shells out to
// `wk ls --porcelain` for dynamic worktree-dir completion so it never drifts
// from the actual listing logic, and degrades to no completions (rather than
// erroring) when run outside a wk project.
const zshCompletionScript = `#compdef wk

_wk() {
  local -a subcommands
  subcommands=(
    'add:create a worktree + branch off base_ref'
    'delete:remove a worktree, its branch, and prune'
    'rm:alias for delete'
    'clean:delete worktrees whose branch PR has merged'
    'ls:show worktrees + tmux session status'
    'init:scaffold a starter .wk.toml'
    'completion:generate shell completion scripts'
    '-h:show usage'
    '--help:show usage'
  )

  if (( CURRENT == 2 )); then
    _describe -t commands 'wk command' subcommands
    return
  fi

  case ${words[2]} in
    add)
      _wk_branches
      ;;
    delete|rm)
      _wk_worktree_dirs
      ;;
    ls)
      _arguments '--porcelain[machine-readable output]'
      ;;
    completion)
      local -a shells
      shells=(zsh bash fish)
      _describe -t shells 'shell' shells
      ;;
  esac
}

_wk_worktree_dirs() {
  local -a lines dirs
  lines=(${(f)"$(wk ls --porcelain 2>/dev/null)"})
  # porcelain is 3-line blocks (branch, dir, session) separated by a blank line.
  local i
  for (( i = 2; i <= $#lines; i += 4 )); do
    dirs+=("${lines[i]}")
  done
  _describe -t worktrees 'worktree directory' dirs
}

_wk_branches() {
  local -a branches
  branches=(${(f)"$(git branch -r --format='%(refname:short)' 2>/dev/null | sed 's#^origin/##')"})
  _describe -t branches 'branch' branches
}

compdef _wk wk
`

const bashCompletionScript = `_wk_completions() {
  local cur prev
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"

  if [[ $COMP_CWORD -eq 1 ]]; then
    COMPREPLY=($(compgen -W "add delete rm clean ls init completion -h --help" -- "$cur"))
    return
  fi

  case "${COMP_WORDS[1]}" in
    delete|rm)
      local dirs
      dirs=$(wk ls --porcelain 2>/dev/null | awk 'NR % 4 == 2')
      COMPREPLY=($(compgen -W "$dirs" -- "$cur"))
      ;;
    add)
      local branches
      branches=$(git branch -r --format='%(refname:short)' 2>/dev/null | sed 's#^origin/##')
      COMPREPLY=($(compgen -W "$branches" -- "$cur"))
      ;;
    ls)
      COMPREPLY=($(compgen -W "--porcelain" -- "$cur"))
      ;;
    completion)
      COMPREPLY=($(compgen -W "zsh bash fish" -- "$cur"))
      ;;
  esac
}
complete -F _wk_completions wk
`

const fishCompletionScript = `function __wk_worktree_dirs
    wk ls --porcelain 2>/dev/null | awk 'NR % 4 == 2'
end

function __wk_branches
    git branch -r --format='%(refname:short)' 2>/dev/null | sed 's#^origin/##'
end

complete -c wk -f
complete -c wk -n '__fish_use_subcommand' -a add -d 'create a worktree + branch off base_ref'
complete -c wk -n '__fish_use_subcommand' -a delete -d 'remove a worktree, its branch, and prune'
complete -c wk -n '__fish_use_subcommand' -a rm -d 'alias for delete'
complete -c wk -n '__fish_use_subcommand' -a clean -d 'delete worktrees whose branch PR has merged'
complete -c wk -n '__fish_use_subcommand' -a ls -d 'show worktrees + tmux session status'
complete -c wk -n '__fish_use_subcommand' -a init -d 'scaffold a starter .wk.toml'
complete -c wk -n '__fish_use_subcommand' -a completion -d 'generate shell completion scripts'
complete -c wk -n '__fish_use_subcommand' -s h -l help -d 'show usage'

complete -c wk -n '__fish_seen_subcommand_from add' -a '(__wk_branches)'
complete -c wk -n '__fish_seen_subcommand_from delete rm' -a '(__wk_worktree_dirs)'
complete -c wk -n '__fish_seen_subcommand_from ls' -l porcelain
complete -c wk -n '__fish_seen_subcommand_from completion' -a 'zsh bash fish'
`
