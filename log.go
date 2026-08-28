package main

import (
	"fmt"
	"os"
	"regexp"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;32m"
	colorBlue   = "\033[0;34m"
	colorReset  = "\033[0m"
)

var deleteWordRe = regexp.MustCompile(`(?i)delete[ds]?`)

// highlightDelete wraps occurrences of "delete" (and delete/deletes/deleted)
// in red so it stands out in the middle of otherwise differently-colored log lines.
func highlightDelete(s string) string {
	return deleteWordRe.ReplaceAllString(s, colorRed+"$0"+colorReset)
}

func logInfo(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorBlue, colorReset, highlightDelete(fmt.Sprintf(format, a...)))
}

func logStep(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorYellow, colorReset, highlightDelete(fmt.Sprintf(format, a...)))
}

func logOK(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorGreen, colorReset, highlightDelete(fmt.Sprintf(format, a...)))
}

func logError(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%sError:%s %s\n", colorRed, colorReset, highlightDelete(fmt.Sprintf(format, a...)))
}
