package clickhouse

import (
	"context"
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func init() {
	adapters.Register("clickhouse", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

var relationKinds = map[string]bool{"table": true, "view": true, "matview": true}

// connState is handle/readOnly: Connect/Disconnect write them from whatever goroutine
// adapterhost dispatches them on, concurrently with any in-flight op reading them — Cancel already
// locked its own read (below); Mutate/Execute/requireHandle did not, a real data race same class as
// postgres/adapter.go's own. Guarded via adapters.Guarded. The running-query bookkeeping
// lives in tracker's own lock, unrelated to this state.
type connState struct {
	handle   *Handle
	readOnly bool
}

// Adapter is index.ts's ClickHouseAdapter.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]

	// tracker's Q is the bare query_id string (D8), not a thread id or backend pid.
	tracker adapters.QueryTracker[string]
}

// setConnected is Connect's own locked write (F3).
func (a *Adapter) setConnected(handle *Handle) {
	a.state.Update(func(s *connState) {
		s.handle = handle
		s.readOnly = handle.ReadOnly
	})
}

// clearConnected is Disconnect's own locked write (F3).
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) { s.handle = nil })
}

func (a *Adapter) Kind() string        { return "clickhouse" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	handle, err := OpenClient(cfg, a.deps.Log)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	// P13 D1: assigned before the probe runs, not after it succeeds — Disconnect must reach the
	// handle from the instant OpenClient returns.
	a.setConnected(handle)

	rows, err := RunCatalogQuery[struct {
		Version  string `json:"version"`
		Database string `json:"database"`
		Timezone string `json:"timezone"`
	}](ctx, handle, a.nextQueryID(op.OpID, 0), "SELECT version() AS version, currentDatabase() AS database, timezone() AS timezone", op, a.trackerFor(op.OpID), nil)
	if err != nil {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, err
	}
	if len(rows) == 0 {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, adapters.Contract("connect probe returned no rows", nil)
	}
	row := rows[0]
	return adapters.ConnectInfo{
		ServerVersion: "ClickHouse " + row.Version,
		Details:       map[string]string{"url": handle.URL, "database": row.Database, "timezone": row.Timezone},
	}, nil
}

// Disconnect is index.ts's disconnect.
func (a *Adapter) Disconnect(ctx context.Context) error {
	// F4: cancel every query this adapter still tracks as running (KILL QUERY, this adapter's
	// existing Cancel path) before Drain, whose own wait is now bounded by ctx rather than able to
	// block this call for as long as the longest still-running query.
	for _, opID := range a.tracker.Snapshot() {
		_, _ = a.Cancel(ctx, opID)
	}
	a.tracker.Drain(ctx)
	if handle := a.state.Load().handle; handle != nil {
		handle.Client.CloseIdleConnections()
	}
	a.clearConnected()
	return nil
}

// nextQueryID is index.ts's opRuntime's own per-call closure — this
// exported-package-level helper is only ever called with a fresh seq per top-level call, matching
// index.ts's own instance-per-call closure rather than an instance-level map, since every top-level
// Adapter method call already gets a fresh, unique op.OpID.
func (a *Adapter) nextQueryID(opID string, seq int) string {
	return "kira-" + opID + "-" + strconv.Itoa(seq)
}

// trackerFor is index.ts's trackerFor (P13 D3): registers the running query_id and hands back its
// own release. The identity check in the release closure is what makes a multi-statement op
// (mutate's insert, console's "Run all") correct.
func (a *Adapter) trackerFor(opID string) TrackQuery {
	inner := a.tracker.TrackerFor(opID)
	return func(q RunningQuery) func() {
		return inner(q.QueryID)
	}
}

// opSeq hands out a fresh, monotonically increasing query_id sequence per top-level call — a
// closure captured once per Adapter method invocation, mirroring index.ts's own opRuntime.
type opSeq struct {
	opID string
	n    int
}

