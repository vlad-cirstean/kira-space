package kubernetes

import "time"

// ClusterRecord is one stored cluster row: a stable id for a (context, server) pair.
type ClusterRecord struct {
	ID         string
	Context    string
	Server     string
	CreatedAt  time.Time
	LastSeenAt time.Time
	Mcp        McpSettings
	SortOrder  *int // nil until the user first reorders
}

// ClusterStore persists cluster identities. The Studio repos package implements it, so this
// package never imports storage.
type ClusterStore interface {
	// Ensure returns the row for (context, server), creating it on first sight and bumping last_seen_at.
	Ensure(context, server string, now time.Time) (ClusterRecord, error)
	Get(id string) (ClusterRecord, bool, error)
	// SetMcp stores a cluster's MCP settings; false when no row has the id.
	SetMcp(id string, m McpSettings) (bool, error)
	// Reorder places ids first, in order; rows left out follow in their existing order. Every row ends with a dense sort_order.
	Reorder(ids []string) error
}
