package maskreview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tableref"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/tree"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const (
	// MaxAge is how long a check stays fresh for the MCP gate.
	MaxAge = 30 * time.Second
	// DefaultMaxContainers and DefaultMaxColumns cap one check; over either the state is incomplete.
	DefaultMaxContainers = 50
	DefaultMaxColumns    = 50000

	backgroundTimeout = 2 * time.Minute
)

// Withheld is what the MCP gate applies to one connection.
type Withheld struct {
	On bool
	// Names is the lowercased result column names to drop.
	Names map[string]bool
	// Pending is the canonical refs of the columns neither masked nor reviewed, sorted.
	Pending []string
	// Incomplete is non-empty when the columns could not be checked in full (over a cap, or the check failed): the gate refuses.
	Incomplete string
}

// ListedColumn is one live column for the Privacy tab.
type ListedColumn struct {
	Ref          string `json:"ref"`
	Table        string `json:"table"`
	Column       string `json:"column"`
	DataType     string `json:"dataType"`
	RelationKind string `json:"relationKind"`
}

// Listing is the live columns of a connection for the Privacy tab.
type Listing struct {
	Applicable bool           `json:"applicable"`
	Columns    []ListedColumn `json:"columns"`
	Truncated  bool           `json:"truncated"`
	// Incomplete names the cap that cut the check short ("over 50000 columns"); empty when complete.
	Incomplete string `json:"incomplete,omitempty"`
	CheckedAt  int64  `json:"checkedAt"`
}

// Deps are the seams Service reads.
type Deps struct {
	Conns interface {
		Get(id string) (*model.ConnectionSummary, error)
	}
	States interface {
		StateOf(id string) model.ConnectionState
	}
	Tree interface {
		Children(ctx context.Context, id, path string, refresh bool) (tree.ChildrenResult, error)
		SchemaColumns(ctx context.Context, id, path string, refresh bool) (tree.SchemaColumnsResult, error)
	}
	Rules interface {
		List(id string) ([]model.MaskRule, error)
	}
	Reviewed interface {
		ListForConnection(id string) ([]string, error)
	}
	// Prompts receives the review popup; nil drops it.
	Prompts prompts.Sink
	Now     func() time.Time
	// MaxContainers and MaxColumns override the caps (tests); 0 takes the defaults.
	MaxContainers, MaxColumns int
}

type entry struct {
	live       []LiveColumn
	truncated  bool
	incomplete string
	eval       Evaluation
	scopeKey   string
	checkedAt  time.Time
	stale      bool
}

func (e *entry) withheld() Withheld {
	return Withheld{On: true, Names: e.eval.Names, Pending: e.eval.Pending, Incomplete: e.incomplete}
}

type runner struct {
	again bool
	opts  opts
}

// Service owns the column review state of every connection.
type Service struct {
	d Deps

	mu      sync.Mutex
	entries map[string]*entry
	gen     map[string]uint64
	running map[string]*runner
	locks   map[string]*sync.Mutex
	wg      sync.WaitGroup
}

// New builds a Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.MaxContainers <= 0 {
		d.MaxContainers = DefaultMaxContainers
	}
	if d.MaxColumns <= 0 {
		d.MaxColumns = DefaultMaxColumns
	}
	return &Service{d: d, entries: map[string]*entry{}, gen: map[string]uint64{}, running: map[string]*runner{}, locks: map[string]*sync.Mutex{}}
}

// opts says which cached layers a check bypasses.
type opts struct {
	// columns bypasses the cached column listing of each container.
	columns bool
	// containers bypasses the cached database and schema listing.
	containers bool
}

func (a opts) merge(b opts) opts {
	return opts{columns: a.columns || b.columns, containers: a.containers || b.containers}
}

func scopeKeyOf(summary model.ConnectionSummary) string {
	if summary.McpTables == nil {
		return "*"
	}
	set, err := tableref.NewSet(summary.Kind, summary.McpTables)
	if err != nil {
		return "!"
	}
	return "[" + strings.Join(set.Entries(), "\n") + "]"
}

func (s *Service) fresh(summary model.ConnectionSummary) *entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[summary.ID]
	if e == nil || e.stale || e.scopeKey != scopeKeyOf(summary) || s.d.Now().Sub(e.checkedAt) >= MaxAge {
		return nil
	}
	return e
}

