package chaos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	ErrFileTooSmall = errors.New("chaos: file too small to corrupt")
)

// CorruptSSTable mutates header magic bytes in an SSTable file without altering its length,
// simulating bit-rot, corruption, or bad sectors that fail header/footer validation.
func CorruptSSTable(sstablePath string) error {
	data, err := os.ReadFile(sstablePath)
	if err != nil {
		return err
	}
	if len(data) < 16 {
		return ErrFileTooSmall
	}

	// Corrupt header magic bytes so reader detects corruption on open
	data[0] ^= 0xFF
	data[1] ^= 0xAA

	return os.WriteFile(sstablePath, data, 0644)
}

// CorruptSnapshot intentionally modifies bytes in a snapshot data or meta file to test CRC32 rejection.
func CorruptSnapshot(snapshotPath string) error {
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("chaos: snapshot file is empty")
	}

	// Flip first byte
	data[0] ^= 0xEE
	return os.WriteFile(snapshotPath, data, 0644)
}

// CrashDuringWALAppend writes a torn/incomplete frame at the end of the WAL file
// simulating a power loss or kernel panic during fsync.
func CrashDuringWALAppend(walPath string) error {
	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// Incomplete 7-byte truncated header/payload with invalid checksum
	tornBytes := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x00, 0x00}
	_, err = f.Write(tornBytes)
	if err != nil {
		return err
	}
	return f.Sync()
}

// CrashDuringMemtableFlush writes an uncommitted .tmp SSTable file into the data directory
// simulating an engine crash after disk write but before the MANIFEST is atomically updated.
func CrashDuringMemtableFlush(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	tmpFile := filepath.Join(dataDir, fmt.Sprintf("sst_crashed_%d.sst.tmp", os.Getpid()))
	dummyData := []byte("uncommitted-sstable-flush-data-before-manifest-update")
	if err := os.WriteFile(tmpFile, dummyData, 0644); err != nil {
		return "", err
	}
	return tmpFile, nil
}

// CrashDuringCompaction creates orphan compaction temporary files simulating a mid-compaction crash.
func CrashDuringCompaction(dataDir string) (string, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}
	tmpFile := filepath.Join(dataDir, fmt.Sprintf("compact_l0_l1_%d.sst.tmp", os.Getpid()))
	dummyData := []byte("half-merged-compaction-output-orphaned")
	if err := os.WriteFile(tmpFile, dummyData, 0644); err != nil {
		return "", err
	}
	return tmpFile, nil
}
