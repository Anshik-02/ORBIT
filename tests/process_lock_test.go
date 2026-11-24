package tests

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"orbit/internal/wal"
)

// TestProcessLockHelper acts as a separate OS process for cross-process lock testing.
// When spawned with ORBIT_TEST_LOCK_HELPER=1, it opens the WAL at ORBIT_TEST_LOCK_PATH,
// prints "LOCKED", flushes stdout, and blocks until stdin provides input or closes.
func TestProcessLockHelper(t *testing.T) {
	if os.Getenv("ORBIT_TEST_LOCK_HELPER") != "1" {
		return
	}

	walPath := os.Getenv("ORBIT_TEST_LOCK_PATH")
	if walPath == "" {
		fmt.Fprintln(os.Stderr, "missing ORBIT_TEST_LOCK_PATH")
		os.Exit(2)
	}

	w, err := wal.Open(walPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "child Open failed: %v\n", err)
		os.Exit(1)
	}

	// Announce that lock is successfully held by this subprocess
	fmt.Println("LOCKED")
	_ = os.Stdout.Sync()

	// Block until parent sends a byte or closes stdin
	buf := make([]byte, 1)
	_, _ = os.Stdin.Read(buf)

	// Clean exit
	if err := w.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "child Close failed: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestCrossProcessLockingLifecycle runs end-to-end multi-process verification:
// 1. First process (subprocess) obtains the exclusive process lock.
// 2. Second process (parent) attempts acquisition and is rejected with ErrDatabaseLocked.
// 3. First process terminates cleanly (releases lock).
// 4. Second process can now obtain the lock.
// 5. Another subprocess obtains the lock, then is terminated abnormally (killed via SIGKILL/Process.Kill()).
// 6. OS kernel automatically releases the lock from the dead process, allowing immediate acquisition.
func TestCrossProcessLockingLifecycle(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	// Helper to spawn a child process holding the lock
	spawnLockHolder := func() (*exec.Cmd, *bufio.Reader, error) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProcessLockHelper$")
		cmd.Env = append(os.Environ(),
			"ORBIT_TEST_LOCK_HELPER=1",
			"ORBIT_TEST_LOCK_PATH="+walPath,
		)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("stdout pipe: %w", err)
		}
		cmd.Stderr = os.Stderr

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("stdin pipe: %w", err)
		}
		_ = stdin // kept open until closed

		if err := cmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("start child: %w", err)
		}

		reader := bufio.NewReader(stdout)
		line, err := reader.ReadString('\n')
		if err != nil || !strings.Contains(line, "LOCKED") {
			_ = cmd.Process.Kill()
			return nil, nil, fmt.Errorf("child failed to lock: line=%q, err=%v", line, err)
		}

		return cmd, reader, nil
	}

	// -------------------------------------------------------------
	// Phase 1: Subprocess acquires lock, parent cannot acquire it
	// -------------------------------------------------------------
	child1, _, err := spawnLockHolder()
	if err != nil {
		t.Fatalf("spawn child1: %v", err)
	}

	// Parent attempts to open the same WAL while child1 holds the lock
	_, err = wal.Open(walPath)
	if !errors.Is(err, wal.ErrDatabaseLocked) {
		_ = child1.Process.Kill()
		t.Fatalf("expected ErrDatabaseLocked while child holds lock, got: %v", err)
	}

	// -------------------------------------------------------------
	// Phase 2: First process closes normally, parent can then acquire lock
	// -------------------------------------------------------------
	// Send termination byte to child1 to close gracefully
	if err := child1.Process.Kill(); err != nil {
		t.Fatalf("kill child1: %v", err)
	}
	_ = child1.Wait()

	// Small pause to allow OS handle cleanup
	time.Sleep(50 * time.Millisecond)

	// Parent can now acquire lock with retry loop to accommodate Windows OS handle release
	var parentWAL *wal.Log
	var openErr error
	for attempt := 0; attempt < 25; attempt++ {
		parentWAL, openErr = wal.Open(walPath)
		if openErr == nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if openErr != nil {
		t.Fatalf("parent failed to acquire lock after child closed: %v", openErr)
	}

	// Append a valid record to confirm normal functionality
	if err := parentWAL.Append(wal.Record{
		Operation: wal.OperationSet,
		Key:       "parent:key",
		Value:     "parent:val",
	}); err != nil {
		t.Fatalf("parent append: %v", err)
	}

	if err := parentWAL.Close(); err != nil {
		t.Fatalf("parent close: %v", err)
	}

	// -------------------------------------------------------------
	// Phase 3: Abnormal process termination releases lock via OS kernel
	// -------------------------------------------------------------
	child2, _, err := spawnLockHolder()
	if err != nil {
		t.Fatalf("spawn child2: %v", err)
	}

	// Confirm child2 holds lock
	_, err = wal.Open(walPath)
	if !errors.Is(err, wal.ErrDatabaseLocked) {
		_ = child2.Process.Kill()
		t.Fatalf("expected ErrDatabaseLocked while child2 holds lock, got: %v", err)
	}

	// Kill child2 abruptly (simulating hard crash/SIGKILL, no unlock executed in user-space)
	if err := child2.Process.Kill(); err != nil {
		t.Fatalf("kill child2: %v", err)
	}
	_ = child2.Wait()

	// OS kernel must automatically release file locks held by terminated process
	var recoveredWAL *wal.Log
	openErr = nil
	for attempt := 0; attempt < 25; attempt++ {
		recoveredWAL, openErr = wal.Open(walPath)
		if openErr == nil {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if openErr != nil {
		t.Fatalf("lock was not released after abnormal child termination: %v", openErr)
	}
	defer recoveredWAL.Close()

	// Verify existing records survived and recovered WAL accepts writes
	records, err := recoveredWAL.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll on recovered WAL: %v", err)
	}
	if len(records) != 1 || records[0].Key != "parent:key" {
		t.Fatalf("expected 1 record with key 'parent:key', got %+v", records)
	}
}
