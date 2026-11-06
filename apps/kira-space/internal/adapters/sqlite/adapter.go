package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"sync"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func init() {
	adapters.Register("sqlite", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is db/file/readOnly: Connect/Disconnect wrote them with no lock, racing runOnConn's own
// reads (a.db == nil, a.db.Conn(ctx)) and Mutate's a.readOnly read — a real data race, same class
// as postgres/mysqlfamily/clickhouse's own adapter.go. Guarded together via adapters.Guarded,
// separately from runningByOp below (its own mutex, an unrelated concern: op-cancellation
// bookkeeping, not connection state, and every access to it was already its own atomic
// lock/unlock, never combined with a connState field in one critical section).
type connState struct {
	db       *sql.DB
	file     string
	readOnly bool
}

// Adapter is index.ts's SqliteAdapter — P35 D17/D18: stands alone, no "family" pattern (that
// exists in mysqlfamily/ because two engines share one wire protocol and one driver, P34 D7).
// SQLite shares neither with anything.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]

	mu          sync.Mutex
	runningByOp map[string]context.CancelFunc
	// draining is set by Disconnect under mu: runOnConn refuses to start (and so never Adds to
	// inFlight) once it is set, since sync.WaitGroup forbids an Add racing a Wait at zero.
	draining bool

	// inFlight mirrors postgres's and mysqlfamily's own field of the same name: it counts
	// runOnConn's own background goroutines that are still touching a *sql.Conn after RunWithAbortRace
	// has already returned to its caller (a local op abort must not itself kill the query — B8's
	// own point is the opposite direction of B6/B14, but the underlying race is identical: closing
	// a connection while a goroutine is still using it is a data race regardless of which adapter).
	inFlight sync.WaitGroup
}

func (a *Adapter) Kind() string        { return "sqlite" }
func (a *Adapter) Caps() adapters.Caps { return caps }

// setConnected is Connect's own locked write of db/file/readOnly together (F3).
func (a *Adapter) setConnected(db *sql.DB, file string, readOnly bool) {
	a.mu.Lock()
	a.draining = false
	a.mu.Unlock()
	a.state.Update(func(s *connState) { s.db = db; s.file = file; s.readOnly = readOnly })
}

// beginOp registers one runOnConn goroutine against inFlight, or refuses once Disconnect started.
func (a *Adapter) beginOp() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.draining {
		return false
	}
	a.inFlight.Add(1)
	return true
}

// clearConnected is Disconnect's own locked write (F3). readOnly is deliberately left set.
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) { s.db = nil; s.file = "" })
}

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	path, err := resolveFilePath(cfg)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	if err := assertFileExists(path); err != nil {
		return adapters.ConnectInfo{}, err
	}

	db, err := sql.Open("sqlite", buildDSN(path, cfg.ReadOnly))
	if err != nil {
		return adapters.ConnectInfo{}, mapError(err)
	}
	db.SetMaxOpenConns(1)

	// P13 D1: assigned before anything is opened, not after the probe succeeds — the handle must
	// be reachable by Disconnect from the instant sql.Open's own lazy dial could happen.
	a.setConnected(db, path, cfg.ReadOnly)

	conn, err := db.Conn(ctx)
	if err != nil {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, connectError(ctx, mapError(err))
	}
	defer conn.Close()

	// The file-format check is lazy, not eager (opening a garbage file succeeds silently and only
	// fails on first real statement) — this probe is what makes a bad-format file surface as
	// E_CONNECT during connect(), not later on the first tree expansion.
	var version string
	found := false
	err = runRows(ctx, conn, "SELECT sqlite_version() AS version", nil, op, false, func(r *sql.Rows) error {
		found = true
		return r.Scan(&version)
	})
	if err != nil {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, connectError(ctx, err)
	}
	if !found {
		_ = a.Disconnect(context.Background())
		return adapters.ConnectInfo{}, adapters.Contract("connect probe returned no rows", nil)
	}

	// D6: read-only pragma reads for the connection tooltip — never written by this adapter.
	var journalMode string
	_ = runRows(ctx, conn, "PRAGMA journal_mode", nil, op, false, func(r *sql.Rows) error {
		return r.Scan(&journalMode)
	})
	var pageSize int
	_ = runRows(ctx, conn, "PRAGMA page_size", nil, op, false, func(r *sql.Rows) error {
		return r.Scan(&pageSize)
	})

	details := map[string]string{"file": path, "journalMode": journalMode}
	if pageSize > 0 {
		details["pageSize"] = strconv.Itoa(pageSize)
	}
	return adapters.ConnectInfo{ServerVersion: "SQLite " + version, Details: details}, nil
}

// connectError reports a cancelled Connect as E_CANCELLED instead of whatever the interrupted
// driver call surfaced.
func connectError(ctx context.Context, err error) error {
	if cancelErr := adapters.CheckCancelled(ctx); cancelErr != nil {
		return cancelErr
	}
	return err
}

