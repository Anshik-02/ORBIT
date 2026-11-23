package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	sstableMagicHeader       = "ORBITSST" // 8 bytes
	sstableMagicFooter       = "SSTFOOTR" // 8 bytes
	sstableVersion    uint32 = 1
	headerSizeBytes          = 32
	footerSizeBytes          = 64
	defaultSparseInterval    = 16 // Index every 16th key for fast sparse index lookup
)

var (
	ErrCorruptSSTable = errors.New("storage: sstable is corrupted")
	ErrClosedSSTable  = errors.New("storage: sstable reader is closed")
)

// IndexEntry represents a sparse index checkpoint mapping a key to its physical byte offset in the SSTable data block.
type IndexEntry struct {
	Key    string
	Offset uint64
}

// SSTableMetadata contains the structural information and bounds of an SSTable.
type SSTableMetadata struct {
	ID         uint64    `json:"id"`
	Path       string    `json:"path"`
	Filename   string    `json:"filename"`
	Level      uint32    `json:"level"`
	NumEntries uint64    `json:"num_entries"`
	MinKey     string    `json:"min_key"`
	MaxKey     string    `json:"max_key"`
	SizeBytes  int64     `json:"size_bytes"`
	Checksum   uint32    `json:"checksum"`
	CreatedAt  time.Time `json:"created_at"`
}

// SSTableWriter writes sorted key-value entries into an immutable SSTable file.
type SSTableWriter struct {
	path           string
	tmpPath        string
	file           *os.File
	id             uint64
	level          uint32
	sparseInterval int
	bloomFPRate    float64

	entriesWritten uint64
	currentOffset  uint64
	minKey         string
	maxKey         string
	lastWrittenKey string

	sparseIndex []IndexEntry
	bloomKeys   []string
	hasher      *crc32.Table
	runningCRC  uint32
}

// NewSSTableWriter creates a new SSTable writer preparing to write to path via path.tmp.
func NewSSTableWriter(path string, level uint32, sparseInterval int, bloomFPRate float64) (*SSTableWriter, error) {
	if sparseInterval <= 0 {
		sparseInterval = defaultSparseInterval
	}
	if bloomFPRate <= 0 || bloomFPRate >= 1.0 {
		bloomFPRate = DefaultBloomFPRate
	}

	var id uint64
	base := filepath.Base(path)
	_, _ = fmt.Sscanf(base, "%06d.sst", &id)

	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("sstable writer: open %s: %w", tmpPath, err)
	}

	w := &SSTableWriter{
		path:           path,
		tmpPath:        tmpPath,
		file:           f,
		id:             id,
		level:          level,
		sparseInterval: sparseInterval,
		bloomFPRate:    bloomFPRate,
		sparseIndex:    make([]IndexEntry, 0),
		bloomKeys:      make([]string, 0),
		hasher:         crc32.IEEETable,
	}

	// Write 32-byte header placeholder
	headerBuf := make([]byte, headerSizeBytes)
	copy(headerBuf[0:8], []byte(sstableMagicHeader))
	binary.BigEndian.PutUint32(headerBuf[8:12], sstableVersion)
	binary.BigEndian.PutUint32(headerBuf[12:16], level)
	// Entry count and timestamp will be written on Finish()
	if _, err := f.Write(headerBuf); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("sstable writer: write header: %w", err)
	}

	w.currentOffset = headerSizeBytes
	w.runningCRC = crc32.Update(0, w.hasher, headerBuf)
	return w, nil
}

