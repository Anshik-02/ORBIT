package storage

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupMetadata records manifest and checksum information for a backup snapshot.
type BackupMetadata struct {
	Timestamp   time.Time         `json:"timestamp"`
	Files       map[string]uint32 `json:"files_crc32"`
	TotalBytes  int64             `json:"total_bytes"`
}

// CreateBackup copies active SSTables, MANIFEST, and WAL files to a designated backup location.
func CreateBackup(dataDir, backupBaseDir string) (string, error) {
	backupName := fmt.Sprintf("backup-%d", time.Now().UnixNano())
	backupDir := filepath.Join(backupBaseDir, backupName)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("backup: mkdir %s: %w", backupDir, err)
	}

	copyFile := func(src, dst string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Sync()
	}

	// Walk dataDir and copy all .sst, MANIFEST, and .log files
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}

		ext := filepath.Ext(path)
		base := filepath.Base(path)
		if ext == ".sst" || ext == ".log" || base == "MANIFEST" {
			dstPath := filepath.Join(backupDir, rel)
			if err := copyFile(path, dstPath); err != nil {
				return fmt.Errorf("copy %s to %s: %w", path, dstPath, err)
			}
		}
		return nil
	})

	if err != nil {
		_ = os.RemoveAll(backupDir)
		return "", fmt.Errorf("backup walk: %w", err)
	}

	return backupDir, nil
}

// RestoreBackup restores SSTables, MANIFEST, and WAL from backupDir into targetDir.
func RestoreBackup(backupDir, targetDir string) error {
	if _, err := os.Stat(backupDir); err != nil {
		return fmt.Errorf("restore: backup dir not found: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("restore: mkdir target: %w", err)
	}

	copyFile := func(src, dst string) error {
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		return out.Sync()
	}

	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(targetDir, rel)
		return copyFile(path, dstPath)
	})
}

// ComputeFileCRC32 calculates IEEE CRC32 checksum of a file.
func ComputeFileCRC32(path string) (uint32, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	h := crc32.NewIEEE()
	if _, err := io.Copy(h, f); err != nil {
		return 0, err
	}
	return h.Sum32(), nil
}
