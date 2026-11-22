//go:build windows

package wal

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock   = 0x00000002
	lockfileFailImmediately = 0x00000001
)

// lockFile attempts to acquire an exclusive, non-blocking process lock on f.
func lockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		f.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		1, 0, // Lock 1 byte
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		return fmt.Errorf("%w: %v", ErrDatabaseLocked, err)
	}
	return nil
}

// unlockFile releases the exclusive lock on f.
func unlockFile(f *os.File) error {
	var overlapped syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		f.Fd(),
		0,
		1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r1 == 0 {
		return err
	}
	return nil
}
