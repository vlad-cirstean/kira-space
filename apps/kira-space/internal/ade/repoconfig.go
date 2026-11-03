package ade

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/codeworkspace"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

func (b *TaskBoard) notifyRepos() {
	if b.deps.OnRepos != nil {
		b.deps.OnRepos()
	}
}

// repoConfig finds one repo's config row.
func (b *TaskBoard) repoConfig(codeRepoID string) (model.AdeRepoConfig, error) {
	configs, err := b.deps.RepoConfig.List()
	if err != nil {
		return model.AdeRepoConfig{}, err
	}
	for _, c := range configs {
		if c.CodeRepoID == codeRepoID {
			return c, nil
		}
	}
	return model.AdeRepoConfig{}, invalid("code repo %s not found", codeRepoID)
}

// RepoConfigChanged applies a batch of repo file changes: marks and env state of removed targets and
// environments are dropped, changed environments rerun their scripts, then the repo list and the
// board refresh once.
func (b *TaskBoard) RepoConfigChanged(changes []repocfg.Change) {
	if b.deps.CodeRepos == nil {
		return
	}
	recs, err := b.deps.CodeRepos.List()
	if err != nil {
		slog.Warn("ade: list code repos", "scope", "ade", "err", err)
		return
	}
	codeRepoOf := make(map[string]string, len(recs))
	for _, r := range recs {
		codeRepoOf[r.RepoID] = r.ID
	}
	touched := false
	for _, ch := range changes {
		id, ok := codeRepoOf[ch.Path]
		if !ok {
			continue
		}
		touched = true
		if err := b.pruneRemoved(id, ch); err != nil {
			slog.Warn("ade: prune removed targets", "scope", "ade", "repo", id, "err", err)
		}
		if !slices.Equal(ch.Old.Environments, ch.New.Environments) {
			b.goTracked(func() { b.refreshEnvScripts(id) })
		}
	}
	if touched {
		b.notifyRepos()
		b.notifyBoard()
	}
}

// pruneRemoved drops the marks and env state of integration targets and environments the change removed.
func (b *TaskBoard) pruneRemoved(id string, ch repocfg.Change) error {
	if b.deps.Facts == nil {
		return nil
	}
	if !slices.Equal(ch.Old.IntegrationBranches, ch.New.IntegrationBranches) {
		if err := b.deps.Facts.DeleteMarksNotIn("target", id, ch.New.IntegrationBranches); err != nil {
			return err
		}
	}
	if !slices.Equal(ch.Old.Environments, ch.New.Environments) {
		names := make([]string, len(ch.New.Environments))
		for i, e := range ch.New.Environments {
			names[i] = e.Name
		}
		if err := b.deps.Facts.DeleteMarksNotIn("env", id, names); err != nil {
			return err
		}
		return b.deps.Facts.DeleteEnvStateNotIn(id, names)
	}
	return nil
}

// withoutMain drops the repo's main branch from a configured target list: main is never an
// integration target, and a file may list it.
func withoutMain(targets []string, main string) []string {
	if main == "" || !slices.Contains(targets, main) {
		return targets
	}
	return slices.DeleteFunc(slices.Clone(targets), func(t string) bool { return t == main })
}

// --- folders -------------------------------------------------------------------------------------

func wireFolder(f model.AdeFolder) adewire.Folder {
	return adewire.Folder{Path: f.Path, Watch: f.Watch, Hidden: f.Hidden, RepoCount: f.RepoCount, HiddenCount: f.HiddenCount}
}

func (b *TaskBoard) folderByPath(path string) (adewire.Folder, bool, error) {
	folders, err := b.deps.RepoConfig.Folders()
	if err != nil {
		return adewire.Folder{}, false, err
	}
	for _, f := range folders {
		if f.Path == path {
			return wireFolder(f), true, nil
		}
	}
	return adewire.Folder{}, false, nil
}

// AddFolder records path, imports every repo found under it and, when watch is set, keeps looking
// for new ones.
func (b *TaskBoard) AddFolder(ctx context.Context, path string, watch bool) (adewire.FolderImportResult, error) {
	if !filepath.IsAbs(path) {
		return adewire.FolderImportResult{}, invalid("path must be absolute")
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return adewire.FolderImportResult{}, invalid("%s is not a folder", path)
	}
	if err := b.deps.RepoConfig.AddFolder(path, watch); err != nil {
		return adewire.FolderImportResult{}, err
	}
	imported, err := b.importFolder(ctx, path)
	if err != nil {
		return adewire.FolderImportResult{}, err
	}
	if watch {
		b.startFolderWatch(path)
	} else {
		b.stopFolderWatch(path)
	}
	folder, _, err := b.folderByPath(path)
	if err != nil {
		return adewire.FolderImportResult{}, err
	}
	b.notifyRepos()
	return adewire.FolderImportResult{Folder: folder, Imported: imported}, nil
}

