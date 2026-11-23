package raft

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	ErrChecksumMismatch   = errors.New("raft: log entry checksum mismatch")
	ErrStorageClosed      = errors.New("raft: storage is closed")
	ErrStoragePoisoned    = errors.New("raft: storage is poisoned due to previous write failure")
	ErrInvalidTruncateIdx = errors.New("raft: cannot truncate log index 0 or negative")
)

// Storage defines the durable persistence contract for a Raft consensus node.
type Storage interface {
	SaveState(term uint64, votedFor string) error
	GetState() (term uint64, votedFor string, err error)
	AppendEntries(entries []LogEntry) error
	Truncate(fromIndex uint64) error
	GetEntry(index uint64) (LogEntry, bool)
	GetEntries(fromIndex uint64) []LogEntry
	LastLogInfo() (lastIndex uint64, lastTerm uint64)
	Close() error
}

// persistentRecord represents an entry as stored on disk with a CRC32 checksum.
type persistentRecord struct {
	Index     uint64 `json:"index"`
	Term      uint64 `json:"term"`
	Operation string `json:"operation"`
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
	Checksum  uint32 `json:"checksum"`
}

// metadataState stores persistent term and voting metadata.
type metadataState struct {
	CurrentTerm uint64 `json:"current_term"`
	VotedFor    string `json:"voted_for"`
}

// FileStorage implements durable disk-backed Raft storage.
//
// Invariants & Safety:
//   - currentTerm and votedFor are persisted atomically to metadata.json via temp-file + fsync + rename.
//   - Log entries are persisted to raft.log with length-prefixed CRC32-IEEE checksums.
//   - Log entries are cached in memory for O(1) reads, indexed starting at index 1.
//   - Index 0 is a sentinel representing the base state (term 0).
//   - All operations are thread-safe protected by mu.
type FileStorage struct {
	mu          sync.Mutex
	dir         string
	metaPath    string
	logPath     string
	logFile     *os.File
	currentTerm uint64
	votedFor    string
	entries     []LogEntry // 1-based indexing; entries[0] is sentinel {Index:0, Term:0}
	closed      bool
	poisoned    bool
	poisonErr   error
}

// computeEntryChecksum calculates a length-prefixed CRC32-IEEE checksum over an entry's fields.
func computeEntryChecksum(index, term uint64, op, key, val string) uint32 {
	h := crc32.NewIEEE()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], index)
	h.Write(b[:])
	binary.BigEndian.PutUint64(b[:], term)
	h.Write(b[:])

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(op)))
	h.Write(lenBuf[:])
	h.Write([]byte(op))

	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(key)))
	h.Write(lenBuf[:])
	h.Write([]byte(key))

	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(val)))
	h.Write(lenBuf[:])
	h.Write([]byte(val))

	return h.Sum32()
}

// OpenFileStorage initializes or recovers Raft storage from the specified directory.
func OpenFileStorage(dataDir string) (*FileStorage, error) {
	raftDir := filepath.Join(dataDir, "raft")
	if err := os.MkdirAll(raftDir, 0700); err != nil {
		return nil, fmt.Errorf("raft storage: create dir %q: %w", raftDir, err)
	}

	fs := &FileStorage{
		dir:      raftDir,
		metaPath: filepath.Join(raftDir, "metadata.json"),
		logPath:  filepath.Join(raftDir, "raft.log"),
		entries:  []LogEntry{{Index: 0, Term: 0}}, // Sentinel entry at index 0
	}

	// 0. Crash recovery during atomic replacement:
	// If a crash occurred after removing the original file but before renaming the .tmp replacement,
	// promote the fully synced .tmp replacement. If the original file is intact, clean up any stale .tmp.
	for _, target := range []string{fs.logPath, fs.metaPath} {
		tmpTarget := target + ".tmp"
		if _, err := os.Stat(tmpTarget); err == nil {
			if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
				_ = os.Rename(tmpTarget, target)
			} else {
				_ = os.Remove(tmpTarget)
			}
		}
	}

	// 1. Recover metadata (currentTerm, votedFor)
	if err := fs.recoverMetadata(); err != nil {
		return nil, fmt.Errorf("raft storage: recover metadata: %w", err)
	}

	// 2. Recover and validate log entries from raft.log
	if err := fs.recoverLog(); err != nil {
		return nil, fmt.Errorf("raft storage: recover log: %w", err)
	}

	// 3. Open raft.log in append-only mode for writing
	f, err := os.OpenFile(fs.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("raft storage: open log file: %w", err)
	}
	fs.logFile = f

	return fs, nil
}

