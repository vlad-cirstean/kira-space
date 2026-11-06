package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"log/slog"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// historyLimit mirrors filter-history.ts's HISTORY_LIMIT.
const historyLimit = 20

// filter_history was capped per (connection_id, path) but had no bound on the number of
// paths a connection accumulates — every table/collection a person has ever filtered, on a
// connection they have never deleted, with no liveness oracle to sweep a dropped one. Not a
// new number: metadata_cache's own 200-paths-per-connection cap (the adjacent "how many
// distinct paths is one connection's tree worth caching" question this app already answered)
// times historyLimit's 20 rows per path. Eviction is by used_at across the whole connection, so a
// path filtered once years ago falls off before one still in regular use.
const historyPerConnectionLimit = 200 * historyLimit

// maxFilterTextBytes caps where_text and the encoded order_by_json — a filter predicate is a line
// a person typed into a filter box, not a pasted document; 4 KiB is ~4,000 characters, past every
// Mongo filter document this app's own tests use. Truncated silently, with no flag: unlike
// op_log's command (which Re-run replays blind), a history entry only ever populates the filter
// box for the user to see and edit before it is applied (bridge/queries.go's HistoryList feeds a
// picker, not an executor) — a truncated entry is visibly truncated at the moment it matters.
const maxFilterTextBytes = 4 * 1024

// capFilterText applies maxFilterTextBytes to where_text — a value that may be absent (nil stays
// nil). Truncates on a UTF-8 rune boundary, never mid-sequence, so a truncated
// entry is still valid UTF-8 rather than a split rune the renderer would have to guess at.
// order_by_json's own cap is capOrderByJSON below, not this function — an *encoded JSON* string
// truncated at any byte offset, rune-boundary-safe or not, is still invalid JSON.
func capFilterText(s *string) *string {
	if s == nil || len(*s) <= maxFilterTextBytes {
		return s
	}
	truncated := string(truncateUTF8ToBoundary([]byte(*s), maxFilterTextBytes))
	return &truncated
}

// truncateUTF8ToBoundary cuts b at maxBytes on a UTF-8 rune boundary, never mid-sequence — mirrors
// internal/page/scratch.go's own unexported helper of the same name (that package's own, cannot be
// imported from here).
func truncateUTF8ToBoundary(b []byte, maxBytes int) []byte {
	if len(b) <= maxBytes {
		return b
	}
	cut := b[:maxBytes]
	for len(cut) > 0 {
		r, size := utf8.DecodeLastRune(cut)
		if r != utf8.RuneError || size != 1 {
			break
		}
		cut = cut[:len(cut)-1]
	}
	return cut
}

// capUTF8 is truncateUTF8ToBoundary for a string: s unchanged when it fits, otherwise cut at a rune
// boundary.
func capUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	return string(truncateUTF8ToBoundary([]byte(s), maxBytes))
}

// capOrderByJSON is order_by_json's own cap: unlike where_text, this value is
// encoded JSON, and truncating it at any offset — even a valid UTF-8 rune boundary — still leaves
// invalid JSON behind. List's own decode guard already drops a row whose order_by_json fails to
// parse (silently, since a bad row must not make the whole history unreadable), so a truncated one
// was being written only to be discarded later — this drops the sort spec outright instead,
// keeping whatever encoded string is short enough to record faithfully.
func capOrderByJSON(s *string) *string {
	if s == nil || len(*s) <= maxFilterTextBytes {
		return s
	}
	return nil
}

type FilterHistoryRepo struct {
	DB *sql.DB
}

