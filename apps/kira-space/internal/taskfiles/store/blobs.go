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

// Put stores data and returns its key; a present blob is not rewritten.
func (b *Blobs) Put(data []byte) (string, error) {
	sum := SumOf(data)
	p, _ := b.path(sum)
	if _, err := os.Stat(p); err == nil {
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
