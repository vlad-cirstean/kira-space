// Package fileservice is the one file seam of the app: every read, write, copy, listing and watch of a
// file on disk goes through a Service, and guard G8 (internal/conventiontest) bans the direct os,
// filepath-walk and renameio calls elsewhere. Leaf package: stdlib, renameio and internal/fswatch only.
//
// A Service is a Policy plus, for a rooted one, the resolved root. It holds no file descriptor: a
// rooted operation opens os.Root(root) per call and closes it before returning. Rules:
//
//  1. Names. Rooted: relative slash path, never absolute, no ".." segment ("." only where an operation
//     names the root itself). Under Contain the name is path.Clean-ed first and an absolute or ".."
//     name is ErrEscapes; under Refuse it must already be clean and is ErrInvalidPath. Host-wide:
//     absolute (filepath.Clean-ed), else ErrInvalidPath. Untrusted names (agent, UI, import archive,
//     shared config repo) always go through a rooted Service.
//  2. Contain. EvalSymlinks of the joined path must stay under the resolved root; a dangling symlink
//     anywhere on the path is ErrEscapes; a missing tail resolves through its deepest existing parent.
//     The operation then runs on the resolved, symlink-free name through os.Root, so a link swapped in
//     after the check still cannot escape. Remove, RemoveAll, Rename and Lstat act on a leaf link
//     itself and never resolve it.
//  3. Refuse. Every component is Lstat-ed through the os.Root; a symlink is ErrUnsafe; a non-dir
//     mid-component is ErrNotExist. Reads open O_NOFOLLOW|O_NONBLOCK and fail ErrUnsafe unless
//     os.SameFile(lstat, fstat).
//  4. PlainOnly. A hard link (nlink > 1) or special file is ErrUnsafe on Stat, Read and Write;
//     ReadDir, Walk and Lstat report it through Info.Refused and never read it.
//  5. Atomic write. A hidden temp file in the target dir (renameio), data and file fsync, rename, then
//     an fsync of the parent dir. A dir-fsync failure returns the error; the new content is in place.
//     Parents are created first. IfVersion is compared before the write, so a writer racing between
//     compare and rename still wins (as before the service).
//  6. Perms. Files 0644, dirs 0755; Private 0600 and 0700. WriteOptions.Perm overrides, Static ignores
//     the umask and re-tightens a looser file, KeepPerm copies the existing file's mode.
//  7. Caps. Read(name, max): max 0 means Policy.MaxRead, else DefaultMaxRead. A file over the cap, or
//     growing past it while read, is ErrTooLarge. Walk MaxFiles is ErrTooManyFiles with the first
//     entries returned.
//  8. Errors. Every error is a *fs.PathError whose Path is the rel name (rooted, never the root) or the
//     absolute name (host-wide). OS failures keep their errno, so os.IsNotExist and
//     errors.Is(err, fs.ErrNotExist) work. Policy failures carry a *PolicyError (a sentinel plus detail
//     text). Remove of a missing name is nil.
//  9. Rename. Same Service only; parents of the target are created; both parent dirs are fsynced.
//     NoReplace checks, then renames (that race is documented, not closed). Cross-Service moves use
//     Copy then RemoveAll.
//  10. Copy. Dirs, regular files and symlinks (kept as links, never followed) with source modes,
//     through to+".tmp", verified (type, size, sha256, link target), then renamed.
//  11. Watch. Each name is checked, then watched through internal/fswatch with the absolute roots;
//     events keep the caller's absolute spelling.
package fileservice

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// DefaultMaxRead caps Read when neither the call nor the Policy sets a cap.
const DefaultMaxRead = 64 << 20

// Sentinels. ErrNotExist and ErrExist are the io/fs ones.
var (
	ErrNotExist = fs.ErrNotExist
	ErrExist    = fs.ErrExist
	// ErrInvalidPath marks a name that breaks rule 1, or a file where a folder is needed and back.
	ErrInvalidPath = errors.New("fileservice: invalid path")
	// ErrEscapes marks a name or symlink that leaves the root.
	ErrEscapes = errors.New("fileservice: path escapes root")
	// ErrUnsafe marks a symlink under Refuse, a hard-linked or special file under PlainOnly.
	ErrUnsafe = errors.New("fileservice: unsafe file")
	// ErrTooLarge marks a read over the cap.
	ErrTooLarge = errors.New("fileservice: file too large")
	// ErrTooManyFiles marks a walk over MaxFiles.
	ErrTooManyFiles = errors.New("fileservice: too many files")
	// ErrConflict marks a write whose IfVersion no longer matches.
	ErrConflict = errors.New("fileservice: version conflict")
)

