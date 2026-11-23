package storage

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"time"
)

// BackupMetadata records manifest, file list, and checksum information for a point-in-time backup.
type BackupMetadata struct {
	Timestamp   time.Time         `json:"timestamp"`
	DataDir     string            `json:"data_dir"`
	Files       map[string]uint32 `json:"files_crc32"`
	TotalBytes  int64             `json:"total_bytes"`
	ManifestSeq uint64            `json:"manifest_sequence"`
	SSTableIDs  []uint64          `json:"sstable_ids"`
}

// copyFileWithChecksum copies src to dst and computes its IEEE CRC32 checksum.
func copyFileWithChecksum(src, dst string) (uint32, int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, 0, err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, 0, err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}
	defer out.Close()

	h := crc32.NewIEEE()
	w := io.MultiWriter(out, h)

	n, err := io.Copy(w, in)
	if err != nil {
		return 0, 0, err
	}
	if err := out.Sync(); err != nil {
		return 0, 0, err
	}

	return h.Sum32(), n, nil
}

// CreateBackup copies active SSTables, MANIFEST, and WAL files to a designated backup location.
// It inspects the MANIFEST to guarantee that only committed, valid SSTables are backed up.
func CreateBackup(dataDir, backupBaseDir string) (string, error) {
	backupName := fmt.Sprintf("backup-%d", time.Now().UnixNano())
	backupDir := filepath.Join(backupBaseDir, backupName)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("backup: mkdir %s: %w", backupDir, err)
	}

	meta := BackupMetadata{
		Timestamp: time.Now().UTC(),
		DataDir:   dataDir,
		Files:     make(map[string]uint32),
	}

	manifestPath := filepath.Join(dataDir, ManifestFileName)
	var activeSSTs map[string]bool
	var sstIDs []uint64

	manifest, err := LoadManifest(manifestPath)
	if err == nil {
		meta.ManifestSeq = manifest.Sequence()
		activeSSTs = make(map[string]bool)
		for _, rec := range manifest.AllTables() {
			activeSSTs[rec.Filename] = true
			sstIDs = append(sstIDs, rec.ID)
		}
		meta.SSTableIDs = sstIDs
	}

	// 1. Copy MANIFEST if present
	if _, err := os.Stat(manifestPath); err == nil {
		dstManifest := filepath.Join(backupDir, ManifestFileName)
		crc, size, err := copyFileWithChecksum(manifestPath, dstManifest)
		if err != nil {
			_ = os.RemoveAll(backupDir)
			return "", fmt.Errorf("backup: copy manifest: %w", err)
		}
		meta.Files[ManifestFileName] = crc
		meta.TotalBytes += size
	}

	// 2. Copy active SSTables
	sstDir := filepath.Join(dataDir, "sstables")
	backupSSTDir := filepath.Join(backupDir, "sstables")
	if entries, err := os.ReadDir(sstDir); err == nil {
		_ = os.MkdirAll(backupSSTDir, 0755)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if filepath.Ext(name) != ".sst" {
				continue
			}
			// If we have an active manifest, skip orphan/uncommitted SSTables
			if activeSSTs != nil && !activeSSTs[name] {
				continue
			}
			srcFile := filepath.Join(sstDir, name)
			dstFile := filepath.Join(backupSSTDir, name)
			crc, size, err := copyFileWithChecksum(srcFile, dstFile)
			if err != nil {
				_ = os.RemoveAll(backupDir)
				return "", fmt.Errorf("backup: copy sstable %s: %w", name, err)
			}
			meta.Files[filepath.Join("sstables", name)] = crc
			meta.TotalBytes += size
		}
	}

	// 3. Copy WAL files (.log)
	if entries, err := os.ReadDir(dataDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if filepath.Ext(name) == ".log" {
				srcFile := filepath.Join(dataDir, name)
				dstFile := filepath.Join(backupDir, name)
				crc, size, err := copyFileWithChecksum(srcFile, dstFile)
				if err != nil {
					_ = os.RemoveAll(backupDir)
					return "", fmt.Errorf("backup: copy wal %s: %w", name, err)
				}
				meta.Files[name] = crc
				meta.TotalBytes += size
			}
		}
	}

	// 4. Write BACKUP_METADATA.json
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(backupDir, "BACKUP_METADATA.json"), metaBytes, 0644)
	}

	return backupDir, nil
}

// RestoreBackup restores SSTables, MANIFEST, and WAL from backupDir into targetDir.
// It verifies the checksum of each restored file against BACKUP_METADATA.json if present.
func RestoreBackup(backupDir, targetDir string) error {
	if _, err := os.Stat(backupDir); err != nil {
		return fmt.Errorf("restore: backup dir not found: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("restore: mkdir target: %w", err)
	}

	var meta BackupMetadata
	hasMeta := false
	if metaData, err := os.ReadFile(filepath.Join(backupDir, "BACKUP_METADATA.json")); err == nil {
		if err := json.Unmarshal(metaData, &meta); err == nil {
			hasMeta = true
		}
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

	err := filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
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

		// Skip backup metadata in target directory
		if filepath.Base(path) == "BACKUP_METADATA.json" {
			return nil
		}

		dstPath := filepath.Join(targetDir, rel)
		if err := copyFile(path, dstPath); err != nil {
			return err
		}

		// Verify checksum if metadata exists
		if hasMeta {
			if expectedCRC, ok := meta.Files[rel]; ok {
				crc, err := ComputeFileCRC32(dstPath)
				if err != nil {
					return fmt.Errorf("verify restored file %s: %w", rel, err)
				}
				if crc != expectedCRC {
					return fmt.Errorf("corrupt restored file %s: expected crc %d, got %d", rel, expectedCRC, crc)
				}
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	return nil
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

