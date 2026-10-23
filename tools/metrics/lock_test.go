package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLockRejectsConcurrentRunAndRecoversAfterProcessDeath(t *testing.T) {
	cache := t.TempDir()
	// Reproduce the empty directory left by the old mkdir-based lock.
	if err := os.Mkdir(filepath.Join(cache, "lock"), 0755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestLockHolderProcess$")
	child.Env = append(os.Environ(), "WK_METRICS_TEST_LOCK="+cache)
	child.Stderr = os.Stderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		child.Process.Kill()
		child.Wait()
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("lock holder did not start: %q, %v", line, err)
	}
	if lock, err := acquireLock(cache); err == nil {
		lock.Close()
		t.Fatal("concurrent run acquired the lock")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	lock, err := acquireLock(cache)
	if err != nil {
		t.Fatalf("process death left a stale lock: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err = acquireLock(cache)
	if err != nil {
		t.Fatalf("normal close did not release the lock: %v", err)
	}
	lock.Close()
}

func TestLockHolderProcess(t *testing.T) {
	cache := os.Getenv("WK_METRICS_TEST_LOCK")
	if cache == "" {
		return
	}
	lock, err := acquireLock(cache)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("locked")
	time.Sleep(10 * time.Second)
}