func (a *Adapter) newOpSeq(opID string) *opSeq { return &opSeq{opID: opID} }
func (s *opSeq) next(a *Adapter) string {
	id := a.nextQueryID(s.opID, s.n)
	s.n++
	return id
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	handle, err := a.requireHandle()
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	seq := a.newOpSeq(op.OpID)
	track := a.trackerFor(op.OpID)
	segments := path.Segments

	if len(segments) == 0 {
		nodes, err := listDatabases(ctx, handle, seq.next(a), op, track)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	databaseSegment := segments[0]
	if databaseSegment.Kind != "database" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, databaseSegment.Kind)
	}
	if len(segments) == 1 {
		nodes, err := listTablesAndViews(ctx, handle, seq.next(a), op, track, databaseSegment.Name)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	objectSegment := segments[1]
	if len(segments) == 2 {
		// Adapter rule 5: Children returns [] for a leaf, never an error — a table/view/matview's
		// columns live in describe()/definition() (P19 D5).
		if relationKinds[objectSegment.Kind] {
			return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil
		}
		return adapters.TreeChildren{}, adapters.New(adapters.CodeNotFound, "unexpected object kind: "+objectSegment.Kind, nil)
	}

	return adapters.TreeChildren{}, adapters.Contract("unrecognized path depth", nil)
}

// requireRelationPath is Describe/Definition/Read/Count's shared database/table path check (P113
// G2) — the object segment accepts relationKinds's own set (table/view/matview), same as Children's
// own leaf check above.
func requireRelationPath(path model.NodePath, opName string) (databaseSegment, objectSegment model.PathSegment, err error) {
	segs, err := adapters.RequirePath(path, opName, adapters.Seg("database"), adapters.Seg("table", "table", "view", "matview"))
	if err != nil {
		return model.PathSegment{}, model.PathSegment{}, err
	}
	return segs[0], segs[1], nil
}

// Describe is index.ts's describe.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	databaseSegment, objectSegment, err := requireRelationPath(path, "describe")
	if err != nil {
		return model.ObjectMeta{}, err
	}
	handle, err := a.requireHandle()
	if err != nil {
		return model.ObjectMeta{}, err
	}
	seq := a.newOpSeq(op.OpID)
	track := a.trackerFor(op.OpID)

	target, err := getReadTarget(ctx, handle, seq.next(a), op, track, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	// Sequential, not concurrent — routes every catalog query for this op through the same tracked
	// query_id sequence, mirroring every other SQL adapter's own describe() discipline.
	indexes, err := listIndexes(ctx, handle, seq.next(a), op, track, databaseSegment.Name, objectSegment.Name, target.PrimaryKeyExpression)
	if err != nil {
		return model.ObjectMeta{}, err
	}

	var totalRows *int
	if target.TotalRows != nil {
		n := int(*target.TotalRows)
		totalRows = &n
	}
	return model.ObjectMeta{
		Path: model.EncodePath(path.Segments), Kind: objectSegment.Kind, Name: objectSegment.Name,
		QualifiedName: databaseSegment.Name + "." + objectSegment.Name, Columns: target.Columns,
		// D18: a MergeTree PRIMARY KEY is a sparse index, not a uniqueness constraint (F16) — never
		// claimed as ObjectMeta.PrimaryKey, even though individual columns still carry their own
		// IsPrimaryKey badge... which toColumnMeta already always reports false too (D18/D23).
		PrimaryKey: nil, ForeignKeys: []model.ForeignKeyMeta{}, ReferencedBy: []model.ForeignKeyMeta{},
		Indexes: indexes, RowEstimate: totalRows, Comment: target.Comment,
	}, nil
}

