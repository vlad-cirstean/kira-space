package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/renameio/v2"
)

const (
	maxSegmentBytes = 255
	maxPathBytes    = 512
)

// Local stores a task's files in a folder of the local filesystem.
type Local struct {
	root string
}

// NewLocal returns the store of root, creating it. The root's own symlinks are resolved once;
// nothing below it is ever followed.
func NewLocal(root string) (*Local, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("taskfiles/store: create root: %w", err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("taskfiles/store: resolve root: %w", err)
	}
	return &Local{root: real}, nil
}

// Root is the resolved folder, for the UI's open and reveal actions and the agent's add-dir.
func (l *Local) Root() string { return l.root }

// ValidRel reports whether rel is a clean relative slash path with no empty, dot or dot-dot segment.
func ValidRel(rel string) error {
	switch {
	case rel == "" || len(rel) > maxPathBytes:
		return fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	case strings.ContainsAny(rel, "\\\x00") || strings.HasPrefix(rel, "/") || path.Clean(rel) != rel:
		return fmt.Errorf("%w: %q", ErrInvalidPath, rel)
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "." || seg == ".." || len(seg) > maxSegmentBytes {
			return fmt.Errorf("%w: %q", ErrInvalidPath, rel)
		}
	}
	return nil
}

// version is size plus mtime nanoseconds.
func version(fi fs.FileInfo) Version {
	return Version(fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano()))
}

func infoOf(fi fs.FileInfo) Info {
	return Info{Size: fi.Size(), Version: version(fi), Dir: fi.IsDir()}
}

// lstat resolves rel under the root, refusing a symlink at any component. A missing leaf returns
// the not-exist error with the joined path, so a writer can still create it.
func (l *Local) lstat(rel string) (string, fs.FileInfo, error) {
	if err := ValidRel(rel); err != nil {
		return "", nil, err
	}
	cur := l.root
	segs := strings.Split(rel, "/")
	var fi fs.FileInfo
	for i, seg := range segs {
		cur = filepath.Join(cur, seg)
		var err error
		fi, err = os.Lstat(cur)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.Join(l.root, filepath.FromSlash(rel)), nil, fmt.Errorf("%w: %s", ErrNotExist, rel)
			}
			return "", nil, err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("%w: %s is a symlink", ErrUnsafe, rel)
		}
		if i < len(segs)-1 && !fi.IsDir() {
			return "", nil, fmt.Errorf("%w: %s", ErrNotExist, rel)
		}
	}
	return cur, fi, nil
}

// refusal names why a file is unusable, "" when it is a plain single-link file or a folder.
func refusal(fi fs.FileInfo) string {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return "symlink"
	case fi.IsDir():
		return ""
	case !fi.Mode().IsRegular():
		return "special file"
	case linkCount(fi) > 1:
		return "hard link"
	}
	return ""
}

func (l *Local) Stat(ctx context.Context, rel string) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	_, fi, err := l.lstat(rel)
	if err != nil {
		return Info{}, err
	}
	if why := refusal(fi); why != "" {
		return Info{}, fmt.Errorf("%w: %s is a %s", ErrUnsafe, rel, why)
	}
	return infoOf(fi), nil
}

func (l *Local) Read(ctx context.Context, rel string, max int64) ([]byte, Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}
	p, lfi, err := l.lstat(rel)
	if err != nil {
		return nil, Info{}, err
	}
	if lfi.IsDir() {
		return nil, Info{}, fmt.Errorf("%w: %s is a folder", ErrInvalidPath, rel)
	}
	if why := refusal(lfi); why != "" {
		return nil, Info{}, fmt.Errorf("%w: %s is a %s", ErrUnsafe, rel, why)
	}
	if lfi.Size() > max {
		return nil, Info{}, fmt.Errorf("%w: %s is %d bytes, the cap is %d", ErrTooLarge, rel, lfi.Size(), max)
	}
	f, err := os.OpenFile(p, openFlags, 0)
	if err != nil {
		return nil, Info{}, fmt.Errorf("%w: %s: %v", ErrUnsafe, rel, err)
	}
	defer f.Close()
	ffi, err := f.Stat()
	if err != nil {
		return nil, Info{}, err
	}
	if !os.SameFile(lfi, ffi) { // swapped between lstat and open
		return nil, Info{}, fmt.Errorf("%w: %s changed while opening", ErrUnsafe, rel)
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, Info{}, err
	}
	if int64(len(data)) > max {
		return nil, Info{}, fmt.Errorf("%w: %s grew past %d bytes", ErrTooLarge, rel, max)
	}
	return data, infoOf(ffi), nil
}

