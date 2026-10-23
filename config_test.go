package main

import (
	"os"
	"path/filepath"
	"testing"
)

func enterTestProject(t *testing.T, p *e2eProject, dir string) {
	t.Helper()
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "PATH", "WK_FAKE_STATE", "HERDR_ENV", "HERDR_PANE_ID", "HERDR_BIN_PATH", "HERDR_SOCKET_PATH", "GIT_CONFIG_NOSYSTEM", "GIT_TERMINAL_PROMPT"} {
		for _, kv := range p.env {
			if len(kv) > len(key) && kv[:len(key)+1] == key+"=" {
				t.Setenv(key, kv[len(key)+1:])
				break
			}
		}
	}
	t.Setenv("TMUX", "")
	t.Setenv("HERDR_WORKSPACE_ID", "")
	t.Chdir(dir)
}

func TestProjectConfigMergesGlobalDefaultsAndLocalOverrides(t *testing.T) {
	p := newTestProject(t, `name = "owner"
herdr_progress = true
setup = ["echo setup"]
teardown = ["echo teardown"]
`, false)
	p.writeGlobal(`default_base_ref = "origin/release"
session_template = ""
session_backend = "tmux"
herdr_progress = false
`)
	enterTestProject(t, p, p.root)
	cfg, err := loadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, cfg.ProjectName, "owner")
	mustEqual(t, cfg.BaseRef, "origin/release")
	mustEqual(t, cfg.SessionTemplate, defaultSessionTemplate)
	mustEqual(t, cfg.SessionBackend, backendTmux)
	if !cfg.HerdrProgress {
		t.Fatal("local progress setting did not override global")
	}
	if len(cfg.Setup) != 1 || cfg.Setup[0] != "echo setup" || len(cfg.Teardown) != 1 || cfg.Teardown[0] != "echo teardown" {
		t.Fatalf("lost project shell steps: %+v", cfg)
	}
	p.writeGlobal("invalid [")
	if _, err := loadProjectConfig(); err == nil {
		t.Fatal("accepted malformed global config")
	}
	p.writeGlobal("")
	mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), "invalid [")
	if _, err := loadProjectConfig(); err == nil {
		t.Fatal("accepted malformed project config")
	}
}

func TestProjectConfigInitPromptResolvesMainRoot(t *testing.T) {
	p := newTestProject(t, "", false)
	linked := filepath.Join(p.root, ".worktrees", "feature")
	testGit(t, p.root, "worktree", "add", "-b", "feature", linked)
	if err := os.Remove(filepath.Join(p.root, ".wk.toml")); err != nil {
		t.Fatal(err)
	}
	enterTestProject(t, p, linked)
	for _, answer := range []string{"n\n", "yes\n"} {
		if err := os.Chdir(linked); err != nil {
			t.Fatal(err)
		}
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.WriteString(answer); err != nil {
			t.Fatal(err)
		}
		writer.Close()
		stdin := os.Stdin
		os.Stdin = reader
		cfg, err := loadProjectConfig()
		os.Stdin = stdin
		reader.Close()
		if answer == "n\n" {
			if err == nil {
				t.Fatal("continued without config after declining init")
			}
			if _, err := os.Stat(filepath.Join(p.root, ".wk.toml")); !os.IsNotExist(err) {
				t.Fatalf("declining init wrote a config: %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			mustEqual(t, cfg.Repository.Root, mustRealPath(t, p.root))
			mustEqual(t, cfg.ProjectName, "project")
		}
	}
	if _, err := os.Stat(filepath.Join(linked, ".wk.toml")); !os.IsNotExist(err) {
		t.Fatalf("wrote config into linked checkout: %v", err)
	}
}

func TestProjectConfigUsesOwningRootNotTrackedCheckoutConfig(t *testing.T) {
	p := newTestProject(t, "name = \"owner\"\nbase_ref = \"origin/release\"\n", false)
	// A tracked copy exists in every linked checkout. Local edits in the
	// main checkout remain authoritative even when invoked from those copies.
	testGit(t, p.root, "config", "user.name", "wk test")
	testGit(t, p.root, "config", "user.email", "wk@example.invalid")
	testGit(t, p.root, "add", ".wk.toml")
	testGit(t, p.root, "commit", "-m", "project config")
	linked := filepath.Join(p.root, ".worktrees", "feature")
	testGit(t, p.root, "worktree", "add", "-b", "feature", linked)
	mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), "name = \"edited-owner\"\nbase_ref = \"origin/main\"\n")
	mustWriteFile(t, filepath.Join(linked, ".wk.toml"), "not valid toml[")
	nested := filepath.Join(linked, "src")
	mustMkdirAll(t, nested)
	enterTestProject(t, p, nested)

	cfg, err := loadProjectConfig()
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, cfg.ProjectName, "edited-owner")
	mustEqual(t, cfg.BaseRef, "origin/main")
	mustEqual(t, cfg.Repository.Root, mustRealPath(t, p.root))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	mustEqual(t, cwd, mustRealPath(t, p.root))
}
