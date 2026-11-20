package taskfiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

// Manifest is what an exporter wrote for one task part: the paths, for the verify and record steps.
type Manifest struct {
	Paths []string `json:"paths"`
	// Counts is free-form per-exporter numbers (rows exported) the verify step compares.
	Counts map[string]int `json:"counts,omitempty"`
}

// Exporter moves one part of a task's Details content from its SQLite rows to files. Parts 3 and 4
// supply the real ones; the migrator owns staging, verify-before-rename, markers and resume.
type Exporter interface {
	// Part is the marker name: one part is migrated and marked at a time.
	Part() string
	// Export writes the task's part into staging and returns what it wrote.
	Export(ctx context.Context, taskID string, staging store.Store) (Manifest, error)
	// Verify re-reads staging and checks it against the rows: parse, per-row hash, counts.
	Verify(ctx context.Context, taskID string, staging store.Store, m Manifest) error
	// Record points the cache rows at the files (rel_path) once they are in place; Adopt-style, in tx.
	Record(ctx context.Context, tx Tx, taskID string, m Manifest) error
}

// Marks stores the per-task per-part marker; *repos.TaskFilesRepo serves it.
type Marks interface {
	Migration(taskID, part string) (*model.TaskFilesMigration, error)
	SetMigration(model.TaskFilesMigration) error
}

// Migrator runs the one-time export. It is resumable: a marker per task and part, a staging folder
// per run that a crash leaves behind and the next run deletes, and a rename only after verify.
type Migrator struct {
	Index Index
	Marks Marks
	// Root is the folder of every task folder (<home>/tasks).
	Root string
	// Tasks lists the ids to migrate, live and archived.
	Tasks     func() ([]string, error)
	Exporters []Exporter
	Now       func() time.Time
}

// Run migrates every task part not yet marked done. A task part that fails is marked failed and
// left on SQLite reads; the others continue. It returns the failed task ids joined, nil on none.
func (m *Migrator) Run(ctx context.Context) error {
	if m.Now == nil {
		m.Now = time.Now
	}
	ids, err := m.Tasks()
	if err != nil {
		return err
	}
	var failed []string
	for i, id := range ids {
		for _, ex := range m.Exporters {
			if err := m.migrate(ctx, id, ex); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				slog.Error("taskfiles: migrate task", "scope", "ade", "task", id, "part", ex.Part(), "err", err)
				failed = append(failed, id)
			}
		}
		if (i+1)%100 == 0 || i+1 == len(ids) {
			slog.Info("taskfiles: migration progress", "scope", "ade", "done", i+1, "of", len(ids))
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("taskfiles: %d task parts failed to migrate: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func (m *Migrator) migrate(ctx context.Context, taskID string, ex Exporter) (err error) {
	if err := store.ValidRel(taskID); err != nil || strings.Contains(taskID, "/") {
		return fmt.Errorf("task id %q is not a folder name", taskID)
	}
	part := ex.Part()
	mark, err := m.Marks.Migration(taskID, part)
	if err != nil {
		return err
	}
	if mark != nil && mark.State == model.TaskFilesMigrationDone {
		return nil
	}
	set := func(state, verify string) error {
		done := int64(0)
		if state == model.TaskFilesMigrationDone {
			done = m.Now().UnixMilli()
		}
		return m.Marks.SetMigration(model.TaskFilesMigration{TaskID: taskID, Part: part, State: state, DoneAt: done, VerifyJSON: verify})
	}
	if err := set(model.TaskFilesMigrationRunning, ""); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = set(model.TaskFilesMigrationFailed, err.Error())
		}
	}()
	staging := filepath.Join(m.Root, taskID+".migrating")
	if err := fileservice.Host().RemoveAll(staging); err != nil { // a crash left it: the task part is redone
		return err
	}
	defer fileservice.Host().RemoveAll(staging) //nolint:errcheck
	stage, err := store.NewLocal(staging)
	if err != nil {
		return err
	}
	manifest, err := ex.Export(ctx, taskID, stage)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}
	if err := ex.Verify(ctx, taskID, stage, manifest); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	final, err := store.NewLocal(filepath.Join(m.Root, taskID))
	if err != nil {
		return err
	}
	if err := mergeAbsent(ctx, stage, final); err != nil {
		return fmt.Errorf("move into place: %w", err)
	}
	err = m.Index.InTx(func(tx Tx) error { return ex.Record(ctx, tx, taskID, manifest) })
	if err != nil {
		return fmt.Errorf("record: %w", err)
	}
	return set(model.TaskFilesMigrationDone, fmt.Sprintf("%d files", len(manifest.Paths)))
}

// mergeAbsent copies every staged file the final folder lacks; a file already there is the user's
// or the agent's newer one and stays.
func mergeAbsent(ctx context.Context, from, to store.Store) error {
	ents, err := from.List(ctx, store.ListOptions{MaxDepth: MaxDepth + 1})
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.Dir || e.Refused != "" {
			continue
		}
		if _, err := to.Stat(ctx, e.Rel); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotExist) {
			return err
		}
		data, _, err := from.Read(ctx, e.Rel, MaxBytesPerTask)
		if err != nil {
			return err
		}
		absent := store.Version("")
		if _, err := to.WriteAtomic(ctx, e.Rel, data, &absent); err != nil && !errors.Is(err, store.ErrConflict) {
			return err
		}
	}
	return nil
}
