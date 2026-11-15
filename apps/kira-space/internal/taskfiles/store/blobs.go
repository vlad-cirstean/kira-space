package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Blobs is a content-addressed store of accepted file bytes, keyed by SHA-256. A proof folder holds
// several files that restore as one unit, so the accepted bytes of every file live here and a restore
// copies them back byte for byte.
type Blobs struct{ root string }

var blobKey = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NewBlobs opens (creating) the store under root.
func NewBlobs(root string) (*Blobs, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("store: blobs: %w", err)
	}
	return &Blobs{root: root}, nil
}

// SumOf is the key of data.
func SumOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (b *Blobs) path(sum string) (string, error) {
	if !blobKey.MatchString(sum) {
		return "", fmt.Errorf("store: bad blob key %q", sum)
	}
	return filepath.Join(b.root, sum[:2], sum), nil
}

// Put stores data and returns its key; a present blob is not rewritten, only touched: a Sweep must
// not take a blob a submit has just reused and not yet committed.
func (b *Blobs) Put(data []byte) (string, error) {
	sum := SumOf(data)
	p, _ := b.path(sum)
	if _, err := os.Stat(p); err == nil {
		now := time.Now()
		if err := os.Chtimes(p, now, now); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		return sum, nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".put-*")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return "", errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), p); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return sum, nil
}

// Get returns the bytes of a key; ok is false when absent.
func (b *Blobs) Get(sum string) ([]byte, bool, error) {
	p, err := b.path(sum)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// Remove deletes a key; an absent one is fine.
func (b *Blobs) Remove(sum string) error {
	p, err := b.path(sum)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Sweep deletes every blob whose key is not in live and whose mtime is before cutoff, plus stale
// temp files of an interrupted Put. The age gate covers a blob stored by a submit that has not
// committed its index rows yet. It returns the number of blobs removed.
func (b *Blobs) Sweep(live map[string]struct{}, cutoff time.Time) (int, error) {
	removed := 0
	err := filepath.WalkDir(b.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if _, ok := live[name]; ok && blobKey.MatchString(name) {
			return nil
		}
		if !blobKey.MatchString(name) && !strings.HasPrefix(name, ".put-") {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !fi.ModTime().Before(cutoff) {
			return nil
		}
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		removed++
		return nil
	})
	return removed, err
}
