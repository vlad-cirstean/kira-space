package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/relational"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

func init() {
	adapters.Register("postgres", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// — a real, race-detector-confirmed data race, not a theoretical one, the same class already found
// for the running-query bookkeeping below (tracker's own, unrelated to this state: pgx's own *Conn
// is not safe for concurrent use, and closing one mid-Query, were Disconnect to race a
// still-in-flight query, is what tracker.Drain() itself guards against — see its own doc
// comment). Guarded together via adapters.Guarded; shared with mysqlfamily's identical
// shape via relational.ConnState.
type connState = relational.ConnState[*ConnSet]

// Adapter is index.ts's PostgresAdapter.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]

	tracker adapters.QueryTracker[RunningQuery]
}

// setConnSet is Connect's own locked write of ConnSet/Cfg together — assigned before
// anything is opened, not after the probe succeeds, so the handle is reachable by Disconnect from
// the instant connSet.Primary() could have opened a socket, and a probe failure (or a dropped
// session mid-probe) never leaks it.
func (a *Adapter) setConnSet(connSet *ConnSet, cfg *model.ResolvedConnectionConfig) {
	a.state.Update(func(s *connState) { s.ConnSet = connSet; s.Cfg = cfg })
}

// setConnected is Connect's own locked write of the two fields only a successful probe fills in
// (F3).
func (a *Adapter) setConnected(primaryDatabase string, readOnly bool) {
	a.state.Update(func(s *connState) { s.PrimaryDatabase = primaryDatabase; s.ReadOnly = readOnly })
}

// clearConnected is Disconnect's own locked write, once CloseAll (a real network call, run with no
// lock held) has returned (F3). readOnly is deliberately left set.
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) { s.ConnSet = nil; s.PrimaryDatabase = "" })
}

func (a *Adapter) Kind() string        { return "postgres" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	connSet := NewConnSet(cfg, a.deps.Log)
	a.setConnSet(connSet, &cfg)

	conn, release, err := connSet.Primary(ctx)
	if err != nil {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, err
	}

	var serverVersion, database, encoding string
	found := false
	execErr := execFor(conn, op, a.trackerFor(op.OpID))(ctx,
		`SELECT version() AS version, current_database() AS database,
		        current_setting('server_encoding') AS encoding`, nil,
		func(rows pgx.Rows) error {
			found = true
			return rows.Scan(&serverVersion, &database, &encoding)
		})
	if execErr != nil {
		// release() must run before Disconnect: Disconnect->ConnSet.CloseAll takes this same
		// entry's lock, and sync.Mutex isn't reentrant — calling Disconnect while still holding
		// the lock this call frame acquired above deadlocks the goroutine permanently (F1).
		release()
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, execErr
	}
	if !found {
		release()
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, adapters.Contract("connect probe returned no rows", nil)
	}
	release()

	a.setConnected(database, cfg.ReadOnly)

	return adapters.ConnectInfo{
		ServerVersion: serverVersion,
		Details:       map[string]string{"database": database, "encoding": encoding},
	}, nil
}

// Disconnect is index.ts's disconnect — relational.Disconnect carries the body shared with
// mysqlfamily's identical Disconnect: the cancel-then-drain sequencing, nothing else
// makes RunWithAbortRace's own background goroutines (inFlight's own doc comment) stop touching
// their connection, and Drain's own wait is bounded by ctx rather than able to block this call for
// as long as the longest still-running query.
func (a *Adapter) Disconnect(ctx context.Context) error {
	return relational.Disconnect(ctx, a.tracker.Snapshot(), a.Cancel, a.tracker.Drain, a.state.Load().ConnSet,
		func(cs *ConnSet, ctx context.Context) { cs.CloseAll(ctx) }, a.clearConnected)
}

// requireClient returns database's connection together with a release func that must be called
// exactly once — it holds the per-connection lock ConnSet.Acquire's own doc comment describes for
// as long as the caller keeps conn (P2 R2).
func (a *Adapter) requireClient(ctx context.Context, database string) (*trackedConn, func(), error) {
	connSet, err := adapters.RequireConnected(a.state.Load().ConnSet)
	if err != nil {
		return nil, nil, err
	}
	return connSet.Acquire(ctx, database)
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	segments := path.Segments

	if len(segments) == 0 {
		conn, release, err := a.requireClient(ctx, "")
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		defer release()
		nodes, err := listDatabases(ctx, execFor(conn, op, a.trackerFor(op.OpID)), a.state.Load().PrimaryDatabase)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	databaseSegment := segments[0]
	if databaseSegment.Kind != "database" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, databaseSegment.Kind)
	}
	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	defer release()
	exec := execFor(conn, op, a.trackerFor(op.OpID))

	if len(segments) == 1 {
		nodes, err := listSchemas(ctx, exec, databaseSegment.Name)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	schemaSegment := segments[1]
	if schemaSegment.Kind != "schema" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(1, schemaSegment.Kind)
	}

	if len(segments) == 2 {
		nodes, err := listRelationsAndFunctions(ctx, exec, databaseSegment.Name, schemaSegment.Name)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	objectSegment := segments[2]
	if len(segments) == 3 {
		// A table lists its declarative partitions ([] when none). view/matview columns moved
		// into the definition view, so every other kind is a leaf and returns [], never an error.
		if objectSegment.Kind == "table" {
			nodes, err := listPartitions(ctx, exec, databaseSegment.Name, schemaSegment.Name, objectSegment.Name)
			if err != nil {
				return adapters.TreeChildren{}, err
			}
			return adapters.TreeChildren{Nodes: nodes}, nil
		}
		return adapters.LeafChildren(objectSegment.Kind, leafObjectKinds)
	}

	return adapters.TreeChildren{}, adapters.Contract("unrecognized path depth", nil)
}

