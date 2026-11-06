// Package tree is the Go analogue of src/main/tree-service.ts: L1 cache-aside for
// children/describe/definition over internal/storage/repos.MetadataCacheRepo, backed by the
// engine when the cache misses, is bypassed, or fails its own validator (a naive
// json.Unmarshal is not a substitute for zod's safeParse, so the model package's explicit
// validators are what makes the "drop the cached row" path reachable at all).
package tree

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kirathecat/kira-studio/internal/kiratime"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Connected is the one thing tree needs from the connections service. A one-method interface
// (P54 D14's discipline) keeps tree's tests able to set a connection's state directly instead of
// driving a real connect, while a real *connections.Service satisfies it too.
type Connected interface {
	StateOf(connectionID string) model.ConnectionState
}

// Backend is the slice of adapter tree operations this service calls through instead of straight
// into *enginehost.Host (the per-consumer-interface discipline — the same shape as
// connections.Backend and bridge.Canceller). *adapterhost.Router satisfies this structurally.
// Return types reuse adapters.TreeChildren/model.ObjectMeta/model.ObjectDefinition rather than
// tree-local wrapper types: Source ("cache" | "server") is this service's own concern, set after
// any real Backend call, never something the backend itself decides.
type Backend interface {
	Children(ctx context.Context, connectionID string, path model.NodePath) (adapters.TreeChildren, error)
	Describe(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectMeta, error)
	Definition(ctx context.Context, connectionID string, path model.NodePath, tabID *string) (model.ObjectDefinition, error)
	// SchemaColumns is P22c D1/D2's schema-wide sibling of Describe.
	SchemaColumns(ctx context.Context, connectionID string, path model.NodePath) ([]model.RelationColumns, error)
	// KeyTypes is P63 §4.3's per-key type batch — each path's engine-level value type, in order.
	KeyTypes(ctx context.Context, connectionID string, paths []model.NodePath) ([]string, error)
}

type ChildrenResult struct {
	Nodes     []model.TreeNode `json:"nodes"`
	Source    string           `json:"source"` // "cache" | "server"
	Truncated bool             `json:"truncated"`
}

type DescribeResult struct {
	Meta   model.ObjectMeta `json:"meta"`
	Source string           `json:"source"`
}

type DefinitionResult struct {
	Definition model.ObjectDefinition `json:"definition"`
	Source     string                 `json:"source"`
}

// SchemaColumnsResult is P22c D2's cache-aside result for the "columns" kind.
type SchemaColumnsResult struct {
	Relations []model.RelationColumns `json:"relations"`
	Source    string                  `json:"source"`
}

type Service struct {
	conns   *repos.ConnectionsRepo
	meta    *repos.MetadataCacheRepo
	backend Backend
	states  Connected
}

func New(conns *repos.ConnectionsRepo, meta *repos.MetadataCacheRepo, backend Backend, states Connected) *Service {
	return &Service{conns: conns, meta: meta, backend: backend, states: states}
}

// wrapErr satisfies P55 §2 D5: every error crossing out of this package is an *ipcerr.Error. An
// engine *adapters.Error keeps its code (E_ENGINE_DOWN, E_CANCELLED, E_TIMEOUT, E_QUERY) rather
// than collapsing to E_INTERNAL: the renderer reads codes only from ipcerr's JSON form.
func wrapErr(err error) error {
	var ae *adapters.Error
	if errors.As(err, &ae) {
		return ipcerr.New(string(ae.Code), ae.Message)
	}
	return ipcerr.Wrap(err)
}

// requireConnected ports tree-service.ts:73-79. name is the connection row's Name, or the id
// itself if the row is gone — tree-service.ts:77's own fallback, and the message P55 §2 D11
// changed ipcerr.Disconnected to match verbatim.
func (s *Service) requireConnected(connectionID string) error {
	if s.states.StateOf(connectionID).Status == "connected" {
		return nil
	}
	name := connectionID
	if summary, err := s.conns.Get(connectionID); err == nil && summary != nil {
		name = summary.Name
	}
	return ipcerr.Disconnected(name)
}