// recoverMetadata loads metadata.json if present.
func (fs *FileStorage) recoverMetadata() error {
	data, err := os.ReadFile(fs.metaPath)
	if errors.Is(err, os.ErrNotExist) {
		fs.currentTerm = 0
		fs.votedFor = ""
		return nil
	}
	if err != nil {
		return err
	}

	var meta metadataState
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("%w: %v", ErrCorruptState, err)
	}
	fs.currentTerm = meta.CurrentTerm
	fs.votedFor = meta.VotedFor
	return nil
}

// recoverLog replays and validates all records from raft.log with strict validation:
//  1. Rejects blank lines anywhere in the log as corruption.
//  2. Disallows unknown fields and secondary trailing JSON tokens.
//  3. Verifies contiguous 1-based indexing (expectedIndex) and monotonically non-decreasing terms.
//  4. Verifies length-prefixed CRC32-IEEE checksums.
//  5. Automatically truncates partial trailing writes from crashes mid-write back to last valid boundary.
func (fs *FileStorage) recoverLog() error {
	f, err := os.OpenFile(fs.logPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	var lastValidOffset int64 = 0
	var currentOffset int64 = 0
	expectedIndex := uint64(1)
	var prevTerm uint64 = 0

	for {
		line, readErr := reader.ReadBytes('\n')
		lineLen := int64(len(line))

		if lineLen > 0 {
			hasNewline := line[lineLen-1] == '\n'
			trimmed := bytes.TrimSpace(line)

			if hasNewline {
				// Strict append-only log: blank records are never written by Raft
				if len(trimmed) == 0 {
					return fmt.Errorf("%w: corrupt record at offset %d: unexpected blank line", ErrCorruptState, currentOffset)
				}

				// Strict JSON decoding: disallow unknown fields
				dec := json.NewDecoder(bytes.NewReader(trimmed))
				dec.DisallowUnknownFields()

				var rec persistentRecord
				if err := dec.Decode(&rec); err != nil {
					return fmt.Errorf("%w: corrupt record at offset %d: %v", ErrCorruptState, currentOffset, err)
				}

				// Check trailing tokens on record line
				var trailing json.RawMessage
				if err := dec.Decode(&trailing); err != io.EOF {
					return fmt.Errorf("%w: unexpected trailing content on record line at offset %d", ErrCorruptState, currentOffset)
				}

				// Verify checksum
				expectedChecksum := computeEntryChecksum(rec.Index, rec.Term, rec.Operation, rec.Key, rec.Value)
				if rec.Checksum != expectedChecksum {
					return fmt.Errorf("%w: record index %d (stored %d, computed %d)",
						ErrChecksumMismatch, rec.Index, rec.Checksum, expectedChecksum)
				}

				// Verify contiguous index
				if rec.Index != expectedIndex {
					return fmt.Errorf("%w: non-contiguous index at offset %d: expected %d, got %d",
						ErrCorruptState, currentOffset, expectedIndex, rec.Index)
				}

				// Verify monotonically non-decreasing terms
				if rec.Term < prevTerm {
					return fmt.Errorf("%w: term regression at index %d: was %d, got %d",
						ErrCorruptState, rec.Index, prevTerm, rec.Term)
				}

				fs.entries = append(fs.entries, LogEntry{
					Index:     rec.Index,
					Term:      rec.Term,
					Operation: rec.Operation,
					Key:       rec.Key,
					Value:     rec.Value,
				})
				prevTerm = rec.Term
				expectedIndex++
				currentOffset += lineLen
				lastValidOffset = currentOffset
			} else {
				// Incomplete trailing record without newline (crash mid-write)
				break
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fmt.Errorf("raft storage: read error at offset %d: %w", currentOffset, readErr)
		}
	}

	// Truncate any trailing partial write back to clean lastValidOffset
	fileInfo, err := f.Stat()
	if err != nil {
		return fmt.Errorf("raft storage: stat log file: %w", err)
	}

	if fileInfo.Size() > lastValidOffset {
		if err := f.Truncate(lastValidOffset); err != nil {
			return fmt.Errorf("raft storage: truncate partial trailing write: %w", err)
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("raft storage: sync truncated log: %w", err)
		}
	}

	return nil
}

// SaveState atomically persists currentTerm and votedFor to disk.
func (fs *FileStorage) SaveState(term uint64, votedFor string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return ErrStorageClosed
	}
	if fs.poisoned {
		return fmt.Errorf("%w: %v", ErrStoragePoisoned, fs.poisonErr)
	}

	meta := metadataState{
		CurrentTerm: term,
		VotedFor:    votedFor,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := atomicWriteFile(fs.metaPath, data, 0600); err != nil {
		fs.poisoned = true
		fs.poisonErr = err
		return fmt.Errorf("write metadata: %w", err)
	}

	fs.currentTerm = term
	fs.votedFor = votedFor
	return nil
}

// GetState returns the current persisted term and candidate vote.
func (fs *FileStorage) GetState() (term uint64, votedFor string, err error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return 0, "", ErrStorageClosed
	}
	if fs.poisoned {
		return 0, "", fmt.Errorf("%w: %v", ErrStoragePoisoned, fs.poisonErr)
	}
	return fs.currentTerm, fs.votedFor, nil
}

// AppendEntries appends one or more entries to the log and fsyncs to disk.
// Invariant: In-memory cache is updated ONLY after physical disk write and fsync succeed.
func (fs *FileStorage) AppendEntries(entries []LogEntry) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return ErrStorageClosed
	}
	if fs.poisoned {
		return fmt.Errorf("%w: %v", ErrStoragePoisoned, fs.poisonErr)
	}
	if len(entries) == 0 {
		return nil
	}

	// 0. Validate contiguous indexes and monotonic terms before touching disk
	lastEntry := fs.entries[len(fs.entries)-1]
	expectedIndex := lastEntry.Index + 1
	prevTerm := lastEntry.Term

	for _, entry := range entries {
		if entry.Index != expectedIndex {
			return fmt.Errorf("%w: append expected index %d, got %d", ErrInvalidIndex, expectedIndex, entry.Index)
		}
		if entry.Term < prevTerm {
			return fmt.Errorf("%w: append term regressed from %d to %d at index %d", ErrCorruptState, prevTerm, entry.Term, entry.Index)
		}
		expectedIndex++
		prevTerm = entry.Term
	}

	// 1. Buffer all records in memory first to prevent partial disk writes
	var buf bytes.Buffer
	for _, entry := range entries {
		cs := computeEntryChecksum(entry.Index, entry.Term, entry.Operation, entry.Key, entry.Value)
		rec := persistentRecord{
			Index:     entry.Index,
			Term:      entry.Term,
			Operation: entry.Operation,
			Key:       entry.Key,
			Value:     entry.Value,
			Checksum:  cs,
		}

		data, err := json.Marshal(rec)
		if err != nil {
			return fmt.Errorf("marshal log record %d: %w", entry.Index, err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	// 2. Write to disk
	if _, err := fs.logFile.Write(buf.Bytes()); err != nil {
		fs.poisoned = true
		fs.poisonErr = err
		return fmt.Errorf("write log records: %w", err)
	}

	// 3. Flush and sync to physical media
	if err := fs.logFile.Sync(); err != nil {
		fs.poisoned = true
		fs.poisonErr = err
		return fmt.Errorf("sync log file: %w", err)
	}

	// 4. Update in-memory entry cache ONLY after physical fsync succeeds
	fs.entries = append(fs.entries, entries...)

	return nil
}

// Truncate removes all entries from fromIndex to the end of the log.
// Used when a follower discovers conflicting entries from a new leader.
// Invariant: Existing log file and in-memory cache remain untouched until the replacement
// truncated log is safely serialized, synced, and atomically renamed.
func (fs *FileStorage) Truncate(fromIndex uint64) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return ErrStorageClosed
	}
	if fs.poisoned {
		return fmt.Errorf("%w: %v", ErrStoragePoisoned, fs.poisonErr)
	}
	if fromIndex == 0 {
		return ErrInvalidTruncateIdx
	}
	if fromIndex >= uint64(len(fs.entries)) {
		return nil // Nothing to truncate
	}

	// 1. Identify remaining prefix entries without mutating fs.entries yet
	remainingEntries := fs.entries[:fromIndex]

	// 2. Re-serialize valid remaining entries to memory buffer
	var buf bytes.Buffer
	for i := 1; i < len(remainingEntries); i++ {
		e := remainingEntries[i]
		cs := computeEntryChecksum(e.Index, e.Term, e.Operation, e.Key, e.Value)
		rec := persistentRecord{
			Index:     e.Index,
			Term:      e.Term,
			Operation: e.Operation,
			Key:       e.Key,
			Value:     e.Value,
			Checksum:  cs,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	// 3. Close open file handle before atomic replacement
	if fs.logFile != nil {
		_ = fs.logFile.Close()
		fs.logFile = nil
	}

	// 4. Atomically write replacement log file
	if err := atomicWriteFile(fs.logPath, buf.Bytes(), 0600); err != nil {
		fs.poisoned = true
		fs.poisonErr = err
		return fmt.Errorf("rewrite truncated log: %w", err)
	}

	// 5. Reopen in append mode
	f, err := os.OpenFile(fs.logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fs.poisoned = true
		fs.poisonErr = err
		return fmt.Errorf("reopen truncated log: %w", err)
	}
	fs.logFile = f

	// 6. Update in-memory cache ONLY after replacement disk log is reopened safely
	newEntries := make([]LogEntry, len(remainingEntries))
	copy(newEntries, remainingEntries)
	fs.entries = newEntries

	return nil
}

// GetEntry retrieves an entry at index.
func (fs *FileStorage) GetEntry(index uint64) (LogEntry, bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if index >= uint64(len(fs.entries)) {
		return LogEntry{}, false
	}
	return fs.entries[index], true
}

// GetEntries returns all entries starting from fromIndex up to the end of the log.
func (fs *FileStorage) GetEntries(fromIndex uint64) []LogEntry {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fromIndex >= uint64(len(fs.entries)) {
		return nil
	}
	result := make([]LogEntry, len(fs.entries)-int(fromIndex))
	copy(result, fs.entries[fromIndex:])
	return result
}

// LastLogInfo returns the index and term of the last entry in the log.
func (fs *FileStorage) LastLogInfo() (lastIndex uint64, lastTerm uint64) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	last := fs.entries[len(fs.entries)-1]
	return last.Index, last.Term
}

// Health returns an error if storage is closed or poisoned.
func (fs *FileStorage) Health() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return ErrStorageClosed
	}
	if fs.poisoned {
		return fmt.Errorf("%w: %v", ErrStoragePoisoned, fs.poisonErr)
	}
	return nil
}

// Close flushes and closes the persistent log handle.
func (fs *FileStorage) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return nil
	}
	fs.closed = true

	if fs.logFile != nil {
		err := fs.logFile.Close()
		fs.logFile = nil
		return err
	}
	return nil
}

// atomicWriteFile safely writes data to path using a deterministic .tmp file, fsync, and atomic rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// On Windows, if target exists, remove it before rename
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}

	return os.Rename(tmpPath, path)
}