var leafObjectKinds = map[string]bool{"sequence": true, "function": true, "view": true, "matview": true}

// Describe is index.ts's describe.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	// P113 G2: adapters.RequirePath carries the depth+kind check requireThreeSegmentObjectPath used
	// to spell out by hand — the object segment stays kind-unconstrained here, same as before,
	// since Describe reports back whatever Kind it was given rather than filtering on it.
	segs, err := adapters.RequirePath(path, "describe", adapters.Seg("database"), adapters.Seg("schema"), adapters.AnySeg("table"))
	if err != nil {
		return model.ObjectMeta{}, err
	}
	databaseSegment, schemaSegment, objectSegment := segs[0], segs[1], segs[2]

	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	defer release()
	exec := execFor(conn, op, a.trackerFor(op.OpID))

	info, err := getRelationInfo(ctx, exec, schemaSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	// Sequential, not concurrent: exec routes every one of these through the same single
	// connection (D14 — one Conn per connection/database, never a pool), and pgx does not support
	// concurrent queries on one connection.
	rawColumns, err := listColumns(ctx, exec, schemaSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	indexes, err := listIndexes(ctx, exec, info.OID)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	foreignKeys, err := listForeignKeys(ctx, exec, info.OID, databaseSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	referencedBy, err := listReferencedBy(ctx, exec, info.OID, databaseSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	primaryKey := adapters.PrimaryKeyFromIndexes(indexes)
	columns := adapters.MarkPrimaryKey(rawColumns, primaryKey)

	return model.ObjectMeta{
		Path: model.EncodePath(path.Segments), Kind: objectSegment.Kind, Name: objectSegment.Name,
		QualifiedName: schemaSegment.Name + "." + objectSegment.Name, Columns: columns,
		PrimaryKey: primaryKey, ForeignKeys: foreignKeys, ReferencedBy: referencedBy,
		Indexes: indexes, RowEstimate: intPtrFrom(info.RowEstimate), Comment: info.Comment,
	}, nil
}

// SchemaColumns is P22c D1's schema-wide sibling of Describe: every relation in a schema together
// with its columns, in one round trip.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	segs, err := adapters.RequirePath(path, "schemaColumns", adapters.Seg("database"), adapters.Seg("schema"))
	if err != nil {
		return nil, err
	}
	databaseSegment, schemaSegment := segs[0], segs[1]

	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return nil, err
	}
	defer release()
	exec := execFor(conn, op, a.trackerFor(op.OpID))
	return listSchemaColumns(ctx, exec, schemaSegment.Name)
}

func intPtrFrom(v *int64) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

var definitionSupportedKinds = map[string]bool{"table": true, "view": true, "matview": true}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	// P113 G2: same depth/kind shape as Describe above — the object segment stays unconstrained
	// here too; definitionSupportedKinds below is a separate, narrower filter Definition applies on
	// top (fewer kinds than Describe's own leafObjectKinds), not a replacement for it.
	segs, err := adapters.RequirePath(path, "definition", adapters.Seg("database"), adapters.Seg("schema"), adapters.AnySeg("table"))
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	databaseSegment, schemaSegment, objectSegment := segs[0], segs[1], segs[2]
	if !definitionSupportedKinds[objectSegment.Kind] {
		return model.ObjectDefinition{}, adapters.Unsupported("postgres", "definition for "+objectSegment.Kind)
	}

	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	defer release()
	exec := execFor(conn, op, a.trackerFor(op.OpID))
	return buildDefinition(ctx, exec, path.Segments, schemaSegment.Name, objectSegment.Kind, objectSegment.Name)
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	// P113 G2: unlike Describe/Definition above, the object segment is kind-checked here — Read only
	// ever targets a table/view/matview, never a bare function or sequence.
	segs, err := adapters.RequirePath(req.Path, "read", adapters.Seg("database"), adapters.Seg("schema"), adapters.Seg("table", "table", "view", "matview"))
	if err != nil {
		return nil, err
	}
	databaseSegment, schemaSegment, objectSegment := segs[0], segs[1], segs[2]
	if err := assertReadOnlyFilterSortSafe(a.state.Load().ReadOnly, req.Filter, req.Sort); err != nil {
		return nil, err
	}
	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return nil, err
	}
	defer release()
	target, err := getReadTarget(ctx, execFor(conn, op, a.trackerFor(op.OpID)), schemaSegment.Name, objectSegment.Name)
	if err != nil {
		return nil, err
	}
	if req.StatementTimeout > 0 {
		restore, err := setSessionTimeout(ctx, conn, op, a.trackerFor(op.OpID), req.StatementTimeout)
		if err != nil {
			return nil, err
		}
		defer restore()
	}
	result, err := readPage(ctx, conn, op, a.trackerFor(op.OpID), target, readReq{
		Projection: req.Projection, Filter: req.Filter, Sort: req.Sort, PageSize: req.PageSize, Cursor: req.Cursor,
		Conditions: req.Conditions,
	})
	if err != nil {
		return nil, timeoutError(ctx, req.StatementTimeout, err)
	}
	return result, nil
}