// freshnessFloor is the timestamp a cached payload must be at or after to count as fresh: the
// moment this connection was most recently established. The second result is false
// while the connection is not connected, which means no floor applies at all — every cached
// payload is servable regardless of age: this is what makes the tree instant on launch and
// what lets a SQL console over a cached container offer completion with no live connection.
func (s *Service) freshnessFloor(connectionID string) (string, bool) {
	st := s.states.StateOf(connectionID)
	if st.Status != "connected" {
		return "", false
	}
	return kiratime.FormatISO(time.UnixMilli(st.Since)), true
}

// getCached is the whole cache-aside read path's choke point, and P24's staleness rule lives here
// (D4): a payload written before the current connection's Since is stale and BYPASSED, never
// dropped — D5 needs it left on disk for a disconnected read, and a failed re-fetch must not lose
// what is already cached (that half is still the caller's job, same as before this phase).
// fetchedAt compares against the floor as a plain string — both are kiratime.FormatISO's fixed-width,
// UTC, three-fractional-digit format, which sorts lexicographically the same as chronologically
// (F12). A payload with no recorded fetchedAt (a pre-P24 row) compares as "", less than every real
// timestamp, so it reads as stale under any floor and is otherwise unaffected — exactly D2's
// upgrade behaviour.
func (s *Service) getCached(connectionID, path, kind string) (json.RawMessage, bool) {
	raw, fetchedAt, err := s.meta.Get(connectionID, path, kind)
	if err != nil || raw == nil {
		return nil, false
	}
	if floor, ok := s.freshnessFloor(connectionID); ok && fetchedAt < floor {
		return nil, false
	}
	return raw, true
}

// tryCache is Children/Describe/Definition/SchemaColumns's own shared cache-lookup half: fetch the
// raw cached row (already floor-checked by getCached), hand it to unmarshal, and drop the row —
// never trust it further — the moment it fails to unmarshal or fails its own model validator. A
// single cachedRead[T] covering lookup-through-return doesn't fit here:
// Children caches only result.Nodes ([]model.TreeNode) while returning a ChildrenResult wrapper,
// and SchemaColumns nil-defaults its fetched slice before caching it — the cached shape and the
// method's own return shape diverge per caller, so only the lookup half is shared.
func tryCache[T any](s *Service, connectionID, path, kind string, unmarshal func(raw json.RawMessage) (T, bool)) (T, bool) {
	var zero T
	raw, ok := s.getCached(connectionID, path, kind)
	if !ok {
		return zero, false
	}
	if v, ok := unmarshal(raw); ok {
		return v, true
	}
	_ = s.meta.Drop(connectionID, path)
	return zero, false
}

// resolvePath is Children/Describe/Definition/SchemaColumns's own shared connected-check + path-
// decode half (P107 I2-10), run only once the cache has already been tried and missed.
func (s *Service) resolvePath(connectionID, path string) (model.NodePath, error) {
	if err := s.requireConnected(connectionID); err != nil {
		return model.NodePath{}, err
	}
	nodePath, err := model.DecodePath(connectionID, path)
	if err != nil {
		return model.NodePath{}, ipcerr.BadRequest(err.Error())
	}
	return nodePath, nil
}

// sinceEpoch returns connectionID's current Since — the reconnect epoch a fetch is about to run
// against. Only meaningful once resolvePath's own requireConnected has already
// confirmed the connection is connected (StateOf's synthetic "disconnected" fallback stamps a
// fresh nowMillis() on every call, which would never compare equal to itself across two calls).
func (s *Service) sinceEpoch(connectionID string) int64 {
	return s.states.StateOf(connectionID).Since
}

