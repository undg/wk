package main

import (
	"regexp"
	"strings"
)

var (
	reSlashSpace = regexp.MustCompile(`[/\s]+`)
	reInvalid    = regexp.MustCompile(`[^a-z0-9._-]`)
	reDashes     = regexp.MustCompile(`-{2,}`)
)

// SanitizeBranch turns a branch name into a dir-safe slug: lowercase,
// '/'+whitespace -> '-', any other invalid char -> '-', collapse repeated
// dashes, trim leading/trailing dash.
func SanitizeBranch(raw string) string {
	s := strings.ToLower(raw)
	s = reSlashSpace.ReplaceAllString(s, "-")
	s = reInvalid.ReplaceAllString(s, "-")
	s = reDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
