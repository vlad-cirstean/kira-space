package configfs

import "errors"

// Problem is a validation failure. Line and Col are 1-based, 0 when not line-specific. Path is a
// JSON pointer to the field. Message never echoes the offending value.
type Problem struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ErrInvalid marks a caller mistake: a bad file name or an oversized file.
var ErrInvalid = errors.New("configfs: invalid")

// ErrConflict marks a save built on a file text that has since changed.
var ErrConflict = errors.New("configfs: conflict")
