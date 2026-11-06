package redis

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/jsonx"
)

// Never an unbudgeted SCAN (ground rules) — same per-round-trip COUNT hint as catalog.go.
const readScanCount = 1000

var knownTypes = map[string]bool{"string": true, "hash": true, "list": true, "set": true, "zset": true, "stream": true}

type keyMeta struct {
	redisType   string // one of knownTypes, or "" if none applies (checked before use)
	ttlMs       *int64
	memoryBytes *int64
}

// readMeta ports read.ts's readMeta.
func readMeta(ctx context.Context, conn *goredis.Client, key string) (keyMeta, error) {
	rawType, err := conn.Type(ctx, key).Result()
	if err != nil {
		return keyMeta{}, mapError(err)
	}
	// A key gone at read time is an ordinary query-time condition (expired/deleted concurrently),
	// not a connection failure — E_QUERY, deliberately not E_NOT_FOUND (P9's D10).
	if rawType == "none" {
		return keyMeta{}, adapters.New(adapters.CodeQuery, "key no longer exists: "+key, nil)
	}
	if !knownTypes[rawType] {
		return keyMeta{}, adapters.New(adapters.CodeUnsupported, "unsupported redis type for "+key+": "+rawType, nil)
	}
	pttl, err := conn.PTTL(ctx, key).Result()
	if err != nil {
		return keyMeta{}, mapError(err)
	}
	// go-redis's PTTL reply carries the -1 ("no expiry") and -2 ("key gone") sentinels as raw
	// nanosecond Durations (time.Duration(-1), time.Duration(-2)), not millisecond-scaled ones —
	// checking pttl itself (not pttl.Milliseconds()) for negativity is required, since integer
	// division truncates both -1ns and -2ns to 0ms and would otherwise report a real TTL of zero
	// for every non-expiring key (read.ts's own PTTL check compared the
	// already-millisecond-scaled ioredis reply, which carries no such sub-millisecond sentinel).
	var ttlMs *int64
	if pttl >= 0 {
		ms := pttl.Milliseconds()
		ttlMs = &ms
	}
	var memoryBytes *int64
	if usage, err := conn.MemoryUsage(ctx, key).Result(); err == nil {
		memoryBytes = &usage
	} // best-effort (§8.8)
	return keyMeta{redisType: rawType, ttlMs: ttlMs, memoryBytes: memoryBytes}, nil
}

func readString(ctx context.Context, conn *goredis.Client, key string, meta keyMeta) (page.KeyValuePage, error) {
	value, err := conn.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, goredis.Nil) {
		return page.KeyValuePage{}, mapError(err)
	}
	builder := page.NewKeyValuePageBuilder("string", meta.ttlMs, meta.memoryBytes, false)
	if !errors.Is(err, goredis.Nil) {
		builder.Push("value", value)
	}
	return builder.Finish(page.UnpagedPosition(1)), nil
}

// scanRoundFn runs one SCAN-family round: cursor in, (elements, nextCursor) out. pairSize is 1
// (set) or 2 (hash/zset) — elements alternate field/value for pairSize 2.
type scanRoundFn func(ctx context.Context, cursor uint64) (elements []string, nextCursor uint64, err error)

// decodeScanCursor decodes readScanFamily's page token for an "after" cursor into the SCAN cursor
// and (pairSize==1 only) the running display-index offset carried from prior pages. A zero-value
// (mode other than "after") is the ordinary fresh-scan start.
func decodeScanCursor(req readReq, pairSize int, fingerprint string) (cursor uint64, rowOffset int, err error) {
	if req.Cursor.Mode != "after" {
		return 0, 0, nil
	}
	keyValues, err := adapters.DecodePageToken(req.Cursor.Token, fingerprint)
	if err != nil {
		return 0, 0, err
	}
	wantFields := 1
	if pairSize == 1 {
		wantFields = 2
	}
	if len(keyValues) != wantFields {
		return 0, 0, adapters.Contract("malformed page token", nil)
	}
	parsed, err := strconv.ParseUint(keyValues[0], 10, 64)
	if err != nil {
		return 0, 0, adapters.Contract("malformed page token", nil)
	}
	cursor = parsed
	if pairSize == 1 {
		offset, err := strconv.Atoi(keyValues[1])
		if err != nil {
			return 0, 0, adapters.Contract("malformed page token", nil)
		}
		rowOffset = offset
	}
	return cursor, rowOffset, nil
}

