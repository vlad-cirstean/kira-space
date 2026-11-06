package mysqlfamily

import (
	"context"
	"database/sql"
	"regexp"
	"strconv"
	"sync/atomic"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters/relational"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
)

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// — a real data race, not a theoretical one, same class as postgres/adapter.go's own (the
// running-query bookkeeping below is unrelated, tracker's own lock). Guarded together
// via adapters.Guarded; shared with postgres's identical shape via relational.ConnState.
type connState = relational.ConnState[*ConnSet]

// Adapter is index.ts's MysqlFamilyAdapter — one implementation for both MariaDB and MySQL,
// parameterized by a Profile (P34 D7/D9) plus a per-engine Caps literal (D10).
type Adapter struct {
	deps    adapters.Deps
	profile Profile
	caps    adapters.Caps

	state adapters.Guarded[connState]

	// mariadbServer is set at Connect from VERSION(): the timeout variable depends on the server,
	// not on the kind the connection was filed under.
	mariadbServer atomic.Bool

	tracker adapters.QueryTracker[RunningQuery]
}

// setConnSet is Connect's own locked write of ConnSet/Cfg together (F3) — P13 D1: assigned before
// anything is opened, not after the probe succeeds.
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

// New constructs an Adapter for profile/caps — mariadb/adapter.go's and mysql/adapter.go's own
// init() call this, each with their own Profile and Caps literal (P34 D7).
func New(deps adapters.Deps, profile Profile, caps adapters.Caps) adapters.Adapter {
	return &Adapter{deps: deps, profile: profile, caps: caps}
}

func (a *Adapter) Kind() string        { return a.profile.Kind }
func (a *Adapter) Caps() adapters.Caps { return a.caps }

var mariadbVersionRE = regexp.MustCompile(`(?i)mariadb`)

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	connSet := NewConnSet(cfg, a.profile, a.deps.Log)
	a.setConnSet(connSet, &cfg)

	entry, release, err := connSet.Primary(ctx)
	if err != nil {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, err
	}

	exec := execFor(entry, op, a.trackerFor(op.OpID))
	var serverVersion, charset string
	// DATABASE() is SQL NULL, not "", whenever the connection was opened with no default
	// schema (client.go's BuildConfig only sets mc.DBName when cfg.Database is non-empty) — a
	// completely ordinary case for a connection meant to browse every database on the server
	// (docs/ARCHITECTURE.md's PostgreSQL / MariaDB / MySQL section, database -> tables), and one
	// Validate (input.go)
	// deliberately allows. Scanning that NULL into a plain string panics-the-query with "converting
	// NULL to string is unsupported", failing Connect outright for every such connection — a
	// sql.NullString is what tolerates it.
	var database sql.NullString
	found := false
	err = exec(ctx, "SELECT VERSION() AS version, DATABASE() AS `database`, @@character_set_server AS charset", nil,
		func(rows *sql.Rows) error {
			found = true
			return rows.Scan(&serverVersion, &database, &charset)
		})
	if err != nil {
		// release() must run before Disconnect: Disconnect->ConnSet.CloseAll takes this same
		// entry's lock, and sync.Mutex isn't reentrant — calling Disconnect while still holding
		// the lock this call frame acquired above deadlocks the goroutine permanently.
		release()
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, err
	}
	if !found {
		release()
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, adapters.Contract("connect probe returned no rows", nil)
	}
	release()

	a.setConnected(database.String, cfg.ReadOnly)
	a.mariadbServer.Store(mariadbVersionRE.MatchString(serverVersion))

	// D6: pointing the MySQL adapter at a MariaDB server (or vice versa) works — same driver, same
	// wire protocol — so this is a warning, not a connect failure.
	if a.profile.Kind == "mysql" && mariadbVersionRE.MatchString(serverVersion) {
		a.deps.Log("warn", "mysql: connected server identifies as MariaDB ("+serverVersion+")")
	}

	return adapters.ConnectInfo{
		ServerVersion: a.profile.ServerLabel + " " + serverVersion,
		Details:       map[string]string{"database": database.String, "charset": charset},
	}, nil
}

// Disconnect is index.ts's disconnect — relational.Disconnect carries the body shared with
// postgres's identical Disconnect: the cancel-then-drain sequencing (KILL QUERY over a
// side connection, this adapter's existing Cancel path, before Drain, whose own wait is bounded by
// ctx rather than able to block this call for as long as the longest still-running query).
func (a *Adapter) Disconnect(ctx context.Context) error {
	return relational.Disconnect(ctx, a.tracker.Snapshot(), a.Cancel, a.tracker.Drain, a.state.Load().ConnSet,
		func(cs *ConnSet, ctx context.Context) { cs.CloseAll(ctx) }, a.clearConnected)
}

