package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Four caps, because a streamed call can grow in a way an HTTP response body cannot —
// the first three are the three (grpc_call_history is a fourth application of the same
// insert-then-trim/global-sweep pattern, following a precedent rather than duplicating an
// abstraction that exists), the fourth (maxGrpcStoredMessages) is genuinely new.
const (
	maxGrpcMessageBytes   = 64 * 1024
	maxGrpcStoredMessages = 100
	// grpcHistoryPerScopeCap is P18 D4's raise from 20 to 30 — mirrored into TS as
	// GRPC_HISTORY_PER_SCOPE_LIMIT (packages/shared/domain/grpcHistory.ts) and pinned equal by
	// tests/unit/go-ts-vocabulary-parity.spec.ts.
	grpcHistoryPerScopeCap = 30
)

// grpcHistoryByteBudget is the table-wide ceiling — a quarter of api_response_history's
// 128 MiB, since a gRPC call's stored payload is capped an order of magnitude lower (100 × 64 KiB
// worst case ≈ 6.4 MiB per entry, against api_response_history's 256 KiB) and the two budgets are
// independent. A var, not a const, only so SetGrpcHistoryByteBudgetForTest can shrink it for one
// test (mirrors response_history.go's own historyByteBudget/SetHistoryByteBudgetForTest split).
var grpcHistoryByteBudget = 32 * 1024 * 1024

type GrpcHistoryRepo struct {
	DB *sql.DB
}

// storedGrpcSnapshot is exactly what snapshot_json holds — Entry is deliberately absent, the same
// reason storedSnapshot (response_history.go) leaves it out: Get rebuilds it from the row's own
// summary columns, so there is no second copy of the same fact to drift from what List reads.
type storedGrpcSnapshot struct {
	Target                  string                          `json:"target"`
	Method                  string                          `json:"method"`
	Streaming               string                          `json:"streaming"`
	Message                 string                          `json:"message"`
	RequestMessageTruncated bool                            `json:"requestMessageTruncated"`
	Metadata                []model.SavedGrpcMetaRow        `json:"metadata"`
	Messages                []model.GrpcCallSnapshotMessage `json:"messages"`
	MessagesElided          bool                            `json:"messagesElided"`
	Header                  []model.SavedGrpcMetaRow        `json:"header"`
	Trailer                 []model.SavedGrpcMetaRow        `json:"trailer"`
	// MetadataElided: Metadata/Header/Trailer cross the control plane uncapped
	// (up to 64 MiB — twice the gRPC budget), unlike Message/Messages, which are capped above.
	// A single call carrying enough of them to push the marshaled snapshot past half the global
	// byte budget on its own would break the sweep's own safety invariant ("no single row can
	// exceed the budget") and empty the whole table, including the row just inserted. When that
	// happens, all three are dropped and this is set instead of ever storing the oversized row.
	MetadataElided bool `json:"metadataElided,omitempty"`
}

