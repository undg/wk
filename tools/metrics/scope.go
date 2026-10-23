package main

import (
	"encoding/csv"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func git(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// FILES is a space-separated list with optional double-quoted paths, not shell code.
func parseFiles(value string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(value))
	r.Comma = ' '
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	var files []string
	for {
		record, err := r.Read()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("FILES: %w", err)
		}
		for _, file := range record {
			if file == "" {
				return nil, fmt.Errorf("FILES contains an empty path")
			}
			files = append(files, file)
		}
	}
}

func eligible(root, path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") || part == "vendor" || part == "testdata" || part == "bin" || part == "node_modules" || part == "dist" || part == "build" {
			return false
		}
	}
	// Symlinks are not copied into metric sandboxes, even when their target is local.
	info, err := os.Lstat(filepath.Join(root, path))
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, path), nil, parser.ParseComments|parser.PackageClauseOnly)
	return err == nil && !ast.IsGenerated(f)
}

func exactPath(root, path string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// Canonicalize parent directories, including macOS's /var -> /private/var
	// alias, but leave the source file itself unresolved so symlinks stay rejected.
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("not an eligible implementation file: %s", path)
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside repository: %s", path)
	}
	if !eligible(root, rel) {
		return "", fmt.Errorf("not an eligible implementation file: %s", path)
	}
	return filepath.ToSlash(rel), nil
}

func repositoryFiles(root string) ([]string, error) {
	out, err := git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return strings.Split(out, "\x00"), nil
}

func selectFiles(root string, files []string, base string, all bool) ([]string, string, error) {
	selected := make(map[string]bool)
	mergeBase := ""
	switch {
	case len(files) > 0:
		for _, file := range files {
			rel, err := exactPath(root, file)
			if err != nil {
				return nil, "", err
			}
			selected[rel] = true
		}
	case all || base != "":
		var candidates []string
		var err error
		if all {
			candidates, err = repositoryFiles(root)
		} else {
			mergeBase, err = git(root, "merge-base", "HEAD", base)
			if err == nil {
				var changed, untracked string
				changed, err = git(root, "diff", "--name-only", "-z", "--diff-filter=ACMRT", "--find-renames", mergeBase, "--")
				if err == nil {
					untracked, err = git(root, "ls-files", "--others", "--exclude-standard", "-z")
				}
				candidates = strings.Split(changed+"\x00"+untracked, "\x00")
			}
		}
		if err != nil {
			return nil, "", err
		}
		for _, path := range candidates {
			if eligible(root, path) {
				selected[path] = true
			}
		}
	default:
		return nil, "", fmt.Errorf("select FILES, BASE, or the explicit -all target")
	}
	result := make([]string, 0, len(selected))
	for file := range selected {
		result = append(result, file)
	}
	sort.Strings(result)
	return result, mergeBase, nil
}