// Append writes a single sorted entry into the SSTable.
// Invariant: Entries MUST be passed in strictly ascending key order.
func (w *SSTableWriter) Append(entry MemtableEntry) error {
	if w.entriesWritten > 0 && entry.Key <= w.lastWrittenKey {
		return fmt.Errorf("sstable writer: key %q out of order (previous %q)", entry.Key, w.lastWrittenKey)
	}

	entryOffset := w.currentOffset

	// Check if this key should be indexed in the sparse index
	if w.entriesWritten%uint64(w.sparseInterval) == 0 {
		w.sparseIndex = append(w.sparseIndex, IndexEntry{
			Key:    entry.Key,
			Offset: entryOffset,
		})
	}

	// Record for bloom filter
	w.bloomKeys = append(w.bloomKeys, entry.Key)

	// Format entry record:
	//   [KeyLen: 2B][ValLen: 4B][Flags: 1B][Key bytes][Val bytes][CRC32: 4B]
	keyBytes := []byte(entry.Key)
	valBytes := []byte(entry.Value)
	flags := byte(0)
	if entry.IsTombstone {
		flags = 1
	}

	recordLen := 2 + 4 + 1 + len(keyBytes) + len(valBytes)
	buf := make([]byte, recordLen+4)

	binary.BigEndian.PutUint16(buf[0:2], uint16(len(keyBytes)))
	binary.BigEndian.PutUint32(buf[2:6], uint32(len(valBytes)))
	buf[6] = flags
	copy(buf[7:7+len(keyBytes)], keyBytes)
	copy(buf[7+len(keyBytes):recordLen], valBytes)

	entryCRC := crc32.Checksum(buf[:recordLen], w.hasher)
	binary.BigEndian.PutUint32(buf[recordLen:], entryCRC)

	if _, err := w.file.Write(buf); err != nil {
		return fmt.Errorf("sstable writer: write record: %w", err)
	}

	w.runningCRC = crc32.Update(w.runningCRC, w.hasher, buf)
	w.currentOffset += uint64(len(buf))

	if w.entriesWritten == 0 {
		w.minKey = entry.Key
	}
	w.maxKey = entry.Key
	w.lastWrittenKey = entry.Key
	w.entriesWritten++

	return nil
}

// Finish writes the sparse index, bloom filter, footer, syncs disk, and atomically renames the file.
func (w *SSTableWriter) Finish() (*SSTableMetadata, error) {
	defer func() {
		if w.file != nil {
			_ = w.file.Close()
			_ = os.Remove(w.tmpPath)
		}
	}()

	dataOffset := uint64(headerSizeBytes)
	dataLength := w.currentOffset - dataOffset

	// 1. Serialize Sparse Index
	indexOffset := w.currentOffset
	var indexBuf bytes.Buffer
	var countBuf [4]byte
	binary.BigEndian.PutUint32(countBuf[:], uint32(len(w.sparseIndex)))
	indexBuf.Write(countBuf[:])

	for _, idx := range w.sparseIndex {
		kBytes := []byte(idx.Key)
		var entryHdr [10]byte
		binary.BigEndian.PutUint16(entryHdr[0:2], uint16(len(kBytes)))
		binary.BigEndian.PutUint64(entryHdr[2:10], idx.Offset)
		indexBuf.Write(entryHdr[:])
		indexBuf.Write(kBytes)
	}

	indexBytes := indexBuf.Bytes()
	if _, err := w.file.Write(indexBytes); err != nil {
		return nil, fmt.Errorf("sstable writer: write index: %w", err)
	}
	w.runningCRC = crc32.Update(w.runningCRC, w.hasher, indexBytes)
	w.currentOffset += uint64(len(indexBytes))
	indexLength := uint64(len(indexBytes))

	// 2. Generate and write Bloom Filter
	bloomOffset := w.currentOffset
	bloom := NewBloomFilter(len(w.bloomKeys), w.bloomFPRate)
	for _, k := range w.bloomKeys {
		bloom.Add(k)
	}
	bloomBytes := bloom.Encode()
	if _, err := w.file.Write(bloomBytes); err != nil {
		return nil, fmt.Errorf("sstable writer: write bloom: %w", err)
	}
	w.runningCRC = crc32.Update(w.runningCRC, w.hasher, bloomBytes)
	w.currentOffset += uint64(len(bloomBytes))
	bloomLength := uint64(len(bloomBytes))

	// 3. Write Footer (64 bytes)
	// Layout:
	//   [0:8]   DataOffset: uint64
	//   [8:16]  DataLength: uint64
	//   [16:24] IndexOffset: uint64
	//   [24:32] IndexLength: uint64
	//   [32:40] BloomOffset: uint64
	//   [40:48] BloomLength: uint64
	//   [48:56] MagicFooter: "SSTFOOTR" (8 bytes)
	//   [56:60] TotalChecksum: uint32
	//   [60:64] Reserved / Padding: 4 bytes
	footerBuf := make([]byte, footerSizeBytes)
	binary.BigEndian.PutUint64(footerBuf[0:8], dataOffset)
	binary.BigEndian.PutUint64(footerBuf[8:16], dataLength)
	binary.BigEndian.PutUint64(footerBuf[16:24], indexOffset)
	binary.BigEndian.PutUint64(footerBuf[24:32], indexLength)
	binary.BigEndian.PutUint64(footerBuf[32:40], bloomOffset)
	binary.BigEndian.PutUint64(footerBuf[40:48], bloomLength)
	copy(footerBuf[48:56], []byte(sstableMagicFooter))

	finalChecksum := crc32.Update(w.runningCRC, w.hasher, footerBuf[:56])
	binary.BigEndian.PutUint32(footerBuf[56:60], finalChecksum)

	if _, err := w.file.Write(footerBuf); err != nil {
		return nil, fmt.Errorf("sstable writer: write footer: %w", err)
	}
	w.currentOffset += uint64(len(footerBuf))

	// 4. Update Header with true entry count and timestamp
	createdAt := time.Now()
	var updateHdr [16]byte
	binary.BigEndian.PutUint64(updateHdr[0:8], w.entriesWritten)
	binary.BigEndian.PutUint64(updateHdr[8:16], uint64(createdAt.UnixNano()))
	if _, err := w.file.WriteAt(updateHdr[:], 16); err != nil {
		return nil, fmt.Errorf("sstable writer: update header: %w", err)
	}

	// 5. Sync and close temporary file
	if err := w.file.Sync(); err != nil {
		return nil, fmt.Errorf("sstable writer: sync: %w", err)
	}
	if err := w.file.Close(); err != nil {
		return nil, fmt.Errorf("sstable writer: close: %w", err)
	}
	w.file = nil

	// 6. Atomic rename
	if _, err := os.Stat(w.path); err == nil {
		_ = os.Remove(w.path)
	}
	if err := os.Rename(w.tmpPath, w.path); err != nil {
		return nil, fmt.Errorf("sstable writer: rename %s to %s: %w", w.tmpPath, w.path, err)
	}

	// 7. Directory fsync
	dir := filepath.Dir(w.path)
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		_ = df.Close()
	}

	return &SSTableMetadata{
		ID:         w.id,
		Path:       w.path,
		Filename:   filepath.Base(w.path),
		Level:      w.level,
		NumEntries: w.entriesWritten,
		MinKey:     w.minKey,
		MaxKey:     w.maxKey,
		SizeBytes:  int64(w.currentOffset),
		Checksum:   finalChecksum,
		CreatedAt:  createdAt,
	}, nil
}