func (s *Service) lockFor(id string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.locks[id]
	if m == nil {
		m = &sync.Mutex{}
		s.locks[id] = m
	}
	return m
}

// For is the gate's read: the cached state while fresh, else one inline check. A kind without column review
// or a connection with it off returns the zero Withheld.
func (s *Service) For(ctx context.Context, summary model.ConnectionSummary) Withheld {
	if !Applies(summary.Kind) || !summary.MaskReviewOn {
		return Withheld{}
	}
	e := s.fresh(summary)
	if e == nil {
		m := s.lockFor(summary.ID)
		m.Lock()
		defer m.Unlock()
		if e = s.fresh(summary); e == nil {
			var err error
			if e, err = s.check(ctx, summary, opts{columns: true}); err != nil {
				return Withheld{On: true, Incomplete: failureReason(err)}
			}
			s.syncPrompt(summary, e)
		}
	}
	return e.withheld()
}

// failureReason names the failure class, never a driver message.
func failureReason(err error) string {
	var pe *ipcerr.Error
	if errors.As(err, &pe) && pe.Code == string(ipcerr.CodeDisconnected) {
		return "the connection is not connected"
	}
	return "the column check failed"
}

// Cached is the last state without any op; ok is false before a first check.
func (s *Service) Cached(id string) (Withheld, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[id]
	if e == nil {
		return Withheld{}, false
	}
	return e.withheld(), true
}

// PendingCount is the cached number of columns needing review.
func (s *Service) PendingCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[id]; e != nil {
		return len(e.eval.Pending)
	}
	return 0
}

// MarkStale makes the next gate read check again, after an MCP write or DDL statement.
func (s *Service) MarkStale(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.entries[id]; e != nil {
		e.stale = true
	}
}

// Forget drops the state and the popup of a connection that disconnected or went away.
func (s *Service) Forget(id string) {
	s.mu.Lock()
	delete(s.entries, id)
	s.gen[id]++
	s.mu.Unlock()
	s.closePrompt(id)
}

// Check runs a check for the Privacy tab and returns the live columns. It works with MCP off so the user can
// prepare a review.
func (s *Service) Check(ctx context.Context, id string, refresh bool) (Listing, error) {
	summary, err := s.summary(id)
	if err != nil {
		return Listing{}, err
	}
	if !Applies(summary.Kind) {
		return Listing{Columns: []ListedColumn{}}, nil
	}
	e, err := s.check(ctx, *summary, opts{columns: refresh, containers: refresh})
	if err != nil {
		return Listing{}, err
	}
	s.syncPrompt(*summary, e)
	return listingOf(summary.Kind, e), nil
}

func (s *Service) summary(id string) (*model.ConnectionSummary, error) {
	summary, err := s.d.Conns.Get(id)
	if err != nil {
		return nil, err
	}
	if summary == nil {
		return nil, ipcerr.NotFound(id)
	}
	return summary, nil
}

func listingOf(kind string, e *entry) Listing {
	out := Listing{Applicable: true, Columns: make([]ListedColumn, 0, len(e.live)), Truncated: e.incomplete != "" || e.truncated, Incomplete: e.incomplete, CheckedAt: e.checkedAt.UnixMilli()}
	for _, c := range e.live {
		out.Columns = append(out.Columns, ListedColumn{
			Ref: tableref.FormatColumn(kind, c.Ref), Table: c.Ref.Name, Column: c.Ref.Column, DataType: c.DataType, RelationKind: c.RelationKind,
		})
	}
	return out
}

// Connected starts a background check after a connect. It bypasses the persisted column cache, which can be
// days old, so a column added while the app was closed is seen.
func (s *Service) Connected(id string) { s.schedule(id, opts{columns: true}) }

// Refresh starts a background check that bypasses the metadata cache, after the user refreshed the tree.
func (s *Service) Refresh(id string) { s.schedule(id, opts{columns: true, containers: true}) }

// Wait blocks until every background check has finished.
func (s *Service) Wait() { s.wg.Wait() }

