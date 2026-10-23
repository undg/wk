package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func executable(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewAndEmptyScopeNeedNoMetricTools(t *testing.T) {
	root := fixture(t)
	t.Chdir(root)
	t.Setenv("FILES", "")
	t.Setenv("BASE", "")
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	for _, kind := range []string{"crap", "mutation"} {
		os.Args = []string{"metrics", kind, "-file", "first.go", "-list"}
		if err := run(); err != nil {
			t.Fatal(err)
		}
		os.Args = []string{"metrics", kind, "-base", "HEAD"}
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".reports")); !os.IsNotExist(err) {
		t.Fatal("preview or skip produced reports")
	}
}

func TestMutationScopeDoesNotExcludeSelectedFilesWithSharedBasenames(t *testing.T) {
	selected := []string{"nested/first.go", "folder with space/second.go"}
	args := mutationArgs([]string{"first.go", "nested/first.go", "folder with space/second.go", "file[1].go"}, selected, "report.json", false)
	var excludes []*regexp.Regexp
	for i, arg := range args {
		if arg == "--exclude-files" {
			excludes = append(excludes, regexp.MustCompile(args[i+1]))
		}
	}
	for _, path := range selected {
		for _, re := range excludes {
			if re.MatchString(path) {
				t.Errorf("selected path %s excluded by %s", path, re)
			}
		}
	}
	for _, path := range []string{"first.go", "file[1].go"} {
		matched := false
		for _, re := range excludes {
			matched = matched || re.MatchString(path)
		}
		if !matched {
			t.Errorf("unselected path %s was not excluded", path)
		}
	}
}

func TestRunnerReportsSuccessDryRunAndBaselineFailureWithoutRealMetricTools(t *testing.T) {
	for _, tc := range []struct {
		kind      string
		dry, fail bool
		outcome   string
	}{
		{"crap", false, false, "completed"},
		{"mutation", false, false, "completed"},
		{"mutation", true, false, "dry-run"},
		{"mutation", false, true, "failed"},
	} {
		t.Run(tc.kind+"-"+tc.outcome, func(t *testing.T) {
			root := fixture(t)
			fakeBin := filepath.Join(t.TempDir(), "bin")
			t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("METRICS_TEST_CACHE", t.TempDir())
			if tc.fail {
				t.Setenv("METRICS_TEST_BASELINE_FAIL", "1")
			}
			executable(t, filepath.Join(fakeBin, "go"), `
case "$1" in
  version)
    case "$3" in
      */gremlins) printf '\tmod\tgithub.com/go-gremlins/gremlins\tv0.6.0\tchecksum\n' ;;
      */go-crap) printf '\tmod\tgithub.com/padiazg/go-crap\tv0.5.1\tchecksum\n' ;;
    esac ;;
  env) printf '%s\n' "$METRICS_TEST_CACHE" ;;
  test)
    test "${METRICS_TEST_BASELINE_FAIL:-0}" != 1
    for arg do
      case "$arg" in
        -coverprofile=*) printf '%s\n' 'mode: set' 'example.invalid/fixture/first.go:2.1,2.30 1 1' > "${arg#-coverprofile=}" ;;
      esac
    done ;;
  list) printf '%s\n' 'example.invalid/fixture' ;;
  tool) exit 0 ;;
  *) exit 99 ;;
esac
`)
			executable(t, filepath.Join(root, "bin/metrics/go-crap"), `
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output ]; then
    printf '%s\n' '{"entries":[{"file":"first.go","function":"First","line":2,"cyclomatic":1,"coverage":100,"crap":1}]}' > "$2"
    exit 0
  fi
  shift
done
exit 99
`)
			executable(t, filepath.Join(root, "bin/metrics/gremlins"), `
# Reject known broken CPU flag and require conservative/full-suite settings.
case " $* " in *' --test-cpu=0 '*) ;; *) exit 99 ;; esac
case " $* " in *' --workers=1 '*) ;; *) exit 99 ;; esac
case " $* " in *' --integration '*) ;; *) exit 99 ;; esac
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output ]; then
    printf '%s\n' '{"go_module":"example.invalid/fixture","files":[{"file_name":"first.go","mutations":[]}]}' > "$2"
    exit 0
  fi
  shift
done
exit 99
`)
			err := execute(context.Background(), root, tc.kind, []string{"first.go"}, "", "", tc.dry)
			if (err != nil) != tc.fail {
				t.Fatalf("run error = %v, want failure %v", err, tc.fail)
			}
			paths, err := filepath.Glob(filepath.Join(root, ".reports", tc.kind, "*.meta.json"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("metadata paths: %v, %v", paths, err)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			var metadata map[string]any
			if err := json.Unmarshal(data, &metadata); err != nil {
				t.Fatal(err)
			}
			if metadata["outcome"] != tc.outcome {
				t.Fatalf("metadata: %s", data)
			}
			if metadata["head"] == "" || metadata["tool"] == "" || metadata["files"] == nil {
				t.Fatalf("missing provenance: %s", data)
			}
			stem := strings.TrimSuffix(paths[0], ".meta.json")
			_, err = os.Stat(stem + ".json")
			if tc.fail && !os.IsNotExist(err) {
				t.Fatal("baseline failure published a metric report")
			}
			if !tc.fail && err != nil {
				t.Fatal(err)
			}
		})
	}
}