// SSTableReader provides efficient, thread-safe concurrent reads from an immutable SSTable file.
type SSTableReader struct {
	mu          sync.RWMutex
	path        string
	file        *os.File
	meta        SSTableMetadata
	sparseIndex []IndexEntry
	bloomFilter *BloomFilter
	dataOffset  uint64
	dataLength  uint64
	closed      bool
}

// OpenSSTableReader opens and verifies an existing SSTable from path.
// Loads and validates footer, bloom filter, and sparse index into memory.
func OpenSSTableReader(path string) (*SSTableReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("sstable reader: open %s: %w", path, err)
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: stat %s: %w", path, err)
	}

	fileSize := fi.Size()
	if fileSize < int64(headerSizeBytes+footerSizeBytes) {
		_ = f.Close()
		return nil, fmt.Errorf("%w: file size %d smaller than minimum header+footer", ErrCorruptSSTable, fileSize)
	}

	// 1. Read and parse Header
	headerBuf := make([]byte, headerSizeBytes)
	if _, err := f.ReadAt(headerBuf, 0); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: read header: %w", err)
	}
	if string(headerBuf[0:8]) != sstableMagicHeader {
		_ = f.Close()
		return nil, fmt.Errorf("%w: invalid header magic %q", ErrCorruptSSTable, string(headerBuf[0:8]))
	}

	version := binary.BigEndian.Uint32(headerBuf[8:12])
	if version != sstableVersion {
		_ = f.Close()
		return nil, fmt.Errorf("%w: unsupported sstable version %d", ErrCorruptSSTable, version)
	}
	level := binary.BigEndian.Uint32(headerBuf[12:16])
	numEntries := binary.BigEndian.Uint64(headerBuf[16:24])
	createdAtUnix := int64(binary.BigEndian.Uint64(headerBuf[24:32]))

	// 2. Read and parse Footer
	footerBuf := make([]byte, footerSizeBytes)
	footerOffset := fileSize - footerSizeBytes
	if _, err := f.ReadAt(footerBuf, footerOffset); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: read footer: %w", err)
	}

	if string(footerBuf[48:56]) != sstableMagicFooter {
		_ = f.Close()
		return nil, fmt.Errorf("%w: invalid footer magic %q", ErrCorruptSSTable, string(footerBuf[48:56]))
	}

	dataOffset := binary.BigEndian.Uint64(footerBuf[0:8])
	dataLength := binary.BigEndian.Uint64(footerBuf[8:16])
	indexOffset := binary.BigEndian.Uint64(footerBuf[16:24])
	indexLength := binary.BigEndian.Uint64(footerBuf[24:32])
	bloomOffset := binary.BigEndian.Uint64(footerBuf[32:40])
	bloomLength := binary.BigEndian.Uint64(footerBuf[40:48])

	// Validate boundaries
	if dataOffset+dataLength > uint64(fileSize) ||
		indexOffset+indexLength > uint64(fileSize) ||
		bloomOffset+bloomLength > uint64(fileSize) {
		_ = f.Close()
		return nil, fmt.Errorf("%w: boundary overflow in footer offsets", ErrCorruptSSTable)
	}

	// 3. Read and decode Bloom Filter
	bloomBuf := make([]byte, bloomLength)
	if _, err := f.ReadAt(bloomBuf, int64(bloomOffset)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: read bloom filter: %w", err)
	}
	bloom, err := DecodeBloomFilter(bloomBuf)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: decode bloom filter: %w", err)
	}

	// 4. Read and decode Sparse Index
	indexBuf := make([]byte, indexLength)
	if _, err := f.ReadAt(indexBuf, int64(indexOffset)); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sstable reader: read sparse index: %w", err)
	}

	if len(indexBuf) < 4 {
		_ = f.Close()
		return nil, fmt.Errorf("%w: truncated index block", ErrCorruptSSTable)
	}
	indexCount := binary.BigEndian.Uint32(indexBuf[0:4])
	sparseIndex := make([]IndexEntry, 0, indexCount)

	offset := 4
	for i := uint32(0); i < indexCount; i++ {
		if offset+10 > len(indexBuf) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: unexpected eof reading index entry %d", ErrCorruptSSTable, i)
		}
		kLen := int(binary.BigEndian.Uint16(indexBuf[offset : offset+2]))
		recOffset := binary.BigEndian.Uint64(indexBuf[offset+2 : offset+10])
		offset += 10

		if offset+kLen > len(indexBuf) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: unexpected eof reading index key %d", ErrCorruptSSTable, i)
		}
		k := string(indexBuf[offset : offset+kLen])
		offset += kLen

		sparseIndex = append(sparseIndex, IndexEntry{
			Key:    k,
			Offset: recOffset,
		})
	}

	var minKey, maxKey string
	if len(sparseIndex) > 0 {
		minKey = sparseIndex[0].Key
		maxKey = sparseIndex[len(sparseIndex)-1].Key
	}

	var sstID uint64
	baseName := filepath.Base(path)
	_, _ = fmt.Sscanf(baseName, "%06d.sst", &sstID)
	storedCRC := binary.BigEndian.Uint32(footerBuf[56:60])

	reader := &SSTableReader{
		path: path,
		file: f,
		meta: SSTableMetadata{
			ID:         sstID,
			Path:       path,
			Filename:   baseName,
			Level:      level,
			NumEntries: numEntries,
			MinKey:     minKey,
			MaxKey:     maxKey,
			SizeBytes:  fileSize,
			Checksum:   storedCRC,
			CreatedAt:  time.Unix(0, createdAtUnix),
		},
		sparseIndex: sparseIndex,
		bloomFilter: bloom,
		dataOffset:  dataOffset,
		dataLength:  dataLength,
	}

	return reader, nil
}