func (s *Service) schedule(id string, o opts) {
	s.mu.Lock()
	if r := s.running[id]; r != nil {
		r.again, r.opts = true, r.opts.merge(o)
		s.mu.Unlock()
		return
	}
	s.running[id] = &runner{}
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			s.background(id, o)
			s.mu.Lock()
			r := s.running[id]
			if !r.again {
				delete(s.running, id)
				s.mu.Unlock()
				return
			}
			o, r.again, r.opts = r.opts, false, opts{}
			s.mu.Unlock()
		}
	}()
}

func (s *Service) active(summary *model.ConnectionSummary) bool {
	return summary != nil && Applies(summary.Kind) && summary.MaskReviewOn && summary.McpEnabled
}

func (s *Service) background(id string, o opts) {
	summary, err := s.summary(id)
	if err != nil || !s.active(summary) || s.d.States.StateOf(id).Status != "connected" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), backgroundTimeout)
	defer cancel()
	m := s.lockFor(id)
	m.Lock()
	defer m.Unlock()
	e, err := s.check(ctx, *summary, o)
	if err != nil {
		slog.Debug("maskreview: background check failed", "scope", "maskreview", "id", id, "err", err)
		s.MarkStale(id)
		return
	}
	s.syncPrompt(*summary, e)
}

// FileChanged reacts to an edit of the connection file: rules, the reviewed list, MCP exposure or the table list.
// It runs no op unless the cached columns no longer cover the file's scope.
func (s *Service) FileChanged(id string) {
	summary, err := s.summary(id)
	if err != nil || summary == nil {
		s.Forget(id)
		return
	}
	s.mu.Lock()
	e := s.entries[id]
	s.gen[id]++
	s.mu.Unlock()
	if !s.active(summary) {
		s.Forget(id)
		return
	}
	if e == nil || e.scopeKey != scopeKeyOf(*summary) {
		s.schedule(id, opts{})
		return
	}
	next := *e
	if err := s.evaluateInto(*summary, &next); err != nil {
		s.MarkStale(id)
		return
	}
	s.mu.Lock()
	s.entries[id] = &next
	s.mu.Unlock()
	s.syncPrompt(*summary, &next)
}

func (s *Service) evaluateInto(summary model.ConnectionSummary, e *entry) error {
	set, err := tableref.NewSet(summary.Kind, summary.McpTables)
	if err != nil {
		return err
	}
	rules, err := s.d.Rules.List(summary.ID)
	if err != nil {
		return err
	}
	reviewed, err := s.d.Reviewed.ListForConnection(summary.ID)
	if err != nil {
		return err
	}
	e.eval = Evaluate(summary.Kind, e.live, rules, ReviewedKeys(summary.Kind, reviewed), set)
	return nil
}

// check walks the live columns, evaluates them and stores the entry unless the connection changed meanwhile.
func (s *Service) check(ctx context.Context, summary model.ConnectionSummary, o opts) (*entry, error) {
	set, err := tableref.NewSet(summary.Kind, summary.McpTables)
	if err != nil {
		return nil, fmt.Errorf("maskreview: table list: %w", err)
	}
	s.mu.Lock()
	startGen := s.gen[summary.ID]
	s.mu.Unlock()

	w := &walker{s: s, ctx: ctx, id: summary.ID, kind: summary.Kind, set: set, scope: newScope(summary.Kind, set), o: o}
	if err := w.run(); err != nil {
		return nil, err
	}
	e := &entry{live: w.live, incomplete: w.incomplete, scopeKey: scopeKeyOf(summary), checkedAt: s.d.Now()}
	if err := s.evaluateInto(summary, e); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.gen[summary.ID] == startGen {
		s.entries[summary.ID] = e
	}
	s.mu.Unlock()
	return e, nil
}

func promptID(id string) string { return prompts.ID(prompts.KindMasking, id) }

func (s *Service) closePrompt(id string) {
	if s.d.Prompts != nil {
		s.d.Prompts.Close(promptID(id))
	}
}