// lister is one List walk: it collects entries and counts files against the cap.
type lister struct {
	ctx   context.Context
	opt   ListOptions
	out   []Entry
	files int
}

func (w *lister) walk(dir, relDir string, depth int) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	for _, e := range ents {
		if err := w.visit(dir, relDir, depth, e); err != nil {
			return err
		}
	}
	return nil
}

func (w *lister) visit(dir, relDir string, depth int, e os.DirEntry) error {
	rel := e.Name()
	if relDir != "" {
		rel = relDir + "/" + e.Name()
	}
	fi, err := e.Info()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	ent := Entry{Rel: rel, Info: infoOf(fi), Refused: refusal(fi)}
	if fi.IsDir() {
		if depth >= w.opt.MaxDepth {
			w.out = append(w.out, ent)
			return nil
		}
		return w.walk(filepath.Join(dir, e.Name()), rel, depth+1)
	}
	if w.files++; w.opt.MaxFiles > 0 && w.files > w.opt.MaxFiles {
		return ErrTooManyFiles
	}
	w.out = append(w.out, ent)
	return nil
}

func (l *Local) List(ctx context.Context, opt ListOptions) ([]Entry, error) {
	w := &lister{ctx: ctx, opt: opt}
	err := w.walk(l.root, "", 1)
	if errors.Is(err, ErrTooManyFiles) {
		return w.out, fmt.Errorf("%w: more than %d files", ErrTooManyFiles, opt.MaxFiles)
	}
	return w.out, err
}

func (l *Local) WriteAtomic(ctx context.Context, rel string, data []byte, ifVersion *Version) (Info, error) {
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	p, fi, err := l.lstat(rel)
	switch {
	case errors.Is(err, ErrNotExist):
		fi = nil
		if ifVersion != nil && *ifVersion != "" {
			return Info{}, fmt.Errorf("%w: %s is gone", ErrConflict, rel)
		}
	case err != nil:
		return Info{}, err
	default:
		if fi.IsDir() {
			return Info{}, fmt.Errorf("%w: %s is a folder", ErrInvalidPath, rel)
		}
		if why := refusal(fi); why != "" {
			return Info{}, fmt.Errorf("%w: %s is a %s", ErrUnsafe, rel, why)
		}
		if ifVersion != nil && *ifVersion != version(fi) {
			return Info{}, fmt.Errorf("%w: %s changed", ErrConflict, rel)
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return Info{}, fmt.Errorf("taskfiles/store: create folder for %s: %w", rel, err)
	}
	if err := renameio.WriteFile(p, data, 0o644); err != nil {
		return Info{}, fmt.Errorf("taskfiles/store: write %s: %w", rel, err)
	}
	nfi, err := os.Lstat(p)
	if err != nil {
		return Info{}, err
	}
	return infoOf(nfi), nil
}

func (l *Local) Remove(ctx context.Context, rel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, _, err := l.lstat(rel)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func (l *Local) Rename(ctx context.Context, from, to string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, _, err := l.lstat(from)
	if err != nil {
		return err
	}
	dst, _, err := l.lstat(to)
	if err == nil {
		return fmt.Errorf("%w: %s", ErrExist, to)
	}
	if !errors.Is(err, ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dst)
}
