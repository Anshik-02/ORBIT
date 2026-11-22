package wal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// Sentinel WAL errors.
var (
	ErrWALClosed   = errors.New("wal: log is closed")
	ErrWALPoisoned = errors.New("wal: log is poisoned due to previous write or sync failure")
)

// Log is the Write-Ahead Log.
//
// Every mutation (SET / DELETE) is appended here and synced to disk BEFORE
// the in-memory Memtable is updated.
//
// Sequence Allocation & Durability Invariant:
//   - Sequence numbers are computed under `mu.Lock()`.
//   - The committed sequence counter is advanced ONLY after the complete record
//     has been successfully flushed and fsynced to physical disk.
//   - Any write or sync failure marks the log as poisoned (`err != nil`),
//     preventing any subsequent writes from being accepted into an uncertain state.
type Log struct {
	file     *os.File
	writer   *bufio.Writer
	sequence uint64     // highest committed sequence number on disk
	mu       sync.Mutex // protects appends, replaying, poisoning, and close
	closed   bool
	err      error // sticky failure; once set, WAL is poisoned and rejects writes
}

// Open opens (or creates) the WAL file at the given path.
// The file is opened in read/write/append mode.
// Any incomplete trailing write from a previous crash is truncated back to
// the last clean record boundary during initial replay.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open file %q: %w", path, err)
	}

	log := &Log{
		file:   f,
		writer: bufio.NewWriter(f),
	}

	// Replay existing records to validate sequence continuity, record integrity,
	// and clean up any partial writes before new writes arrive.
	records, err := log.Replay()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: replay during open: %w", err)
	}

	if len(records) > 0 {
		log.sequence = records[len(records)-1].Sequence
	}

	return log, nil
}

// Append writes a Record to the WAL in a thread-safe manner.
//
// Sequence guarantee:
//  1. Assigns `nextSeq = sequence + 1`.
//  2. Serializes, writes, flushes, and fsyncs to physical disk.
//  3. Advances `sequence = nextSeq` ONLY after `file.Sync()` succeeds.
//  4. If write or sync fails, WAL is marked poisoned and all subsequent appends fail.
func (l *Log) Append(r Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}
	if l.err != nil {
		return fmt.Errorf("%w: %v", ErrWALPoisoned, l.err)
	}

	nextSeq := l.sequence + 1
	r.Sequence = nextSeq

	// Encode record as JSON.
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("wal: marshal record: %w", err)
	}

	// Write record followed by newline.
	if _, err := l.writer.Write(line); err != nil {
		l.err = fmt.Errorf("write record: %w", err)
		return l.err
	}
	if err := l.writer.WriteByte('\n'); err != nil {
		l.err = fmt.Errorf("write newline: %w", err)
		return l.err
	}

	// Flush buffer to OS page cache.
	if err := l.writer.Flush(); err != nil {
		l.err = fmt.Errorf("flush buffer: %w", err)
		return l.err
	}

	// Fsync to physical disk.
	if err := l.file.Sync(); err != nil {
		l.err = fmt.Errorf("sync disk: %w", err)
		return l.err
	}

	// Advance sequence number only after complete, successful durability sync.
	l.sequence = nextSeq
	return nil
}

// Replay reads, validates, and replays all records in the WAL.
//
// Validation rules enforced:
//  1. Sequence validation: Records must form a contiguous progression:
//     1 -> 2 -> 3 -> ... (gaps or duplicates are rejected as corrupted).
//  2. Strict append-only log: Blank lines are strictly rejected as corruption.
//  3. Operation & Field validation: Only SET and DELETE are allowed; keys
//     and values must be non-empty where required.
//  4. Complete records: Any record with a terminating newline that fails
//     JSON parsing or validation is treated as corruption and halts replay.
//  5. Partial write recovery: An incomplete final record without a newline
//     (from a crash mid-write) is permitted; the file is truncated back
//     to the last valid record boundary.
func (l *Log) Replay() ([]Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil, ErrWALClosed
	}

	// Ensure any buffered data is flushed first.
	if err := l.writer.Flush(); err != nil {
		return nil, fmt.Errorf("wal: flush before replay: %w", err)
	}

	// Seek to the beginning of the file.
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("wal: seek to start: %w", err)
	}

	reader := bufio.NewReader(l.file)
	var records []Record
	var lastValidOffset int64 = 0
	var currentOffset int64 = 0

	for {
		line, readErr := reader.ReadBytes('\n')
		lineLen := int64(len(line))

		if lineLen > 0 {
			hasNewline := line[lineLen-1] == '\n'
			trimmed := bytes.TrimSpace(line)

			if hasNewline {
				// Strict append-only log: blank records are never written by ORBIT.
				// Blank lines indicate corruption or file tampering.
				if len(trimmed) == 0 {
					return nil, fmt.Errorf("wal: corrupt record at offset %d: unexpected blank line", currentOffset)
				}

				// A complete newline-terminated line
				var r Record
				if err := json.Unmarshal(trimmed, &r); err != nil {
					return nil, fmt.Errorf("wal: corrupt record at offset %d: %w", currentOffset, err)
				}

				// Sequence validation: contiguous progression (1, 2, 3...)
				expectedSeq := uint64(1)
				if len(records) > 0 {
					expectedSeq = records[len(records)-1].Sequence + 1
				}
				if r.Sequence != expectedSeq {
					return nil, fmt.Errorf("wal: non-contiguous sequence at offset %d: got %d, expected %d", currentOffset, r.Sequence, expectedSeq)
				}

				// Record integrity & operation validation
				if err := r.Validate(); err != nil {
					return nil, fmt.Errorf("wal: invalid record at offset %d (seq %d): %w", currentOffset, r.Sequence, err)
				}

				records = append(records, r)
				currentOffset += lineLen
				lastValidOffset = currentOffset
			} else {
				// Incomplete trailing record without newline (crash mid-write).
				break
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, fmt.Errorf("wal: read error at offset %d: %w", currentOffset, readErr)
		}
	}

	// Check if there is trailing incomplete data that needs to be truncated.
	stat, err := l.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("wal: stat file: %w", err)
	}

	if stat.Size() > lastValidOffset {
		// Truncate dirty trailing bytes back to last valid record boundary.
		if err := l.file.Truncate(lastValidOffset); err != nil {
			return nil, fmt.Errorf("wal: truncate partial record: %w", err)
		}
		if err := l.file.Sync(); err != nil {
			return nil, fmt.Errorf("wal: sync truncated file: %w", err)
		}
	}

	// Position file pointer at the clean end for subsequent appends.
	if _, err := l.file.Seek(lastValidOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("wal: seek to end of valid log: %w", err)
	}

	// Reset the buffered writer so its internal buffer starts clean at the file pointer.
	l.writer.Reset(l.file)

	return records, nil
}

// Health checks whether the WAL is open and unpoisoned.
func (l *Log) Health() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}
	if l.err != nil {
		return fmt.Errorf("%w: %v", ErrWALPoisoned, l.err)
	}
	return nil
}

// Close flushes remaining buffers and closes the WAL file cleanly.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	var flushErr error
	if l.err == nil {
		flushErr = l.writer.Flush()
	}

	closeErr := l.file.Close()
	if flushErr != nil {
		return fmt.Errorf("wal: flush on close: %w", flushErr)
	}
	if closeErr != nil {
		return fmt.Errorf("wal: close file: %w", closeErr)
	}
	return nil
}