// Record mirrors filter-history.ts's recordFilterUse. "I cleared the filter" (both nil) is not
// history and records nothing. The dedupe delete uses SQLite's IS operator (NULL-safe equality),
// replacing the TS build's isNull-vs-eq conditional with one SQL string — re-recording an
// identical entry moves it to the top rather than duplicating it.
func (r *FilterHistoryRepo) Record(connectionID, path string, where *string, orderBy *model.SortSpec) error {
	if where == nil && orderBy == nil {
		return nil
	}

	var orderByJSON *string
	if orderBy != nil {
		encoded, err := json.Marshal(orderBy)
		if err != nil {
			return fmt.Errorf("repos/filter_history: encode orderBy: %w", err)
		}
		s := string(encoded)
		orderByJSON = &s
	}
	// D4's per-row cap, applied before the dedupe delete/insert so both compare (and store)
	// exactly what will be kept.
	where = capFilterText(where)
	orderByJSON = capOrderByJSON(orderByJSON)
	// F9: an oversized orderBy is dropped by capOrderByJSON above, not truncated — re-check the
	// same "nothing worth recording" condition Record's own top already applies, now that this
	// value can end up nil here too (an oversized sort spec with no where_text of its own).
	if where == nil && orderByJSON == nil {
		return nil
	}

	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos/filter_history: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`DELETE FROM filter_history
		  WHERE connection_id = ? AND path = ? AND where_text IS ? AND order_by_json IS ?`,
		connectionID, path, where, orderByJSON,
	); err != nil {
		return fmt.Errorf("repos/filter_history: dedupe delete: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO filter_history (id, connection_id, path, where_text, order_by_json, used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ids.NewID(), connectionID, path, where, orderByJSON, kiratime.NowISO(),
	); err != nil {
		return fmt.Errorf("repos/filter_history: insert: %w", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM filter_history
		 WHERE connection_id = ? AND path = ?
		   AND id NOT IN (
		     SELECT id FROM filter_history
		      WHERE connection_id = ? AND path = ?
		      ORDER BY used_at DESC, rowid DESC
		      LIMIT ?
		   )
	`, connectionID, path, connectionID, path, historyLimit); err != nil {
		return fmt.Errorf("repos/filter_history: cap: %w", err)
	}

	// The bound a per-path cap alone cannot give — across every path a connection has ever
	// been filtered on, keeping the historyPerConnectionLimit most recently used rows regardless
	// of which path they belong to, so a path filtered once long ago falls off before one still in
	// regular use.
	if _, err := tx.Exec(`
		DELETE FROM filter_history
		 WHERE connection_id = ?
		   AND id NOT IN (
		     SELECT id FROM filter_history
		      WHERE connection_id = ?
		      ORDER BY used_at DESC, rowid DESC
		      LIMIT ?
		   )
	`, connectionID, connectionID, historyPerConnectionLimit); err != nil {
		return fmt.Errorf("repos/filter_history: cap connection: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/filter_history: commit: %w", err)
	}
	return nil
}

func (r *FilterHistoryRepo) List(connectionID, path string, limit int) ([]model.FilterHistoryEntry, error) {
	rows, err := r.DB.Query(
		`SELECT id, connection_id, path, where_text, order_by_json, used_at
		   FROM filter_history
		  WHERE connection_id = ? AND path = ?
		  ORDER BY used_at DESC, rowid DESC
		  LIMIT ?`,
		connectionID, path, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/filter_history: query: %w", err)
	}
	defer rows.Close()

	out := []model.FilterHistoryEntry{}
	for rows.Next() {
		var (
			e           model.FilterHistoryEntry
			whereText   sql.NullString
			orderByJSON sql.NullString
		)
		if err := rows.Scan(&e.ID, &e.ConnectionID, &e.Path, &whereText, &orderByJSON, &e.UsedAt); err != nil {
			return nil, fmt.Errorf("repos/filter_history: scan: %w", err)
		}
		if whereText.Valid {
			e.Where = &whereText.String
		}
		if orderByJSON.Valid {
			var spec model.SortSpec
			if err := json.Unmarshal([]byte(orderByJSON.String), &spec); err != nil {
				slog.Warn("dropping filter history row: order_by_json is not valid", "scope", "storage/filter-history", "id", e.ID)
				continue
			}
			e.OrderBy = &spec
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/filter_history: rows: %w", err)
	}
	return out, nil
}
