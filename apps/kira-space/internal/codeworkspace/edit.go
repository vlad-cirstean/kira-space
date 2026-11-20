package codeworkspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

var (
	// ErrConflict means the file on disk no longer matches the caller's base hash.
	ErrConflict = errors.New("codeworkspace: file changed on disk")
	// ErrTooLarge means the text to save exceeds MaxReadBytes, so the viewer could not reopen it.
	ErrTooLarge = errors.New("codeworkspace: text exceeds the size limit")
	// ErrNotRegular means the save target exists but is not a regular file.
	ErrNotRegular = errors.New("codeworkspace: target is not a regular file")
	// ErrInvalidUTF8 means the text to save is not valid UTF-8.
	ErrInvalidUTF8 = errors.New("codeworkspace: text is not valid UTF-8")
)

// EditContent is the edit session's read: FileContent's classification plus the on-disk hash that
// guards the later save, and `notUtf8` for a found file that JSON cannot carry byte-exact.
type EditContent struct {
	Kind     string `json:"kind"` // found | missing | binary | tooLarge | notUtf8
	Text     string `json:"text"`
	Hash     string `json:"hash"`
	Language string `json:"language"`
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ReadFileForEdit reads name like ReadFile and adds the SHA-256 of the on-disk bytes. Text that
// is not valid UTF-8 is refused: JSON would replace the bad bytes and a save would corrupt the file.
func ReadFileForEdit(svc *fileservice.Service, name string) (EditContent, error) {
	fc, err := ReadFile(svc, name)
	if err != nil {
		return EditContent{}, err
	}
	out := EditContent{Kind: fc.Kind, Language: fc.Language}
	if fc.Kind != "found" {
		return out, nil
	}
	if !utf8.ValidString(fc.Text) {
		out.Kind = "notUtf8"
		return out, nil
	}
	out.Text = fc.Text
	out.Hash = hashBytes([]byte(fc.Text))
	return out, nil
}

// currentHash hashes name's bytes, "" when it is absent. Callers have already type-checked it; the read
// is bounded so a file grown past the limit cannot load unbounded (a file over the bound never matches
// a base hash, so it is a conflict).
func currentHash(svc *fileservice.Service, name string) (string, error) {
	data, _, err := svc.Read(name, MaxReadBytes+1)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case errors.Is(err, fileservice.ErrTooLarge):
		return "", ErrConflict
	case err != nil:
		return "", fmt.Errorf("codeworkspace: read for save: %w", err)
	}
	return hashBytes(data), nil
}

// WriteFile atomically replaces name with text when its current hash equals baseHash ("" means
// the file must be absent), and returns the new hash. The caller serializes calls (steps hash,
// compare, replace are not atomic against an external writer). Existing permissions are kept; a
// recreated file gets 0644 minus umask.
func WriteFile(svc *fileservice.Service, name, text, baseHash string) (string, error) {
	if len(text) > MaxReadBytes {
		return "", ErrTooLarge
	}
	if !utf8.ValidString(text) {
		return "", ErrInvalidUTF8
	}
	info, err := svc.Stat(name)
	switch {
	case err == nil && !info.Mode.IsRegular():
		return "", ErrNotRegular
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("codeworkspace: stat for save: %w", err)
	}
	cur, err := currentHash(svc, name)
	if err != nil {
		return "", err
	}
	if cur != baseHash {
		return "", ErrConflict
	}
	if _, err := svc.Write(name, []byte(text), fileservice.WriteOptions{KeepPerm: true}); err != nil {
		return "", fmt.Errorf("codeworkspace: save %s: %w", name, err)
	}
	return hashBytes([]byte(text)), nil
}
