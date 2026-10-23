package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func verifyCoveragePaths(profile, module string, selected []string) error {
	data, err := os.ReadFile(profile)
	if err != nil {
		return err
	}
	paths := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		path, _, ok := strings.Cut(line, ":")
		if ok {
			paths[strings.TrimPrefix(path, module+"/")] = true
		}
	}
	for _, path := range selected {
		if !paths[path] {
			return fmt.Errorf("coverage profile has no blocks for %s; source/profile mapping or build constraints do not match", path)
		}
	}
	return nil
}

func verifyMutationReport(path string, selected []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var report struct {
		Module string `json:"go_module"`
		Files  []struct {
			Name string `json:"file_name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	if report.Module == "" {
		return fmt.Errorf("mutation report has no go_module")
	}
	chosen := make(map[string]bool)
	for _, file := range selected {
		chosen[file] = true
	}
	for _, file := range report.Files {
		if !chosen[filepath.ToSlash(filepath.Clean(file.Name))] {
			return fmt.Errorf("mutation report contains unselected source: %s", file.Name)
		}
	}
	return nil
}

func filterCRAP(input, output, work string, selected []string, log io.Writer) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	var report struct {
		Schema  string           `json:"$schema"`
		Version string           `json:"version"`
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return err
	}
	if report.Entries == nil {
		return fmt.Errorf("go-crap report has no entries field")
	}
	chosen := make(map[string]bool)
	for _, path := range selected {
		chosen[path] = true
	}
	entries := make([]map[string]any, 0)
	fmt.Fprintln(log, "CRAP     Complexity  Coverage  Function")
	for _, entry := range report.Entries {
		path, ok := entry["file"].(string)
		if !ok {
			return fmt.Errorf("go-crap entry has no file")
		}
		if filepath.IsAbs(path) {
			path, err = filepath.Rel(work, path)
			if err != nil {
				return err
			}
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path == ".." || strings.HasPrefix(path, "../") {
			return fmt.Errorf("go-crap source path escapes sandbox: %s", path)
		}
		if !chosen[path] {
			continue
		}
		if entry["coverage"] == nil || entry["coverage_warning"] != nil {
			return fmt.Errorf("coverage unavailable for %s: %v", path, entry["coverage_warning"])
		}
		entry["file"] = path
		entries = append(entries, entry)
		fmt.Fprintf(log, "%8.2f %10.0f %8.1f%%  %s:%.0f %s\n", entry["crap"], entry["cyclomatic"], entry["coverage"], path, entry["line"], entry["function"])
	}
	if len(entries) == 0 {
		return fmt.Errorf("no CRAP functions matched the selected source paths; check scope and profile mapping")
	}
	// Do not retain the tool's full-project summary in a scoped report.
	report.Entries = entries
	return writeJSON(output, report)
}
