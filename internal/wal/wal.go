package wal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// Sentinel WAL errors.
var (
	ErrWALClosed        = errors.New("wal: log is closed")
	ErrWALPoisoned      = errors.New("wal: log is poisoned due to previous write or sync failure")
	ErrDatabaseLocked   = errors.New("wal: database directory or file is already locked by another process")
	ErrSequenceOverflow = errors.New("wal: sequence number overflow")
)

// HandlerFunc is called sequentially for each valid record replayed from the WAL.
// Returning an error aborts the replay.
type HandlerFunc func(Record) error

// Log is the Write-Ahead Log.
//
// Every mutation (SET / DELETE) is appended here and synced to disk BEFORE
// the in-memory Memtable is updated.
//
// Invariants, Concurrency, and Process Locks:
//   - An exclusive OS file lock (<path>.lock) ensures only ONE process can open
//     a node's WAL at any given time.
//   - Sequence numbers and checksums are computed under `mu.Lock()`.
//   - An overflow check protects sequence number wrap-around.
//   - The committed sequence counter is advanced ONLY after the record
//     has been successfully flushed and fsynced to physical disk.
//   - Replay is streaming with strict O(1) memory and verifies trailing-content cleanliness.
type Log struct {
	file     *os.File
	lockFile *os.File
	writer   *bufio.Writer
	sequence uint64     // highest committed sequence number on disk
	mu       sync.Mutex // protects appends, replaying, poisoning, and close
	closed   bool
	closeErr error // sticky error from Close()
	err      error // sticky failure; once set, WAL is poisoned and rejects writes
}

// Open opens (or creates) the WAL file at the given path.
// Acquires an exclusive, process-level file lock (<path>.lock) to prevent
// multiple ORBIT nodes or processes from interfering with the same data directory.
// Restricts file permissions to 0600 (owner-only) and directory to 0700.
func Open(path string) (*Log, error) {
	// Ensure parent directory exists with restrictive 0700 permissions
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("wal: create directory %q: %w", dir, err)
		}
	}

	// 1. Acquire process-level exclusive lock on <path>.lock (0600)
	lockPath := path + ".lock"
	lFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("wal: open lock file %q: %w", lockPath, err)
	}

	if err := lockFile(lFile); err != nil {
		lFile.Close()
		return nil, err
	}

	// 2. Open WAL file in read/write mode with 0600 permissions
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		unlockFile(lFile)
		lFile.Close()
		return nil, fmt.Errorf("wal: open file %q: %w", path, err)
	}

	return &Log{
		file:     f,
		lockFile: lFile,
		writer:   bufio.NewWriter(f),
	}, nil
}

// Sequence returns the highest committed sequence number in the log.
func (l *Log) Sequence() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sequence
}

// SetSequence sets the sequence counter baseline (e.g. from restored manifest watermark).
func (l *Log) SetSequence(seq uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if seq > l.sequence {
		l.sequence = seq
	}
}

// SetSequenceForTesting allows test suites to set the sequence counter directly.
func (l *Log) SetSequenceForTesting(seq uint64) {
	l.SetSequence(seq)
}

// Append writes a Record to the WAL in a thread-safe manner.
func (l *Log) Append(r Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}
	if l.err != nil {
		return fmt.Errorf("%w: %v", ErrWALPoisoned, l.err)
	}

	// Sequence overflow protection
	if l.sequence == math.MaxUint64 {
		l.err = ErrSequenceOverflow
		return ErrSequenceOverflow
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

				// Strict trailing content check: ensure no secondary JSON token exists on the same line
				var trailing json.RawMessage
				if err := dec.Decode(&trailing); err != io.EOF {
					return fmt.Errorf("wal: corrupt record at offset %d: unexpected trailing content on record line", currentOffset)
				}

				// Sequence validation: contiguous progression (e.g. 1, 2, 3... or starting after truncation)
				expectedSeq := lastReplayedSeq + 1
				if lastReplayedSeq == 0 {
					expectedSeq = r.Sequence
				}
				if r.Sequence != expectedSeq {
					return fmt.Errorf("wal: non-contiguous sequence at offset %d: got %d, expected %d", currentOffset, r.Sequence, expectedSeq)
				}

				// Format versioning, operations, and length-prefixed CRC32 checksum validation
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

	// Set the committed sequence counter to the highest replayed sequence if advanced.
	if lastReplayedSeq > l.sequence {
		l.sequence = lastReplayedSeq
	}

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

// Reset truncates the WAL file to zero bytes, flushes the writer, and resets sequence to zero.
func (l *Log) Reset() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}
	if l.err != nil {
		return fmt.Errorf("%w: %v", ErrWALPoisoned, l.err)
	}

	if err := l.writer.Flush(); err != nil {
		l.err = err
		return fmt.Errorf("wal: flush before reset: %w", err)
	}

	if err := l.file.Truncate(0); err != nil {
		l.err = err
		return fmt.Errorf("wal: truncate on reset: %w", err)
	}

	if _, err := l.file.Seek(0, 0); err != nil {
		l.err = err
		return fmt.Errorf("wal: seek on reset: %w", err)
	}

	if err := l.file.Sync(); err != nil {
		l.err = err
		return fmt.Errorf("wal: sync on reset: %w", err)
	}

	l.writer.Reset(l.file)
	l.sequence = 0
	return nil
}