// Disconnect is index.ts's disconnect.
func (a *Adapter) Disconnect(ctx context.Context) error {
	// F4: cancel every still-running statement's own driverCtx first — a real, local
	// sqlite3_interrupt via ctx cancellation (modernc.org/sqlite's own interruptOnDone), no network
	// round trip needed here unlike postgres/mysqlfamily/clickhouse's own side-connection kill —
	// then wait bounded by ctx rather than block this call for as long as the longest still-running
	// query. Every runOnConn background goroutine still touching a connection must actually stop
	// before the *sql.DB is closed (see inFlight's own doc comment): db.Close() and the rest of this
	// cleanup only run once inFlight has actually reached zero, in a goroutine that keeps running
	// past ctx's own deadline if cancelling every statement hasn't unblocked it by then, so a
	// connection is never closed out from under a still-running goroutine even when this call
	// itself already returned.
	a.mu.Lock()
	a.draining = true
	cancels := make([]context.CancelFunc, 0, len(a.runningByOp))
	for _, cancel := range a.runningByOp {
		cancels = append(cancels, cancel)
	}
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		a.inFlight.Wait()
		if db := a.state.Load().db; db != nil {
			if err := db.Close(); err != nil {
				a.deps.Log("warn", "sqlite disconnect: "+err.Error())
			}
		}
		a.clearConnected()
		a.mu.Lock()
		a.runningByOp = nil
		a.mu.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

// runOnConn is the cancellation design B8/P58 D8 exists for: every statement runs on a context the
// adapter owns, derived from Background rather than from the op's own ctx. adapterhost.Host.CancelOp
// cancels the op context first (unblocking the caller immediately) and then calls Cancel(opID) —
// and only that second step may reach the statement, or "cancelled" would mean "we stopped
// waiting", not "the server actually stopped". modernc.org/sqlite's own interruptOnDone semantics
// turn cancelling driverCtx into a real sqlite3_interrupt on this op's own dedicated *sql.Conn, so
// it can only ever reach the statement it was aimed at (P58 D8's third part: a conn per op, not a
// shared pinned one — mysqlfamily's and postgres's own dedicated connection is pinned per adapter
// instance instead, since their cancellation goes through a side connection, not sqlite3_interrupt).
func runOnConn[T any](ctx context.Context, a *Adapter, opID string, fn func(context.Context, *sql.Conn) (T, error)) (T, error) {
	var zero T
	db := a.state.Load().db
	if db == nil || !a.beginOp() {
		return zero, adapters.T(adapters.CodeConnect, "backend.studio.adapter.notConnected.body", nil, nil)
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		a.inFlight.Done()
		// SetMaxOpenConns(1) means a second op can genuinely queue here waiting for the
		// sole connection — database/sql's own db.conn() returns exactly ctx.Err() when the wait
		// is cut short by cancellation, and only then, so this can't misfire on a real connect
		// failure. Without this check that op:end reported E_QUERY instead of E_CANCELLED — a
		// generic error toast in the renderer instead of the silent stop Cancel is supposed to be.
		if cancelErr := adapters.CheckCancelled(ctx); cancelErr != nil {
			return zero, cancelErr
		}
		return zero, mapError(err)
	}

	driverCtx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.draining {
		// Disconnect started while this op queued for the connection and could not cancel it.
		a.mu.Unlock()
		cancel()
		_ = conn.Close()
		a.inFlight.Done()
		return zero, adapters.T(adapters.CodeConnect, "backend.studio.adapter.notConnected.body", nil, nil)
	}
	if a.runningByOp == nil {
		a.runningByOp = make(map[string]context.CancelFunc)
	}
	a.runningByOp[opID] = cancel
	a.mu.Unlock()

	return adapters.RunWithAbortRace(ctx, func() {
		// release: called exactly once, whenever fn actually settles — not merely when ctx fires
		// (RunWithAbortRace's own contract) — so the connection is never closed out from under a
		// goroutine that is still using it, even when the caller's own ctx won the race.
		cancel()
		a.mu.Lock()
		delete(a.runningByOp, opID)
		a.mu.Unlock()
		_ = conn.Close()
		a.inFlight.Done()
	}, func(context.Context) (T, error) {
		return fn(driverCtx, conn)
	})
}

// Cancel is index.ts's cancel — B8's second half. caps.Cancel is true because this actually
// interrupts a running statement, a change from the TypeScript adapter, whose node:sqlite had no
// sqlite3_interrupt at all (P58 D8).
func (a *Adapter) Cancel(_ context.Context, opID string) (bool, error) {
	a.mu.Lock()
	cancel, ok := a.runningByOp[opID]
	delete(a.runningByOp, opID)
	a.mu.Unlock()
	if !ok {
		return false, nil
	}
	cancel()
	return true, nil
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (adapters.TreeChildren, error) {
		segments := path.Segments
		exec := execFor(driverCtx, conn, op)

		if len(segments) == 0 {
			nodes, err := listDatabases(exec)
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
			nodes, err := listTablesAndViews(exec, databaseSegment.Name)
			if err != nil {
				return adapters.TreeChildren{}, err
			}
			return adapters.TreeChildren{Nodes: nodes}, nil
		}

		objectSegment := segments[1]
		if len(segments) == 2 {
			// Rule 5 (Adapter doc comment): every relation is a leaf (P19 D5) — its columns live
			// in the definition view, not the tree.
			return adapters.LeafChildren(objectSegment.Kind, leafObjectKinds)
		}

		return adapters.TreeChildren{}, adapters.Contract("unrecognized path depth", nil)
	})
}

