package gitrpc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// repoSettingsSnapshotFrom projects storage/model.GitRepoSettings onto the wire's own dotted-key
// shape. ReviewBaseCandidates is normalised to a non-nil slice — a repo that has never
// patched it reads back model.DefaultGitRepoSettings()'s own literal, never nil, but this keeps
// the projection defensive against a future zero-value GitRepoSettings reaching here directly.
func repoSettingsSnapshotFrom(s model.GitRepoSettings) RepoSettingsSnapshot {
	candidates := s.ReviewBaseCandidates
	if candidates == nil {
		candidates = []string{}
	}
	return RepoSettingsSnapshot{
		GraphPageSize:         s.GraphPageSize,
		GraphScope:            s.GraphScope,
		StashShowInGraph:      s.StashShowInGraph,
		StashIncludeUntracked: s.StashIncludeUntracked,
		ReviewBaseCandidates:  candidates,
		PullStrategy:          s.PullStrategy,
		GitHostProvider:       s.GitHostProvider,
		WorktreePrepareScript: s.WorktreePrepareScript,
		WorktreeBasePath:      s.WorktreeBasePath,
		CheckoutAutoStash:     s.CheckoutAutoStash,
	}
}

// toModel converts the wire's own dotted-key patch into storage/model's own patch shape — a plain
// field-for-field rename, since both are already "every leaf optional".
// WorktreePrepareScript is deliberately not mapped: handleRepoSettingsSet refuses it first.
//
// WorktreeBasePath is a client-supplied directory parameter, normalized to NFC when set to a
// non-empty value — nil (leave unset) and "" (explicitly reset to no override,
// storage/repos.gitreposettings_test.go's own precedent) both stay exactly as the client sent
// them; CleanNFC("") would turn a deliberate reset into ".", which is not what an empty patch
// value means here.
func (p RepoSettingsPatchWire) toModel() model.GitRepoSettingsPatch {
	worktreeBasePath := p.WorktreeBasePath
	if worktreeBasePath != nil && *worktreeBasePath != "" {
		v := gitpath.CleanNFC(*worktreeBasePath)
		worktreeBasePath = &v
	}
	return model.GitRepoSettingsPatch{
		GraphPageSize:         p.GraphPageSize,
		GraphScope:            p.GraphScope,
		StashShowInGraph:      p.StashShowInGraph,
		StashIncludeUntracked: p.StashIncludeUntracked,
		ReviewBaseCandidates:  p.ReviewBaseCandidates,
		PullStrategy:          p.PullStrategy,
		GitHostProvider:       p.GitHostProvider,
		WorktreeBasePath:      worktreeBasePath,
		CheckoutAutoStash:     p.CheckoutAutoStash,
	}
}

func (r *Router) handleRepoSettingsGet(_ context.Context, _ *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleCall("repoSettings.get", params,
		func(p RepoSettingsGetParams) error {
			return requireNonEmpty("repoSettings.get", "repoId", p.RepoID)
		},
		func(p RepoSettingsGetParams) (RepoSettingsSnapshot, error) {
			// F8 (P108 Part 17 review): every other repo-keyed handler normalizes to NFC before use
			// (G27 D6/G31 #4) — this one didn't, so a decomposed (non-NFC) repoId read back
			// whatever the storage layer's own NFC-normalized RepoEntry.RepoSettings() lookup
			// never actually matched.
			p.RepoID = gitpath.CleanNFC(p.RepoID)
			s, err := r.deps.Registry.RepoSettingsGet(p.RepoID)
			if err != nil {
				return RepoSettingsSnapshot{}, ipcerr.Internal("gitrpc: repoSettings.get: " + err.Error())
			}
			return repoSettingsSnapshotFrom(s), nil
		},
	)
}

// handleRepoSettingsSet writes the patch, then emits repoSettings.changed to EVERY currently
// connected client (D7) — not only the one that made the change, via this Router's own
// notify.Emitter[RepoSettingsChangedPayload] (repoSettingsChanged, ForConn below).
func (r *Router) handleRepoSettingsSet(_ context.Context, _ *gitsession.Conn, params json.RawMessage) (any, error) {
	return handleCall("repoSettings.set", params,
		func(p RepoSettingsSetParams) error {
			if p.Patch.WorktreePrepareScript != nil {
				return ipcerr.T(ipcerr.CodeReadOnly, "backend.space.gitrpc.prepareScriptSpaceOnly.body", nil)
			}
			return requireNonEmpty("repoSettings.set", "repoId", p.RepoID)
		},
		func(p RepoSettingsSetParams) (RepoSettingsSnapshot, error) {
			// Normalize before both the storage write and the emitted
			// event, matching repo.changed's own always-NFC convention elsewhere in this codebase
			// — a client sending a decomposed repoId must not write settings rows that no entry
			// ever reads back, nor have repoSettings.changed echo the raw, possibly non-NFC
			// spelling.
			p.RepoID = gitpath.CleanNFC(p.RepoID)
			snapshot, err := r.setRepoSettings(p.RepoID, p.Patch.toModel())
			if err != nil {
				var structured *ipcerr.Error
				if errors.As(err, &structured) {
					return RepoSettingsSnapshot{}, structured
				}
				return RepoSettingsSnapshot{}, ipcerr.BadRequest("gitrpc: repoSettings.set: " + err.Error())
			}
			return snapshot, nil
		},
	)
}

func (r *Router) setRepoSettings(repoID string, patch model.GitRepoSettingsPatch) (RepoSettingsSnapshot, error) {
	s, err := r.deps.Registry.RepoSettingsSet(repoID, patch)
	if err != nil {
		return RepoSettingsSnapshot{}, err
	}
	snapshot := repoSettingsSnapshotFrom(s)
	r.repoSettingsChanged.Emit(RepoSettingsChangedPayload{RepoID: repoID, Settings: snapshot})
	return snapshot, nil
}

// EmitRepoSettingsChanged fans a settings change that did not come through repoSettings.set (a repo
// file edited on disk) out to every connected client.
func (r *Router) EmitRepoSettingsChanged(repoID string, s model.GitRepoSettings) {
	r.repoSettingsChanged.Emit(RepoSettingsChangedPayload{RepoID: gitpath.CleanNFC(repoID), Settings: repoSettingsSnapshotFrom(s)})
}
