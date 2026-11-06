package redis

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Never an unbudgeted SCAN (ground rules): a fixed COUNT hint per round-trip, and a hard cap on
// how many rounds one children() call will run — a call degrades to "not everything shown yet
// under this prefix" rather than turning into a full-keyspace crawl.
const (
	scanCount     = 1000
	maxScanRounds = 200
)

var dbKeyspaceLineRE = regexp.MustCompile(`^db(\d+):keys=(\d+)`)
var dbNameRE = regexp.MustCompile(`^db(\d+)$`)

// listDatabases ports catalog.ts's listDatabases: INFO keyspace's db<N>:keys=<M> lines, sorted
// numerically — db10 must not sort before db2.
func listDatabases(ctx context.Context, primary *goredis.Client) ([]model.TreeNode, error) {
	info, err := primary.Info(ctx, "keyspace").Result()
	if err != nil {
		return nil, mapError(err)
	}
	var nodes []model.TreeNode
	indices := map[string]int{}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimRight(line, "\r")
		m := dbKeyspaceLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		index, _ := strconv.Atoi(m[1])
		keys, _ := strconv.ParseInt(m[2], 10, 64)
		name := "db" + m[1]
		plural := "s"
		if keys == 1 {
			plural = ""
		}
		detail := adapters.AbbreviateCount(keys) + " key" + plural
		indices[name] = index
		nodes = append(nodes, model.TreeNode{
			Kind: "database", Name: name,
			Path: model.EncodePath([]model.PathSegment{{Kind: "database", Name: name}}),
			// P41 D5: a db index's key namespace is unbounded — the tree stops here; the space
			// itself is navigated in a Browse tab, reached via listNamespaceChildren below.
			HasChildren: false,
			Detail:      &detail,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return indices[nodes[i].Name] < indices[nodes[j].Name] })
	return nodes, nil
}

// dbIndexFromName ports catalog.ts's dbIndexFromName.
func dbIndexFromName(name string) (int, error) {
	m := dbNameRE.FindStringSubmatch(name)
	if m == nil {
		return 0, adapters.New(adapters.CodeNotFound, "not a redis database node: "+name, nil)
	}
	n, _ := strconv.Atoi(m[1])
	return n, nil
}

// scanner is the one *goredis.Client method listNamespaceChildren needs (P58f D14) — declared here
// so catalog_test.go can drive the truncation conjunction with a fake instead of a live server.
type scanner interface {
	Scan(ctx context.Context, cursor uint64, match string, count int64) *goredis.ScanCmd
}

