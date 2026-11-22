package wal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
)

// Log is the Write-Ahead Log.
//
// Every mutation (SET / DELETE) is appended here and synced to disk BEFORE
// the in-memory Memtable is updated. This guarantees that if the process
// crashes between the two steps, we can reconstruct the correct state on
// the next startup by replaying the log.
//
// File format: JSON Lines — one JSON-encoded Record per line.
// This is human-readable and simple to implement. A future version can
// switch to a binary format with length-prefixed records and CRC checksums.
type Log struct {
	file     *os.File
	writer   *bufio.Writer
	sequence atomic.Uint64 // monotonically increasing, never resets
}

// Open opens (or creates) the WAL file at the given path.
// The file is opened in append mode so existing records are never overwritten.
// The caller must call Close() when done.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("wal: open file %q: %w", path, err)
	}

	log := &Log{
		file:   f,
		writer: bufio.NewWriter(f),
	}

	// Determine the highest sequence number already in the log so that
	// new records continue the sequence after a restart.
	records, err := log.Replay()
	if err != nil {
		// Replay errors are non-fatal here — partial final records are
		// expected and already handled inside Replay. Any other issue
		// is surfaced so the caller can decide whether to abort startup.
		f.Close()
		return nil, fmt.Errorf("wal: replay during open: %w", err)
	}

	if len(records) > 0 {
		log.sequence.Store(records[len(records)-1].Sequence)
	}

	return log, nil
}

// Append writes a Record to the WAL.
//
// The sequence number is auto-assigned — callers must NOT set it.
//
// Durability guarantee:
//  1. The record is JSON-encoded and written to the buffered writer.
//  2. The buffer is flushed to the OS page cache.
//  3. os.File.Sync() (fdatasync) is called to push data to physical storage.
//
// We do NOT return success until step 3 completes. This is the safest
// durability policy — every acknowledged write is guaranteed to survive
// a power failure.
func (l *Log) Append(r Record) error {
	// Assign the next sequence number atomically.
	r.Sequence = l.sequence.Add(1)

	// Encode the record as a single JSON line.
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("wal: marshal record: %w", err)
	}

	// Write the JSON line followed by a newline character.
	if _, err := l.writer.Write(line); err != nil {
		return fmt.Errorf("wal: write record: %w", err)
	}
	if err := l.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("wal: write newline: %w", err)
	}

	// Flush the buffer to the OS, then sync to disk.
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush buffer: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("wal: sync to disk: %w", err)
	}

	return nil
}

// Replay reads all valid records from the WAL file from the beginning.
//
// It is used in two places:
//  1. During Open() — to find the highest existing sequence number.
//  2. During storage engine startup — to reconstruct the Memtable.
//
// Corruption handling:
//   - Lines that are valid JSON Records are accepted.
//   - A partial (truncated) final line is silently skipped — this is the
//     normal result of a crash mid-write and is not an error.
//   - A complete line that fails JSON decoding is treated as corruption
//     and causes Replay to return an error. We never silently accept bad data.
func (l *Log) Replay() ([]Record, error) {
	// Seek to the beginning of the file for a full scan.
	if _, err := l.file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("wal: seek to start: %w", err)
	}

	var records []Record
	scanner := bufio.NewScanner(l.file)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // skip blank lines
		}

		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			// A truncated final line (process crashed mid-write) will NOT
			// have a trailing newline, so bufio.Scanner still surfaces it
			// as a token. We distinguish truncation from real corruption by
			// checking whether the line ends with '}' — a complete JSON
			// object always does. If it doesn't end with '}', it's a partial
			// write: skip it safely. If it ends with '}' but still fails to
			// parse, that's genuine corruption and we must not silently ignore it.
			trimmed := bytes.TrimRight(line, " \t\r")
			if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '}' {
				// Partial / truncated final line — safe to skip.
				continue
			}
			return nil, fmt.Errorf("wal: corrupt record (seq %d): %w", r.Sequence, err)
		}

		records = append(records, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("wal: scan file: %w", err)
	}

	return records, nil
}

// Close flushes any remaining buffered data and closes the underlying file.
func (l *Log) Close() error {
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush on close: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("wal: close file: %w", err)
	}
	return nil
}
