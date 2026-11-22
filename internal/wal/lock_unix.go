//go:build !windows

package wal

import (
	"fmt"
	"os"
	"syscall"
)

// lockFile attempts to acquire an exclusive, non-blocking process lock on f.
func lockFile(f *os.File) error {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseLocked, err)
	}
	return nil
}

// unlockFile releases the exclusive lock on f.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
