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

// HandlerFunc is called sequentially for each valid record replayed from the WAL.
// Returning an error aborts the replay.
type HandlerFunc func(Record) error

// Log is the Write-Ahead Log.
//
// Every mutation (SET / DELETE) is appended here and synced to disk BEFORE
// the in-memory Memtable is updated.
//
// Invariants & Streaming Replay:
//   - Sequence numbers and checksums are computed under `mu.Lock()`.
//   - The committed sequence counter is advanced ONLY after the record
//     has been successfully flushed and fsynced to physical disk.
//   - Replay is streaming: records are validated and passed to a HandlerFunc
//     one-by-one, keeping memory consumption strictly O(1) regardless of WAL size.
//   - Startup eliminates redundant double-scans: Open() prepares the handle,
//     and NewEngine() performs a single streaming recovery pass.
type Log struct {
	file     *os.File
	writer   *bufio.Writer
	sequence uint64     // highest committed sequence number on disk
	mu       sync.Mutex // protects appends, replaying, poisoning, and close
	closed   bool
	err      error // sticky failure; once set, WAL is poisoned and rejects writes
}

// Open opens (or creates) the WAL file at the given path.
// The file is opened in read/write mode. Startup recovery is decoupled
// so callers can perform a single streaming pass to reconstruct state.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open file %q: %w", path, err)
	}

	return &Log{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// Sequence returns the highest committed sequence number in the log.
func (l *Log) Sequence() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sequence
}

// Append writes a Record to the WAL in a thread-safe manner.
//
// Durability & Integrity:
//  1. Sets Version to CurrentFormatVersion.
//  2. Assigns `nextSeq = sequence + 1`.
//  3. Computes the CRC32 payload checksum.
//  4. Writes, flushes, and fsyncs to physical disk.
//  5. Advances committed sequence counter only after disk sync succeeds.
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
	r.Version = CurrentFormatVersion
	r.Sequence = nextSeq
	r.Checksum = r.ComputeChecksum()

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

// Replay streams, validates, and applies each record in the WAL one-by-one.
//
// Streaming Performance:
//   - Memory consumption is strictly O(1) — records are processed individually.
//   - Unknown JSON fields are strictly rejected.
//   - Blank records and sequence gaps are strictly rejected as corruption.
//   - Incomplete final records without a newline (crash mid-write) are truncated
//     back to the clean record boundary.
//   - Updates the committed sequence counter and sets the file pointer for appending.
func (l *Log) Replay(handler HandlerFunc) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}

	// Ensure any buffered data is flushed first.
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush before replay: %w", err)
	}

	// Seek to the beginning of the file.
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("wal: seek to start: %w", err)
	}

	reader := bufio.NewReader(l.file)
	var lastValidOffset int64 = 0
	var currentOffset int64 = 0
	var lastReplayedSeq uint64 = 0

	for {
		line, readErr := reader.ReadBytes('\n')
		lineLen := int64(len(line))

		if lineLen > 0 {
			hasNewline := line[lineLen-1] == '\n'
			trimmed := bytes.TrimSpace(line)

			if hasNewline {
				// Strict append-only log: blank records are never written by ORBIT.
				if len(trimmed) == 0 {
					return fmt.Errorf("wal: corrupt record at offset %d: unexpected blank line", currentOffset)
				}

				// Strict JSON decoding: disallow unknown fields
				dec := json.NewDecoder(bytes.NewReader(trimmed))
				dec.DisallowUnknownFields()

				var r Record
				if err := dec.Decode(&r); err != nil {
					return fmt.Errorf("wal: corrupt record at offset %d: %w", currentOffset, err)
				}

				// Sequence validation: contiguous progression (1, 2, 3...)
				expectedSeq := lastReplayedSeq + 1
				if r.Sequence != expectedSeq {
					return fmt.Errorf("wal: non-contiguous sequence at offset %d: got %d, expected %d", currentOffset, r.Sequence, expectedSeq)
				}

				// Format versioning, operations, and CRC32 checksum validation
				if err := r.Validate(); err != nil {
					return fmt.Errorf("wal: invalid record at offset %d (seq %d): %w", currentOffset, r.Sequence, err)
				}

				// Stream directly to handler
				if handler != nil {
					if err := handler(r); err != nil {
						return fmt.Errorf("wal: replay handler error at seq %d: %w", r.Sequence, err)
					}
				}

				lastReplayedSeq = r.Sequence
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
			return fmt.Errorf("wal: read error at offset %d: %w", currentOffset, readErr)
		}
	}

	// Check if there is trailing incomplete data that needs to be truncated.
	stat, err := l.file.Stat()
	if err != nil {
		return fmt.Errorf("wal: stat file: %w", err)
	}

	if stat.Size() > lastValidOffset {
		// Truncate dirty trailing bytes back to last valid record boundary.
		if err := l.file.Truncate(lastValidOffset); err != nil {
			return fmt.Errorf("wal: truncate partial record: %w", err)
		}
		if err := l.file.Sync(); err != nil {
			return fmt.Errorf("wal: sync truncated file: %w", err)
		}
	}

	// Position file pointer at the clean end for subsequent appends.
	if _, err := l.file.Seek(lastValidOffset, io.SeekStart); err != nil {
		return fmt.Errorf("wal: seek to end of valid log: %w", err)
	}

	// Reset the buffered writer so its internal buffer starts clean at the file pointer.
	l.writer.Reset(l.file)

	// Set the committed sequence counter to the highest replayed sequence.
	l.sequence = lastReplayedSeq

	return nil
}

// ReadAll is a convenience helper (primarily for tests) that streams all records
// into a slice using Replay().
func (l *Log) ReadAll() ([]Record, error) {
	var records []Record
	err := l.Replay(func(r Record) error {
		records = append(records, r)
		return nil
	})
	return records, err
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
