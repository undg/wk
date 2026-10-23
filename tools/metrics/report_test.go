package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCRAPReportKeepsOnlyExactSelectedFilesAndTheirMetrics(t *testing.T) {
	root := t.TempDir()
	input, output := filepath.Join(root, "raw.json"), filepath.Join(root, "scoped.json")
	if err := writeJSON(input, map[string]any{
		"version": "1.1.0", "summary": map[string]any{"total_funcs": 3},
		"entries": []map[string]any{
			{"file": filepath.Join(root, "first.go"), "function": "First", "line": 12, "cyclomatic": 3, "coverage": 50, "crap": 4.125},
			{"file": "./folder with space/second.go", "function": "Second", "line": 5, "cyclomatic": 1, "coverage": 100, "crap": 1},
			{"file": "not-first.go", "function": "Other", "line": 1, "cyclomatic": 40, "coverage": 0, "crap": 1640},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := filterCRAP(input, output, root, []string{"first.go", "folder with space/second.go"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report["summary"] != nil {
		t.Fatal("retained full-project summary")
	}
	entries := report["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %v", entries)
	}
	first := entries[0].(map[string]any)
	if first["file"] != "first.go" || first["line"] != float64(12) || first["cyclomatic"] != float64(3) || first["coverage"] != float64(50) || first["crap"] != 4.125 {
		t.Fatalf("lost or changed metrics: %v", first)
	}
}

func TestCoverageAndMutationReportsRejectScopeMismatch(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "coverage.out")
	put(t, root, "coverage.out", "mode: set\nexample.invalid/fixture/first.go:2.1,2.20 1 0\n")
	if err := verifyCoveragePaths(profile, "example.invalid/fixture", []string{"first.go"}); err != nil {
		t.Fatal(err)
	}
	if err := verifyCoveragePaths(profile, "wrong/module", []string{"first.go"}); err == nil {
		t.Fatal("accepted mismatched coverage paths")
	}
	mutation := filepath.Join(root, "mutation.json")
	put(t, root, "mutation.json", `{"go_module":"example.invalid/fixture","files":[{"file_name":"other.go"}]}`)
	if err := verifyMutationReport(mutation, []string{"first.go"}); err == nil {
		t.Fatal("accepted out-of-scope mutations")
	}
	if err := verifyMutationReport(filepath.Join(root, "missing.json"), []string{"first.go"}); err == nil {
		t.Fatal("accepted missing mutation report")
	}
}

func TestCRAPRejectsUnavailableCoverage(t *testing.T) {
	root := t.TempDir()
	input, output := filepath.Join(root, "raw.json"), filepath.Join(root, "scoped.json")
	if err := writeJSON(input, map[string]any{"entries": []map[string]any{{"file": "first.go", "coverage_warning": "profile mismatch"}}}); err != nil {
		t.Fatal(err)
	}
	if err := filterCRAP(input, output, root, []string{"first.go"}, io.Discard); err == nil {
		t.Fatal("missing coverage treated as valid")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("published invalid report")
	}
}