// MayContain quickly checks the in-memory Bloom filter to test if key might be in this SSTable.
// If false, guarantees the key does NOT exist in this SSTable without performing any disk I/O.
func (r *SSTableReader) MayContain(key string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed || r.bloomFilter == nil {
		return true
	}
	return r.bloomFilter.MayContain(key)
}

// Get looks up a key in the SSTable.
// Returns:
//   - (value, true, false, nil) if key is found with a live value.
//   - ("", true, true, nil) if key is found as a tombstone (deleted).
//   - ("", false, false, nil) if key does not exist in this SSTable.
func (r *SSTableReader) Get(key string) (value string, found bool, isTombstone bool, err error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return "", false, false, ErrClosedSSTable
	}

	// 1. Bloom filter check: if bloom returns false, the key definitely is not in this SSTable
	if r.bloomFilter != nil && !r.bloomFilter.MayContain(key) {
		return "", false, false, nil
	}

	if len(r.sparseIndex) == 0 {
		return "", false, false, nil
	}

	// 2. Binary search sparse index to locate the narrow byte offset interval
	idx := sort.Search(len(r.sparseIndex), func(i int) bool {
		return r.sparseIndex[i].Key > key
	})

	startIdx := 0
	if idx > 0 {
		startIdx = idx - 1
	}

	searchStartOffset := r.sparseIndex[startIdx].Offset
	searchEndOffset := r.dataOffset + r.dataLength
	if startIdx+1 < len(r.sparseIndex) {
		searchEndOffset = r.sparseIndex[startIdx+1].Offset
	}

	// 3. Scan the interval within the SSTable data block
	currOffset := searchStartOffset
	headerBuf := make([]byte, 7) // 2 (klen) + 4 (vlen) + 1 (flags)

	for currOffset < searchEndOffset {
		if _, err := r.file.ReadAt(headerBuf, int64(currOffset)); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", false, false, fmt.Errorf("sstable reader: read header at %d: %w", currOffset, err)
		}

		kLen := int(binary.BigEndian.Uint16(headerBuf[0:2]))
		vLen := int(binary.BigEndian.Uint32(headerBuf[2:6]))
		isTomb := headerBuf[6] == 1
		currOffset += 7

		payload := make([]byte, kLen+vLen+4) // key + val + crc32
		if _, err := r.file.ReadAt(payload, int64(currOffset)); err != nil {
			return "", false, false, fmt.Errorf("sstable reader: read payload at %d: %w", currOffset, err)
		}

		storedCRC := binary.BigEndian.Uint32(payload[kLen+vLen:])
		checkBuf := append(headerBuf, payload[:kLen+vLen]...)
		if crc32.ChecksumIEEE(checkBuf) != storedCRC {
			return "", false, false, fmt.Errorf("%w: record at offset %d failed crc verification", ErrCorruptSSTable, currOffset-7)
		}

		currKey := string(payload[:kLen])
		if currKey == key {
			currVal := string(payload[kLen : kLen+vLen])
			return currVal, true, isTomb, nil
		}

		// Since keys are strictly sorted, if currKey > key, the key cannot exist in this table
		if currKey > key {
			return "", false, false, nil
		}

		currOffset += uint64(len(payload))
	}

	return "", false, false, nil
}

