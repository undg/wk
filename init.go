package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const starterRepoConfigTemplate = `name = %q                    # optional; overrides folder name for tmux's {project}
base_ref = "origin/main"     # optional; overrides global default_base_ref

setup = [
  # e.g.:
  # "pnpm i --frozen-lockfile",
  # "ln -s ../AGENTS.md .",
]

teardown = [
  # e.g.:
  # "docker compose down -v",
]
`

// runInit scaffolds a starter .wk.toml in the current directory (assumed to
// be the project root). Refuses to overwrite an existing one.
func runInit() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolving working directory: %w", err)
	}

	path := filepath.Join(cwd, repoConfigFileName)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	content := fmt.Sprintf(starterRepoConfigTemplate, filepath.Base(cwd))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	logOK("scaffolded %s", path)
	return nil
}
