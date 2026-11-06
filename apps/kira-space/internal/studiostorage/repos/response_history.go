package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Three caps, because three independent things can grow — bytes per entry, count per
// scope, and bytes across the whole table. maxHistoryBodyBytes applies once to the response body
// and once to the request body; historyPerScopeLimit mirrors filter_history.go's/
// variables.go's own per-scope count cap.
const (
	maxHistoryBodyBytes = 256 * 1024
	// historyPerScopeLimit is mirrored into TS as HISTORY_PER_SCOPE_LIMIT
	// (packages/shared/domain/responseHistory.ts) and pinned equal by
	// tests/unit/go-ts-vocabulary-parity.spec.ts, so the two numbers cannot drift the way
	// ResponseHistoryList.vue's own hand-written "20" already had.
	historyPerScopeLimit = 30
)

// historyByteBudget is the table-wide ceiling — the same order of magnitude as
// cache.l2BudgetMb's own budget, deliberately. A var, not a const, only so
// SetHistoryByteBudgetForTest (response_history_internal_test.go) can shrink it for one test —
// the cross-scope eviction case, which needs Record's own real sweep to fire without
// reproducing 128 MiB of real rows. Every real caller only ever reads it.
var historyByteBudget = 128 * 1024 * 1024

type ResponseHistoryRepo struct {
	DB *sql.DB
}

// storedSnapshot is exactly what snapshot_json holds — Entry is deliberately absent (D4's Get
// rebuilds it from the row's own summary columns instead, so there is no second copy of the same
// fact to drift from the columns a List projection already reads).
type storedSnapshot struct {
	Request                     model.ResponseHistoryRequest `json:"request"`
	Response                    httpclient.Response          `json:"response"`
	BodyStored                  bool                         `json:"bodyStored"`
	BodyStorageTruncated        bool                         `json:"bodyStorageTruncated"`
	RequestBodyStorageTruncated bool                         `json:"requestBodyStorageTruncated"`
	// RequestFieldsElided: capBody only caps 'raw'/'code' bodies — urlencoded and
	// formdata field values have no per-field cap at all, and only stay safe today because the
	// control plane's 64 MiB ceiling happens to sit below the 128 MiB history budget, a coincidence
	// of two unrelated numbers rather than an invariant. If a snapshot still ends up past half the
	// global byte budget after the per-field caps above, the request's field values are dropped
	// (Body.Mode/structure kept, values cleared) rather than ever storing an oversized row — the
	// same structural backstop grpc_history.go's own MetadataElided gives the gRPC table.
	RequestFieldsElided bool `json:"requestFieldsElided,omitempty"`
}