// requireEntry returns database's pinned connection together with a release func that must be
// called exactly once — it holds the per-connection lock connEntry's own doc comment describes,
// for as long as the caller keeps entry (P21 round 2 performance finding 3).
func (a *Adapter) requireEntry(ctx context.Context, database string) (Entry, func(), error) {
	connSet, err := adapters.RequireConnected(a.state.Load().ConnSet)
	if err != nil {
		return Entry{}, nil, err
	}
	return connSet.Acquire(ctx, database)
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	segments := path.Segments

	if len(segments) == 0 {
		entry, release, err := a.requireEntry(ctx, "")
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		defer release()
		nodes, err := listDatabases(ctx, execFor(entry, op, a.trackerFor(op.OpID)), a.state.Load().PrimaryDatabase)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	databaseSegment := segments[0]
	if databaseSegment.Kind != "database" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, databaseSegment.Kind)
	}
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	defer release()
	exec := execFor(entry, op, a.trackerFor(op.OpID))

	if len(segments) == 1 {
		nodes, err := listTablesAndRoutines(ctx, exec, databaseSegment.Name)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	objectSegment := segments[1]
	if len(segments) == 2 {
		// Rule 5: Children returns [] for a leaf, never an error. P19 D5: every relation is a leaf.
		return adapters.LeafChildren(objectSegment.Kind, leafObjectKinds)
	}

	return adapters.TreeChildren{}, adapters.Contract("unrecognized path depth", nil)
}

var leafObjectKinds = map[string]bool{"sequence": true, "function": true, "table": true, "view": true}