var leafObjectKinds = map[string]bool{"table": true, "view": true}

// Describe is index.ts's describe.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	segs, err := adapters.RequirePath(path, "describe", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return model.ObjectMeta{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (model.ObjectMeta, error) {
		exec := execFor(driverCtx, conn, op)
		target, err := getReadTarget(exec, databaseSegment.Name, objectSegment.Name)
		if err != nil {
			return model.ObjectMeta{}, err
		}
		indexes, err := listIndexes(exec, databaseSegment.Name, objectSegment.Name)
		if err != nil {
			return model.ObjectMeta{}, err
		}
		foreignKeys, err := listForeignKeys(exec, databaseSegment.Name, objectSegment.Name)
		if err != nil {
			return model.ObjectMeta{}, err
		}
		allTables, err := listAllTableNames(exec, databaseSegment.Name)
		if err != nil {
			return model.ObjectMeta{}, err
		}
		referencedBy, err := listReferencedBy(exec, databaseSegment.Name, objectSegment.Name, allTables)
		if err != nil {
			return model.ObjectMeta{}, err
		}
		var rowEstimate *int
		if objectSegment.Kind == "table" {
			rowEstimate, err = getRowEstimateFor(exec, objectSegment.Name)
			if err != nil {
				return model.ObjectMeta{}, err
			}
		}

		return model.ObjectMeta{
			Path: model.EncodePath(path.Segments), Kind: objectSegment.Kind, Name: objectSegment.Name,
			QualifiedName: databaseSegment.Name + "." + objectSegment.Name, Columns: target.Columns,
			PrimaryKey: target.PrimaryKey, ForeignKeys: foreignKeys, ReferencedBy: referencedBy,
			Indexes: indexes, RowEstimate: rowEstimate, Comment: nil, // SQLite has no column/table comment concept
		}, nil
	})
}

// SchemaColumns is P22c D1's schema-wide sibling of Describe: every relation in the database
// together with its columns, in one round trip.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	segs, err := adapters.RequirePath(path, "schemaColumns", adapters.Seg("database"))
	if err != nil {
		return nil, err
	}
	databaseSegment := segs[0]
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) ([]model.RelationColumns, error) {
		exec := execFor(driverCtx, conn, op)
		return listSchemaColumns(exec, databaseSegment.Name)
	})
}

// Definition is index.ts's definition.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	segs, err := adapters.RequirePath(path, "definition", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (model.ObjectDefinition, error) {
		exec := execFor(driverCtx, conn, op)
		return buildDefinition(exec, path.Segments, databaseSegment.Name, objectSegment.Kind, objectSegment.Name)
	})
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	segs, err := adapters.RequirePath(req.Path, "read", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return nil, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (page.Page, error) {
		exec := execFor(driverCtx, conn, op)
		target, err := getReadTarget(exec, databaseSegment.Name, objectSegment.Name)
		if err != nil {
			return nil, err
		}
		result, err := readPage(driverCtx, conn, op, target, readReq{
			Projection: req.Projection, Filter: req.Filter, Sort: req.Sort,
			PageSize: req.PageSize, Cursor: req.Cursor, Conditions: req.Conditions,
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	segs, err := adapters.RequirePath(req.Path, "count", adapters.Seg("database"), adapters.Seg("table", "table", "view"))
	if err != nil {
		return adapters.CountResult{}, err
	}
	databaseSegment, objectSegment := segs[0], segs[1]
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (adapters.CountResult, error) {
		target := QualifiedName{Database: databaseSegment.Name, Table: objectSegment.Name}
		return countRows(driverCtx, conn, op, target, req.Filter)
	})
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	return preview(plan)
}

// Mutate is index.ts's mutate.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) (model.MutationResult, error) {
		return mutate(driverCtx, conn, op, a.state.Load().readOnly, plan)
	})
}

// Execute is index.ts's execute.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	return runOnConn(ctx, a, op.OpID, func(driverCtx context.Context, conn *sql.Conn) ([]page.Page, error) {
		return execute(driverCtx, conn, op, req.Statements, adapters.ConsoleCapFor(req))
	})
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false, so no UI ever offers
// Download for sqlite; never reached.
func (a *Adapter) DownloadObject(context.Context, model.ObjectDownloadRequest, *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("sqlite", "file transfer")
}

// KeyTypes — caps.KeyTypes is false; unreachable. A table's rows have no per-item engine-level
// "type" the way a redis key does.
func (a *Adapter) KeyTypes(context.Context, []model.NodePath, *adapters.OpCtx) ([]string, error) {
	return nil, adapters.Unsupported("sqlite", "key types")
}