// SetFolderWatch turns the background look for new repos on or off.
func (b *TaskBoard) SetFolderWatch(_ context.Context, path string, watch bool) (adewire.Folder, error) {
	path = filepath.Clean(path)
	ok, err := b.deps.RepoConfig.SetFolderWatch(path, watch)
	if err != nil {
		return adewire.Folder{}, err
	}
	if !ok {
		return adewire.Folder{}, invalid("folder %s is not in the list", path)
	}
	if watch {
		b.startFolderWatch(path)
	} else {
		b.stopFolderWatch(path)
	}
	folder, _, err := b.folderByPath(path)
	if err != nil {
		return adewire.Folder{}, err
	}
	b.notifyRepos()
	return folder, nil
}

// SetFolderHidden hides or shows every repo imported from path; later discoveries in the folder
// import hidden while the flag is set.
func (b *TaskBoard) SetFolderHidden(_ context.Context, path string, hidden bool) (adewire.Folder, error) {
	path = filepath.Clean(path)
	ok, err := b.deps.RepoConfig.SetFolderHidden(path, hidden)
	if err != nil {
		return adewire.Folder{}, err
	}
	if !ok {
		return adewire.Folder{}, invalid("folder %s is not in the list", path)
	}
	folder, _, err := b.folderByPath(path)
	if err != nil {
		return adewire.Folder{}, err
	}
	b.notifyRepos()
	return folder, nil
}

// RemoveFolder forgets path. Repos imported from it stay imported (Git-module tabs would close
// otherwise) and become plain added repos.
func (b *TaskBoard) RemoveFolder(_ context.Context, path string) error {
	path = filepath.Clean(path)
	b.stopFolderWatch(path)
	ok, err := b.deps.RepoConfig.RemoveFolder(path)
	if err != nil {
		return err
	}
	if !ok {
		return invalid("folder %s is not in the list", path)
	}
	b.notifyRepos()
	return nil
}

// importFolder scans path and imports every repo not yet known; it returns the new code repo ids.
func (b *TaskBoard) importFolder(ctx context.Context, path string) ([]string, error) {
	b.importMu.Lock()
	defer b.importMu.Unlock()
	scan := scanFolder(path)
	imported := make([]string, 0, len(scan.repos))
	revs := make([]repos.ImportedRepo, 0, len(scan.repos))
	if len(scan.repos) == 0 {
		return imported, nil
	}
	status := b.deps.GitStatus(ctx)
	if status.Kind != "ok" {
		return nil, errors.New("ade: git is unavailable: " + status.Kind)
	}
	folderHidden, err := b.deps.RepoConfig.FolderHidden(path)
	if err != nil {
		return nil, err
	}
	for _, root := range scan.repos {
		rec, err := codeworkspace.Import(ctx, b.deps.CodeRepos, b.deps.Runner, status.Path, root,
			codeworkspace.ImportOptions{RejectLinkedWorktree: true, Hidden: folderHidden})
		switch {
		case err == nil:
		case errors.Is(err, codeworkspace.ErrAlreadyImported), errors.Is(err, codeworkspace.ErrLinkedWorktree),
			errors.Is(err, codeworkspace.ErrBare), errors.Is(err, codeworkspace.ErrNotRepo):
			continue
		default:
			slog.Warn("ade folder import", "scope", "ade", "path", root, "err", err)
			continue
		}
		if err := b.deps.RepoConfig.SetSource(rec.ID, path); err != nil {
			return nil, err
		}
		imported = append(imported, rec.ID)
		revs = append(revs, repos.ImportedRepo{ID: rec.ID, Rev: rec.HiddenRev})
		if b.afterImport != nil {
			b.afterImport(rec.ID)
		}
	}
	// SetFolderHidden does not wait for a scan: a flag flipped mid-scan is re-applied to the repos
	// this scan imported, which the flip's own UPDATE could not have seen. Only repos untouched
	// since import (same hidden_rev) are written, so a repo the user toggled meanwhile keeps its
	// state; the flag is read inside the UPDATE, so a later flip is never overwritten.
	if now, err := b.deps.RepoConfig.FolderHidden(path); err == nil && now != folderHidden {
		if err := b.deps.RepoConfig.ApplyFolderHidden(path, revs); err != nil {
			return nil, err
		}
	}
	return imported, nil
}
