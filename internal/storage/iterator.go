package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sort"
)

// KVPair represents a key-value record returned by range queries and scans.
type KVPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Iterator provides sequential access to keys and values across the LSM hierarchy.
type Iterator interface {
	// Valid returns true if the iterator is currently positioned at a valid entry.
	Valid() bool
	// Next advances the iterator to the next entry.
	Next()
	// Key returns the current entry's key.
	Key() string
	// Value returns the current entry's value.
	Value() string
	// IsTombstone returns true if the current entry represents a deleted key.
	IsTombstone() bool
	// Close releases any resources associated with the iterator.
	Close() error
}

// MemtableIterator iterates over an in-memory Memtable within [startKey, endKey).
type MemtableIterator struct {
	entries  []MemtableEntry
	idx      int
	startKey string
	endKey   string
}

// NewMemtableIterator creates an iterator over a memtable snapshot.
func NewMemtableIterator(m *Memtable, startKey, endKey string) *MemtableIterator {
	all := m.EntriesSorted()
	var filtered []MemtableEntry

	for _, e := range all {
		if startKey != "" && e.Key < startKey {
			continue
		}
		if endKey != "" && e.Key >= endKey {
			break
		}
		filtered = append(filtered, e)
	}

	return &MemtableIterator{
		entries:  filtered,
		idx:      0,
		startKey: startKey,
		endKey:   endKey,
	}
}

func (it *MemtableIterator) Valid() bool {
	return it.idx < len(it.entries)
}

func (it *MemtableIterator) Next() {
	if it.idx < len(it.entries) {
		it.idx++
	}
}

func (it *MemtableIterator) Key() string {
	if !it.Valid() {
		return ""
	}
	return it.entries[it.idx].Key
}

func (it *MemtableIterator) Value() string {
	if !it.Valid() {
		return ""
	}
	return it.entries[it.idx].Value
}

func (it *MemtableIterator) IsTombstone() bool {
	if !it.Valid() {
		return false
	}
	return it.entries[it.idx].IsTombstone
}

func (it *MemtableIterator) Close() error {
	it.entries = nil
	it.idx = 0
	return nil
}

// SSTableIterator streams records from an SSTable within [startKey, endKey) with O(1) memory.
type SSTableIterator struct {
	reader       *SSTableReader
	currOffset   uint64
	endOffset    uint64
	startKey     string
	endKey       string
	currentEntry MemtableEntry
	valid        bool
	err          error
}

// NewSSTableIterator initializes a streaming iterator over reader.
func NewSSTableIterator(reader *SSTableReader, startKey, endKey string) *SSTableIterator {
	it := &SSTableIterator{
		reader:     reader,
		currOffset: reader.dataOffset,
		endOffset:  reader.dataOffset + reader.dataLength,
		startKey:   startKey,
		endKey:     endKey,
	}

	// Fast check: if SSTable bounds don't overlap with requested range, mark invalid immediately
	meta := reader.Metadata()
	if endKey != "" && meta.MinKey != "" && meta.MinKey >= endKey {
		it.valid = false
		return it
	}
	if startKey != "" && meta.MaxKey != "" && meta.MaxKey < startKey {
		it.valid = false
		return it
	}

	// Seek to approximate start offset using sparse index
	if startKey != "" && len(reader.sparseIndex) > 0 {
		idx := sort.Search(len(reader.sparseIndex), func(i int) bool {
			return reader.sparseIndex[i].Key > startKey
		})
		startIdx := 0
		if idx > 0 {
			startIdx = idx - 1
		}
		it.currOffset = reader.sparseIndex[startIdx].Offset
	}

	// Advance to first entry in range
	it.Next()
	return it
}

func (it *SSTableIterator) Valid() bool {
	return it.valid && it.err == nil
}

