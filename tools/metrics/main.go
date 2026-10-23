// Command metrics runs optional development metrics. It is not part of wk.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

const (
	gremlinsVersion = "v0.6.0"
	crapVersion     = "v0.5.1"
)

type fileFlags []string

func (f *fileFlags) String() string     { return strings.Join(*f, ", ") }
func (f *fileFlags) Set(s string) error { *f = append(*f, s); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "metrics:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || (os.Args[1] != "crap" && os.Args[1] != "mutation") {
		return fmt.Errorf("usage: go run ./tools/metrics {crap|mutation} [-file exact.go ... | -base ref | -all] [-list]")
	}
	kind := os.Args[1]
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	var files fileFlags
	flags.Var(&files, "file", "exact source path (repeatable; overrides BASE)")
	base := flags.String("base", os.Getenv("BASE"), "explicit feature base ref")
	all := flags.Bool("all", false, "explicit full-project scope")
	list := flags.Bool("list", os.Getenv("LIST") == "1", "preview scope without metric tools")
	dry := flags.Bool("dry-run", os.Getenv("DRY_RUN") == "1", "discover mutants only; not a mutation score")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if len(files) == 0 {
		parsed, err := parseFiles(os.Getenv("FILES"))
		if err != nil {
			return err
		}
		files = parsed
	}
	root, err := git(".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	selected, mergeBase, err := selectFiles(root, files, *base, *all)
	if err != nil {
		return err
	}
	fmt.Println("Selected source files:")
	for _, path := range selected {
		fmt.Println("  " + path)
	}
	if len(selected) == 0 {
		fmt.Println("SKIP: no eligible implementation files; no reports produced.")
		return nil
	}
	if *list {
		return nil
	}
	if len(files) > 0 || *all {
		*base = ""
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	return execute(ctx, root, kind, selected, *base, mergeBase, *dry)
}

func verifyTool(path, module, version string) error {
	// Go's binary build info is authoritative; source builds often print 0.0.0.
	out, err := exec.Command("go", "version", "-m", path).Output()
	if err != nil || !strings.Contains(string(out), "\tmod\t"+module+"\t"+version+"\t") {
		return fmt.Errorf("missing or wrong tool %s; run make tools", path)
	}
	return nil
}

func command(ctx context.Context, dir string, env []string, output io.Writer, name string, args ...string) error {
	fmt.Fprintf(output, "+ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = output, output
	// Kill the test process group too when interrupted or the overall budget expires.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	return cmd.Run()
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func execute(ctx context.Context, root, kind string, selected []string, base, mergeBase string, dry bool) (result error) {
	toolName, module, version := "go-crap", "github.com/padiazg/go-crap", crapVersion
	if kind == "mutation" {
		toolName, module, version = "gremlins", "github.com/go-gremlins/gremlins", gremlinsVersion
	}
	tool := filepath.Join(root, "bin", "metrics", runtime.GOOS+"-"+runtime.GOARCH, toolName)
	if _, err := os.Stat(tool); os.IsNotExist(err) {
		// Keep tools installed by the original layout usable on their native OS.
		tool = filepath.Join(root, "bin", "metrics", toolName)
	}
	if err := verifyTool(tool, module, version); err != nil {
		return err
	}
	cache := filepath.Join(root, ".cache", "metrics")
	if err := os.MkdirAll(cache, 0755); err != nil {
		return err
	}
	lock, err := acquireLock(cache)
	if err != nil {
		return err
	}
	defer lock.Close()
	head, err := git(root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}
	status, err := git(root, "status", "--porcelain")
	if err != nil {
		return err
	}
	reports := filepath.Join(root, ".reports", kind)
	if err := os.MkdirAll(reports, 0755); err != nil {
		return err
	}
	// O_EXCL reserves one unique stem across all report formats, including same-minute runs.
	stem := filepath.Join(reports, time.Now().UTC().Format("2006-01-02_15-04")+"_"+head)
	for n := 0; ; n++ {
		candidate := stem
		if n > 0 {
			candidate += fmt.Sprintf("_%d", n)
		}
		f, err := os.OpenFile(candidate+".meta.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		f.Close()
		stem = candidate
		break
	}
	metadata := map[string]any{
		"kind": kind, "files": selected, "base": base, "merge_base": mergeBase,
		"head": head, "dirty": status != "", "git_status": status,
		"tool": module + "@" + version, "started_at": time.Now().UTC(), "outcome": "running",
		"dry_run": dry && kind == "mutation", "go_flags": os.Getenv("GOFLAGS"),
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		metadata["go_version"] = info.GoVersion
	}
	if err := writeJSON(stem+".meta.json", metadata); err != nil {
		return err
	}
	defer func() {
		metadata["finished_at"] = time.Now().UTC()
		metadata["outcome"] = "completed"
		if dry && kind == "mutation" {
			metadata["outcome"] = "dry-run"
		}
		if result != nil {
			metadata["outcome"], metadata["error"] = "failed", result.Error()
		}
		if ctx.Err() != nil {
			metadata["outcome"] = "interrupted"
		}
		if err := writeJSON(stem+".meta.json", metadata); result == nil {
			result = err
		}
		fmt.Println("Report stem:", stem)
	}()
	log, err := os.Create(stem + ".log")
	if err != nil {
		return err
	}
	defer log.Close()
	output := io.MultiWriter(os.Stdout, log)
	scratch, err := os.MkdirTemp(cache, "run-")
	if err != nil {
		return err
	}
	work := filepath.Join(scratch, "source")
	if err := os.Mkdir(work, 0755); err != nil {
		return err
	}
	// Scratch copies stay outside reports. Retained for diagnosis; never copied recursively.
	metadata["workdir"] = work
	if err := copySources(root, work, selected...); err != nil {
		return err
	}
	// Tests deliberately search parent directories for .wk.toml. Keeping their
	// temporary state under the checkout would expose the caller's project config.
	testTmp, err := os.MkdirTemp("", "wk-metrics-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(testTmp)
	env, err := isolatedEnv(work, testTmp)
	if err != nil {
		return err
	}
	if kind == "crap" {
		profile := stem + ".coverage.out"
		if err := command(ctx, work, env, output, "go", "test", "-count=1", "-timeout=2m", "-coverprofile="+profile, "./..."); err != nil {
			return fmt.Errorf("coverage baseline: %w", err)
		}
		moduleCmd := exec.CommandContext(ctx, "go", "list", "-m")
		moduleCmd.Dir, moduleCmd.Env = work, env
		modulePath, err := moduleCmd.Output()
		if err != nil {
			return err
		}
		if err := verifyCoveragePaths(profile, strings.TrimSpace(string(modulePath)), selected); err != nil {
			return err
		}
		if err := command(ctx, work, env, output, "go", "tool", "cover", "-html="+profile, "-o", stem+".coverage.html"); err != nil {
			return err
		}
		raw := filepath.Join(work, "crap.json")
		if err := command(ctx, work, env, output, tool, "scan", work, "--coverage-profile", profile, "--format", "json", "--no-progress", "--output", raw); err != nil {
			return err
		}
		return filterCRAP(raw, stem+".json", work, selected, output)
	}
	// This copied baseline catches missing fixtures before they become false mutant kills.
	if err := command(ctx, work, env, output, "go", "test", "-count=1", "-timeout=2m", "./..."); err != nil {
		return fmt.Errorf("copied baseline: %w", err)
	}
	allFiles, err := repositoryFiles(root)
	if err != nil {
		return err
	}
	args := mutationArgs(allFiles, selected, stem+".json", dry)
	if err := command(ctx, work, env, output, tool, args...); err != nil {
		return err
	}
	return verifyMutationReport(stem+".json", selected)
}

func mutationArgs(files, selected []string, output string, dry bool) []string {
	// v0.6.0 misformats nonzero test-cpu as one argv element. Keep it disabled.
	args := []string{"unleash", "--integration", "--workers=1", "--test-cpu=0", "--timeout-coefficient=3", "--threshold-efficacy=0", "--threshold-mcover=0", "--output", output}
	if dry {
		args = append(args, "--dry-run")
	}
	chosen := make(map[string]bool)
	for _, path := range selected {
		chosen[path] = true
	}
	for _, path := range files {
		if strings.HasSuffix(path, ".go") && !chosen[path] {
			// Gremlins matches module-relative paths: anchor both ends, not suffixes.
			args = append(args, "--exclude-files", `^`+regexp.QuoteMeta(path)+`$`)
		}
	}
	return args
}

func copySources(root, work string, explicit ...string) error {
	files, err := repositoryFiles(root)
	if err != nil {
		return err
	}
	files = append(files, explicit...)
	for _, path := range files {
		// The current suite creates its own fixtures. Do not copy credentials, config,
		// .git, tool binaries, caches, reports, or unrelated workspace content.
		if path != "go.mod" && path != "go.sum" && !strings.HasSuffix(path, ".go") {
			continue
		}
		if strings.HasPrefix(path, ".") || strings.Contains(path, "/.") || strings.HasPrefix(path, "vendor/") || strings.HasPrefix(path, "bin/") {
			continue
		}
		src := filepath.Join(root, path)
		info, err := os.Lstat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		dst := filepath.Join(work, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func isolatedEnv(work, testTmp string) ([]string, error) {
	work = filepath.Dir(work)
	// Preserve Go's build/module caches when moving HOME, not user daemon context.
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "HERDR_") || strings.HasPrefix(key, "TMUX") || strings.HasPrefix(key, "GREMLINS_") || strings.HasPrefix(key, "GIT_") || key == "HOME" || strings.HasPrefix(key, "XDG_") || key == "TMPDIR" || key == "GOTMPDIR" || key == "GOFLAGS" || key == "GOWORK" {
			continue
		}
		env = append(env, item)
	}
	for _, key := range []string{"GOCACHE", "GOPATH"} {
		out, err := exec.Command("go", "env", key).Output()
		if err != nil {
			return nil, err
		}
		env = append(env, key+"="+strings.TrimSpace(string(out)))
	}
	if err := os.MkdirAll(filepath.Join(work, "home"), 0755); err != nil {
		return nil, err
	}
	return append(env,
		"HOME="+filepath.Join(work, "home"), "XDG_CONFIG_HOME="+filepath.Join(work, "home", ".config"),
		"TMPDIR="+testTmp, "GOTMPDIR="+testTmp, "GOWORK=off", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CEILING_DIRECTORIES="+work+string(os.PathListSeparator)+testTmp,
		"GIT_TERMINAL_PROMPT=0", "GOFLAGS="+strings.TrimSpace(os.Getenv("GOFLAGS")+" -count=1 -timeout=2m -buildvcs=false")), nil
}
