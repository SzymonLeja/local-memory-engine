package ingest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileEntry struct {
	Path         string
	Hash         string
	LastModified time.Time
}

func sanitizePath(vaultRoot, userPath string) (string, error) {
	full := filepath.Join(vaultRoot, userPath)
	clean := filepath.Clean(full)
	vaultClean := filepath.Clean(vaultRoot)

	if !strings.HasPrefix(clean, vaultClean+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid path: must be inside vault")
	}

	return clean, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func WalkVault(vaultRoot string, relPath string) ([]FileEntry, error) {
	root, err := sanitizePath(vaultRoot, relPath)
	if err != nil {
		return nil, err
	}

	var entries []FileEntry

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		for _, ignored := range []string{".git", "node_modules", ".cache", ".obsidian"} {
			if strings.Contains(path, string(filepath.Separator)+ignored+string(filepath.Separator)) {
				return nil
			}
		}

		hash, err := hashFile(path)
		if err != nil {
			return err
		}

		relFilePath, err := filepath.Rel(vaultRoot, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		entries = append(entries, FileEntry{
			Path:         relFilePath,
			Hash:         hash,
			LastModified: info.ModTime(),
		})

		return nil
	})

	return entries, err
}
