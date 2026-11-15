package ade

import (
	"context"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"log/slog"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// Migration parts of this chapter's content: each exports one kind of row to files once per task.
const (
	partNotes     = "notes"
	partHandovers = "handovers"
	partLinks     = "links"
)

// handoverExporter exports the note rows or the draft and handed rows of a task. Record points the
// rows at their files; the index rows follow from the mirror pass that runs after the migration.
type handoverExporter struct {
	df    *detailsFiles
	b     *TaskBoard
	part  string
	mu    sync.Mutex
	paths map[string]map[string]string // task id -> row id -> rel
}

func (e *handoverExporter) Part() string { return e.part }

func (e *handoverExporter) plan(taskID string) ([]wantFile, error) {
	rows, err := e.b.deps.Tasks.Handovers(taskID)
	if err != nil {
		return nil, err
	}
	var out []wantFile
	for _, w := range planHandoverFiles(rows, nil) {
		if (w.kind == taskfiles.KindNote) == (e.part == partNotes) {
			out = append(out, w)
		}
	}
	return out, nil
}

func (e *handoverExporter) Export(ctx context.Context, taskID string, st store.Store) (taskfiles.Manifest, error) {
	files, err := e.plan(taskID)
	if err != nil {
		return taskfiles.Manifest{}, err
	}
	m := taskfiles.Manifest{Counts: map[string]int{"rows": len(files)}}
	paths := map[string]string{}
	for _, w := range files {
		rel := w.rel
		if w.renameFrom != "" || w.kind == taskfiles.KindHandover && rel == "" {
			return m, fmt.Errorf("row %s has no file name", w.rowID)
		}
		if _, err := st.WriteAtomic(ctx, rel, w.data, nil); err != nil {
			return m, err
		}
		m.Paths = append(m.Paths, rel)
		paths[w.rowID] = rel
	}
	e.mu.Lock()
	if e.paths == nil {
		e.paths = map[string]map[string]string{}
	}
	e.paths[taskID+"/"+e.part] = paths
	e.mu.Unlock()
	return m, nil
}

func (e *handoverExporter) Verify(ctx context.Context, taskID string, st store.Store, m taskfiles.Manifest) error {
	files, err := e.plan(taskID)
	if err != nil {
		return err
	}
	if len(files) != m.Counts["rows"] || len(files) != len(m.Paths) {
		return fmt.Errorf("%d rows, %d files", len(files), len(m.Paths))
	}
	for _, w := range files {
		raw, _, err := st.Read(ctx, w.rel, taskfiles.MaxBytes(w.kind, w.rel))
		if err != nil {
			return fmt.Errorf("%s: %w", w.rel, err)
		}
		if sha(raw) != sha(w.data) {
			return fmt.Errorf("%s: content differs from its row", w.rel)
		}
	}
	return nil
}

func (e *handoverExporter) Record(_ context.Context, tx taskfiles.Tx, taskID string, _ taskfiles.Manifest) error {
	e.mu.Lock()
	paths := e.paths[taskID+"/"+e.part]
	delete(e.paths, taskID+"/"+e.part)
	e.mu.Unlock()
	for id, rel := range paths {
		if err := e.df.cache.SetHandoverRel(tx.Querier(), id, rel); err != nil {
			return err
		}
	}
	return nil
}

// linksExporter exports a task's slots and stored links to links.yaml; a task with none gets no file.
type linksExporter struct {
	df *detailsFiles
	b  *TaskBoard
}

func (linksExporter) Part() string { return partLinks }

func (e linksExporter) current(taskID string) (linksDoc, error) {
	return e.df.currentLinks(e.b, taskID)
}

func (e linksExporter) Export(ctx context.Context, taskID string, st store.Store) (taskfiles.Manifest, error) {
	l, err := e.current(taskID)
	if err != nil || l.empty() {
		return taskfiles.Manifest{Counts: map[string]int{"entries": 0}}, err
	}
	if _, err := st.WriteAtomic(ctx, taskfiles.LinksFile, renderLinks(l), nil); err != nil {
		return taskfiles.Manifest{}, err
	}
	return taskfiles.Manifest{Paths: []string{taskfiles.LinksFile}, Counts: map[string]int{"entries": len(l.Entries)}}, nil
}

func (e linksExporter) Verify(ctx context.Context, taskID string, st store.Store, m taskfiles.Manifest) error {
	if len(m.Paths) == 0 {
		return nil
	}
	raw, _, err := st.Read(ctx, taskfiles.LinksFile, taskfiles.MaxLinksBytes)
	if err != nil {
		return err
	}
	got, err := parseLinks(raw, func(a agentmcp.ArtifactInput) agentmcp.ArtifactInput { return a })
	if err != nil {
		return err
	}
	want, err := e.current(taskID)
	if err != nil {
		return err
	}
	if len(got.Entries) != m.Counts["entries"] || len(taskfiles.DiffLinks(want.Links, got.Links)) != 0 {
		return fmt.Errorf("links.yaml differs from the rows")
	}
	return nil
}

// Record has nothing to point: links keep no path; the mirror pass indexes the file.
func (linksExporter) Record(context.Context, taskfiles.Tx, string, taskfiles.Manifest) error {
	return nil
}

// startDetails runs at app start: finish interrupted writes, export old rows to files once, then
// mirror every live task so the files, the README and the index match the cache.
func (b *TaskBoard) startDetails() {
	df := b.files
	if df == nil {
		return
	}
	if err := df.svc.Recover(b.ctx); err != nil {
		slog.Error("ade: recover task files", "scope", "ade", "err", err)
	}
	mig := &taskfiles.Migrator{
		Index: df.idx, Marks: b.deps.TaskFiles, Root: df.root, Now: b.deps.Now,
		Tasks: func() ([]string, error) {
			var out []string
			for _, list := range []func() ([]model.AdeTask, error){b.deps.Tasks.ListLive, b.deps.Tasks.ListArchived} {
				ts, err := list()
				if err != nil {
					return nil, err
				}
				for _, t := range ts {
					out = append(out, t.ID)
				}
			}
			return out, nil
		},
		Exporters: []taskfiles.Exporter{
			&handoverExporter{df: df, b: b, part: partNotes}, &handoverExporter{df: df, b: b, part: partHandovers}, linksExporter{df: df, b: b},
		},
	}
	if links := b.proofLinks(); links != nil && df.blobs != nil {
		mig.Exporters = append(mig.Exporters, &taskfiles.ProofExporter{
			Source: links, Cache: repos.ProofCache{Dialect: b.deps.Tasks.Dialect}, Blobs: df.blobs, Now: b.deps.Now,
		})
	}
	err := mig.Run(b.ctx)
	if links := b.proofLinks(); links != nil {
		links.PurgeTaskless()
	}
	if err != nil && b.ctx.Err() == nil {
		slog.Error("ade: migrate task files", "scope", "ade", "err", err)
	}
	live, err := b.deps.Tasks.ListLive()
	if err != nil {
		slog.Error("ade: list tasks for files", "scope", "ade", "err", err)
		return
	}
	for _, t := range live {
		if b.ctx.Err() != nil {
			return
		}
		mu := b.taskMu(t.ID)
		mu.Lock()
		b.mirrorTask(t.ID, mirrorOpts{})
		mu.Unlock()
	}
}