// putIfSinceUnchanged stores encoded for (connectionID, path, kind) only if the connection's own
// Since is still what it was when the fetch that produced encoded began (since) — a fetch
// started just before a reconnect gets its own metadata_cache row's fetchedAt stamped at Put time
// (metadata_cache.go's Put), which lands *after* the new freshness floor the reconnect just
// established (freshnessFloor reads this same Since) — without this guard, a listing fetched
// against the connection's old target would be wrongly treated as fresh the moment the reconnect
// completes, matters especially right after an edit-and-reconnect sequence.
func (s *Service) putIfSinceUnchanged(connectionID, path, kind string, since int64, encoded []byte) {
	if s.sinceEpoch(connectionID) != since {
		return
	}
	_ = s.meta.Put(connectionID, path, kind, encoded)
}

// cacheAside is Describe/Definition/SchemaColumns's own shared cache-miss path: resolve
// path, capture Since, run load, marshal-and-store the result if the connection's Since hasn't
// moved on meanwhile, and return it. Children keeps its own logic — a truncated listing is dropped
// rather than cached, and Children returns a different result shape than Describe/Definition/
// SchemaColumns's own (v, source) pairing.
func cacheAside[T any](s *Service, connectionID, path, kind string, load func(nodePath model.NodePath) (T, error)) (T, error) {
	var zero T
	nodePath, err := s.resolvePath(connectionID, path)
	if err != nil {
		return zero, err
	}
	since := s.sinceEpoch(connectionID)
	v, err := load(nodePath)
	if err != nil {
		return zero, wrapErr(err)
	}
	if encoded, err := json.Marshal(v); err == nil {
		s.putIfSinceUnchanged(connectionID, path, kind, since, encoded)
	}
	return v, nil
}

// Children ports tree-service.ts:82-108, including P43 iter2 D22 (a truncated listing is never
// cached) and P43 iter3 D38 (a truncated refresh drops any older complete row for the same path).
func (s *Service) Children(ctx context.Context, connectionID, path string, refresh bool) (ChildrenResult, error) {
	if !refresh {
		if nodes, ok := tryCache(s, connectionID, path, "children", func(raw json.RawMessage) ([]model.TreeNode, bool) {
			var nodes []model.TreeNode
			return nodes, json.Unmarshal(raw, &nodes) == nil && model.ValidateTreeNodes(nodes)
		}); ok {
			return ChildrenResult{Nodes: nodes, Source: "cache", Truncated: false}, nil
		}
	}
	nodePath, err := s.resolvePath(connectionID, path)
	if err != nil {
		return ChildrenResult{}, err
	}
	since := s.sinceEpoch(connectionID)
	result, err := s.backend.Children(ctx, connectionID, nodePath)
	if err != nil {
		return ChildrenResult{}, wrapErr(err)
	}
	truncated := result.Truncated != nil && *result.Truncated
	if truncated {
		_ = s.meta.Drop(connectionID, path)
	} else if encoded, err := json.Marshal(result.Nodes); err == nil {
		s.putIfSinceUnchanged(connectionID, path, "children", since, encoded)
	}
	return ChildrenResult{Nodes: result.Nodes, Source: "server", Truncated: truncated}, nil
}

// Describe ports tree-service.ts:110-128. `tabId` tags the resulting op-log row so the requesting
// tab's RunState can find its own duration — a cache hit makes no engine call and so tags
// nothing, which is correct: there is no duration to show for work that never happened.
func (s *Service) Describe(ctx context.Context, connectionID, path string, refresh bool, tabID *string) (DescribeResult, error) {
	if !refresh {
		if meta, ok := tryCache(s, connectionID, path, "describe", func(raw json.RawMessage) (model.ObjectMeta, bool) {
			var meta model.ObjectMeta
			return meta, json.Unmarshal(raw, &meta) == nil && model.ValidateObjectMeta(&meta)
		}); ok {
			return DescribeResult{Meta: meta, Source: "cache"}, nil
		}
	}
	meta, err := cacheAside(s, connectionID, path, "describe", func(nodePath model.NodePath) (model.ObjectMeta, error) {
		return s.backend.Describe(ctx, connectionID, nodePath, tabID)
	})
	if err != nil {
		return DescribeResult{}, err
	}
	return DescribeResult{Meta: meta, Source: "server"}, nil
}