// scanFamilyRound runs one round of readScanFamily's SCAN-family loop: fetches one batch via
// scanOnce and pushes every element (or field/value pair) into builder, returning the next cursor
// and how many rows this round pushed.
func scanFamilyRound(ctx context.Context, scanOnce scanRoundFn, cursor uint64, pairSize, rowOffset, rowCount int, builder *page.KeyValuePageBuilder) (nextCursor uint64, pushed int, err error) {
	elements, nextCursor, err := scanOnce(ctx, cursor)
	if err != nil {
		return 0, 0, mapError(err)
	}
	for i := 0; i < len(elements); i += pairSize {
		if pairSize == 2 {
			builder.Push(elements[i], elements[i+1])
		} else {
			builder.Push(strconv.Itoa(rowOffset+rowCount+pushed), elements[i])
		}
		pushed++
	}
	return nextCursor, pushed, nil
}

// readScanFamily is read.ts's own shared cursor-loop body for hash/set/zset: accumulates whole SCAN
// rounds without slicing mid-round, so a round's remaining elements are never dropped — the page
// can overshoot req.pageSize by up to one round. An offset cursor on a cursor-paged key silently
// restarts the scan from 0 rather than seeking or erroring
// (views/keyvalue/state.ts depends on this).
//
// pairSize==1 (set) has no natural per-row key, so it synthesizes one as a running display index
// (readList's own strconv.Itoa(offset+i) pattern) — the running total is carried across pages in
// the token itself, alongside the SSCAN cursor, since nothing else here is a page-spanning
// counter: without it every page's index restarted at 0, relabelling page 2's rows over page 1's.
func readScanFamily(ctx context.Context, scanOnce scanRoundFn, pairSize int, redisType string, meta keyMeta, req readReq, fingerprint string) (page.KeyValuePage, error) {
	if req.Cursor.Mode == "before" {
		return page.KeyValuePage{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisCursorForwardOnly.body", nil, nil)
	}
	cursor, rowOffset, err := decodeScanCursor(req, pairSize, fingerprint)
	if err != nil {
		return page.KeyValuePage{}, err
	}

	builder := page.NewKeyValuePageBuilder(redisType, meta.ttlMs, meta.memoryBytes, false)
	if pairSize == 1 {
		// A set has no natural per-row key (readScanFamily's own doc comment above) — Field here is
		// a synthetic running display index, never a genuine identifier a mask rule could match.
		builder.SetFieldsAreColumns(false)
	}
	rowCount := 0
	exhausted := false

	for {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return page.KeyValuePage{}, err
		}
		nextCursor, pushed, err := scanFamilyRound(ctx, scanOnce, cursor, pairSize, rowOffset, rowCount, builder)
		if err != nil {
			return page.KeyValuePage{}, err
		}
		cursor = nextCursor
		rowCount += pushed
		if cursor == 0 {
			exhausted = true
		}
		if rowCount >= req.PageSize || exhausted {
			break
		}
	}

	hasMore := !exhausted
	var nextToken *string
	if hasMore {
		tokenValues := []string{strconv.FormatUint(cursor, 10)}
		if pairSize == 1 {
			tokenValues = append(tokenValues, strconv.Itoa(rowOffset+rowCount))
		}
		token := adapters.EncodePageToken(tokenValues, fingerprint)
		nextToken = &token
	}
	position := page.PagePosition{
		Offset: nil, PageSize: req.PageSize, HasMore: hasMore,
		NextToken: nextToken, PrevToken: nil, Strategy: "cursor",
	}
	return builder.Finish(position), nil
}

func readHash(ctx context.Context, conn *goredis.Client, key string, meta keyMeta, req readReq, fingerprint string) (page.KeyValuePage, error) {
	return readScanFamily(ctx, func(ctx context.Context, cursor uint64) ([]string, uint64, error) {
		return conn.HScan(ctx, key, cursor, "", readScanCount).Result()
	}, 2, "hash", meta, req, fingerprint)
}

func readSet(ctx context.Context, conn *goredis.Client, key string, meta keyMeta, req readReq, fingerprint string) (page.KeyValuePage, error) {
	return readScanFamily(ctx, func(ctx context.Context, cursor uint64) ([]string, uint64, error) {
		return conn.SScan(ctx, key, cursor, "", readScanCount).Result()
	}, 1, "set", meta, req, fingerprint)
}

func readZSet(ctx context.Context, conn *goredis.Client, key string, meta keyMeta, req readReq, fingerprint string) (page.KeyValuePage, error) {
	return readScanFamily(ctx, func(ctx context.Context, cursor uint64) ([]string, uint64, error) {
		return conn.ZScan(ctx, key, cursor, "", readScanCount).Result()
	}, 2, "zset", meta, req, fingerprint)
}