// SchemaColumns is P22c D1's schema-wide sibling of Describe: every relation in the database
// together with its columns, in one round trip (well, two — relevantTables plus one schema-wide
// system.columns query — still orders of magnitude cheaper than a listColumnsRaw call per table).
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	segs, err := adapters.RequirePath(path, "schemaColumns", adapters.Seg("database"))
	if err != nil {
		return nil, err
	}
	databaseSegment := segs[0]
	handle, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	seq := a.newOpSeq(op.OpID)
	track := a.trackerFor(op.OpID)
	return listSchemaColumns(ctx, handle, seq.next(a), seq.next(a), op, track, databaseSegment.Name)
}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	databaseSegment, objectSegment, err := requireRelationPath(path, "definition")
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	handle, err := a.requireHandle()
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	seq := a.newOpSeq(op.OpID)
	return buildDefinition(ctx, handle, seq.next(a), op, a.trackerFor(op.OpID), path.Segments, databaseSegment.Name, objectSegment.Kind, objectSegment.Name)
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	databaseSegment, objectSegment, err := requireRelationPath(req.Path, "read")
	if err != nil {
		return nil, err
	}
	handle, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	seq := a.newOpSeq(op.OpID)
	track := a.trackerFor(op.OpID)
	target, err := getReadTarget(ctx, handle, seq.next(a), op, track, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return nil, err
	}
	result, err := withEngineTimeout(handle, req.StatementTimeout, a.deps.Log, func(extra map[string]string) (page.TabularPage, error) {
		return readPage(ctx, handle, seq.next(a), op, track, target, readReq{
			Projection: req.Projection, Filter: req.Filter, Sort: req.Sort, PageSize: req.PageSize, Cursor: req.Cursor,
			Conditions: req.Conditions,
		}, extra)
	})
	if err != nil {
		if req.StatementTimeout > 0 && ctx.Err() == nil && codeOf(err) == timeoutExceeded {
			err = adapters.StatementTimeout(req.StatementTimeout, 0, err)
		}
		return nil, err
	}
	return result, nil
}

// Count is index.ts's count — no getReadTarget call (D19/scenario 30): count() needs only the
// qualified name, not the columns/engine/keys catalog round trips read() genuinely uses.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	databaseSegment, objectSegment, err := requireRelationPath(req.Path, "count")
	if err != nil {
		return adapters.CountResult{}, err
	}
	handle, err := a.requireHandle()
	if err != nil {
		return adapters.CountResult{}, err
	}
	seq := a.newOpSeq(op.OpID)
	return countRows(ctx, handle, seq.next(a), op, a.trackerFor(op.OpID), QualifiedName{Database: databaseSegment.Name, Table: objectSegment.Name}, req.Filter)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	return preview(plan)
}

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	handle, err := a.requireHandle()
	if err != nil {
		return model.MutationResult{}, err
	}
	seq := a.newOpSeq(op.OpID)
	return mutate(ctx, handle, seq.next(a), op, a.trackerFor(op.OpID), a.state.Load().readOnly, plan)
}

// Execute is index.ts's execute.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	handle, err := a.requireHandle()
	if err != nil {
		return nil, err
	}
	seq := a.newOpSeq(op.OpID)
	return execute(ctx, handle, op, a.trackerFor(op.OpID), req.Statements, func() string { return seq.next(a) }, adapters.ConsoleCapFor(req), req.StatementTimeout, a.deps.Log)
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false, so no UI ever offers
// Download for clickhouse; never reached.
func (a *Adapter) DownloadObject(context.Context, model.ObjectDownloadRequest, *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("clickhouse", "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A table's rows have no per-item engine-level
// "type" the way a redis key does.
func (a *Adapter) KeyTypes(context.Context, []model.NodePath, *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("clickhouse", "key types")
}

// Cancel is index.ts's cancel — D7/D8: the KILL QUERY request never carries readonly, a second,
// free HTTP request on the client's own connection pool (F7/F9), never scoped by this connection's
// own read-only flag.
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	queryID, ok := a.tracker.PopRunning(opID)
	handle := a.state.Load().handle
	if !ok || handle == nil {
		return false, nil
	}

	resp, err := doRequest(ctx, handle, "KILL QUERY WHERE query_id = {qid:String} SYNC", "", nil, map[string]string{"qid": queryID}, false)
	if err != nil {
		a.deps.Log("warn", "clickhouse cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.deps.Log("warn", "clickhouse cancel("+opID+") failed: server returned "+strconv.Itoa(resp.StatusCode))
		return false, nil
	}
	return true, nil
}

func (a *Adapter) requireHandle() (*Handle, error) {
	return adapters.RequireConnected(a.state.Load().handle)
}