// escapeGlobPrefix backslash-escapes redis MATCH's glob metacharacters (`* ? [ ] \`) in a literal
// namespace prefix, so a segment name that happens to contain one (e.g. "a*b:") scans as that
// literal text instead of as a wildcard — an unescaped prefix can both miss real children (glob
// swallows too much) and pull in unrelated keys that only coincidentally match the pattern.
func escapeGlobPrefix(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// namespaceScanAccumulator collects one listNamespaceChildren walk's namespace/key nodes as SCAN
// rounds discover them, plus each name's first-seen order (map iteration order is not stable, so
// the order slices are what buildNamespaceNodes sorts and walks).
type namespaceScanAccumulator struct {
	namespaceNodes map[string]model.TreeNode
	namespaceOrder []string
	keyNodes       map[string]model.TreeNode
	keyOrder       []string
}

// add classifies one SCANned key as a namespace segment or a leaf key under prefix, merging it
// into acc if not already seen.
func (acc *namespaceScanAccumulator) add(key, prefix, dbName string, namespaceSegments []string) {
	// Defensive, not load-bearing given the caller's own escaping: skip rather than panic on a
	// key MATCH somehow returned that doesn't actually start with the literal prefix.
	if !strings.HasPrefix(key, prefix) {
		return
	}
	rest := key[len(prefix):]
	sep := strings.IndexByte(rest, ':')
	if sep < 0 {
		if _, seen := acc.keyNodes[key]; !seen {
			acc.keyOrder = append(acc.keyOrder, key)
		}
		segments := make([]model.PathSegment, 0, len(namespaceSegments)+2)
		segments = append(segments, model.PathSegment{Kind: "database", Name: dbName})
		for _, s := range namespaceSegments {
			segments = append(segments, model.PathSegment{Kind: "namespace", Name: s})
		}
		segments = append(segments, model.PathSegment{Kind: "key", Name: key})
		acc.keyNodes[key] = model.TreeNode{
			Kind: "key", Name: key, Path: model.EncodePath(segments), HasChildren: false,
		}
		return
	}
	segment := rest[:sep]
	if _, seen := acc.namespaceNodes[segment]; seen {
		return
	}
	segments := make([]model.PathSegment, 0, len(namespaceSegments)+2)
	segments = append(segments, model.PathSegment{Kind: "database", Name: dbName})
	for _, s := range namespaceSegments {
		segments = append(segments, model.PathSegment{Kind: "namespace", Name: s})
	}
	segments = append(segments, model.PathSegment{Kind: "namespace", Name: segment})
	acc.namespaceNodes[segment] = model.TreeNode{
		Kind: "namespace", Name: segment, Path: model.EncodePath(segments), HasChildren: true,
	}
	acc.namespaceOrder = append(acc.namespaceOrder, segment)
}

// scanRound runs one SCAN round of listNamespaceChildren's walk, merging any keys it returns into
// acc and returning the next cursor.
func scanRound(ctx context.Context, conn scanner, cursor uint64, matchPattern, prefix, dbName string, namespaceSegments []string, acc *namespaceScanAccumulator) (uint64, error) {
	keys, nextCursor, err := conn.Scan(ctx, cursor, matchPattern, scanCount).Result()
	if err != nil {
		return 0, mapError(err)
	}
	for _, key := range keys {
		acc.add(key, prefix, dbName, namespaceSegments)
	}
	return nextCursor, nil
}

// buildNamespaceNodes sorts acc's namespace and key names and assembles the final ordered node
// list: namespaces first, then keys, each alphabetical.
func buildNamespaceNodes(acc namespaceScanAccumulator) []model.TreeNode {
	sort.Strings(acc.namespaceOrder)
	sort.Strings(acc.keyOrder)
	nodes := make([]model.TreeNode, 0, len(acc.namespaceOrder)+len(acc.keyOrder))
	for _, name := range acc.namespaceOrder {
		nodes = append(nodes, acc.namespaceNodes[name])
	}
	for _, name := range acc.keyOrder {
		nodes = append(nodes, acc.keyNodes[name])
	}
	return nodes
}

// listNamespaceChildren ports catalog.ts's listNamespaceChildren: the ':'-splitting SCAN walk.
// namespaceSegments is just the local segments collected while descending the tree, joined back
// into a scan prefix here, never reconstructed from a leaf.
func listNamespaceChildren(ctx context.Context, conn scanner, dbName string, namespaceSegments []string, op *adapters.OpCtx) (adapters.TreeChildren, error) {
	prefix := ""
	if len(namespaceSegments) > 0 {
		prefix = strings.Join(namespaceSegments, ":") + ":"
	}
	matchPattern := escapeGlobPrefix(prefix) + "*"
	acc := namespaceScanAccumulator{
		namespaceNodes: map[string]model.TreeNode{},
		keyNodes:       map[string]model.TreeNode{},
	}

	var cursor uint64
	rounds := 0
	for {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return adapters.TreeChildren{}, err
		}
		nextCursor, err := scanRound(ctx, conn, cursor, matchPattern, prefix, dbName, namespaceSegments, &acc)
		if err != nil {
			return adapters.TreeChildren{}, err
		}
		cursor = nextCursor
		rounds++
		if cursor == 0 || rounds >= maxScanRounds {
			break
		}
	}

	nodes := buildNamespaceNodes(acc)

	// P43 iter2 F16/D21: true only when the round cap cut the scan short (cursor != 0 means SCAN
	// itself says there is more) — never for an ordinary complete scan that happened to take fewer
	// rounds.
	if cursor != 0 && rounds >= maxScanRounds {
		truncated := true
		return adapters.TreeChildren{Nodes: nodes, Truncated: &truncated}, nil
	}
	return adapters.TreeChildren{Nodes: nodes}, nil
}

// typer is the one *goredis.Client method keyTypes needs (mirrors scanner's own discipline, P58f
// D14) — declared here so catalog_test.go can drive it with a fake instead of a live server.
type typer interface {
	Pipeline() goredis.Pipeliner
}

// groupByDB partitions parallel dbIndices/keys slices (index i is one path's own resolved db
// index) into per-db-index batches, each carrying its own indices back into a caller's `out`
// slice — the shape Adapter.KeyTypes needs to route each db index through its own connection
// while still writing every result back in the original, caller-given order. `order` is the
// db indices in first-seen order, so a caller iterating it visits each db index exactly once. A
// free function so catalog_test.go can verify the routing/ordering discipline without a live
// dbConnectionSet.
func groupByDB(dbIndices []int) (order []int, byDB map[int][]int) {
	byDB = map[int][]int{}
	for i, dbIndex := range dbIndices {
		if _, seen := byDB[dbIndex]; !seen {
			order = append(order, dbIndex)
		}
		byDB[dbIndex] = append(byDB[dbIndex], i)
	}
	return order, byDB
}

// keyTypes is the pipelined TYPE batch: one round trip for every key in `keys`, answered in
// the same order they were given. TYPE never errors for a missing key — it replies "none" — so a
// key deleted between the SCAN that found it and this call reports "none" here rather than failing
// the whole batch; the renderer's own redisTypeIcon falls back to the generic glyph for that case.
func keyTypes(ctx context.Context, conn typer, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return []string{}, nil
	}
	pipe := conn.Pipeline()
	cmds := make([]*goredis.StatusCmd, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Type(ctx, key)
	}
	// Exec's own error is non-nil only when a command in the pipeline actually failed — TYPE has
	// no failure mode short of a connection error, so this is the real "something went wrong" case,
	// not a per-key "not found" one (that's "none", answered successfully).
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, mapError(err)
	}
	out := make([]string, len(keys))
	for i, cmd := range cmds {
		t, err := cmd.Result()
		if err != nil {
			return nil, mapError(err)
		}
		out[i] = t
	}
	return out, nil
}
