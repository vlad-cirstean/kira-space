// Package configfs is the shared config-as-files foundation: a folder of plain files under an app
// home, atomic writes, a debounced watch, last good state kept in a DB projection, JSON Schema
// validation with file lines, ${ENV} and ${secret:name} references, and a generated AGENTS.md.
package configfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

// MaxFileBytes caps one config file, refused on read and write.
const MaxFileBytes = 1 << 20

// ErrTooLarge marks a file over MaxFileBytes.
var ErrTooLarge = errors.New("file is larger than 1 MiB")

// WriteAtomic writes data to path through a hidden temp file in the same dir and a rename, creating
// the dir (0755) on first write. A reader or watcher never sees a half-written file.
func WriteAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("configfs: create dir: %w", err)
	}
	if err := renameio.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("configfs: write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// ReadCapped reads a file, returning ErrTooLarge for one over max bytes.
func ReadCapped(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(src) > max {
		return nil, ErrTooLarge
	}
	return src, nil
}

// TextHash identifies a file's text for conflict checks; "" for no file.
func TextHash(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])
}