func (it *SSTableIterator) Next() {
	headerBuf := make([]byte, 7) // 2 klen + 4 vlen + 1 flags

	for it.currOffset < it.endOffset {
		if _, err := it.reader.file.ReadAt(headerBuf, int64(it.currOffset)); err != nil {
			if errors.Is(err, io.EOF) {
				it.valid = false
				return
			}
			it.err = fmt.Errorf("sstable iterator: read header at %d: %w", it.currOffset, err)
			it.valid = false
			return
		}

		kLen := int(binary.BigEndian.Uint16(headerBuf[0:2]))
		vLen := int(binary.BigEndian.Uint32(headerBuf[2:6]))
		isTomb := headerBuf[6] == 1
		it.currOffset += 7

		payload := make([]byte, kLen+vLen+4)
		if _, err := it.reader.file.ReadAt(payload, int64(it.currOffset)); err != nil {
			it.err = fmt.Errorf("sstable iterator: read payload at %d: %w", it.currOffset, err)
			it.valid = false
			return
		}

		storedCRC := binary.BigEndian.Uint32(payload[kLen+vLen:])
		checkBuf := append(headerBuf, payload[:kLen+vLen]...)
		if crc32.ChecksumIEEE(checkBuf) != storedCRC {
			it.err = fmt.Errorf("%w: record at offset %d failed crc verification", ErrCorruptSSTable, it.currOffset-7)
			it.valid = false
			return
		}

		it.currOffset += uint64(len(payload))
		currKey := string(payload[:kLen])

		if it.startKey != "" && currKey < it.startKey {
			continue // skip entries before startKey
		}
		if it.endKey != "" && currKey >= it.endKey {
			it.valid = false // reached end of range
			return
		}

		it.currentEntry = MemtableEntry{
			Key:         currKey,
			Value:       string(payload[kLen : kLen+vLen]),
			IsTombstone: isTomb,
		}
		it.valid = true
		return
	}

	it.valid = false
}

func (it *SSTableIterator) Key() string {
	if !it.Valid() {
		return ""
	}
	return it.currentEntry.Key
}

func (it *SSTableIterator) Value() string {
	if !it.Valid() {
		return ""
	}
	return it.currentEntry.Value
}

func (it *SSTableIterator) IsTombstone() bool {
	if !it.Valid() {
		return false
	}
	return it.currentEntry.IsTombstone
}

func (it *SSTableIterator) Close() error {
	it.valid = false
	return it.err
}

// MergedIterator performs a multi-way merge across child iterators ordered from newest to oldest.
// Resolves duplicate keys (newest version wins) and handles tombstones safely.
type MergedIterator struct {
	children []Iterator // index 0 is newest, higher index is older
	current  MemtableEntry
	valid    bool
	closed   bool
}

// NewMergedIterator creates a MergedIterator over child iterators ordered from newest to oldest.
func NewMergedIterator(children []Iterator) *MergedIterator {
	it := &MergedIterator{
		children: children,
	}
	it.findNext()
	return it
}

func (m *MergedIterator) Valid() bool {
	return m.valid && !m.closed
}

func (m *MergedIterator) Next() {
	if !m.Valid() {
		return
	}
	m.findNext()
}

func (m *MergedIterator) Key() string {
	if !m.Valid() {
		return ""
	}
	return m.current.Key
}

func (m *MergedIterator) Value() string {
	if !m.Valid() {
		return ""
	}
	return m.current.Value
}

func (m *MergedIterator) IsTombstone() bool {
	if !m.Valid() {
		return false
	}
	return m.current.IsTombstone
}

func (m *MergedIterator) Close() error {
	m.closed = true
	m.valid = false
	var lastErr error
	for _, ch := range m.children {
		if err := ch.Close(); err != nil && lastErr == nil {
			lastErr = err
		}
	}
	return lastErr
}

// findNext locates the next smallest key across all child iterators.
// Advances all iterators sharing that minimum key, selecting the version from the newest layer.
// If the authoritative version is a tombstone, it continues looping until a live key or EOF is found.
func (m *MergedIterator) findNext() {
	for {
		var minKey string
		hasValid := false

		// 1. Find the lexicographically smallest key among all valid children
		for _, ch := range m.children {
			if ch.Valid() {
				k := ch.Key()
				if !hasValid || k < minKey {
					minKey = k
					hasValid = true
				}
			}
		}

		if !hasValid {
			m.valid = false
			return
		}

		// 2. The first child matching minKey holds the authoritative version
		// (because children are ordered from newest to oldest)
		var authoritativeEntry MemtableEntry
		foundAuthoritative := false

		for _, ch := range m.children {
			if ch.Valid() && ch.Key() == minKey {
				if !foundAuthoritative {
					authoritativeEntry = MemtableEntry{
						Key:         minKey,
						Value:       ch.Value(),
						IsTombstone: ch.IsTombstone(),
					}
					foundAuthoritative = true
				}
				// Advance all children that currently have this minKey
				ch.Next()
			}
		}

		// 3. If authoritative entry is a tombstone, it shadows all older versions:
		// do NOT yield to user, continue loop to find the next live key
		if authoritativeEntry.IsTombstone {
			continue
		}

		m.current = authoritativeEntry
		m.valid = true
		return
	}
}
