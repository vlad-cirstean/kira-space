// Package configfs is the shared config-as-files foundation: a folder of plain files under an app
// home, atomic writes, a debounced watch, last good state kept in a DB projection, JSON Schema
// validation with file lines, ${ENV} and ${secret:name} references, and a generated AGENTS.md.
package configfs

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// MaxFileBytes caps one config file, refused on read and write. Readers pass it to fileservice.Read.
const MaxFileBytes = 1 << 20

// ErrTooLarge marks a file over MaxFileBytes.
var ErrTooLarge = errors.New("file is larger than 1 MiB")

// TextHash identifies a file's text for conflict checks; "" for no file.
func TextHash(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])
}
