package repos

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// connectionSelectColumns is the column list scanConnectionRow reads, in scan order.
var connectionSelectColumns = []string{
	kiraschema.Connections.ID, "COALESCE(" + kiraschema.Connections.Stem + " || '.yaml', '')", kiraschema.Connections.Name,
	kiraschema.Connections.Kind, kiraschema.Connections.Color, kiraschema.Connections.Mode, kiraschema.Connections.ReadOnly,
	kiraschema.Connections.Host, kiraschema.Connections.Port, kiraschema.Connections.Database, kiraschema.Connections.Username,
	kiraschema.Connections.OptionsJSON, kiraschema.Connections.Preconnect, kiraschema.Connections.PreconnectSidecar,
	kiraschema.Connections.PreconnectDelaySec, kiraschema.Connections.AutoExplain, kiraschema.Connections.ThrottlePerSec,
	kiraschema.Connections.MCPEnabled, kiraschema.Connections.MCPDescription, kiraschema.Connections.MCPReadMode,
	kiraschema.Connections.MCPWriteMode, kiraschema.Connections.MCPDdlMode, kiraschema.Connections.MCPMaskedMode,
	kiraschema.Connections.MCPAutoExplain, kiraschema.Connections.MCPReviewRead, kiraschema.Connections.MCPReviewWrite,
	kiraschema.Connections.MCPReviewDdl, kiraschema.Connections.MCPReviewMasked, kiraschema.Connections.MCPStatementTimeoutSec,
	kiraschema.Connections.MCPTablesJSON, kiraschema.Connections.MaskReviewOn, kiraschema.Connections.SortOrder,
	kiraschema.Connections.CreatedAt, kiraschema.Connections.UpdatedAt,
}

// ConnectionsRepo reads the connections table, the projection of the connection files. Only
// connfiles writes it, except Reorder: order is state, not config.
type ConnectionsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
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
		mcpTables                                                  sql.NullString
		readOnly, sidecar, autoExplain, mcpEnabled, mcpAutoExplain int
		maskReview                                                 int
		createdMs, updatedMs                                       int64
	)
	if err := row.Scan(
		&c.ID, &c.File, &c.Name, &c.Kind, &c.Color, &c.Mode, &readOnly, &host, &port, &database,
		&username, &options, &preconnect, &sidecar, &c.PreconnectDelaySec, &autoExplain, &c.ThrottlePerSec,
		&mcpEnabled, &c.McpDescription, &c.McpReadMode, &c.McpWriteMode, &c.McpDdlMode, &c.McpMaskedMode, &mcpAutoExplain, &c.McpReview.Read, &c.McpReview.Write, &c.McpReview.Ddl, &c.McpReview.Masked, &c.McpStatementTimeoutSec, &mcpTables, &maskReview,
		&c.SortOrder, &createdMs, &updatedMs,
	); err != nil {
		return nil, err
	}
	c.CreatedAt, c.UpdatedAt = kiratime.MsToISO(createdMs), kiratime.MsToISO(updatedMs)

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
	// P535: an unreadable masked word coerces to deny, same as the other permissions.
	if !model.ValidMcpMaskedMode(c.McpMaskedMode) {
		slog.Warn("coercing connection row: unrecognised MCP masked mode", "scope", "storage/connections", "id", c.ID, "mode", c.McpMaskedMode)
		c.McpMaskedMode = "deny"
	}
	// P382: same fail-closed coercion; an unreadable value holds results for review.
	for mode, v := range map[string]*string{"read": &c.McpReview.Read, "write": &c.McpReview.Write, "ddl": &c.McpReview.Ddl, "masked": &c.McpReview.Masked} {
		if !model.ValidMcpReviewResults(*v) {
			slog.Warn("coercing connection row: unrecognised MCP review setting", "scope", "storage/connections", "id", c.ID, "mode", mode, "value", *v)
			*v = "review"
		}
	}
	// P529: an unreadable timeout falls back to the default, never to off.
	if !model.ValidMcpStatementTimeout(c.McpStatementTimeoutSec) {
		slog.Warn("coercing connection row: unrecognised MCP statement timeout", "scope", "storage/connections", "id", c.ID, "value", c.McpStatementTimeoutSec)
		c.McpStatementTimeoutSec = model.McpStatementTimeoutDefault
	}

	// P532: a list that cannot be read must not widen to all; it becomes the empty list.
	if mcpTables.Valid {
		c.McpTables = []string{}
		if err := json.Unmarshal([]byte(mcpTables.String), &c.McpTables); err != nil || c.McpTables == nil {
			slog.Warn("coercing connection row: unreadable MCP table list", "scope", "storage/connections", "id", c.ID)
			c.McpTables = []string{}
		}
	}

	c.MaskReviewOn = maskReview != 0
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
	c := kiraschema.Connections
	sb := r.Dialect.Select(connectionSelectColumns...).From(c.Table).OrderBy(c.SortOrder+" ASC", c.Name+" ASC")
	rows, err := sqlb.Query(r.DB, sb)
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
	sb := r.Dialect.Select(connectionSelectColumns...).From(kiraschema.Connections.Table)
	sb.Where(sb.Equal(kiraschema.Connections.ID, connID))
	query, args := sb.Build()
	c, err := sqlitex.QueryOne(r.DB, func(row *sql.Row) (*model.ConnectionSummary, error) {
		return scanConnectionRow(row)
	}, query, args...)
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
	c := kiraschema.Connections
	if err := sqlb.ReorderSortOrder(tx, r.Dialect, sqlb.SortSet{
		Table: c.Table, Key: c.ID, Order: c.SortOrder,
		Select: r.Dialect.Select(c.ID).From(c.Table).OrderBy(c.SortOrder+" ASC", c.Name+" ASC"),
	}, ids); err != nil {
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