// readList is read.ts's readList: offset-only, LRANGE offset..offset+pageSize-1 honouring the
// requested page size in full (P43 iter2 D25/F18 — no LIST_WINDOW clamp).
func readList(ctx context.Context, conn *goredis.Client, key string, meta keyMeta, req readReq) (page.KeyValuePage, error) {
	if req.Cursor.Mode != "offset" {
		return page.KeyValuePage{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisListOffsetOnly.body", nil, nil)
	}
	offset := req.Cursor.Offset
	elements, err := conn.LRange(ctx, key, int64(offset), int64(offset+req.PageSize-1)).Result()
	if err != nil {
		return page.KeyValuePage{}, mapError(err)
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return page.KeyValuePage{}, err
	}
	total, err := conn.LLen(ctx, key).Result()
	if err != nil {
		return page.KeyValuePage{}, mapError(err)
	}

	builder := page.NewKeyValuePageBuilder("list", meta.ttlMs, meta.memoryBytes, false)
	builder.SetFieldsAreColumns(false) // a list index is positional, never a genuine field identifier
	for i, value := range elements {
		builder.Push(strconv.Itoa(offset+i), value)
	}

	hasMore := int64(offset+len(elements)) < total
	off := offset
	position := page.PagePosition{
		Offset: &off, PageSize: req.PageSize, HasMore: hasMore,
		NextToken: nil, PrevToken: nil, Strategy: "offset",
	}
	return builder.Finish(position), nil
}

// streamFieldValue is one field/value pair off a stream entry, in wire order.
type streamFieldValue struct {
	Field string
	Value string
}

// streamFields is a stream entry's field/value pairs, marshaled as a JSON object in the order
// XADD received them: go-redis's own XMessage.Values is a map[string]interface{}, which
// discards that order both on assignment and again were it re-marshaled through encoding/json
// (map keys sort alphabetically) — so this type is built straight from XRANGE's own raw reply
// (still an ordered array on the wire, RESP2 or RESP3 alike) rather than through XMessage at all.
type streamFields []streamFieldValue

func (f streamFields) MarshalJSON() ([]byte, error) {
	pairs := make([]jsonx.Pair, len(f))
	for i, fv := range f {
		pairs[i] = jsonx.Pair{Name: fv.Field, Value: fv.Value}
	}
	return jsonx.MarshalOrderedObject(pairs)
}

// parseStreamEntry reads one [id, [field, value, field, value, ...]] tuple off a raw XRANGE
// reply — the shape every element of that reply's own top-level array takes.
func parseStreamEntry(raw any) (id string, fields streamFields, err error) {
	tuple, ok := raw.([]interface{})
	if !ok || len(tuple) != 2 {
		return "", nil, adapters.Contract("malformed XRANGE reply: expected a 2-element entry tuple", nil)
	}
	id, ok = tuple[0].(string)
	if !ok {
		return "", nil, adapters.Contract("malformed XRANGE reply: entry id is not a string", nil)
	}
	if tuple[1] == nil {
		return id, streamFields{}, nil
	}
	rawFields, ok := tuple[1].([]interface{})
	if !ok {
		return "", nil, adapters.Contract("malformed XRANGE reply: entry fields are not an array", nil)
	}
	fields = make(streamFields, 0, len(rawFields)/2)
	for i := 0; i+1 < len(rawFields); i += 2 {
		field, fok := rawFields[i].(string)
		value, vok := rawFields[i+1].(string)
		if !fok || !vok {
			return "", nil, adapters.Contract("malformed XRANGE reply: field/value is not a string", nil)
		}
		fields = append(fields, streamFieldValue{Field: field, Value: value})
	}
	return id, fields, nil
}

// readStream is read.ts's readStream: XRANGE key <start> + COUNT pageSize+1, issued via the raw
// command path (not XRangeN) so the reply's own field order survives into the page (see
// streamFields above).
func readStream(ctx context.Context, conn *goredis.Client, key string, meta keyMeta, req readReq, fingerprint string) (page.KeyValuePage, error) {
	if req.Cursor.Mode == "before" {
		return page.KeyValuePage{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.redisStreamForwardOnly.body", nil, nil)
	}
	startID := "-"
	if req.Cursor.Mode == "after" {
		keyValues, err := adapters.DecodePageToken(req.Cursor.Token, fingerprint)
		if err != nil {
			return page.KeyValuePage{}, err
		}
		if len(keyValues) != 1 {
			return page.KeyValuePage{}, adapters.Contract("malformed page token", nil)
		}
		startID = "(" + keyValues[0] // exclusive lower bound, per XRANGE syntax
	}

	limit := req.PageSize + 1 // D24's +1 probe, mirroring the SQL adapters
	raw, err := conn.Do(ctx, "xrange", key, startID, "+", "count", limit).Result()
	if err != nil {
		return page.KeyValuePage{}, mapError(err)
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return page.KeyValuePage{}, err
	}
	entries, ok := raw.([]interface{})
	if !ok {
		return page.KeyValuePage{}, adapters.Contract("malformed XRANGE reply: expected an array", nil)
	}

	probedExtra := len(entries) > req.PageSize
	kept := entries
	if probedExtra {
		kept = entries[:req.PageSize]
	}

	builder := page.NewKeyValuePageBuilder("stream", meta.ttlMs, meta.memoryBytes, false)
	// Field here is the entry id, not a real field identifier, and Value is a JSON blob of every
	// field/value pair in the entry together — a single column_name rule can't address one field
	// inside it, the same reason a genuine DocumentPage refuses outright when rules exist.
	builder.SetFieldsAreColumns(false)
	var lastID string
	for _, rawEntry := range kept {
		id, fields, err := parseStreamEntry(rawEntry)
		if err != nil {
			return page.KeyValuePage{}, err
		}
		body, err := json.Marshal(fields)
		if err != nil {
			return page.KeyValuePage{}, err
		}
		builder.Push(id, string(body))
		lastID = id
	}

	hasMore := probedExtra
	var nextToken *string
	if hasMore {
		token := adapters.EncodePageToken([]string{lastID}, fingerprint)
		nextToken = &token
	}
	position := page.PagePosition{
		Offset: nil, PageSize: req.PageSize, HasMore: hasMore,
		NextToken: nextToken, PrevToken: nil, Strategy: "cursor",
	}
	return builder.Finish(position), nil
}

// readReq is the field subset of adapters.ReadRequest readKey needs, minus Path.
type readReq struct {
	PageSize int
	Cursor   model.PageCursor
}

// readKey is read.ts's readKey (D6): per-type dispatch after TYPE + PTTL + best-effort MEMORY
// USAGE classification.
func readKey(ctx context.Context, conn *goredis.Client, key string, req readReq, op *adapters.OpCtx) (page.KeyValuePage, error) {
	meta, err := readMeta(ctx, conn, key)
	if err != nil {
		return page.KeyValuePage{}, err
	}
	op.SetCommand("TYPE " + key)
	fingerprint := adapters.RequestFingerprint(struct {
		Key       string `json:"key"`
		PageSize  int    `json:"pageSize"`
		RedisType string `json:"redisType"`
	}{key, req.PageSize, meta.redisType})

	switch meta.redisType {
	case "string":
		return readString(ctx, conn, key, meta)
	case "hash":
		return readHash(ctx, conn, key, meta, req, fingerprint)
	case "set":
		return readSet(ctx, conn, key, meta, req, fingerprint)
	case "zset":
		return readZSet(ctx, conn, key, meta, req, fingerprint)
	case "list":
		return readList(ctx, conn, key, meta, req)
	case "stream":
		return readStream(ctx, conn, key, meta, req, fingerprint)
	default:
		return page.KeyValuePage{}, adapters.New(adapters.CodeUnsupported, "unsupported redis type for "+key+": "+meta.redisType, nil)
	}
}

// countKey is read.ts's countKey (D6): exact via O(1) type-length commands.
func countKey(ctx context.Context, conn *goredis.Client, key string) (adapters.CountResult, error) {
	rawType, err := conn.Type(ctx, key).Result()
	if err != nil {
		return adapters.CountResult{}, mapError(err)
	}
	if rawType == "none" {
		return adapters.CountResult{}, adapters.New(adapters.CodeQuery, "key no longer exists: "+key, nil)
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return adapters.CountResult{}, err
	}

	var value int64
	switch rawType {
	case "string":
		value = 1
	case "hash":
		value, err = conn.HLen(ctx, key).Result()
	case "set":
		value, err = conn.SCard(ctx, key).Result()
	case "zset":
		value, err = conn.ZCard(ctx, key).Result()
	case "list":
		value, err = conn.LLen(ctx, key).Result()
	case "stream":
		value, err = conn.XLen(ctx, key).Result()
	default:
		return adapters.CountResult{}, adapters.New(adapters.CodeUnsupported, "unsupported redis type for "+key+": "+rawType, nil)
	}
	if err != nil {
		return adapters.CountResult{}, mapError(err)
	}
	return adapters.CountResult{Value: value, Exact: true}, nil
}
