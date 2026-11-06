package redis

import (
	"context"
	"regexp"
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func init() {
	adapters.Register("redis", func(deps adapters.Deps) (adapters.Adapter, error) {
		return &Adapter{deps: deps}, nil
	})
}

// connState is every field Connect/Disconnect write concurrently with an in-flight op reading them
// (F3) — set/defaultDbIndex/readOnly, guarded together via adapters.Guarded (P113 G1).
type connState struct {
	set            *dbConnectionSet
	defaultDbIndex int
	readOnly       bool
}

// Adapter is index.ts's RedisAdapter.
type Adapter struct {
	deps adapters.Deps

	state adapters.Guarded[connState]
}

// setConnected is Connect's own locked write of every field a successful connect fills in (F3).
func (a *Adapter) setConnected(set *dbConnectionSet, defaultDbIndex int, readOnly bool) {
	a.state.Update(func(s *connState) {
		s.set = set
		s.defaultDbIndex = defaultDbIndex
		s.readOnly = readOnly
	})
}

// clearConnected is Disconnect's own locked write, once closeAll (a real network call, run with no
// lock held) has returned (F3). readOnly is deliberately left set.
func (a *Adapter) clearConnected() {
	a.state.Update(func(s *connState) { s.set = nil })
}

func (a *Adapter) Kind() string        { return "redis" }
func (a *Adapter) Caps() adapters.Caps { return caps }

var redisVersionRE = regexp.MustCompile(`redis_version:([^\r\n]+)`)

// Connect is index.ts's connect.
func (a *Adapter) Connect(ctx context.Context, cfg model.ResolvedConnectionConfig, op *adapters.OpCtx) (adapters.ConnectInfo, error) {
	set, defaultDbIndex, err := connectRedis(ctx, cfg, a.deps.Log)
	if err != nil {
		return adapters.ConnectInfo{}, err
	}
	primary, release, err := set.primary(ctx)
	if err != nil {
		set.closeAll()
		return adapters.ConnectInfo{}, err
	}
	defer release()

	a.setConnected(set, defaultDbIndex, cfg.ReadOnly)

	version := "unknown"
	// INFO is in Redis's own @dangerous ACL category, so a perfectly ordinary
	// least-privilege user (~* +@all -@dangerous) cannot run it. The server version it yields is a
	// tooltip detail — refusing the whole connection over it reported NOPERM as E_AUTH and read as
	// a wrong password.
	if serverInfo, err := primary.Info(ctx, "server").Result(); err == nil {
		if m := redisVersionRE.FindStringSubmatch(serverInfo); m != nil {
			version = m[1]
		}
	} else {
		a.deps.Log("warn", "redis: INFO refused, server version unknown: "+err.Error())
	}
	return adapters.ConnectInfo{
		ServerVersion: "Redis " + version,
		Details:       map[string]string{"database": "db" + strconv.Itoa(defaultDbIndex)},
	}, nil
}

// Disconnect is index.ts's disconnect.
func (a *Adapter) Disconnect(ctx context.Context) error {
	if set := a.state.Load().set; set != nil {
		set.closeAll()
	}
	a.clearConnected()
	return nil
}

func (a *Adapter) requireSet() (*dbConnectionSet, error) {
	return adapters.RequireConnected(a.state.Load().set)
}

// Children is index.ts's children.
func (a *Adapter) Children(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	segments := path.Segments
	set, err := a.requireSet()
	if err != nil {
		return adapters.TreeChildren{}, err
	}

	if len(segments) == 0 {
		primary, release, err := set.primary(ctx)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		defer release()
		nodes, err := listDatabases(ctx, primary)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		return adapters.TreeChildren{Nodes: nodes}, nil
	}

	dbSegment := segments[0]
	if dbSegment.Kind != "database" {
		return adapters.TreeChildren{}, adapters.UnexpectedPathKind(0, dbSegment.Kind)
	}
	rest := segments[1:]
	// Rule 5 (Adapter doc comment): Children returns [] for a leaf, never an error — a 'key' node
	// never has children.
	if len(rest) > 0 && rest[len(rest)-1].Kind == "key" {
		return adapters.TreeChildren{Nodes: []model.TreeNode{}}, nil
	}

	namespaceSegments := make([]string, 0, len(rest))
	for _, seg := range rest {
		if seg.Kind != "namespace" {
			return adapters.TreeChildren{}, adapters.New(adapters.CodeNotFound, "unexpected path segment kind: "+seg.Kind, nil)
		}
		namespaceSegments = append(namespaceSegments, seg.Name)
	}

	dbIndex, err := dbIndexFromName(dbSegment.Name)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	conn, release, err := set.get(ctx, dbIndex)
	if err != nil {
		return adapters.TreeChildren{}, err
	}
	defer release()
	return listNamespaceChildren(ctx, conn, dbSegment.Name, namespaceSegments, op)
}

// Describe is index.ts's describe — caps.Describe is false; unreachable while that flag gates
// every caller.
func (a *Adapter) Describe(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectMeta, error) {
	return model.ObjectMeta{}, adapters.Unsupported("redis", "describe")
}

// Definition is index.ts's definition — caps.Definition is false; unreachable.
func (a *Adapter) Definition(ctx context.Context, path model.NodePath, op *adapters.OpCtx) (model.ObjectDefinition, error) {
	return model.ObjectDefinition{}, adapters.Unsupported("redis", "definition")
}

// SchemaColumns — caps.SchemaColumns is false; unreachable. A redis key has no field-level schema
// to complete.
func (a *Adapter) SchemaColumns(ctx context.Context, path model.NodePath, op *adapters.OpCtx) ([]model.RelationColumns, error) {
	return nil, adapters.Unsupported("redis", "schemaColumns")
}

func (a *Adapter) resolveKeyTarget(path model.NodePath) (dbIndex int, key string, err error) {
	segments := path.Segments
	if len(segments) < 2 || segments[0].Kind != "database" || segments[len(segments)-1].Kind != "key" {
		return 0, "", adapters.New(adapters.CodeNotFound, "read requires a database/.../key path, got: "+model.EncodePath(segments), nil)
	}
	dbIndex, err = dbIndexFromName(segments[0].Name)
	if err != nil {
		return 0, "", err
	}
	return dbIndex, segments[len(segments)-1].Name, nil
}

// Read is index.ts's read.
func (a *Adapter) Read(ctx context.Context, req adapters.ReadRequest, op *adapters.OpCtx) (page.Page, error) {
	if err := adapters.RejectConditions("redis", req.Conditions); err != nil {
		return nil, err
	}
	set, err := a.requireSet()
	if err != nil {
		return nil, err
	}
	dbIndex, key, err := a.resolveKeyTarget(req.Path)
	if err != nil {
		return nil, err
	}
	conn, release, err := set.get(ctx, dbIndex)
	if err != nil {
		return nil, err
	}
	defer release()
	result, err := readKey(ctx, conn, key, readReq{PageSize: req.PageSize, Cursor: req.Cursor}, op)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Count is index.ts's count.
func (a *Adapter) Count(ctx context.Context, req adapters.CountRequest, op *adapters.OpCtx) (adapters.CountResult, error) {
	set, err := a.requireSet()
	if err != nil {
		return adapters.CountResult{}, err
	}
	dbIndex, key, err := a.resolveKeyTarget(req.Path)
	if err != nil {
		return adapters.CountResult{}, err
	}
	conn, release, err := set.get(ctx, dbIndex)
	if err != nil {
		return adapters.CountResult{}, err
	}
	defer release()
	return countKey(ctx, conn, key)
}

// Preview is index.ts's preview.
func (a *Adapter) Preview(plan model.MutationPlan) ([]string, error) {
	return preview(plan)
}

// Mutate is index.ts's mutate. SET/DEL only: edit is scoped to string-type keys, delete is
// type-agnostic. plan.Path only ever resolves to a database — never a specific key, since a
// mutation plan's own ops name their target key via the _key sentinel — so this only needs to
// pick the right per-db-index connection.
func (a *Adapter) Mutate(ctx context.Context, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	set, err := a.requireSet()
	if err != nil {
		return model.MutationResult{}, err
	}
	if len(plan.Path.Segments) == 0 || plan.Path.Segments[0].Kind != "database" {
		return model.MutationResult{}, adapters.Contract("unexpected root path segment kind", nil)
	}
	dbIndex, err := dbIndexFromName(plan.Path.Segments[0].Name)
	if err != nil {
		return model.MutationResult{}, err
	}
	conn, release, err := set.get(ctx, dbIndex)
	if err != nil {
		return model.MutationResult{}, err
	}
	defer release()
	return mutateDB(ctx, conn, op, a.state.Load().readOnly, plan)
}

// Execute is index.ts's execute.
func (a *Adapter) Execute(ctx context.Context, req model.ConsoleRequest, op *adapters.OpCtx) ([]page.Page, error) {
	set, err := a.requireSet()
	if err != nil {
		return nil, err
	}
	dbIndex := a.state.Load().defaultDbIndex
	if len(req.Path.Segments) > 0 && req.Path.Segments[0].Kind == "database" {
		idx, err := dbIndexFromName(req.Path.Segments[0].Name)
		if err != nil {
			return nil, err
		}
		dbIndex = idx
	}
	return execute(ctx, set, dbIndex, a.state.Load().readOnly, op, req.Statements, adapters.ConsoleCapFor(req))
}

// DownloadObject is index.ts's downloadObject — caps.FileTransfer is false; never reached.
func (a *Adapter) DownloadObject(ctx context.Context, req model.ObjectDownloadRequest, op *adapters.OpCtx) (model.ObjectTransferResult, error) {
	return model.ObjectTransferResult{}, adapters.Unsupported("redis", "file transfer")
}

// KeyTypes is each path's TYPE, in the order given, one TYPE pipeline per distinct db
// index (BrowseView.vue's own window is capped at 200 paths, well under one round trip's worth,
// but a level can still mix keys from different db indices in principle — grouping keeps this
// correct rather than assuming every path shares one).
func (a *Adapter) KeyTypes(ctx context.Context, paths []model.NodePath, op *adapters.OpCtx) ([]string, error) {
	set, err := a.requireSet()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(paths))
	keys := make([]string, len(paths))
	dbIndices := make([]int, len(paths))
	for i, p := range paths {
		dbIndex, key, err := a.resolveKeyTarget(p)
		if err != nil {
			return nil, err
		}
		keys[i] = key
		dbIndices[i] = dbIndex
	}
	dbOrder, byDB := groupByDB(dbIndices)

	for _, dbIndex := range dbOrder {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		conn, release, err := set.get(ctx, dbIndex)
		if err != nil {
			return nil, err
		}
		indices := byDB[dbIndex]
		dbKeys := make([]string, len(indices))
		for j, idx := range indices {
			dbKeys[j] = keys[idx]
		}
		types, err := keyTypes(ctx, conn, dbKeys)
		release()
		if err != nil {
			return nil, err
		}
		for j, idx := range indices {
			out[idx] = types[j]
		}
	}
	return out, nil
}

// Cancel is index.ts's cancel: browse ops are bounded SCAN-family loops that
// CheckCancelled between rounds, and the console races Stop (runConsoleCommand), so this stays a
// permanent no-op rather than attempting a CLIENT KILL that would be unsafe under
// dbConnectionSet's one-client-per-db-index sharing.
func (a *Adapter) Cancel(ctx context.Context, opID string) (bool, error) {
	return false, nil
}