// PolicyError is a policy refusal: Kind is one of the sentinels, Detail the specific text.
type PolicyError struct {
	Kind   error
	Detail string
}

func (e *PolicyError) Error() string { return e.Kind.Error() + ": " + e.Detail }
func (e *PolicyError) Unwrap() error { return e.Kind }

func policy(kind error, format string, args ...any) error {
	return &PolicyError{Kind: kind, Detail: fmt.Sprintf(format, args...)}
}

// SymlinkRule says what a rooted Service does with symlinks.
type SymlinkRule uint8

const (
	// Contain lets a symlink resolve only inside Root (host-wide: links are followed).
	Contain SymlinkRule = iota
	// Refuse makes any symlink component ErrUnsafe; a leaf is opened O_NOFOLLOW.
	Refuse
)

// Policy configures a Service.
type Policy struct {
	Root      string      // "" = host-wide (absolute names); else clean relative slash names under Root
	Symlinks  SymlinkRule // rooted only
	PlainOnly bool        // refuse hard-linked and special files
	Private   bool        // new files 0600, dirs 0700; default 0644 / 0755
	MaxRead   int64       // Read cap when the call passes 0; 0 = DefaultMaxRead
}

// Service is one file policy. Safe for concurrent use.
type Service struct {
	p    Policy
	root string // resolved Root, "" host-wide
}

var host = &Service{}

// Host is the shared host-wide Service: names are absolute, symlinks are followed.
func Host() *Service { return host }

// New returns the Service of p. A rooted Policy creates Root and resolves its own symlinks once;
// nothing below it is resolved again except under Contain.
func New(p Policy) (*Service, error) {
	if p.Root == "" {
		return &Service{p: p}, nil
	}
	s := &Service{p: p}
	if err := os.MkdirAll(p.Root, s.dirPerm()); err != nil {
		return nil, fmt.Errorf("fileservice: create root: %w", err)
	}
	real, err := filepath.EvalSymlinks(p.Root)
	if err != nil {
		return nil, fmt.Errorf("fileservice: resolve root: %w", err)
	}
	if real, err = filepath.Abs(real); err != nil {
		return nil, fmt.Errorf("fileservice: resolve root: %w", err)
	}
	s.root = real
	return s, nil
}

// Root is the resolved root folder, "" for a host-wide Service.
func (s *Service) Root() string { return s.root }

func (s *Service) dirPerm() fs.FileMode {
	if s.p.Private {
		return 0o700
	}
	return 0o755
}

func (s *Service) filePerm() fs.FileMode {
	if s.p.Private {
		return 0o600
	}
	return 0o644
}

func (s *Service) maxRead(max int64) int64 {
	switch {
	case max > 0:
		return max
	case s.p.MaxRead > 0:
		return s.p.MaxRead
	}
	return DefaultMaxRead
}

// Version identifies a file's content cheaply: size plus mtime nanoseconds.
type Version string

// Info describes one entry.
type Info struct {
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	Dir     bool
	Version Version
	// Refused is why the entry is unusable ("symlink", "hard link", "special file"); "" for a usable one.
	Refused string
}

// Entry is one listed path: Name is the base name (ReadDir) or the slash path under the walked dir (Walk).
type Entry struct {
	Name string
	Info
}

// WriteOptions tune Write and Create.
type WriteOptions struct {
	Perm      fs.FileMode // 0 = policy default
	KeepPerm  bool        // keep the existing file's mode
	Static    bool        // Perm ignores the umask and re-tightens a looser file
	IfVersion *Version    // nil unconditional; "" must be absent; else must equal the current version
}

// RenameOptions tune Rename.
type RenameOptions struct{ NoReplace bool }

// WalkOptions bound a Walk.
type WalkOptions struct {
	MaxDepth int                   // deepest listed path in segments; a dir at the limit is listed, not entered; 0 = none
	MaxFiles int                   // over it: ErrTooManyFiles with the first MaxFiles files; 0 = none
	SkipDir  func(rel string) bool // true = the dir is neither listed nor entered
}

func versionOf(fi fs.FileInfo) Version {
	return Version(fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano()))
}

func (s *Service) infoOf(fi fs.FileInfo) Info {
	return Info{Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime(), Dir: fi.IsDir(), Version: versionOf(fi), Refused: s.refusal(fi)}
}

// refusal names why fi is unusable; "" for a usable file or a folder.
func (s *Service) refusal(fi fs.FileInfo) string {
	if !s.p.PlainOnly && s.p.Symlinks != Refuse {
		return ""
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return "symlink"
	case fi.IsDir() || !s.p.PlainOnly:
		return ""
	case !fi.Mode().IsRegular():
		return "special file"
	case linkCount(fi) > 1:
		return "hard link"
	}
	return ""
}
