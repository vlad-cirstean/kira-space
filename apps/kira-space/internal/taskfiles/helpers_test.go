package taskfiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
)

// textHandler treats every file as text cached in memory: a stand-in for the Part 3 handlers.
type textHandler struct {
	mu    sync.Mutex
	cache map[string]string
	own   map[string]bool // rel -> the writer authored the change
}

func newTextHandler() *textHandler {
	return &textHandler{cache: map[string]string{}, own: map[string]bool{}}
}

func (h *textHandler) Parse(rel string, _ Class, raw []byte) (Parsed, error) {
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "BAD") {
			return nil, &ParseError{Line: i + 1, Msg: "bad token"}
		}
	}
	return string(raw), nil
}

func (h *textHandler) Diff(_ context.Context, _ Tx, _, rel string, _ Class, parsed Parsed) ([]Change, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	old, had := h.cache[rel]
	switch {
	case parsed == nil && had:
		return []Change{{Op: OpDelete, Own: h.own[rel]}}, nil
	case parsed == nil:
		return nil, nil
	case !had:
		return []Change{{Op: OpCreate, Own: true, Text: TextDiff(rel, "", parsed.(string))}}, nil
	case old != parsed.(string):
		return []Change{{Op: OpEdit, Own: h.own[rel], Text: TextDiff(rel, old, parsed.(string))}}, nil
	}
	return nil, nil
}

func (h *textHandler) Apply(_ context.Context, _ Tx, _, rel string, _ Class, parsed Parsed, _ []Change) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache[rel] = parsed.(string)
	return nil
}

func (h *textHandler) Adopt(ctx context.Context, tx Tx, taskID, rel string, c Class, parsed Parsed) error {
	return h.Apply(ctx, tx, taskID, rel, c, parsed, nil)
}

func (h *textHandler) Remove(_ context.Context, _ Tx, _, rel string, _ Class) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.cache, rel)
	return nil
}

func (h *textHandler) Render(_ context.Context, _ Tx, _, rel string, _ Class) ([]byte, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.cache[rel]
	return []byte(s), ok, nil
}

func (h *textHandler) get(rel string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.cache[rel]
	return s, ok
}

// fakeEvents records what a submit raised.
type fakeEvents struct {
	pending  []PendingSubmit
	accepted []string
}

func (e *fakeEvents) Pending(_ context.Context, p PendingSubmit) error {
	e.pending = append(e.pending, p)
	return nil
}

func (e *fakeEvents) Accepted(_ context.Context, _, rel string, n Notify) error {
	e.accepted = append(e.accepted, rel+":"+string(n))
	return nil
}

// countStore counts reads, to prove an unchanged file is never read.
type countStore struct {
	store.Store
	reads int
}

func (c *countStore) Read(ctx context.Context, rel string, max int64) ([]byte, store.Info, error) {
	c.reads++
	return c.Store.Read(ctx, rel, max)
}

type env struct {
	t      *testing.T
	svc    *Service
	h      *textHandler
	ev     *fakeEvents
	st     *countStore
	local  *store.Local
	repo   *repos.TaskFilesRepo
	now    time.Time
	taskID string
}

const testTask = "t1"

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := storage.OpenAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := (&repos.AdeTaskRepo{DB: db.DB}).CreateTask(model.AdeTask{ID: testTask, Kind: model.AdeTaskKindTask, Title: "t", CreatedAt: 1}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	local, err := store.NewLocal(filepath.Join(t.TempDir(), testTask))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, h: newTextHandler(), ev: &fakeEvents{}, st: &countStore{Store: local}, local: local,
		repo: &repos.TaskFilesRepo{DB: db.DB}, now: time.UnixMilli(1_000_000), taskID: testTask}
	handlers := map[Kind]Handler{}
	for _, k := range []Kind{KindReadme, KindTask, KindState, KindLinks, KindNote, KindDraft, KindHandover, KindProof, KindProofData, KindProofImage} {
		handlers[k] = e.h
	}
	e.svc = New(Config{Index: RepoIndex(e.repo), Open: func(string) (store.Store, error) { return e.st, nil },
		Handlers: handlers, Events: e.ev, Now: func() time.Time { return e.now }})
	return e
}

// seed writes a file as Kira (accepted at once), like a UI edit or the migration.
func (e *env) seed(rel, body string) {
	e.t.Helper()
	if err := e.svc.Write(context.Background(), e.taskID, rel, []byte(body), string(RoleUser), func(Tx) error {
		e.h.mu.Lock()
		e.h.cache[rel] = body
		e.h.mu.Unlock()
		return nil
	}); err != nil {
		e.t.Fatal(err)
	}
}

// disk writes a file the way an agent's Edit does: no Kira involvement.
func (e *env) disk(rel, body string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(e.local.Root(), filepath.FromSlash(rel))), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.local.Root(), filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
	later := e.now.Add(time.Duration(len(body)+1) * time.Second) // distinct mtimes, whatever the clock grain
	_ = os.Chtimes(filepath.Join(e.local.Root(), filepath.FromSlash(rel)), later, later)
}

func (e *env) read(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.local.Root(), filepath.FromSlash(rel)))
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

func (e *env) submit(role Role, key, reason string, paths ...string) Result {
	e.t.Helper()
	res, err := e.svc.Submit(context.Background(), Request{TaskID: e.taskID, Paths: paths, Reason: reason,
		Session: Session{Key: key, Role: role, Step: "implement", LastSync: e.now.Add(-time.Hour)}})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func (r Result) find(path string) FileResult {
	for _, f := range r.Files {
		if f.Path == path {
			return f
		}
	}
	return FileResult{Path: path, Status: "absent"}
}