// syncPrompt opens the masking popup while columns need review, closes it otherwise.
func (s *Service) syncPrompt(summary model.ConnectionSummary, e *entry) {
	if s.d.Prompts == nil {
		return
	}
	if !s.active(&summary) || len(e.eval.Pending) == 0 {
		s.closePrompt(summary.ID)
		return
	}
	s.d.Prompts.Open(prompts.Prompt{
		ID: promptID(summary.ID), Kind: prompts.KindMasking, Ref: summary.ID,
		Title: i18n.T("backend.prompt.masking.title", map[string]string{"name": summary.Name}),
	})
}

// scope limits the walk to containers holding a table of the P532 allow list.
type scope struct {
	all        bool
	databases  map[string]bool
	containers map[string]bool
}

func newScope(kind string, set *tableref.Set) scope {
	if set == nil {
		return scope{all: true}
	}
	sc := scope{databases: map[string]bool{}, containers: map[string]bool{}}
	for _, e := range set.Entries() {
		r, err := tableref.Parse(kind, e)
		if err != nil {
			continue
		}
		if kind == "sqlite" {
			sc.all = true
		}
		sc.databases[r.Database] = true
		sc.containers[r.Database+"\x00"+r.Schema] = true
	}
	return sc
}

func (sc scope) wantsDatabase(db string) bool { return sc.all || sc.databases[db] }

func (sc scope) wantsContainer(segs []model.PathSegment) bool {
	if sc.all {
		return true
	}
	switch len(segs) {
	case 1:
		return sc.databases[segs[0].Name]
	case 2:
		return sc.containers[segs[0].Name+"\x00"+segs[1].Name]
	}
	return false
}

type container struct {
	path string
	segs []model.PathSegment
}

type walker struct {
	s     *Service
	ctx   context.Context
	id    string
	kind  string
	set   *tableref.Set
	scope scope
	o     opts

	containers []container
	live       []LiveColumn
	incomplete string
}

func (w *walker) run() error {
	if err := w.findContainers(""); err != nil {
		return err
	}
	return w.fetchColumns()
}

// findContainers walks from the root through database and schema nodes. A postgres schema and every other
// kind's database holds relations.
func (w *walker) findContainers(path string) error {
	res, err := w.s.d.Tree.Children(w.ctx, w.id, path, w.o.containers)
	if err != nil {
		return err
	}
	for _, n := range res.Nodes {
		if w.incomplete != "" {
			return nil
		}
		p, err := model.DecodePath(w.id, n.Path)
		if err != nil {
			continue
		}
		if err := w.visit(n, p.Segments); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) visit(n model.TreeNode, segs []model.PathSegment) error {
	switch {
	case n.Kind == "database" && w.kind == "postgres":
		if w.scope.wantsDatabase(n.Name) {
			return w.findContainers(n.Path)
		}
	case n.Kind == "schema" || (n.Kind == "database" && w.kind != "postgres"):
		if w.scope.wantsContainer(segs) {
			w.add(container{path: n.Path, segs: segs})
		}
	}
	return nil
}

func (w *walker) add(c container) {
	if len(w.containers) >= w.s.d.MaxContainers {
		w.incomplete = fmt.Sprintf("over %d schemas", w.s.d.MaxContainers)
		return
	}
	w.containers = append(w.containers, c)
}

func (w *walker) fetchColumns() error {
	for _, c := range w.containers {
		if w.incomplete != "" {
			return nil
		}
		res, err := w.s.d.Tree.SchemaColumns(w.ctx, w.id, c.path, w.o.columns)
		if err != nil {
			return err
		}
		for _, rel := range res.Relations {
			w.addRelation(c, rel)
		}
	}
	return nil
}

func (w *walker) addRelation(c container, rel model.RelationColumns) {
	segs := append(append([]model.PathSegment{}, c.segs...), model.PathSegment{Kind: rel.Kind, Name: rel.Name})
	ref, ok := tableref.FromPath(w.kind, segs)
	if !ok || !w.set.Allows(ref) {
		return
	}
	for _, col := range rel.Columns {
		if w.incomplete != "" {
			return
		}
		if len(w.live) >= w.s.d.MaxColumns {
			w.incomplete = fmt.Sprintf("over %d columns", w.s.d.MaxColumns)
			return
		}
		w.live = append(w.live, LiveColumn{Ref: tableref.ColumnRef{Ref: ref, Column: col.Name}, DataType: col.DataType, RelationKind: rel.Kind})
	}
}