// TruncateBefore removes records with Sequence <= seq from the WAL, reclaiming disk space.
// If all records are <= seq, the file is truncated to zero while preserving the current sequence baseline.
func (l *Log) TruncateBefore(seq uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrWALClosed
	}
	if l.err != nil {
		return fmt.Errorf("%w: %v", ErrWALPoisoned, l.err)
	}
	if seq == 0 {
		return nil
	}

	// 1. Flush any pending buffered writes
	if err := l.writer.Flush(); err != nil {
		l.err = err
		return fmt.Errorf("wal: flush before truncate: %w", err)
	}

	// 2. If the target sequence is >= highest committed sequence, all records are obsolete
	if seq >= l.sequence {
		if err := l.file.Truncate(0); err != nil {
			l.err = err
			return fmt.Errorf("wal: truncate on reclaim: %w", err)
		}
		if _, err := l.file.Seek(0, io.SeekStart); err != nil {
			l.err = err
			return fmt.Errorf("wal: seek on reclaim: %w", err)
		}
		if err := l.file.Sync(); err != nil {
			l.err = err
			return fmt.Errorf("wal: sync on reclaim: %w", err)
		}
		l.writer.Reset(l.file)
		return nil
	}

	// 3. Otherwise, some records are still active (> seq).
	// Read records > seq into memory and rewrite the log cleanly.
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		l.err = err
		return fmt.Errorf("wal: seek start for rewrite: %w", err)
	}

	reader := bufio.NewReader(l.file)
	var retainedLines [][]byte

	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				var r Record
				if err := json.Unmarshal(trimmed, &r); err == nil {
					if r.Sequence > seq {
						cpy := make([]byte, len(line))
						copy(cpy, line)
						retainedLines = append(retainedLines, cpy)
					}
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// Truncate file and rewrite retained lines from byte offset 0
	if err := l.file.Truncate(0); err != nil {
		l.err = err
		return fmt.Errorf("wal: truncate for rewrite: %w", err)
	}
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		l.err = err
		return fmt.Errorf("wal: seek for rewrite: %w", err)
	}

	for _, lBytes := range retainedLines {
		if _, err := l.file.Write(lBytes); err != nil {
			l.err = err
			return fmt.Errorf("wal: write retained record: %w", err)
		}
	}

	if err := l.file.Sync(); err != nil {
		l.err = err
		return fmt.Errorf("wal: sync rewritten log: %w", err)
	}

	l.writer.Reset(l.file)
	return nil
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

// Close flushes remaining buffers, closes the WAL file, and releases the process lock.
// Retains sticky error: if close fails, subsequent invocations return the same error.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return l.closeErr
	}
	l.closed = true

	var flushErr error
	if l.err == nil && l.writer != nil {
		flushErr = l.writer.Flush()
	}

	var closeErr error
	if l.file != nil {
		closeErr = l.file.Close()
	}

	var unlockErr error
	if l.lockFile != nil {
		unlockErr = unlockFile(l.lockFile)
		l.lockFile.Close()
	}

	if flushErr != nil {
		l.closeErr = fmt.Errorf("wal: flush on close: %w", flushErr)
		return l.closeErr
	}
	if closeErr != nil {
		l.closeErr = fmt.Errorf("wal: close file: %w", closeErr)
		return l.closeErr
	}
	if unlockErr != nil {
		l.closeErr = fmt.Errorf("wal: unlock on close: %w", unlockErr)
		return l.closeErr
	}
	return nil
}
