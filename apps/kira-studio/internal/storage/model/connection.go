package model

import "sort"

// ConnectionFields is the shape both ConnectionsRepo.Insert/Update accept and ConnectionSummary
// embeds — packages/shared/domain/connection.ts's connectionFieldsSchema, minus `password` (no
// password field anywhere in this package, enforced by the type itself, same as the TS build's
// `.omit({password: true})`).
type ConnectionFields struct {
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Color    string  `json:"color"`
	Mode     string  `json:"mode"`
	ReadOnly bool    `json:"readOnly"`
	Host     *string `json:"host"`
	Port     *int    `json:"port"`
	Database *string `json:"database"`
	Username *string `json:"username"`
	// URI is input-only (P181): the column holds ciphertext ConnectionsRepo never selects, so it is
	// always nil on output. SecretsRepo.GetURI and Service.Reveal are the only ways to read it.
	URI               *string        `json:"uri"`
	Options           map[string]any `json:"options"`
	Preconnect        *string        `json:"preconnect"`
	PreconnectSidecar bool           `json:"preconnectSidecar"`
	// P307 Part 2: seconds to wait after the script starts before the first connect attempt; 0 = off.
	PreconnectDelaySec int `json:"preconnectDelaySec"`
	// P18 (v1.1) D18: runs the connection's own EXPLAIN before every SELECT a console run issues
	// on it, and warns when the threshold or a structural issue fires (never blocks). A first-class
	// column, not an options_json key — see the migration's own comment for why.
	AutoExplain bool `json:"autoExplain"`
	// P28 §5.3: commands per second this connection is paced to; 0 = unlimited (default). A
	// first-class column, not an options_json key — see the migration's own comment for why.
	ThrottlePerSec float64 `json:"throttlePerSec"`
	// M1 §6.1: whether this connection is exposed to the local DB MCP server. Deny by default. A
	// first-class column, not an options_json key — see the migration's own comment for why.
	McpEnabled bool `json:"mcpEnabled"`
	// M2: free-text "what this DB is for", passed to an AI client as connection metadata
	// (list_connections). Never interpreted by this app.
	McpDescription string `json:"mcpDescription"`
	// M2: per-operation MCP permission, one of deny/allow/prompt. Enforced only on the DB MCP
	// server's run_query path — the human console is governed by ReadOnly, unchanged.
	McpReadMode  string `json:"mcpReadMode"`
	McpWriteMode string `json:"mcpWriteMode"`
	McpDdlMode   string `json:"mcpDdlMode"`
	// Runs this connection's own EXPLAIN before every explainable run_query on the DB MCP path,
	// and pauses the query for a human decision when the plan crosses the expensive-query row
	// threshold. Separate from AutoExplain, which governs this app's own console — see the
	// migration's own comment for why one switch cannot mean both.
	McpAutoExplain bool `json:"mcpAutoExplain"`
	// Result review on the DB MCP path: inherit (app setting), review (always hold), send (never).
	McpReviewResults string `json:"mcpReviewResults"`
	// McpStatementTimeoutSec bounds each statement an agent runs (P529): seconds, 0 off. A first-class
	// column, never options: a safety limit must not be settable through options or a URI.
	McpStatementTimeoutSec int `json:"mcpStatementTimeoutSec"`
	// P532: tables, views and collections MCP statements may touch, as tableref entries. nil = all,
	// including ones created later; non-nil (maybe empty) = exactly those. Not applicable to redis, kafka,
	// s3 or sqs.
	McpTables []string `json:"mcpTables"`
}

// ConnectionSummary mirrors packages/shared/domain/connection.ts's connectionSummarySchema.
// ConnectionFields is embedded (not nested) so its fields JSON-inline into this struct's own
// object, matching the TS schema's flat shape.
type ConnectionSummary struct {
	ID string `json:"id"`
	ConnectionFields
	// File is the name of the connection's config file in the connections folder.
	File      string `json:"file"`
	SortOrder int    `json:"sortOrder"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ConnectionState mirrors packages/shared/domain/connection.ts's connectionStateSchema. It is kept
// in-memory by the connections service (not read from a table) — listed here only as the shared
// wire type the repo's callers also use.
type ConnectionState struct {
	ConnectionID  string  `json:"connectionId"`
	Status        string  `json:"status"`
	ServerVersion *string `json:"serverVersion"`
	Error         *string `json:"error"`
	Since         int64   `json:"since"`
	Caps          any     `json:"caps"`
}

// connectionKinds mirrors connection.ts's connectionKindSchema — all v1 kinds.
var connectionKinds = map[string]bool{
	"postgres": true, "mariadb": true, "mysql": true, "sqlite": true, "clickhouse": true,
	"mongodb": true, "redis": true, "kafka": true, "sqs": true, "s3": true, "kibana": true,
}

// paletteColors mirrors domain/color.ts's paletteColorSchema — the whole storable set, not
// PALETTE_COLOR_CHOICES/CONNECTION_COLOR_CHOICES (the picker's offered subset). A row saved with
// a retired colour must keep parsing, listing and painting its own rail, so trimming this set to
// match the picker would silently corrupt existing rows. ValidConnectionColor below stays as a
// one-line alias so repos/connections.go and the connection bridge are untouched.
var paletteColors = map[string]bool{
	"none": true, "red": true, "orange": true, "amber": true, "olive": true, "green": true,
	"teal": true, "cyan": true, "blue": true, "indigo": true, "violet": true, "magenta": true,
	"grey": true,
}

// ConnectionKinds lists every connection kind, sorted.
func ConnectionKinds() []string { return sortedKeys(connectionKinds) }

func sortedKeys[K ~string](m map[K]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, string(k))
	}
	sort.Strings(out)
	return out
}

// ValidConnectionKind mirrors connection.ts's connectionKindSchema.
func ValidConnectionKind(v string) bool { return connectionKinds[v] }

// ValidPaletteColor mirrors domain/color.ts's paletteColorSchema (the full 13-colour set).
func ValidPaletteColor(v string) bool { return paletteColors[v] }

// ValidConnectionColor is ValidPaletteColor under its original name — kept so
// repos/connections.go and the connection bridge need no changes.
func ValidConnectionColor(v string) bool { return ValidPaletteColor(v) }

// ValidConnectionMode mirrors connection.ts's connectionModeSchema.
func ValidConnectionMode(v string) bool { return v == "fields" || v == "uri" }

// mcpPermissionModes mirrors connection.ts's mcpPermissionModeSchema.
var mcpPermissionModes = map[string]bool{"deny": true, "allow": true, "prompt": true}

// ValidMcpPermissionMode mirrors connection.ts's mcpPermissionModeSchema (M2).
func ValidMcpPermissionMode(v string) bool { return mcpPermissionModes[v] }

// Statement timeout bounds (P529); mirrors connection.ts's CONNECTION_MCP_STATEMENT_TIMEOUT.
const (
	McpStatementTimeoutDefault = 10
	McpStatementTimeoutMax     = 3600
)

// ValidMcpStatementTimeout reports whether v is 0 (off) or a whole number of seconds up to the max.
func ValidMcpStatementTimeout(v int) bool { return v >= 0 && v <= McpStatementTimeoutMax }

// mcpReviewResultsValues mirrors connection.ts's mcpReviewResultsSchema.
var mcpReviewResultsValues = map[string]bool{"inherit": true, "review": true, "send": true}

// ValidMcpReviewResults mirrors connection.ts's mcpReviewResultsSchema (P382).
func ValidMcpReviewResults(v string) bool { return mcpReviewResultsValues[v] }
