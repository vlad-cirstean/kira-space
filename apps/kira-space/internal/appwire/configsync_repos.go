package appwire

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configsync"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// repoSyncFiles adapts the repo config store and the registered code repositories to configsync.RepoFiles,
// so configsync never imports repocfg.
type repoSyncFiles struct {
	store *repocfg.Store
	repos *repos.Repos
}

func (r repoSyncFiles) Targets() []configsync.RepoTarget {
	recs, err := r.repos.CodeRepos.List()
	if err != nil {
		return nil
	}
	out := make([]configsync.RepoTarget, 0, len(recs))
	for _, rec := range recs {
		id := repocfg.Key(rec.RepoID)
		file, problem := r.store.FileInfo(id)
		out = append(out, configsync.RepoTarget{RepoID: id, File: file, LocalProblem: problem != nil})
	}
	return out
}

func (r repoSyncFiles) CheckShared(data []byte) string { return r.store.CheckShared(data) }

func (r repoSyncFiles) CheckLocal(file string, data []byte) string {
	return r.store.CheckLocal(file, data)
}

func (r repoSyncFiles) NewFile(repoID string, taken map[string]bool) (string, []byte, error) {
	return r.store.NewFile(repoID, taken)
}

func (r repoSyncFiles) SharedText(repoID, origin string) ([]byte, error) {
	return r.store.SharedText(repoID, origin)
}

func (r repoSyncFiles) Reload() { r.store.Reload() }
