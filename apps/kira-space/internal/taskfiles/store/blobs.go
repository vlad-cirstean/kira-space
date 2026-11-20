package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// blobMaxRead bounds Get: accepted proof bytes are small next to this, and a blob is never a stream.
const blobMaxRead = 1 << 30

// Blobs is a content-addressed store of accepted file bytes, keyed by SHA-256. A proof folder holds
// several files that restore as one unit, so the accepted bytes of every file live here and a restore
// copies them back byte for byte.
type Blobs struct{ svc *fileservice.Service }

var blobKey = regexp.MustCompile(`^[0-9a-f]{64}$`)

// NewBlobs opens (creating) the store under root.
func NewBlobs(root string) (*Blobs, error) {
	svc, err := fileservice.New(fileservice.Policy{Root: root, Private: true, MaxRead: blobMaxRead})
	if err != nil {
		return nil, fmt.Errorf("store: blobs: %w", err)
	}
	return &Blobs{svc: svc}, nil
}

// SumOf is the key of data.
func SumOf(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (b *Blobs) name(sum string) (string, error) {
	if !blobKey.MatchString(sum) {
		return "", fmt.Errorf("store: bad blob key %q", sum)
	}
	return path.Join(sum[:2], sum), nil
}

// Put stores data and returns its key; a present blob is not rewritten, only touched: a Sweep must
// not take a blob a submit has just reused and not yet committed.
func (b *Blobs) Put(data []byte) (string, error) {
	sum := SumOf(data)
	name, _ := b.name(sum)
	if _, err := b.svc.Stat(name); err == nil {
		if err := b.svc.Chtimes(name, time.Now()); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		return sum, nil
	}
	if _, err := b.svc.Write(name, data, fileservice.WriteOptions{}); err != nil {
		return "", err
	}
	return sum, nil
}

// Get returns the bytes of a key; ok is false when absent.
func (b *Blobs) Get(sum string) ([]byte, bool, error) {
	name, err := b.name(sum)
	if err != nil {
		return nil, false, err
	}
	data, err := b.svc.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

// Remove deletes a key; an absent one is fine.
func (b *Blobs) Remove(sum string) error {
	name, err := b.name(sum)
	if err != nil {
		return err
	}
	return b.svc.Remove(name)
}

// Sweep deletes every blob whose key is not in live and whose mtime is before cutoff, plus stale
// temp files of an interrupted Put (hidden: renameio names them .<key><n>). The age gate covers a blob
// stored by a submit that has not committed its index rows yet. It returns the number of blobs removed.
func (b *Blobs) Sweep(live map[string]struct{}, cutoff time.Time) (int, error) {
	removed := 0
	err := b.svc.Walk(".", fileservice.WalkOptions{}, func(e fileservice.Entry) error {
		if e.Dir {
			return nil
		}
		name := path.Base(e.Name)
		if _, ok := live[name]; ok && blobKey.MatchString(name) {
			return nil
		}
		if !blobKey.MatchString(name) && !strings.HasPrefix(name, ".") {
			return nil
		}
		if !e.ModTime.Before(cutoff) {
			return nil
		}
		if err := b.svc.Remove(e.Name); err != nil {
			return err
		}
		removed++
		return nil
	})
	return removed, err
}