// Count is index.ts's count. P13 D13: count() never reads columns/PK/indexes/oid off the target,
// so it resolves only the qualified name — not the three catalog queries getReadTarget costs.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	segs, err := adapters.RequirePath(req.Path, "count", adapters.Seg("database"), adapters.Seg("schema"), adapters.Seg("table", "table", "view", "matview"))
	if err != nil {
		return adapters.CountResult{}, err
	}
	databaseSegment, schemaSegment, objectSegment := segs[0], segs[1], segs[2]
	if err := assertReadOnlyFilterSortSafe(a.state.Load().ReadOnly, req.Filter, nil); err != nil {
		return adapters.CountResult{}, err
	}
	conn, release, err := a.requireClient(ctx, databaseSegment.Name)
	if err != nil {
		return adapters.CountResult{}, err
	}
	defer release()
	target := QualifiedName{Schema: schemaSegment.Name, Relation: objectSegment.Name}
	return countRows(ctx, conn, op, a.trackerFor(op.OpID), target, req.Filter)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	return preview(plan)
}

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	if len(plan.Path.Segments) == 0 || plan.Path.Segments[0].Kind != "database" {
		return model.MutationResult{}, adapters.Contract("unexpected root path segment kind", nil)
	}
	conn, release, err := a.requireClient(ctx, plan.Path.Segments[0].Name)
	if err != nil {
		return model.MutationResult{}, err
	}
	defer release()
	return mutate(ctx, conn, op, a.trackerFor(op.OpID), a.state.Load().ReadOnly, plan)
}

// Execute is index.ts's execute.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	database := ""
	if len(req.Path.Segments) > 0 && req.Path.Segments[0].Kind == "database" {
		database = req.Path.Segments[0].Name
	}
	conn, release, err := a.requireClient(ctx, database)
	if err != nil {
		return nil, err
	}
	defer release()
	return execute(ctx, conn, op, a.trackerFor(op.OpID), a.state.Load().ReadOnly || req.ReadOnly, a.state.Load().ReadOnly, req.Statements, adapters.ConsoleCapFor(req), req.StatementTimeout)
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false, so no UI ever offers
// Download for postgres; never reached.
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("postgres", "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A relational table's rows have no per-item
// engine-level "type" the way a redis key does.
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("postgres", "key types")
}

// Cancel is index.ts's cancel.
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	running, ok := a.tracker.PopRunningWith(opID, func(q RunningQuery) {
		if q.Guard != nil {
			q.Guard.BeginCancel()
		}
	})
	if ok && running.Guard != nil {
		defer running.Guard.EndCancel()
	}
	cfg := a.state.Load().Cfg
	if !ok || cfg == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, cancelTimeout)
	defer cancel()

	connConfig, err := buildConfig(*cfg, "", a.deps.Log)
	if err != nil {
		a.deps.Log("warn", "postgres cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	side, err := pgx.ConnectConfig(ctx, connConfig)
	if err != nil {
		a.deps.Log("warn", "postgres cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	defer side.Close(context.Background())

	var cancelled bool
	if err := side.QueryRow(ctx, "SELECT pg_cancel_backend($1)", running.BackendPID).Scan(&cancelled); err != nil {
		a.deps.Log("warn", "postgres cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	return cancelled, nil
}

// trackerFor is index.ts's trackerFor: registers the running query and hands back its
// own release. The identity check in the release closure is what makes a multi-statement op
// (mutate's BEGIN/…/COMMIT, console's "Run all") correct — an earlier statement settling after a
// later one has started must not unregister the later one, since both share this one opId.
func (a *Adapter) trackerFor(opID string) TrackQuery {
	return a.tracker.TrackerFor(opID)
}
