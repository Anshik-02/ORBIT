package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"math"
)

const (
	bloomFilterVersion uint32 = 1
	// DefaultBloomFPRate is the default target false-positive rate (1% / 0.01).
	DefaultBloomFPRate float64 = 0.01
)

var (
	ErrCorruptBloomFilter = errors.New("storage: bloom filter is corrupted")
)

// BloomFilter is a space-efficient probabilistic set representation used to skip
// unnecessary disk reads for non-existent keys in SSTables.
type BloomFilter struct {
	numBits          uint64
	numHashFunctions uint32
	bitset           []byte
}

// NewBloomFilter creates an optimal BloomFilter for expectedElements and falsePositiveRate.
func NewBloomFilter(expectedElements int, falsePositiveRate float64) *BloomFilter {
	if expectedElements <= 0 {
		expectedElements = 1
	}
	if falsePositiveRate <= 0 || falsePositiveRate >= 1.0 {
		falsePositiveRate = DefaultBloomFPRate
	}

	// m = - (n * ln(p)) / (ln(2)^2)
	n := float64(expectedElements)
	ln2 := math.Ln2
	mBits := - (n * math.Log(falsePositiveRate)) / (ln2 * ln2)
	if mBits < 64 {
		mBits = 64
	}
	numBits := uint64(math.Ceil(mBits))

	// k = (m / n) * ln(2)
	k := (float64(numBits) / n) * ln2
	numHash := uint32(math.Round(k))
	if numHash < 1 {
		numHash = 1
	}
	if numHash > 30 {
		numHash = 30
	}

	numBytes := (numBits + 7) / 8
	return &BloomFilter{
		numBits:          numBits,
		numHashFunctions: numHash,
		bitset:           make([]byte, numBytes),
	}
}

// hashes returns two 64-bit hash values using FNV-1a variations for double hashing.
func hashKey(key string) (uint64, uint64) {
	h1 := fnv.New64a()
	_, _ = h1.Write([]byte(key))
	v1 := h1.Sum64()

	// Secondary hash with alternate salt
	h2 := fnv.New64()
	_, _ = h2.Write([]byte(key))
	v2 := h2.Sum64()
	if v2 == 0 {
		v2 = 0x9e3779b97f4a7c15 // Knuth's 64-bit golden ratio constant
	}
	return v1, v2
}

// Add inserts a key into the BloomFilter.
func (b *BloomFilter) Add(key string) {
	if b == nil || b.numBits == 0 {
		return
	}
	h1, h2 := hashKey(key)
	for i := uint32(0); i < b.numHashFunctions; i++ {
		combined := h1 + uint64(i)*h2
		bitIdx := combined % b.numBits
		byteIdx := bitIdx / 8
		bitOffset := bitIdx % 8
		b.bitset[byteIdx] |= (1 << bitOffset)
	}
}

// MayContain checks whether key might be in the set.
// If false, the key is definitely not in the set.
// If true, the key might be in the set (with configured false-positive rate).
func (b *BloomFilter) MayContain(key string) bool {
	if b == nil || b.numBits == 0 || len(b.bitset) == 0 {
		return true // fallback to safe assumption
	}
	h1, h2 := hashKey(key)
	for i := uint32(0); i < b.numHashFunctions; i++ {
		combined := h1 + uint64(i)*h2
		bitIdx := combined % b.numBits
		byteIdx := bitIdx / 8
		bitOffset := bitIdx % 8
		if (b.bitset[byteIdx] & (1 << bitOffset)) == 0 {
			return false
		}
	}
	return true
}

// Encode serializes the Bloom filter into a binary payload with CRC32 checksum.
// Layout:
//   - Version: 4 bytes (uint32)
//   - NumHashFunctions: 4 bytes (uint32)
//   - NumBits: 8 bytes (uint64)
//   - Bitset: len(bitset) bytes
//   - Checksum: 4 bytes (uint32 IEEE CRC32 over the preceding payload)
func (b *BloomFilter) Encode() []byte {
	if b == nil {
		return nil
	}
	payloadLen := 4 + 4 + 8 + len(b.bitset)
	buf := make([]byte, payloadLen+4)

	binary.BigEndian.PutUint32(buf[0:4], bloomFilterVersion)
	binary.BigEndian.PutUint32(buf[4:8], b.numHashFunctions)
	binary.BigEndian.PutUint64(buf[8:16], b.numBits)
	copy(buf[16:payloadLen], b.bitset)

	checksum := crc32.ChecksumIEEE(buf[:payloadLen])
	binary.BigEndian.PutUint32(buf[payloadLen:], checksum)

	return buf
}

// DecodeBloomFilter deserializes and validates a Bloom filter from raw binary data.
func DecodeBloomFilter(data []byte) (*BloomFilter, error) {
	if len(data) < 20 { // minimum: 4 + 4 + 8 + 0 + 4 = 20 bytes
		return nil, fmt.Errorf("%w: payload too short (%d bytes)", ErrCorruptBloomFilter, len(data))
	}

	payloadLen := len(data) - 4
	expectedChecksum := binary.BigEndian.Uint32(data[payloadLen:])
	actualChecksum := crc32.ChecksumIEEE(data[:payloadLen])
	if expectedChecksum != actualChecksum {
		return nil, fmt.Errorf("%w: checksum mismatch (stored %d, computed %d)",
			ErrCorruptBloomFilter, expectedChecksum, actualChecksum)
	}

	version := binary.BigEndian.Uint32(data[0:4])
	if version != bloomFilterVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrCorruptBloomFilter, version)
	}

	numHash := binary.BigEndian.Uint32(data[4:8])
	numBits := binary.BigEndian.Uint64(data[8:16])

	expectedBytes := (numBits + 7) / 8
	bitsetBytes := data[16:payloadLen]
	if uint64(len(bitsetBytes)) != expectedBytes {
		return nil, fmt.Errorf("%w: bitset byte length mismatch (expected %d, got %d)",
			ErrCorruptBloomFilter, expectedBytes, len(bitsetBytes))
	}

	bitsetCopy := make([]byte, len(bitsetBytes))
	copy(bitsetCopy, bitsetBytes)

	return &BloomFilter{
		numBits:          numBits,
		numHashFunctions: numHash,
		bitset:           bitsetCopy,
	}, nil
}
