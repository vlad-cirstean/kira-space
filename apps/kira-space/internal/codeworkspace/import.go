package codeworkspace

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// Import failure kinds, matched with errors.Is.
var (
	ErrNotRepo         = errors.New("codeworkspace: not a git repository")
	ErrBare            = errors.New("codeworkspace: bare repository")
	ErrLinkedWorktree  = errors.New("codeworkspace: linked worktree")
	ErrAlreadyImported = errors.New("codeworkspace: already imported")
)

type importError struct {
	kind     error
	msg      string
	existing *model.CodeRepo
}

// ExistingRecord returns the already imported record behind an ErrAlreadyImported failure.
func ExistingRecord(err error) (model.CodeRepo, bool) {
	var ie *importError
	if errors.As(err, &ie) && ie.existing != nil {
		return *ie.existing, true
	}
	return model.CodeRepo{}, false
}

func (e *importError) Error() string        { return e.msg }
func (e *importError) Is(target error) bool { return target == e.kind }

// RepoStore is the slice of storage/repos.CodeReposRepo Import needs.
type RepoStore interface {
	List() ([]model.CodeRepo, error)
	Create(model.CodeRepo) (model.CodeRepo, error)
}

// ImportOptions tunes Import. RejectLinkedWorktree is set by the ade folder scan, which must never
// import a worktree checked out under a scanned folder.
type ImportOptions struct {
	RejectLinkedWorktree bool
	// Hidden stores the new record hidden (a scan of a hidden folder).
	Hidden bool
}

// Import identifies path with git (gitPath is the resolved binary), refuses a bare repository, a
// non-repository, an already imported checkout (by RepoID) and, when asked, a linked worktree, then
// stores it. The error messages are the user-facing text.
func Import(ctx context.Context, store RepoStore, runner gitclient.Runner, gitPath, path string, opts ImportOptions) (model.CodeRepo, error) {
	summary, err := gitclient.Identify(ctx, runner, gitPath, path)
	if err != nil {
		return model.CodeRepo{}, &importError{kind: ErrNotRepo, msg: "not a git repository: " + err.Error()}
	}
	if summary.IsBare {
		return model.CodeRepo{}, &importError{kind: ErrBare, msg: "a bare repository has no worktree to browse"}
	}
	if opts.RejectLinkedWorktree && summary.IsLinkedWorktree {
		return model.CodeRepo{}, &importError{kind: ErrLinkedWorktree, msg: summary.Root + " is a linked worktree"}
	}
	existing, err := store.List()
	if err != nil {
		return model.CodeRepo{}, err
	}
	for _, r := range existing {
		if r.RepoID == summary.RepoID {
			return model.CodeRepo{}, &importError{kind: ErrAlreadyImported, msg: r.Name + " is already imported", existing: &r}
		}
	}
	name := filepath.Base(summary.Root)
	rec, err := store.Create(model.CodeRepo{
		ID:        ids.NewID(),
		Name:      name,
		Root:      summary.Root,
		RepoID:    summary.RepoID,
		CreatedAt: kiratime.NowISO(),
		Hidden:    opts.Hidden,
	})
	if errors.Is(err, repos.ErrCodeRepoExists) {
		return model.CodeRepo{}, &importError{kind: ErrAlreadyImported, msg: name + " is already imported"}
	}
	return rec, err
}