// Record is the whole storage policy: apply the two body rules, then
// responseHistoryTable.Record runs the shared transaction/environment-resolve/
// elide-once/insert/cap/sweep/commit chain. The caps live here, not in bridge/http.go —
// Record is the only writer, so they cannot be bypassed by a future caller.
func (r *ResponseHistoryRepo) Record(rec model.ResponseHistoryRecord) error {
	if err := rec.Validate(); err != nil {
		return err
	}

	var snap storedSnapshot
	_, err := responseHistoryTable.Record(r.DB,
		historyRecordInput{TabID: rec.TabID, ItemID: rec.ItemID, EnvironmentID: rec.EnvironmentID},
		historyPerScopeLimit, historyByteBudget,
		func() ([]byte, error) {
			reqBody, requestBodyStorageTruncated := capBody(rec.Body)
			resp := rec.Response
			// The rendered exchange is live-only. It is the resolved request in text
			// form, and even masked it would double a snapshot's size for a pane that
			// cannot be opened from a stored entry anyway. Wire is a pointer with
			// json:"wire,omitempty" (httpclient.Response), so nil here means a stored snapshot's
			// JSON carries no "wire" key at all, not just a null one.
			resp.Wire = nil
			bodyStored := resp.BodyEncoding != "base64"
			bodyStorageTruncated := false
			if bodyStored {
				if len(resp.Body) > maxHistoryBodyBytes {
					resp.Body = capUTF8(resp.Body, maxHistoryBodyBytes)
					bodyStorageTruncated = true
				}
			} else {
				// D5 rule 2: a binary body is not stored at all — every other field (including
				// BodyBytes, F10's "412 KB of binary data") stays intact.
				resp.Body = ""
			}

			snap = storedSnapshot{
				Request: model.ResponseHistoryRequest{
					Method:  rec.Method,
					URL:     rec.URL,
					Headers: rec.Headers,
					Body:    reqBody,
				},
				Response:                    resp,
				BodyStored:                  bodyStored,
				BodyStorageTruncated:        bodyStorageTruncated,
				RequestBodyStorageTruncated: requestBodyStorageTruncated,
			}
			return json.Marshal(snap)
		},
		func() ([]byte, error) {
			// Cap the snapshot as a whole, not just field by field. capBody only
			// bounds 'raw'/'code'; a urlencoded/formdata body's field values have no per-field cap
			// at all and could otherwise push a single row's stored_bytes past the sweep's own
			// safety margin (the sweep is safe only because no single row can exceed the budget by
			// itself).
			snap.Request.Body.URLEncoded = nil
			snap.Request.Body.FormData = nil
			snap.Request.Body.Raw = ""
			snap.Request.Body.Code = ""
			snap.RequestFieldsElided = true
			// Response-side header sets are the other unbounded part; drop them too so no row
			// stays over the budget the sweep assumes.
			snap.Response.Headers = nil
			snap.Response.SentCookies = nil
			snap.Response.ReceivedCookies = nil
			hops := make([]httpclient.TimelineHop, len(snap.Response.Timeline.Hops))
			copy(hops, snap.Response.Timeline.Hops)
			for i := range hops {
				hops[i].Headers = nil
			}
			snap.Response.Timeline.Hops = hops
			return json.Marshal(snap)
		},
		func(tx *sql.Tx, id string, itemID *string, sentAt, environment string, storedBytes int, snapshotJSON []byte) error {
			_, err := tx.Exec(
				`INSERT INTO api_response_history
				   (id, item_id, tab_id, sent_at, method, url, environment, status, status_text,
				    elapsed_ms, body_bytes, stored_bytes, snapshot_json)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, itemID, rec.TabID, sentAt, rec.Method, rec.URL, environment,
				rec.Response.Status, rec.Response.StatusText, rec.Response.ElapsedMs,
				rec.Response.BodyBytes, storedBytes, string(snapshotJSON),
			)
			return err
		},
	)
	return err
}

// capBody applies the per-entry byte cap to whichever member of Body carries free-form
// text — 'raw' and 'code' are the only two modes whose single string can be arbitrarily long;
// urlencoded/formdata are structured field lists (small in practice) and 'file' carries a local
// path, never bytes, on the wire.
func capBody(b httpclient.Body) (httpclient.Body, bool) {
	switch b.Mode {
	case "raw":
		if len(b.Raw) > maxHistoryBodyBytes {
			b.Raw = capUTF8(b.Raw, maxHistoryBodyBytes)
			return b, true
		}
	case "code":
		if len(b.Code) > maxHistoryBodyBytes {
			b.Code = capUTF8(b.Code, maxHistoryBodyBytes)
			return b, true
		}
	}
	return b, false
}

const responseHistoryEntryColumns = `id, item_id, tab_id, sent_at, method, url, environment,
	status, status_text, elapsed_ms, body_bytes, stored_bytes`

func scanResponseHistoryEntry(row rowScanner) (model.ResponseHistoryEntry, error) {
	var (
		e      model.ResponseHistoryEntry
		itemID sql.NullString
	)
	if err := row.Scan(
		&e.ID, &itemID, &e.TabID, &e.SentAt, &e.Method, &e.URL, &e.Environment,
		&e.Status, &e.StatusText, &e.ElapsedMs, &e.BodyBytes, &e.StoredBytes,
	); err != nil {
		return model.ResponseHistoryEntry{}, err
	}
	if itemID.Valid {
		e.ItemID = &itemID.String
	}
	return e, nil
}

// responseHistoryTable is the historyTable[T] instance List/Delete/Adopt/SweepOrphans below
// delegate to (T2-14) — api_response_history's own table name/scope/order column/summary
// columns/scan func.
var responseHistoryTable = historyTable[model.ResponseHistoryEntry]{
	table: "api_response_history", scope: "response_history", timeColumn: "sent_at",
	columns: responseHistoryEntryColumns, scan: scanResponseHistoryEntry,
}

// List is the summary projection (every column but snapshot_json), newest first — ≤
// historyPerScopeLimit by construction (Record's own trim).
func (r *ResponseHistoryRepo) List(scopeKey string) ([]model.ResponseHistoryEntry, error) {
	return responseHistoryTable.List(r.DB, scopeKey)
}

// Get decodes one row's snapshot_json and rebuilds Entry from the row's own summary columns
// — a corrupt snapshot_json is reported, not silently blanked, since Get is a single-entry
// lookup rather than a list a bad row could otherwise blank entirely (repos/saved_queries.go's
// posture is for List, not Get). GetHistory runs the shared query, this closure does the rest.
func (r *ResponseHistoryRepo) Get(id string) (model.ResponseHistorySnapshot, error) {
	return GetHistory(r.DB, "api_response_history", responseHistoryEntryColumns, id, func(row rowScanner) (model.ResponseHistorySnapshot, error) {
		var (
			e            model.ResponseHistoryEntry
			itemID       sql.NullString
			snapshotJSON string
		)
		if err := row.Scan(
			&e.ID, &itemID, &e.TabID, &e.SentAt, &e.Method, &e.URL, &e.Environment,
			&e.Status, &e.StatusText, &e.ElapsedMs, &e.BodyBytes, &e.StoredBytes, &snapshotJSON,
		); err != nil {
			return model.ResponseHistorySnapshot{}, fmt.Errorf("repos/response_history: get: %w", err)
		}
		if itemID.Valid {
			e.ItemID = &itemID.String
		}

		var snap storedSnapshot
		if err := json.Unmarshal([]byte(snapshotJSON), &snap); err != nil {
			return model.ResponseHistorySnapshot{}, fmt.Errorf("repos/response_history: decode snapshot %s: %w", id, err)
		}

		return model.ResponseHistorySnapshot{
			Entry:                       e,
			Request:                     snap.Request,
			Response:                    snap.Response,
			BodyStored:                  snap.BodyStored,
			BodyStorageTruncated:        snap.BodyStorageTruncated,
			RequestBodyStorageTruncated: snap.RequestBodyStorageTruncated,
			RequestFieldsElided:         snap.RequestFieldsElided,
		}, nil
	})
}

// Delete removes one entry.
func (r *ResponseHistoryRepo) Delete(id string) error {
	return responseHistoryTable.Delete(r.DB, id)
}

// Clear removes every entry in one scope.
func (r *ResponseHistoryRepo) Clear(scopeKey string) error {
	if _, err := r.DB.Exec(`DELETE FROM api_response_history WHERE scope_key = ?`, scopeKey); err != nil {
		return fmt.Errorf("repos/response_history: clear: %w", err)
	}
	return nil
}

// Adopt moves a scratch tab's history onto a newly-saved request (D14) — one UPDATE, scope
// follows via the generated column (F8) with no second write.
func (r *ResponseHistoryRepo) Adopt(tabID, itemID string) (int, error) {
	return responseHistoryTable.Adopt(r.DB, tabID, itemID)
}

// SweepOrphans deletes a scratch tab's history once its tab is gone (D7) — run once at launch
// (main.go, beside oplog's own startup prune, F18). tabs is the liveness oracle: TabsRepo.Save
// rewrites a window's whole tab set but always re-inserts what is currently open, so a row absent
// there is a tab that was actually closed.
func (r *ResponseHistoryRepo) SweepOrphans() error {
	return responseHistoryTable.SweepOrphans(r.DB)
}