// Describe is index.ts's describe.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	// P113 G2: adapters.RequirePath carries the depth+kind check requireTwoSegmentPath used to spell
	// out by hand — the object segment stays kind-unconstrained here, same as before, since Describe
	// reports back whatever Kind it was given rather than filtering on it.
	segs, err := adapters.RequirePath(path, "describe", adapters.Seg("database"), adapters.AnySeg("table"))
	if err != nil {
		return model.ObjectMeta{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	defer release()
	exec := execFor(entry, op, a.trackerFor(op.OpID))

	rawColumns, err := listColumns(ctx, exec, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	indexes, err := listIndexes(ctx, exec, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	foreignKeys, err := listForeignKeys(ctx, exec, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	referencedBy, err := listReferencedBy(ctx, exec, databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	primaryKey := adapters.PrimaryKeyFromIndexes(indexes)
	columns := adapters.MarkPrimaryKey(rawColumns, primaryKey)

	var tableRows *int64
	var comment *string
	err = exec(ctx, `SELECT TABLE_ROWS AS table_rows, TABLE_COMMENT AS comment
	 FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`,
		[]any{databaseSegment.Name, objectSegment.Name}, func(rows *sql.Rows) error {
			return rows.Scan(&tableRows, &comment)
		})
	if err != nil {
		return model.ObjectMeta{}, err
	}
	var rowEstimate *int
	if tableRows != nil {
		n := int(*tableRows)
		rowEstimate = &n
	}
	if comment != nil && *comment == "" {
		comment = nil
	}

	return model.ObjectMeta{
		Path: model.EncodePath(path.Segments), Kind: objectSegment.Kind, Name: objectSegment.Name,
		QualifiedName: databaseSegment.Name + "." + objectSegment.Name, Columns: columns,
		PrimaryKey: primaryKey, ForeignKeys: foreignKeys, ReferencedBy: referencedBy,
		Indexes: indexes, RowEstimate: rowEstimate, Comment: comment,
	}, nil
}

// SchemaColumns is P22c D1's schema-wide sibling of Describe: every relation in the database
// together with its columns, in one round trip.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	segs, err := adapters.RequirePath(path, "schemaColumns", adapters.Seg("database"))
	if err != nil {
		return nil, err
	}
	databaseSegment := segs[0]
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return nil, err
	}
	defer release()
	exec := execFor(entry, op, a.trackerFor(op.OpID))
	return listSchemaColumns(ctx, exec, databaseSegment.Name)
}

var definitionSupportedKinds = map[string]bool{"table": true, "view": true}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	// P113 G2: same depth/kind shape as Describe above — the object segment stays unconstrained
	// here too; definitionSupportedKinds below is a separate, narrower filter applied on top.
	segs, err := adapters.RequirePath(path, "definition", adapters.Seg("database"), adapters.AnySeg("table"))
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	if !definitionSupportedKinds[objectSegment.Kind] {
		return model.ObjectDefinition{}, adapters.Unsupported(a.Kind(), "definition for "+objectSegment.Kind)
	}
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	defer release()
	exec := execFor(entry, op, a.trackerFor(op.OpID))
	return buildDefinition(ctx, exec, path.Segments, databaseSegment.Name, objectSegment.Kind, objectSegment.Name)
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	// P113 G2: unlike Describe/Definition above, the object segment is kind-checked here — Read only
	// ever targets a table/view, never a bare function/sequence.
	segs, err := adapters.RequirePath(req.Path, "read", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return nil, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return nil, err
	}
	defer release()
	target, err := getReadTarget(ctx, execFor(entry, op, a.trackerFor(op.OpID)), databaseSegment.Name, objectSegment.Name)
	if err != nil {
		return nil, err
	}
	if req.StatementTimeout > 0 {
		restore, err := setSessionTimeout(ctx, entry, req.StatementTimeout, a.mariadbServer.Load())
		if err != nil {
			return nil, err
		}
		defer restore()
	}
	result, err := readPage(ctx, entry, op, a.trackerFor(op.OpID), target, readReq{
		Projection: req.Projection, Filter: req.Filter, Sort: req.Sort, PageSize: req.PageSize, Cursor: req.Cursor,
		Conditions: req.Conditions,
	})
	if err != nil {
		return nil, timeoutError(ctx, req.StatementTimeout, err)
	}
	return result, nil
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	segs, err := adapters.RequirePath(req.Path, "count", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return adapters.CountResult{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	entry, release, err := a.requireEntry(ctx, databaseSegment.Name)
	if err != nil {
		return adapters.CountResult{}, err
	}
	defer release()
	target := QualifiedName{Database: databaseSegment.Name, Table: objectSegment.Name}
	return countRows(ctx, entry, op, a.trackerFor(op.OpID), target, req.Filter)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) { return preview(plan) }

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	if len(plan.Path.Segments) == 0 || plan.Path.Segments[0].Kind != "database" {
		return model.MutationResult{}, adapters.Contract("unexpected root path segment kind", nil)
	}
	entry, release, err := a.requireEntry(ctx, plan.Path.Segments[0].Name)
	if err != nil {
		return model.MutationResult{}, err
	}
	defer release()
	return mutate(ctx, entry, op, a.trackerFor(op.OpID), a.state.Load().ReadOnly, plan)
}

// Execute is index.ts's execute.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	database := ""
	if len(req.Path.Segments) > 0 && req.Path.Segments[0].Kind == "database" {
		database = req.Path.Segments[0].Name
	}
	entry, release, err := a.requireEntry(ctx, database)
	if err != nil {
		return nil, err
	}
	defer release()
	return execute(ctx, entry, op, a.trackerFor(op.OpID), a.state.Load().ReadOnly || req.ReadOnly, a.state.Load().ReadOnly, req.Statements, adapters.ConsoleCapFor(req), req.StatementTimeout, a.mariadbServer.Load())
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false; never reached.
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported(a.Kind(), "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A relational table's rows have no per-item
// engine-level "type" the way a redis key does.
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported(a.Kind(), "key types")
}

// Cancel is index.ts's cancel — a short-lived side connection, mirroring Postgres's
// pg_cancel_backend path (D26). Killing your own query needs no PROCESS/SUPER privilege — only
// killing someone else's does.
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

	mc, err := BuildConfig(*cfg, "", a.profile, a.deps.Log)
	if err != nil {
		a.deps.Log("warn", a.Kind()+" cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		a.deps.Log("warn", a.Kind()+" cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	side := sql.OpenDB(connector)
	defer side.Close()

	if _, err := side.ExecContext(ctx, "KILL QUERY "+strconv.FormatUint(uint64(running.ThreadID), 10)); err != nil {
		a.deps.Log("warn", a.Kind()+" cancel("+opID+") failed: "+err.Error())
		return false, nil
	}
	return true, nil
}

// trackerFor is index.ts's trackerFor (P13 D3): registers the running query and hands back its own
// release, identity-checked so an earlier statement settling after a later one has started never
// unregisters the later one.
func (a *Adapter) trackerFor(opID string) TrackQuery {
	return a.tracker.TrackerFor(opID)
}
