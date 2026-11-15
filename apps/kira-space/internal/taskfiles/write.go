package taskfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// restoreResult answers a read-only violation: the file goes back to its accepted copy.
func (s *Service) restoreResult(ctx context.Context, taskID, rel string) (FileResult, *PendingItem) {
	if err := s.Restore(ctx, taskID, rel); err != nil {
		return FileResult{Path: rel, Status: StatusInvalid, Message: fmt.Sprintf("%s: could not restore: %v", rel, err)}, nil
	}
	return FileResult{Path: rel, Status: StatusRestored, Message: rel + ": read-only, Kira restored it."}, nil
}

// Restore rewrites the file from the accepted copy the kind's handler renders. With no accepted copy
// the file is removed. It is what a denied edit and a read-only violation both end in.
func (s *Service) Restore(ctx context.Context, taskID, rel string) error {
	class := Classify(rel)
	h := s.cfg.Handlers[class.Kind]
	if h == nil {
		return fmt.Errorf("%w: %s", ErrNoHandler, rel)
	}
	var data []byte
	var ok bool
	err := s.cfg.Index.InTx(func(tx Tx) error {
		var err error
		data, ok, err = h.Render(ctx, tx, taskID, rel, class)
		return err
	})
	if err != nil {
		return err
	}
	if !ok {
		return s.Delete(ctx, taskID, rel, string(RoleKira), nil)
	}
	return s.Write(ctx, taskID, rel, data, string(RoleKira), nil)
}

// Write is Kira's own file write: an intent row first, then the atomic file write, then the cache
// update (apply, may be nil), the index row and the intent's removal in one transaction. A crash in
// between leaves the intent for Recover; the file is truth, so a retry is safe.
func (s *Service) Write(ctx context.Context, taskID, rel string, data []byte, by string, apply func(Tx) error) error {
	class := Classify(rel)
	hash := sum(data)
	if err := s.putIntent(taskID, rel, hash, by); err != nil {
		return err
	}
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return err
	}
	info, err := st.WriteAtomic(ctx, rel, data, nil)
	if err != nil {
		_ = s.dropIntent(taskID, rel)
		return err
	}
	return s.cfg.Index.InTx(func(tx Tx) error {
		if apply != nil {
			if err := apply(tx); err != nil {
				return err
			}
		}
		row := model.TaskFile{TaskID: taskID, RelPath: rel, Kind: string(class.Kind), Size: info.Size, Version: string(info.Version),
			SHA256: hash, AcceptedSHA256: hash, AcceptedBy: by, AcceptedAt: s.nowMs(), Review: model.TaskFileAccepted}
		if err := tx.PutFile(row); err != nil {
			return err
		}
		return tx.DeleteIntent(taskID, rel)
	})
}

// Delete is Kira's own file removal, journaled like Write. apply may be nil.
func (s *Service) Delete(ctx context.Context, taskID, rel string, by string, apply func(Tx) error) error {
	if err := s.putIntent(taskID, rel, "", by); err != nil {
		return err
	}
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return err
	}
	if err := st.Remove(ctx, rel); err != nil && !errors.Is(err, store.ErrNotExist) {
		_ = s.dropIntent(taskID, rel)
		return err
	}
	return s.cfg.Index.InTx(func(tx Tx) error {
		if apply != nil {
			if err := apply(tx); err != nil {
				return err
			}
		}
		if err := tx.DeleteFile(taskID, rel); err != nil {
			return err
		}
		return tx.DeleteIntent(taskID, rel)
	})
}

func (s *Service) putIntent(taskID, rel, hash, by string) error {
	return s.cfg.Index.InTx(func(tx Tx) error {
		return tx.PutIntent(model.TaskFileIntent{TaskID: taskID, RelPath: rel, SHA256: hash, Writer: by, CreatedAt: s.nowMs()})
	})
}

func (s *Service) dropIntent(taskID, rel string) error {
	return s.cfg.Index.InTx(func(tx Tx) error { return tx.DeleteIntent(taskID, rel) })
}

// Adopt makes the file on disk the accepted copy: the handler caches the whole parsed file and the
// row moves to its hash. It is the user's consent (Accept on the disk banner, approving a pending
// edit) and what recovery and migration end in. A file that is gone is removed from the cache.
func (s *Service) Adopt(ctx context.Context, taskID, rel, by string) error {
	class := Classify(rel)
	h := s.cfg.Handlers[class.Kind]
	if h == nil {
		return fmt.Errorf("%w: %s", ErrNoHandler, rel)
	}
	st, err := s.cfg.Open(taskID)
	if err != nil {
		return err
	}
	raw, info, err := st.Read(ctx, rel, MaxBytes(class.Kind, rel))
	if errors.Is(err, store.ErrNotExist) {
		return s.cfg.Index.InTx(func(tx Tx) error {
			if err := h.Remove(ctx, tx, taskID, rel, class); err != nil {
				return err
			}
			return tx.DeleteFile(taskID, rel)
		})
	}
	if err != nil {
		return err
	}
	parsed, err := h.Parse(rel, class, raw)
	if err != nil {
		return err
	}
	hash := sum(raw)
	return s.cfg.Index.InTx(func(tx Tx) error {
		if err := h.Adopt(ctx, tx, taskID, rel, class, parsed); err != nil {
			return err
		}
		row := model.TaskFile{TaskID: taskID, RelPath: rel, Kind: string(class.Kind), Size: info.Size, Version: string(info.Version),
			SHA256: hash, AcceptedSHA256: hash, AcceptedBy: by, AcceptedAt: s.nowMs(), Review: model.TaskFileAccepted}
		if err := tx.PutFile(row); err != nil {
			return err
		}
		return tx.DeleteIntent(taskID, rel)
	})
}

// Recover finishes or drops every unfinished write intent; run it at start. An intent whose file
// hash matches completes its database side; any other is dropped.
func (s *Service) Recover(ctx context.Context) error {
	intents, err := s.cfg.Index.Intents()
	if err != nil {
		return err
	}
	for _, in := range intents {
		if err := s.recoverOne(ctx, in); err != nil {
			slog.Warn("taskfiles: recover intent", "scope", "ade", "task", in.TaskID, "path", in.RelPath, "err", err)
			if err := s.dropIntent(in.TaskID, in.RelPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) recoverOne(ctx context.Context, in model.TaskFileIntent) error {
	st, err := s.cfg.Open(in.TaskID)
	if err != nil {
		return err
	}
	raw, _, err := st.Read(ctx, in.RelPath, MaxBytes(Classify(in.RelPath).Kind, in.RelPath))
	switch {
	case errors.Is(err, store.ErrNotExist):
		if in.SHA256 == "" { // a delete that finished on disk
			return s.Adopt(ctx, in.TaskID, in.RelPath, in.Writer)
		}
		return s.dropIntent(in.TaskID, in.RelPath)
	case err != nil:
		return err
	}
	if in.SHA256 == "" || sum(raw) != in.SHA256 {
		return s.dropIntent(in.TaskID, in.RelPath)
	}
	return s.Adopt(ctx, in.TaskID, in.RelPath, in.Writer)
}
