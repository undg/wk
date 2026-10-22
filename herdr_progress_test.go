package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func enableE2EHerdrProgress(p *e2eProject) {
	p.env = setEnv(p.env, map[string]string{
		"HERDR_ENV":         "1",
		"HERDR_PANE_ID":     "wCaller:p1",
		"HERDR_BIN_PATH":    filepath.Join(filepath.Dir(p.fakeState), "fake-bin", "herdr"),
		"HERDR_SOCKET_PATH": filepath.Join(p.fakeState, "herdr.sock"),
	})
}

func TestE2EHerdrProgressConfig(t *testing.T) {
	for _, tc := range []struct {
		name, global, repo string
		wantProgress       bool
	}{
		{"default enabled", "", "", true},
		{"global opt out", "herdr_progress = false", "", false},
		{"repo opt out", "", "herdr_progress = false", false},
		{"repo overrides global false", "herdr_progress = false", "herdr_progress = true", true},
		{"repo overrides global true", "herdr_progress = true", "herdr_progress = false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newE2EProject(t, tc.repo)
			p.writeGlobal(tc.global)
			enableE2EHerdrProgress(p)
			got := p.run("", "add", "feat/progress")
			if got.err != nil {
				t.Fatalf("add failed: %s", got.output)
			}
			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			if strings.Contains(log, "pane report-agent") != tc.wantProgress {
				t.Fatalf("want progress %v, log:\n%s", tc.wantProgress, log)
			}
			if !tc.wantProgress {
				mustNotContain(t, log, "pane get")
				mustNotContain(t, log, "pane release-agent")
				return
			}
			mustContain(t, log, "pane report-agent wCaller:p1 --source wk --agent wk --state working --seq ")
			mustContain(t, log, "pane release-agent wCaller:p1 --source wk --agent wk --seq ")
			if strings.Count(log, "pane release-agent") != 1 || strings.Index(log, "pane release-agent") > strings.Index(log, "workspace focus") {
				t.Fatalf("must release once before attaching:\n%s", log)
			}
		})
	}
}

func TestE2EHerdrProgressReleasesOnFailure(t *testing.T) {
	for _, operation := range []string{"add", "delete"} {
		t.Run(operation, func(t *testing.T) {
			p := newE2EProject(t, "")
			if operation == "delete" {
				if got := p.run("", "add", "feat/progress"); got.err != nil {
					t.Fatalf("initial add failed: %s", got.output)
				}
			}
			step := "setup"
			if operation == "delete" {
				step = "teardown"
			}
			mustWriteFile(t, filepath.Join(p.root, ".wk.toml"), step+" = ['exit 1']\n")
			enableE2EHerdrProgress(p)
			got := p.run("", operation, "feat/progress")
			if got.err == nil {
				t.Fatal("expected shell step failure")
			}
			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			mustContain(t, log, "pane report-agent")
			if strings.Count(log, "pane release-agent") != 1 {
				t.Fatalf("must release on failure:\n%s", log)
			}
		})
	}
}

func TestE2EHerdrProgressDeleteReleasesBeforeClose(t *testing.T) {
	p := newE2EProject(t, "")
	if got := p.run("", "add", "feat/progress"); got.err != nil {
		t.Fatalf("add failed: %s", got.output)
	}
	enableE2EHerdrProgress(p)
	got := p.run("", "delete", "feat/progress")
	if got.err != nil {
		t.Fatalf("delete failed: %s", got.output)
	}
	log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
	mustContain(t, log, "pane report-agent")
	mustContain(t, log, "workspace close")
	if strings.Count(log, "pane release-agent") != 1 || strings.Index(log, "pane release-agent") > strings.Index(log, "workspace close") {
		t.Fatalf("must release once before closing:\n%s", log)
	}
}

func TestE2EHerdrProgressSkipsUnsafePanesAndIgnoresFailures(t *testing.T) {
	for _, scenario := range []string{"outside herdr", "missing pane", "existing agent", "invalid reply", "report failure"} {
		t.Run(scenario, func(t *testing.T) {
			p := newE2EProject(t, "")
			enableE2EHerdrProgress(p)
			switch scenario {
			case "outside herdr":
				p.env = setEnv(p.env, map[string]string{"HERDR_ENV": ""})
			case "missing pane":
				p.env = setEnv(p.env, map[string]string{"HERDR_PANE_ID": ""})
			case "existing agent":
				p.env = setEnv(p.env, map[string]string{"WK_FAKE_AGENT": "claude"})
			case "invalid reply":
				mustWriteFile(t, filepath.Join(p.fakeState, "herdr-progress-invalid"), "")
			case "report failure":
				mustWriteFile(t, filepath.Join(p.fakeState, "herdr-progress-fail"), "")
			}
			got := p.run("", "add", "feat/progress")
			if got.err != nil {
				t.Fatalf("progress must not break add: %s", got.output)
			}
			mustNotContain(t, got.output, "progress unavailable")
			log := mustReadFile(t, filepath.Join(p.fakeState, "herdr.log"))
			if scenario != "report failure" {
				mustNotContain(t, log, "pane report-agent")
				mustNotContain(t, log, "pane release-agent")
			} else {
				mustContain(t, log, "pane release-agent")
			}
		})
	}
}

func TestHerdrProgressCommandTimeout(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "herdr")
	mustWriteExecutable(t, bin, "#!/bin/sh\nexec sleep 10\n")
	start := time.Now()
	if _, err := runHerdrProgress(bin); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("progress command exceeded its timeout")
	}
}
