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
//
// Concurrency & Safety:
//   - An internal sync.Mutex (mu) protects all file writes, buffer flushes,
//     and disk syncs from concurrent goroutines.
//   - Replay validates contiguous sequence numbers (1, 2, 3, ...) and
//     truncates any incomplete trailing records left by crash mid-write.
type Log struct {
	file     *os.File
	writer   *bufio.Writer
	sequence atomic.Uint64 // current sequence number
	mu       sync.Mutex    // protects concurrent appends, replaying, and file close
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

	// Replay existing records to validate sequence continuity and
	// clean up any partial writes before new writes arrive.
	records, err := log.Replay()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("wal: replay during open: %w", err)
	}

	if len(records) > 0 {
		log.sequence.Store(records[len(records)-1].Sequence)
	}

	return log, nil
}

// Append writes a Record to the WAL in a thread-safe manner.
//
// Durability guarantee:
//  1. The sequence number is allocated monotonically.
//  2. The record is serialized and written to the buffered writer.
//  3. The buffer is flushed to the OS.
//  4. os.File.Sync() is called to flush data to persistent physical disk.
//
// The internal mutex guarantees no interleaved writes or race conditions
// when multiple concurrent requests issue writes.
func (l *Log) Append(r Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Assign the next sequence number monotonically.
	r.Sequence = l.sequence.Add(1)

	// Encode record as JSON.
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("wal: marshal record: %w", err)
	}

	// Write record followed by newline.
	if _, err := l.writer.Write(line); err != nil {
		return fmt.Errorf("wal: write record: %w", err)
	}
	if err := l.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("wal: write newline: %w", err)
	}

	// Flush buffer to OS page cache, then sync to physical disk.
	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush buffer: %w", err)
	}
	if err := l.file.Sync(); err != nil {
		return fmt.Errorf("wal: sync to disk: %w", err)
	}

	return nil
}

// Replay reads, validates, and replays all records in the WAL.
//
// Rules enforced:
//  1. Sequence validation: Records must form a contiguous progression:
//     1 -> 2 -> 3 -> ... (gaps or duplicates are rejected as corrupted).
//  2. Complete records: Any record with a terminating newline that fails
//     JSON parsing is treated as real corruption and returns an error.
//  3. Partial write recovery: An incomplete final record without a newline
//     (from a crash mid-write) is permitted; the file is truncated back
//     to the last valid record boundary so dirty trailing data is removed.
func (l *Log) Replay() ([]Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

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

			if len(trimmed) == 0 {
				// Blank line with a newline
				if hasNewline {
					currentOffset += lineLen
					lastValidOffset = currentOffset
				}
			} else if hasNewline {
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

				records = append(records, r)
				currentOffset += lineLen
				lastValidOffset = currentOffset
			} else {
				// Incomplete trailing record without a terminating newline (crash mid-write).
				// We stop scanning and will truncate back to lastValidOffset.
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

// Close flushes remaining buffers and closes the WAL file cleanly.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.writer.Flush(); err != nil {
		return fmt.Errorf("wal: flush on close: %w", err)
	}
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("wal: close file: %w", err)
	}
	return nil
}
