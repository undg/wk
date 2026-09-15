package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const defaultSessionTemplate = "{branch} [{project}]"

// GlobalConfig is ~/.config/wk/config.toml — options that apply to every
// project. Nothing project-specific belongs here.
type GlobalConfig struct {
	DefaultBaseRef  string `toml:"default_base_ref"`
	SessionTemplate string `toml:"session_template"`
	SessionBackend  string `toml:"session_backend"`
}

func defaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		DefaultBaseRef:  "origin/main",
		SessionTemplate: defaultSessionTemplate,
		SessionBackend:  backendHerdr,
	}
}

func loadGlobalConfig() (GlobalConfig, error) {
	cfg := defaultGlobalConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, fmt.Errorf("resolving home directory: %w", err)
	}

	path := filepath.Join(home, ".config", "wk", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading global config %s: %w", path, err)
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("parsing global config %s: %w", path, err)
	}
	return cfg, nil
}

// RepoConfig is .wk.toml — one per project, living at the project root.
type RepoConfig struct {
	Name     string   `toml:"name"`
	BaseRef  string   `toml:"base_ref"`
	Setup    []string `toml:"setup"`
	Teardown []string `toml:"teardown"`
}

const repoConfigFileName = ".wk.toml"

// loadRepoConfig reads .wk.toml from dir. The returned error satisfies
// os.IsNotExist when the file is missing, so callers can distinguish "no
// config here" from an actual parse failure.
func loadRepoConfig(dir string) (RepoConfig, error) {
	var cfg RepoConfig
	path := filepath.Join(dir, repoConfigFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("parsing repo config %s: %w", path, err)
	}
	return cfg, nil
}

// ProjectConfig is the global defaults merged with the current project's
// .wk.toml overrides, plus fields derived from cwd.
type ProjectConfig struct {
	ProjectName     string
	BaseRef         string
	SessionTemplate string
	SessionBackend  string
	Setup           []string
	Teardown        []string
}

// loadProjectConfig detects the project root as cwd (wk is only ever run
// from there, matching the POC's bare-repo assumption; no walk-up) and
// merges global config with .wk.toml.
func loadProjectConfig() (ProjectConfig, error) {
	global, err := loadGlobalConfig()
	if err != nil {
		return ProjectConfig{}, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ProjectConfig{}, fmt.Errorf("resolving working directory: %w", err)
	}

	repo, err := loadRepoConfig(cwd)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectConfig{}, offerInit(cwd)
		}
		return ProjectConfig{}, err
	}

	projectName := repo.Name
	if projectName == "" {
		projectName = filepath.Base(cwd)
	}

	baseRef := repo.BaseRef
	if baseRef == "" {
		baseRef = global.DefaultBaseRef
	}

	sessionTemplate := global.SessionTemplate
	if sessionTemplate == "" {
		sessionTemplate = defaultSessionTemplate
	}

	return ProjectConfig{
		ProjectName:     projectName,
		BaseRef:         baseRef,
		SessionTemplate: sessionTemplate,
		SessionBackend:  global.SessionBackend,
		Setup:           repo.Setup,
		Teardown:        repo.Teardown,
	}, nil
}

// offerInit implements the "no .wk.toml found" prompt from the project
// detection design: default answer is No, since scaffolding in the wrong
// directory is worse than requiring an explicit re-run.
func offerInit(cwd string) error {
	logError("no .wk.toml found in the current directory")
	fmt.Fprint(os.Stderr, "Run 'wk init' here? [y/N] ")

	answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "y" || answer == "yes" {
		return runInit()
	}
	return fmt.Errorf("no .wk.toml found in %s", cwd)
}

// runShellSteps runs each step via `sh -c`, in order, inside dir, stopping
// at the first failure. Used for both .wk.toml's setup and teardown lists.
func runShellSteps(dir string, steps []string) error {
	for _, step := range steps {
		logStep("running: %s", step)
		cmd := exec.Command("sh", "-c", step)
		cmd.Dir = dir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("step failed: %q: %w", step, err)
		}
	}
	return nil
}