// Metadata returns the immutable structural metadata of this SSTable.
func (r *SSTableReader) Metadata() SSTableMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.meta
}

// AllEntries reads and returns all records stored in this SSTable in ascending sorted order.
func (r *SSTableReader) AllEntries() ([]MemtableEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return nil, ErrClosedSSTable
	}

	entries := make([]MemtableEntry, 0, r.meta.NumEntries)
	currOffset := r.dataOffset
	endOffset := r.dataOffset + r.dataLength
	headerBuf := make([]byte, 7)

	for currOffset < endOffset {
		if _, err := r.file.ReadAt(headerBuf, int64(currOffset)); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("sstable reader: read header at %d: %w", currOffset, err)
		}

		kLen := int(binary.BigEndian.Uint16(headerBuf[0:2]))
		vLen := int(binary.BigEndian.Uint32(headerBuf[2:6]))
		isTomb := headerBuf[6] == 1
		currOffset += 7

		payload := make([]byte, kLen+vLen+4)
		if _, err := r.file.ReadAt(payload, int64(currOffset)); err != nil {
			return nil, fmt.Errorf("sstable reader: read payload at %d: %w", currOffset, err)
		}

		storedCRC := binary.BigEndian.Uint32(payload[kLen+vLen:])
		checkBuf := append(headerBuf, payload[:kLen+vLen]...)
		if crc32.ChecksumIEEE(checkBuf) != storedCRC {
			return nil, fmt.Errorf("%w: record at offset %d failed crc verification", ErrCorruptSSTable, currOffset-7)
		}

		currKey := string(payload[:kLen])
		currVal := string(payload[kLen : kLen+vLen])
		entries = append(entries, MemtableEntry{
			Key:         currKey,
			Value:       currVal,
			IsTombstone: isTomb,
		})

		currOffset += uint64(len(payload))
	}

	return entries, nil
}

// Close closes the underlying file descriptor.
func (r *SSTableReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}
