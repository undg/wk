package main

import (
	"fmt"
	"os"
)

const (
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;32m"
	colorBlue   = "\033[0;34m"
	colorReset  = "\033[0m"
)

func logInfo(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorBlue, colorReset, fmt.Sprintf(format, a...))
}

func logStep(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorYellow, colorReset, fmt.Sprintf(format, a...))
}

func logOK(format string, a ...any) {
	fmt.Printf("%s[wk]%s %s\n", colorGreen, colorReset, fmt.Sprintf(format, a...))
}

func logError(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%sError:%s %s\n", colorRed, colorReset, fmt.Sprintf(format, a...))
}
