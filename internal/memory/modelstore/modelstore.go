// Package modelstore installs and checks pinned model files under <home>/models/<id>. It is used
// by the embedding worker and imports nothing from package memory.
package modelstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gomlx/go-huggingface/hub"

	"github.com/google/renameio/v2"
)

const manifestName = "installed.json"

// ErrChecksum means a downloaded file does not match its pinned SHA-256.
var ErrChecksum = errors.New("model: downloaded file failed its checksum")

// File is one downloadable model file, pinned by size and SHA-256.
type File struct {
	Name   string
	URL    string
	SHA256 string
	Size   int64
}

// TotalSize is the sum of the files' sizes.
func TotalSize(files []File) int64 {
	var n int64
	for _, f := range files {
		n += f.Size
	}
	return n
}

// ModelDir is where a model's files live: <home>/models/<id>.
func ModelDir(home, id string) string {
	return filepath.Join(home, "models", id)
}

type manifest struct {
	Model string           `json:"model"`
	Files map[string]int64 `json:"files"`
}

// Installed reports whether dir holds a complete install of model id: the manifest written last by
// Install, naming this model, with every file present at its pinned size. It does not rehash.
func Installed(dir, id string, files []File) bool {
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		return false
	}
	var m manifest
	if json.Unmarshal(b, &m) != nil || m.Model != id {
		return false
	}
	for _, f := range files {
		st, err := os.Stat(filepath.Join(dir, f.Name))
		if err != nil || st.Size() != f.Size || m.Files[f.Name] != f.Size {
			return false
		}
	}
	return true
}

// WriteManifest marks dir as a complete install of model id.
func WriteManifest(dir, id string, files []File) error {
	m := manifest{Model: id, Files: map[string]int64{}}
	for _, f := range files {
		m.Files[f.Name] = f.Size
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := renameio.WriteFile(filepath.Join(dir, manifestName), b, 0o600); err != nil {
		return fmt.Errorf("model: write manifest: %w", err)
	}
	return nil
}

// Install downloads every file into dir and writes the manifest last, so a partial install is
// never mistaken for a complete one. progress reports bytes across all files. A checksum mismatch
// deletes the file and returns ErrChecksum.
func Install(ctx context.Context, dir, id string, files []File, progress func(done, total int64)) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("model: create model dir: %w", err)
	}
	_ = os.Remove(filepath.Join(dir, manifestName))
	mgr := hub.New("").GetDownloadManager()
	total := TotalSize(files)
	var (
		mu       sync.Mutex
		finished int64
	)
	report := func(inFile int64) {
		if progress != nil {
			mu.Lock()
			done := finished + inFile
			mu.Unlock()
			progress(min(done, total), total)
		}
	}
	for _, f := range files {
		dst := filepath.Join(dir, f.Name)
		tmp := dst + ".part"
		_ = os.Remove(tmp)
		if err := mgr.Download(ctx, f.URL, tmp, func(done, _ int64) { report(done) }); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("model: download %s: %w", f.Name, err)
		}
		if err := verifyAndSync(tmp, f); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return fmt.Errorf("model: install %s: %w", f.Name, err)
		}
		mu.Lock()
		finished += f.Size
		mu.Unlock()
		report(0)
	}
	return WriteManifest(dir, id, files)
}

func verifyAndSync(path string, f File) error {
	fh, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("model: open %s: %w", f.Name, err)
	}
	defer fh.Close()
	h := sha256.New()
	n, err := io.Copy(h, fh)
	if err != nil {
		return fmt.Errorf("model: hash %s: %w", f.Name, err)
	}
	if n != f.Size || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
		return fmt.Errorf("%w: %s", ErrChecksum, f.Name)
	}
	if err := fh.Sync(); err != nil {
		return fmt.Errorf("model: sync %s: %w", f.Name, err)
	}
	return nil
}
