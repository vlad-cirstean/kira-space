package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/kirathecat/kira-studio/internal/fileservice"
)

const (
	maxSegmentBytes = 255
	maxPathBytes    = 512
)

// Local stores a task's files in a folder of the local filesystem: a thin adapter over a
// fileservice.Service that refuses every symlink, hard link and special file.
type Local struct {
	svc *fileservice.Service
}

// NewLocal returns the store of root, creating it. The root's own symlinks are resolved once;
// nothing below it is ever followed.
func NewLocal(root string) (*Local, error) {
	svc, err := fileservice.New(fileservice.Policy{Root: root, Symlinks: fileservice.Refuse, PlainOnly: true})
	if err != nil {
		return nil, fmt.Errorf("taskfiles/store: %w", err)
	}
	return &Local{svc: svc}, nil
}

// Root is the resolved folder, for the UI's open and reveal actions and the agent's add-dir.
func (l *Local) Root() string { return l.svc.Root() }

// ValidRel reports whether rel is a clean relative slash path with no empty, dot or dot-dot segment.
func ValidRel(rel string) error {
	if len(rel) > maxPathBytes || strings.Contains(rel, "\\") || fileservice.ValidName(rel) != nil {
		return fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}
	for _, seg := range strings.Split(rel, "/") {
		if len(seg) > maxSegmentBytes {
			return fmt.Errorf("%w: %q", ErrInvalidPath, rel)
		}
	}
	return nil
}

func infoOf(i fileservice.Info) Info {
	return Info{Size: i.Size, Version: Version(i.Version), Dir: i.Dir}
}

// mapErr turns a service error into the store's sentinels, keeping the text agents already read.
func mapErr(err error, rel string) error {
	if err == nil {
		return nil
	}
	var pol *fileservice.PolicyError
	if errors.As(err, &pol) {
		switch {
		case errors.Is(pol.Kind, fileservice.ErrInvalidPath), errors.Is(pol.Kind, fileservice.ErrEscapes):
			return fmt.Errorf("%w: %s", ErrInvalidPath, pol.Detail)
		case errors.Is(pol.Kind, fileservice.ErrUnsafe):
			return fmt.Errorf("%w: %s", ErrUnsafe, pol.Detail)
		case errors.Is(pol.Kind, fileservice.ErrTooLarge):
			return fmt.Errorf("%w: %s", ErrTooLarge, pol.Detail)
		case errors.Is(pol.Kind, fileservice.ErrTooManyFiles):
			return fmt.Errorf("%w: %s", ErrTooManyFiles, pol.Detail)
		case errors.Is(pol.Kind, fileservice.ErrConflict):
			return fmt.Errorf("%w: %s", ErrConflict, pol.Detail)
		case errors.Is(pol.Kind, fileservice.ErrExist):
			return fmt.Errorf("%w: %s", ErrExist, pol.Detail)
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotExist, rel)
	}
	return err
}

func (l *Local) Stat(ctx context.Context, rel string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if err := ValidRel(rel); err != nil {
		return Info{}, err
	}
	i, err := l.svc.Stat(rel)
	if err != nil {
		return Info{}, mapErr(err, rel)
	}
	return infoOf(i), nil
}

func (l *Local) Read(ctx context.Context, rel string, max int64) ([]byte, Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}
	if err := ValidRel(rel); err != nil {
		return nil, Info{}, err
	}
	data, i, err := l.svc.Read(rel, max)
	if err != nil {
		return nil, Info{}, mapErr(err, rel)
	}
	return data, infoOf(i), nil
}

func (l *Local) List(ctx context.Context, opt ListOptions) ([]Entry, error) {
	depth := max(opt.MaxDepth, 1)
	var out []Entry
	err := l.svc.Walk(".", fileservice.WalkOptions{MaxDepth: depth, MaxFiles: opt.MaxFiles}, func(e fileservice.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A folder the walk entered is not an entry; one at the depth limit is, listed not entered.
		if e.Dir && strings.Count(e.Name, "/")+1 < depth {
			return nil
		}
		out = append(out, Entry{Rel: e.Name, Info: infoOf(e.Info), Refused: e.Refused})
		return nil
	})
	return out, mapErr(err, "")
}

func (l *Local) WriteAtomic(ctx context.Context, rel string, data []byte, ifVersion *Version) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if err := ValidRel(rel); err != nil {
		return Info{}, err
	}
	var iv *fileservice.Version
	if ifVersion != nil {
		v := fileservice.Version(*ifVersion)
		iv = &v
	}
	i, err := l.svc.Write(rel, data, fileservice.WriteOptions{IfVersion: iv})
	if err != nil {
		return Info{}, mapErr(err, rel)
	}
	return infoOf(i), nil
}

func (l *Local) Remove(ctx context.Context, rel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidRel(rel); err != nil {
		return err
	}
	if _, err := l.svc.Lstat(rel); err != nil {
		return mapErr(err, rel)
	}
	return mapErr(l.svc.RemoveAll(rel), rel)
}

func (l *Local) Rename(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidRel(from); err != nil {
		return err
	}
	if err := ValidRel(to); err != nil {
		return err
	}
	return mapErr(l.svc.Rename(from, to, fileservice.RenameOptions{NoReplace: true}), from)
}