// Record is the whole storage policy: apply the four caps, then grpcHistoryTable.Record
// runs the shared transaction/environment-resolve/elide-once/insert/cap/sweep/commit
// chain. The caps live here, not in bridge/grpc.go — Record is the only writer, so they
// cannot be bypassed by a future caller. Only a COMPLETED call is ever recorded (any terminal
// status, including non-OK and a cancellation that received messages) — an in-flight stream
// writes nothing.
func (r *GrpcHistoryRepo) Record(rec model.GrpcCallHistoryRecord) error {
	if err := rec.Validate(); err != nil {
		return err
	}

	var snap storedGrpcSnapshot
	_, err := grpcHistoryTable.Record(r.DB,
		historyRecordInput{TabID: rec.TabID, ItemID: rec.ItemID, EnvironmentID: rec.EnvironmentID},
		grpcHistoryPerScopeCap, grpcHistoryByteBudget,
		func() ([]byte, error) {
			// The request message, capped at 256 KiB — response_history.go's own
			// maxHistoryBodyBytes (same package: "the request body a user typed" is one policy, not
			// two). This was the one free-form string in either history table with no per-entry cap
			// at all, which broke the global sweep's own safety invariant — the sweep is safe
			// only because no single row can exceed its budget, and an uncapped multi-MiB request
			// message could make the running-sum window match zero rows, deleting the entire table
			// including the row just inserted. maxGrpcMessageBytes (64 KiB) stays reserved for
			// *response* messages, of which a call can store up to maxGrpcStoredMessages of; a
			// request message is exactly one per entry, so it gets HTTP's own request-body cap
			// instead.
			message := rec.Message
			requestMessageTruncated := false
			if len(message) > maxHistoryBodyBytes {
				message = capUTF8(message, maxHistoryBodyBytes)
				requestMessageTruncated = true
			}

			messages := make([]model.GrpcCallSnapshotMessage, 0, len(rec.Messages))
			for _, m := range rec.Messages {
				if len(messages) >= maxGrpcStoredMessages {
					break
				}
				if len(m.JSON) > maxGrpcMessageBytes {
					m.JSON = capUTF8(m.JSON, maxGrpcMessageBytes)
					m.Truncated = true
				}
				messages = append(messages, m)
			}
			// Compared against the call's own true MessageCount, not len(rec.Messages) —
			// the caller (grpcclient.ServerStream) already bounds its own Messages slice at the
			// identical cap to keep a long-running stream's in-memory footprint flat, so the
			// slice's length alone can no longer tell an elided call apart from one that produced
			// exactly maxGrpcStoredMessages.
			messagesElided := rec.MessageCount > maxGrpcStoredMessages

			snap = storedGrpcSnapshot{
				Target: rec.Target, Method: rec.Method, Streaming: rec.Streaming,
				Message: message, RequestMessageTruncated: requestMessageTruncated, Metadata: rec.Metadata,
				Messages: messages, MessagesElided: messagesElided,
				Header: rec.Header, Trailer: rec.Trailer,
			}
			return json.Marshal(snap)
		},
		func() ([]byte, error) {
			// Cap the snapshot as a whole, not just field by field — Message and
			// Messages are already bounded (256 KiB / 100 × 64 KiB, well under half the budget), so
			// a snapshot this large can only mean Metadata/Header/Trailer (still uncapped) grew
			// unreasonably. Dropping them and re-marshalling makes the sweep's safety property
			// structural rather than a per-field audit that has to be redone every time a field is
			// added.
			snap.Metadata = nil
			snap.Header = nil
			snap.Trailer = nil
			snap.MetadataElided = true
			return json.Marshal(snap)
		},
		func(tx *sql.Tx, id string, itemID *string, calledAt, environment string, storedBytes int, snapshotJSON []byte) error {
			_, err := tx.Exec(
				`INSERT INTO grpc_call_history
				   (id, item_id, tab_id, called_at, target, method, streaming, environment,
				    code, code_name, status_message, elapsed_ms, message_count, message_bytes, stored_bytes, snapshot_json)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, itemID, rec.TabID, calledAt, rec.Target, rec.Method, rec.Streaming, environment,
				rec.Code, rec.CodeName, rec.StatusMessage, rec.ElapsedMs, rec.MessageCount, rec.MessageBytes,
				storedBytes, string(snapshotJSON),
			)
			return err
		},
	)
	return err
}

const grpcHistoryEntryColumns = `id, item_id, tab_id, called_at, target, method, streaming, environment,
	code, code_name, status_message, elapsed_ms, message_count, message_bytes, stored_bytes`

func scanGrpcHistoryEntry(row rowScanner) (model.GrpcCallHistoryEntry, error) {
	var (
		e      model.GrpcCallHistoryEntry
		itemID sql.NullString
	)
	if err := row.Scan(
		&e.ID, &itemID, &e.TabID, &e.CalledAt, &e.Target, &e.Method, &e.Streaming, &e.Environment,
		&e.Code, &e.CodeName, &e.StatusMessage, &e.ElapsedMs, &e.MessageCount, &e.MessageBytes, &e.StoredBytes,
	); err != nil {
		return model.GrpcCallHistoryEntry{}, err
	}
	if itemID.Valid {
		e.ItemID = &itemID.String
	}
	return e, nil
}

// grpcHistoryTable is the historyTable[T] instance List/Delete/Adopt/SweepOrphans below delegate
// to (T2-14) — grpc_call_history's own table name/scope/order column/summary columns/scan func.
var grpcHistoryTable = historyTable[model.GrpcCallHistoryEntry]{
	table: "grpc_call_history", scope: "grpc_history", timeColumn: "called_at",
	columns: grpcHistoryEntryColumns, scan: scanGrpcHistoryEntry,
}

// List is the summary projection (every column but snapshot_json), newest first — ≤
// grpcHistoryPerScopeCap by construction (Record's own trim).
func (r *GrpcHistoryRepo) List(scopeKey string) ([]model.GrpcCallHistoryEntry, error) {
	return grpcHistoryTable.List(r.DB, scopeKey)
}

// Get decodes one row's snapshot_json and rebuilds Entry from the row's own summary columns —
// GetHistory (P107 I2-8) runs the shared query, this closure does the rest.
func (r *GrpcHistoryRepo) Get(id string) (model.GrpcCallSnapshot, error) {
	return GetHistory(r.DB, "grpc_call_history", grpcHistoryEntryColumns, id, func(row rowScanner) (model.GrpcCallSnapshot, error) {
		var (
			e            model.GrpcCallHistoryEntry
			itemID       sql.NullString
			snapshotJSON string
		)
		if err := row.Scan(
			&e.ID, &itemID, &e.TabID, &e.CalledAt, &e.Target, &e.Method, &e.Streaming, &e.Environment,
			&e.Code, &e.CodeName, &e.StatusMessage, &e.ElapsedMs, &e.MessageCount, &e.MessageBytes, &e.StoredBytes,
			&snapshotJSON,
		); err != nil {
			return model.GrpcCallSnapshot{}, fmt.Errorf("repos/grpc_history: get: %w", err)
		}
		if itemID.Valid {
			e.ItemID = &itemID.String
		}

		var snap storedGrpcSnapshot
		if err := json.Unmarshal([]byte(snapshotJSON), &snap); err != nil {
			return model.GrpcCallSnapshot{}, fmt.Errorf("repos/grpc_history: decode snapshot %s: %w", id, err)
		}

		return model.GrpcCallSnapshot{
			Entry: e, Target: snap.Target, Method: snap.Method, Streaming: snap.Streaming,
			Message: snap.Message, RequestMessageTruncated: snap.RequestMessageTruncated,
			Metadata: snap.Metadata,
			Messages: snap.Messages, MessagesElided: snap.MessagesElided,
			MetadataElided: snap.MetadataElided,
			Header:         snap.Header, Trailer: snap.Trailer,
		}, nil
	})
}

// Delete removes one entry.
func (r *GrpcHistoryRepo) Delete(id string) error {
	return grpcHistoryTable.Delete(r.DB, id)
}

// Clear removes every entry in one scope.
func (r *GrpcHistoryRepo) Clear(scopeKey string) error {
	if _, err := r.DB.Exec(`DELETE FROM grpc_call_history WHERE scope_key = ?`, scopeKey); err != nil {
		return fmt.Errorf("repos/grpc_history: clear: %w", err)
	}
	return nil
}

// Adopt moves a scratch tab's history onto a newly-saved request (mirrors
// response_history.go's own D14) — one UPDATE, scope follows via the generated column.
func (r *GrpcHistoryRepo) Adopt(tabID, itemID string) (int, error) {
	return grpcHistoryTable.Adopt(r.DB, tabID, itemID)
}

// SweepOrphans deletes a scratch tab's history once its tab is gone — run once at launch beside
// response_history's own startup prune (mirrors D7).
func (r *GrpcHistoryRepo) SweepOrphans() error {
	return grpcHistoryTable.SweepOrphans(r.DB)
}