// Definition ports tree-service.ts:130-148.
func (s *Service) Definition(ctx context.Context, connectionID, path string, refresh bool, tabID *string) (DefinitionResult, error) {
	if !refresh {
		if def, ok := tryCache(s, connectionID, path, "definition", func(raw json.RawMessage) (model.ObjectDefinition, bool) {
			var def model.ObjectDefinition
			return def, json.Unmarshal(raw, &def) == nil && model.ValidateObjectDefinition(&def)
		}); ok {
			return DefinitionResult{Definition: def, Source: "cache"}, nil
		}
	}
	definition, err := cacheAside(s, connectionID, path, "definition", func(nodePath model.NodePath) (model.ObjectDefinition, error) {
		return s.backend.Definition(ctx, connectionID, nodePath, tabID)
	})
	if err != nil {
		return DefinitionResult{}, err
	}
	return DefinitionResult{Definition: definition, Source: "server"}, nil
}

// SchemaColumns is the fourth cache kind, "columns" — same cache-aside shape as Children/Describe/
// Definition, sharing their metadata_cache row per (connectionID, path) rather than adding a new
// one (the cache's 200-row-per-connection budget is shared by all four kinds, keyed by path —
// a design that wrote one row per relation would evict the tree's own children on a large schema).
func (s *Service) SchemaColumns(ctx context.Context, connectionID, path string, refresh bool) (SchemaColumnsResult, error) {
	if !refresh {
		if rels, ok := tryCache(s, connectionID, path, "columns", func(raw json.RawMessage) ([]model.RelationColumns, bool) {
			var rels []model.RelationColumns
			return rels, json.Unmarshal(raw, &rels) == nil && model.ValidateRelationColumns(rels)
		}); ok {
			return SchemaColumnsResult{Relations: rels, Source: "cache"}, nil
		}
	}
	relations, err := cacheAside(s, connectionID, path, "columns", func(nodePath model.NodePath) ([]model.RelationColumns, error) {
		relations, err := s.backend.SchemaColumns(ctx, connectionID, nodePath)
		if err != nil {
			return nil, err
		}
		if relations == nil {
			relations = []model.RelationColumns{}
		}
		return relations, nil
	})
	if err != nil {
		return SchemaColumnsResult{}, err
	}
	return SchemaColumnsResult{Relations: relations, Source: "server"}, nil
}

// KeyTypes is the per-key type batch. Deliberately uncached, unlike Children/Describe/
// Definition/SchemaColumns above: those each answer one node's own metadata, cached per (path,
// kind) in the metadata_cache table; this answers a caller-chosen batch of up to 200 sibling
// paths (BrowseView.vue's own visible-range window), which has no single node identity to cache
// against and would otherwise grow one cache row per distinct window instead of per node.
func (s *Service) KeyTypes(ctx context.Context, connectionID string, paths []string) ([]string, error) {
	if err := s.requireConnected(connectionID); err != nil {
		return nil, err
	}
	nodePaths := make([]model.NodePath, len(paths))
	for i, p := range paths {
		nodePath, err := model.DecodePath(connectionID, p)
		if err != nil {
			return nil, ipcerr.BadRequest(err.Error())
		}
		nodePaths[i] = nodePath
	}
	types, err := s.backend.KeyTypes(ctx, connectionID, nodePaths)
	if err != nil {
		return nil, wrapErr(err)
	}
	if types == nil {
		types = []string{}
	}
	return types, nil
}

// Invalidate drops L1 for one node (path non-nil) or the whole connection (path nil). No push of
// its own — the caller already knows what it asked to invalidate (the D11 reconnect push is
// connections.OnMetadataInvalidated, a separate concern owned by internal/connections).
func (s *Service) Invalidate(connectionID string, path *string) error {
	if path == nil {
		return ipcerr.Wrap(s.meta.DropConnection(connectionID))
	}
	return ipcerr.Wrap(s.meta.Drop(connectionID, *path))
}
