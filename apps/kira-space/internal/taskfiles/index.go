package taskfiles

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// Tx is one transaction over the file index. Cache writes of a kind's handler go through Querier so
// they commit with the index rows.
type Tx interface {
	File(taskID, relPath string) (*model.TaskFile, error)
	FilesUnder(taskID, prefix string) ([]model.TaskFile, error)
	PutFile(model.TaskFile) error
	DeleteFile(taskID, relPath string) error
	PutIntent(model.TaskFileIntent) error
	DeleteIntent(taskID, relPath string) error
	Querier() sqlb.Querier
}

// Index is the SQLite side of the file layer: the task_files rows and the write intents.
type Index interface {
	Files(taskID string) ([]model.TaskFile, error)
	Intents() ([]model.TaskFileIntent, error)
	InTx(fn func(Tx) error) error
}

// RepoIndex serves Index from the repository.
func RepoIndex(r *repos.TaskFilesRepo) Index { return repoIndex{r} }

type repoIndex struct{ r *repos.TaskFilesRepo }

func (x repoIndex) Files(taskID string) ([]model.TaskFile, error) { return x.r.Files(taskID) }
func (x repoIndex) Intents() ([]model.TaskFileIntent, error)      { return x.r.Intents() }
func (x repoIndex) InTx(fn func(Tx) error) error {
	return x.r.InTx(func(t *repos.TaskFilesTx) error { return fn(t) })
}
