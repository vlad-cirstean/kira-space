// Package store is the storage seam of a task's files. Paths are relative and slash-separated;
// versions are opaque. Only an adapter joins a root, follows the filesystem or reads file times, so
// the layers above (submit, the proof tool, migration, the UI wire) carry nothing host-specific.
package store

import (
	"context"
	"errors"
)

// Version is an opaque token that changes whenever a file's content may have. The local adapter
// uses size plus mtime nanoseconds; object storage would use an ETag.
type Version string

// Info describes one entry.
type Info struct {
	Size    int64
	Version Version
	Dir     bool
}

// Entry is one listed path.
type Entry struct {
	Rel string
	Info
	// Refused is why the entry is unusable (symlink, hard link, special file); "" for a usable one.
	// A refused entry is never followed or read.
	Refused string
}

// ListOptions bound a listing.
type ListOptions struct {
	// MaxDepth is the deepest path, in segments, whose files are listed. A directory at the limit is
	// listed as a Dir entry and not entered.
	MaxDepth int
	// MaxFiles caps the files returned; a listing over it returns ErrTooManyFiles with the first MaxFiles.
	MaxFiles int
}

var (
	// ErrNotExist marks a missing file.
	ErrNotExist = errors.New("taskfiles/store: file does not exist")
	// ErrExist marks a rename onto an existing path.
	ErrExist = errors.New("taskfiles/store: file already exists")
	// ErrInvalidPath marks a path that is not a clean relative slash path.
	ErrInvalidPath = errors.New("taskfiles/store: invalid path")
	// ErrUnsafe marks a symlink, a hard-linked file or a special file.
	ErrUnsafe = errors.New("taskfiles/store: unsafe file")
	// ErrTooLarge marks a file over the read cap.
	ErrTooLarge = errors.New("taskfiles/store: file too large")
	// ErrTooManyFiles marks a listing over MaxFiles.
	ErrTooManyFiles = errors.New("taskfiles/store: too many files")
	// ErrConflict marks a WriteAtomic whose ifVersion no longer matches.
	ErrConflict = errors.New("taskfiles/store: version conflict")
)

// Store is one task's file store.
type Store interface {
	// Stat returns the entry's info without reading it.
	Stat(ctx context.Context, rel string) (Info, error)
	// Read returns the file, ErrTooLarge when it is over max bytes.
	Read(ctx context.Context, rel string, max int64) ([]byte, Info, error)
	// List returns every entry under the root in path order.
	List(ctx context.Context, opt ListOptions) ([]Entry, error)
	// WriteAtomic replaces the file whole, creating folders. A nil ifVersion writes unconditionally;
	// a pointer to "" requires the file to be absent; any other value must equal the current version.
	WriteAtomic(ctx context.Context, rel string, data []byte, ifVersion *Version) (Info, error)
	// Remove deletes a file, or a folder with everything in it.
	Remove(ctx context.Context, rel string) error
	// Rename moves a file or folder, ErrExist when the target is there.
	Rename(ctx context.Context, from, to string) error
}
