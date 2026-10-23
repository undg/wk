package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// acquireLock keeps the file in place: unlinking it could let two processes
// lock different inodes. Closing the descriptor or exiting releases the lock.
func acquireLock(cache string) (*os.File, error) {
	dir := filepath.Join(cache, "lock")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "active.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("cannot acquire metrics lock %s (another run may be active): %w", path, err)
	}
	return file, nil
}
