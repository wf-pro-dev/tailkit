package tailkit

import (
	"io/fs"
	"os"
	"path/filepath"
)

// readLocalFile reads a local file into memory.
func readLocalFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// writeLocalFile writes data to a local file, creating parent directories.
func writeLocalFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// walkDir returns all regular file paths under dir recursively.
func walkDir(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}
