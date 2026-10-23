package main

import (
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	for _, c := range []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		want     string
	}{
		{
			name: "release takes precedence over dirty revision", injected: "v1.2.3",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "true"},
			}},
			want: "v1.2.3",
		},
		{name: "release without build info", injected: "v1.2.3", want: "v1.2.3"},
		{name: "no build info", injected: "dev", want: "dev"},
		{name: "no settings", injected: "dev", info: &debug.BuildInfo{}, want: "dev"},
		{
			name: "modified flag without revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}},
			want: "dev",
		},
		{
			name: "unrelated build settings", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOARCH", Value: "arm64"}}},
			want: "dev",
		},
		{
			name: "short revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}}},
			want: "abc123",
		},
		{
			name: "twelve character revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "0123456789ab"}}},
			want: "0123456789ab",
		},
		{
			name: "long clean revision", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef"},
				{Key: "vcs.modified", Value: "false"},
			}},
			want: "0123456789ab",
		},
		{
			name: "dirty revision with settings in reverse order", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.modified", Value: "true"},
				{Key: "vcs.revision", Value: "0123456789abcdef"},
			}},
			want: "0123456789ab-dirty",
		},
		{
			name: "only literal true marks revision dirty", injected: "dev",
			info: &debug.BuildInfo{Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc123"},
				{Key: "vcs.modified", Value: "1"},
			}},
			want: "abc123",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			mustEqual(t, versionFromBuildInfo(c.injected, c.info), c.want)
		})
	}
}

func TestResolveVersionUsesInjectedRelease(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "v1.2.3"

	mustEqual(t, resolveVersion(), "v1.2.3")
}

func TestE2EVersionReportsInjectedRelease(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "wk")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=v1.2.3", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building release binary: %v\n%s", err, output)
	}
	for _, arg := range []string{"version", "-v", "--version"} {
		output, err := exec.Command(binary, arg).CombinedOutput()
		if err != nil {
			t.Fatalf("wk %s: %v\n%s", arg, err, output)
		}
		mustEqual(t, string(output), "wk v1.2.3\n")
	}
}
