package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const connectionSelectColumns = `
	id, COALESCE(stem || '.yaml', ''), name, kind, color, mode, read_only, host, port, database, username,
	options_json, preconnect, preconnect_sidecar, preconnect_delay_sec, auto_explain, throttle_per_sec,
	mcp_enabled,
	mcp_description, mcp_read_mode, mcp_write_mode, mcp_ddl_mode, mcp_auto_explain, mcp_review_results,
	sort_order, created_at, updated_at
`

// ConnectionsRepo reads the connections table, the projection of the connection files. Only
// connfiles writes it, except Reorder: order is state, not config.
type ConnectionsRepo struct {
	DB *sql.DB
}

// rowScanner is satisfied by both *sql.Rows and *sql.Row, letting scanConnectionRow serve both
// List (many rows) and Get (one row) with the same column layout and validation.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanConnectionRow never sets URI: a URI is a secret the file only names, so every summary the repo
// returns has URI == nil. It scans one row in connectionSelectColumns's
// order and validates it the same way connections.ts's parseRow does: a hand-mangled row is
// dropped (nil, nil), not propagated —
// unlike settings/layout, a bad connection row must not make the whole app unlaunchable.
func scanConnectionRow(row rowScanner) (*model.ConnectionSummary, error) {
	var (
		c                                                          model.ConnectionSummary
		host, database, username                                   sql.NullString
		port                                                       sql.NullInt64
		options                                                    sql.NullString
		preconnect                                                 sql.NullString
		readOnly, sidecar, autoExplain, mcpEnabled, mcpAutoExplain int
	)
	if err := row.Scan(
		&c.ID, &c.File, &c.Name, &c.Kind, &c.Color, &c.Mode, &readOnly, &host, &port, &database,
		&username, &options, &preconnect, &sidecar, &c.PreconnectDelaySec, &autoExplain, &c.ThrottlePerSec,
		&mcpEnabled, &c.McpDescription, &c.McpReadMode, &c.McpWriteMode, &c.McpDdlMode, &mcpAutoExplain, &c.McpReviewResults,
		&c.SortOrder, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if !model.ValidConnectionKind(c.Kind) {
		slog.Warn("dropping connection row: unrecognised kind", "scope", "storage/connections", "id", c.ID, "kind", c.Kind)
		return nil, nil
	}
	if !model.ValidConnectionColor(c.Color) {
		slog.Warn("dropping connection row: unrecognised color", "scope", "storage/connections", "id", c.ID, "color", c.Color)
		return nil, nil
	}
	if !model.ValidConnectionMode(c.Mode) {
		slog.Warn("dropping connection row: unrecognised mode", "scope", "storage/connections", "id", c.ID, "mode", c.Mode)
		return nil, nil
	}
	// M2: an unreadable permission word coerces to the strictest value (deny) rather than dropping
	// the row — dropping would hide the connection from the app entirely; coercing keeps it visible
	// and keeps it safe.
	if !model.ValidMcpPermissionMode(c.McpReadMode) {
		slog.Warn("coercing connection row: unrecognised MCP read mode", "scope", "storage/connections", "id", c.ID, "mode", c.McpReadMode)
		c.McpReadMode = "deny"
	}
	if !model.ValidMcpPermissionMode(c.McpWriteMode) {
		slog.Warn("coercing connection row: unrecognised MCP write mode", "scope", "storage/connections", "id", c.ID, "mode", c.McpWriteMode)
		c.McpWriteMode = "deny"
	}
	if !model.ValidMcpPermissionMode(c.McpDdlMode) {
		slog.Warn("coercing connection row: unrecognised MCP DDL mode", "scope", "storage/connections", "id", c.ID, "mode", c.McpDdlMode)
		c.McpDdlMode = "deny"
	}
	// P382: same fail-closed coercion; an unreadable value holds results for review.
	if !model.ValidMcpReviewResults(c.McpReviewResults) {
		slog.Warn("coercing connection row: unrecognised MCP review setting", "scope", "storage/connections", "id", c.ID, "value", c.McpReviewResults)
		c.McpReviewResults = "review"
	}

	c.ReadOnly = readOnly != 0
	c.PreconnectSidecar = sidecar != 0
	c.AutoExplain = autoExplain != 0
	c.McpEnabled = mcpEnabled != 0
	c.McpAutoExplain = mcpAutoExplain != 0
	if host.Valid {
		c.Host = &host.String
	}
	if port.Valid {
		p := int(port.Int64)
		c.Port = &p
	}
	if database.Valid {
		c.Database = &database.String
	}
	if username.Valid {
		c.Username = &username.String
	}
	if preconnect.Valid {
		c.Preconnect = &preconnect.String
	}

	c.Options = map[string]any{}
	if options.Valid && options.String != "" {
		if err := json.Unmarshal([]byte(options.String), &c.Options); err != nil {
			slog.Warn("dropping connection row: options_json is not valid JSON", "scope", "storage/connections", "id", c.ID)
			return nil, nil
		}
	}
	return &c, nil
}

// List orders by sort_order ASC, name ASC (the name tiebreak connections.ts's own listConnections
// applies — two rows sharing a sort_order must not come back in arbitrary order) and never selects
// password.
func (r *ConnectionsRepo) List() ([]model.ConnectionSummary, error) {
	rows, err := r.DB.Query(`SELECT ` + connectionSelectColumns + ` FROM connections ORDER BY sort_order ASC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("repos/connections: query: %w", err)
	}
	defer rows.Close()

	out := []model.ConnectionSummary{}
	for rows.Next() {
		c, err := scanConnectionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repos/connections: scan: %w", err)
		}
		if c != nil {
			out = append(out, *c)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/connections: rows: %w", err)
	}
	return out, nil
}

// KindOf satisfies adapterhost.KindLookup — the router's per-connection native/Node-served
// routing decision. A plain DB read for now, not the connections service's own in-memory state
// map: main.go constructs the router before connections.Service exists (the router is one of
// that service's own Deps), so there is no earlier in-memory source to prefer yet. Worth
// revisiting once nativeKinds is non-empty and this is on every data op's hot path.
func (r *ConnectionsRepo) KindOf(connID string) (string, bool) {
	summary, err := r.Get(connID)
	if err != nil || summary == nil {
		return "", false
	}
	return summary.Kind, true
}

// Get returns (nil, nil) when no row with this id exists (or the row failed validation).
func (r *ConnectionsRepo) Get(connID string) (*model.ConnectionSummary, error) {
	c, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.ConnectionSummary, error) {
		return scanConnectionRow(row)
	}, `SELECT `+connectionSelectColumns+` FROM connections WHERE id = ?`, connID)
	if err != nil {
		return nil, fmt.Errorf("repos/connections: get %s: %w", connID, err)
	}
	return c, nil
}

// Reorder applies a user's drag order and rewrites sort_order dense. updated_at stays: order is
// not content.
func (r *ConnectionsRepo) Reorder(ids []string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos/connections: reorder: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := sqlitex.ReorderSortOrder(tx,
		`SELECT id FROM connections ORDER BY sort_order ASC, name ASC`,
		`UPDATE connections SET sort_order = ? WHERE id = ?`,
		ids,
	); err != nil {
		return fmt.Errorf("repos/connections: reorder: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/connections: reorder: commit: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
