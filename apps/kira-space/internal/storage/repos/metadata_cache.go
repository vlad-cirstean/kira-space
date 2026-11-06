package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"log/slog"
)

// maxMetadataPayloadBytes and maxMetadataRowsPerConnection mirror metadata-cache.ts's
// MAX_PAYLOAD_BYTES and MAX_ROWS_PER_CONNECTION (P43 iter2 F15/D20).
const (
	maxMetadataPayloadBytes      = 4 * 1024 * 1024
	maxMetadataRowsPerConnection = 200
)

type MetadataCacheRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// readPayload returns the row's payload object (or nil if there is no row / it fails to parse —
// treated as a miss, mirroring metadata-cache.ts's own try/catch).
func (r *MetadataCacheRepo) readPayload(q sqlb.Querier, connectionID, path string) map[string]json.RawMessage {
	c := kiraschema.ConnectionMetadataCache
	sb := r.Dialect.Select(c.PayloadJSON).From(c.Table)
	sb.Where(sb.Equal(c.ConnectionID, connectionID), sb.Equal(c.Path, path))
	var payloadJSON string
	if err := sqlb.QueryRow(q, sb).Scan(&payloadJSON); err != nil {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil
	}
	return payload
}

// fetchedAtKey is the reserved payload key holding each kind's own last-write timestamp:
// { "children": …, "columns": …, "fetchedAt": {"children": "…", "columns": "…"} }. The four kind
// names are a closed set fixed in internal/tree/service.go, so this can never collide with a
// payload key.
const fetchedAtKey = "fetchedAt"

// fetchedAtFor reads kind's own entry out of payload's reserved fetchedAt map — "" when the
// map or the entry is absent, which is the correct answer for both a pre-P24 row (D2's upgrade
// path) and a row that has simply never cached this kind.
func fetchedAtFor(payload map[string]json.RawMessage, kind string) string {
	raw, ok := payload[fetchedAtKey]
	if !ok {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return m[kind]
}

// Get is JSON.parse'd, NOT further validated here — callers parse through their own domain
// shape and treat a mismatch as a miss (metadata-cache.ts's own discipline). The second return is
// the payload's own per-kind fetch time — internal/tree.Service is the sole judge of
// whether that makes it stale.
func (r *MetadataCacheRepo) Get(connectionID, path, kind string) (json.RawMessage, string, error) {
	payload := r.readPayload(r.DB, connectionID, path)
	return payload[kind], fetchedAtFor(payload, kind), nil
}

// Put merges payload into the existing row's {children?, describe?, definition?, columns?} object
// (the unique index is (connection_id, path) — kind is not part of the key, so a 'children'
// payload and a 'describe' payload for the same path share one row) and stamps this kind's own
// entry in the reserved fetchedAt map — every other kind's payload AND fetch time is left
// untouched. Then runs the per-connection eviction — all in one transaction.
func (r *MetadataCacheRepo) Put(connectionID, path, kind string, payload json.RawMessage) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos/metadata_cache: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	existing := r.readPayload(tx, connectionID, path)
	merged := map[string]json.RawMessage{}
	for k, v := range existing {
		merged[k] = v
	}
	merged[kind] = payload

	now := kiratime.NowISO()
	fetchedAt := map[string]string{}
	if raw, ok := existing[fetchedAtKey]; ok {
		_ = json.Unmarshal(raw, &fetchedAt) // best-effort; a corrupt map just resets to {kind: now}
	}
	fetchedAt[kind] = now
	encodedFetchedAt, err := json.Marshal(fetchedAt)
	if err != nil {
		return fmt.Errorf("repos/metadata_cache: encode fetchedAt map: %w", err)
	}
	merged[fetchedAtKey] = encodedFetchedAt

	encoded, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("repos/metadata_cache: encode merged payload: %w", err)
	}
	if len(encoded) > maxMetadataPayloadBytes {
		slog.Warn("payload exceeds 4 MB, not cached", "scope", "storage/metadata-cache", "connectionId", connectionID, "path", path)
		return nil // not an error; the deferred Rollback is a no-op since nothing was written.
	}

	c := kiraschema.ConnectionMetadataCache
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ConnectionID, c.Path, c.Kind, c.PayloadJSON, c.FetchedAt, c.Etag).
		Values(connectionID, path, kind, string(encoded), kiratime.ISOOrZero(now), nil)
	sqlb.Upsert(ib, []string{c.ConnectionID, c.Path}, []string{c.Kind, c.PayloadJSON, c.FetchedAt}, "")
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("repos/metadata_cache: upsert: %w", err)
	}

	keep := r.Dialect.Select(c.Path).From(c.Table)
	keep.Where(keep.Equal(c.ConnectionID, connectionID)).
		OrderBy(c.FetchedAt+" DESC", sqlb.RowID+" DESC").Limit(maxMetadataRowsPerConnection)
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ConnectionID, connectionID), del.NotIn(c.Path, keep))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return fmt.Errorf("repos/metadata_cache: evict: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/metadata_cache: commit: %w", err)
	}
	return nil
}

// Drop clears one cached path (path == "" clears the whole connection — see DropConnection for
// the explicit form callers should prefer for that case).
func (r *MetadataCacheRepo) Drop(connectionID, path string) error {
	c := kiraschema.ConnectionMetadataCache
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ConnectionID, connectionID), del.Equal(c.Path, path))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos/metadata_cache: drop %s:%s: %w", connectionID, path, err)
	}
	return nil
}

// DropConnection drops every cached row for the whole connection (metadata-cache.ts's
// dropCached with path omitted).
func (r *MetadataCacheRepo) DropConnection(connectionID string) error {
	c := kiraschema.ConnectionMetadataCache
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ConnectionID, connectionID))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos/metadata_cache: drop connection %s: %w", connectionID, err)
	}
	return nil
}
