package raft

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
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
	mu           sync.Mutex
	dir          string
	metaPath     string
	logPath      string
	logFile      *os.File
	currentTerm  uint64
	votedFor     string
	entries      []LogEntry // 1-based indexing; entries[0] is sentinel {Index:0, Term:0}
	closed       bool
	poisoned     bool
	poisonErr    error
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

// recoverLog replays and validates all records from raft.log.
func (fs *FileStorage) recoverLog() error {
	f, err := os.OpenFile(fs.logPath, os.O_CREATE|os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	expectedIndex := uint64(1)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue // skip empty lines if any
		}

		var rec persistentRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return fmt.Errorf("%w: unmarshal record at index %d: %v", ErrCorruptState, expectedIndex, err)
		}

		// Verify checksum
		expectedChecksum := computeEntryChecksum(rec.Index, rec.Term, rec.Operation, rec.Key, rec.Value)
		if rec.Checksum != expectedChecksum {
			return fmt.Errorf("%w: record index %d (stored %d, computed %d)",
				ErrChecksumMismatch, rec.Index, rec.Checksum, expectedChecksum)
		}

		// Verify contiguous index
		if rec.Index != expectedIndex {
			return fmt.Errorf("%w: expected index %d, found %d", ErrCorruptState, expectedIndex, rec.Index)
		}

		fs.entries = append(fs.entries, LogEntry{
			Index:     rec.Index,
			Term:      rec.Term,
			Operation: rec.Operation,
			Key:       rec.Key,
			Value:     rec.Value,
		})
		expectedIndex++
	}

	return scanner.Err()
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

// atomicWriteFile safely writes data to path using a temp file, fsync, and rename.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "tmp.*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// On Windows, if target exists, remove it before rename
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(path)
	}

	return os.Rename(tmpPath, path)
}
