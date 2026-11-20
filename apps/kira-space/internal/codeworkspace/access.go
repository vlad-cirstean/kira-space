package codeworkspace

import "github.com/kirathecat/kira-studio/internal/fileservice"

// ErrPathEscapesRoot marks a path that leaves the worktree: absolute, "..", or a symlink resolving
// outside it. A repository can hold a symlink pointing anywhere on the machine, so every path crossing
// a boundary goes through Files(root).
var ErrPathEscapesRoot = fileservice.ErrEscapes

// Files is the file Service of a worktree root: rooted, symlinks resolve only inside it. The root
// must exist; nothing is created.
func Files(root string) (*fileservice.Service, error) {
	return fileservice.New(fileservice.Policy{Root: root, Existing: true})
}

// Files is the file Service of the session's root.
func (s *Session) Files() (*fileservice.Service, error) { return Files(s.Root) }
