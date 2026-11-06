package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/kubernetes"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// KubeClustersRepo reads and writes kube_clusters (P439).
type KubeClustersRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

var _ kubernetes.ClusterStore = (*KubeClustersRepo)(nil)

var kubeClusterCols = []string{
	kiraschema.KubeClusters.ID, kiraschema.KubeClusters.Context, kiraschema.KubeClusters.Server,
	kiraschema.KubeClusters.CreatedAt, kiraschema.KubeClusters.LastSeenAt,
	kiraschema.KubeClusters.MCPDescription, kiraschema.KubeClusters.MCPDiscoverMode, kiraschema.KubeClusters.MCPListMode,
	kiraschema.KubeClusters.MCPDescribeMode, kiraschema.KubeClusters.MCPLogsMode, kiraschema.KubeClusters.SortOrder,
}

func (r *KubeClustersRepo) Ensure(context, server string, now time.Time) (kubernetes.ClusterRecord, error) {
	ts := now.UnixMilli()
	c := kiraschema.KubeClusters
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.Context, c.Server, c.CreatedAt, c.LastSeenAt).
		Values(ids.NewID(), context, server, ts, ts)
	sqlb.Upsert(ib, []string{c.Context, c.Server}, []string{c.LastSeenAt}, "")
	ib.Returning(kubeClusterCols...)
	return scanKubeCluster(sqlb.QueryRow(r.DB, ib))
}

func (r *KubeClustersRepo) Get(id string) (kubernetes.ClusterRecord, bool, error) {
	c := kiraschema.KubeClusters
	sb := r.Dialect.Select(kubeClusterCols...).From(c.Table)
	sb.Where(sb.Equal(c.ID, id))
	rec, err := scanKubeCluster(sqlb.QueryRow(r.DB, sb))
	if errors.Is(err, sql.ErrNoRows) {
		return kubernetes.ClusterRecord{}, false, nil
	}
	return rec, err == nil, err
}

func (r *KubeClustersRepo) SetMcp(id string, m kubernetes.McpSettings) (bool, error) {
	c := kiraschema.KubeClusters
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.MCPDescription, m.Description), ub.Assign(c.MCPDiscoverMode, m.Discover),
		ub.Assign(c.MCPListMode, m.List), ub.Assign(c.MCPDescribeMode, m.Describe), ub.Assign(c.MCPLogsMode, m.Logs)).
		Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Reorder applies a user's drag order and rewrites sort_order dense.
func (r *KubeClustersRepo) Reorder(ids []string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos/kube_clusters: reorder: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	c := kiraschema.KubeClusters
	sel := r.Dialect.Select(c.ID).From(c.Table).OrderBy(c.SortOrder+" IS NULL", c.SortOrder, c.Context)
	set := sqlb.SortSet{Table: c.Table, Key: c.ID, Order: c.SortOrder, Select: sel}
	if err := sqlb.ReorderSortOrder(tx, r.Dialect, set, ids); err != nil {
		return fmt.Errorf("repos/kube_clusters: reorder: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos/kube_clusters: reorder: commit: %w", err)
	}
	return nil
}

func scanKubeCluster(row *sql.Row) (kubernetes.ClusterRecord, error) {
	var (
		rec           kubernetes.ClusterRecord
		created, seen int64
		order         sql.NullInt64
	)
	if err := row.Scan(&rec.ID, &rec.Context, &rec.Server, &created, &seen,
		&rec.Mcp.Description, &rec.Mcp.Discover, &rec.Mcp.List, &rec.Mcp.Describe, &rec.Mcp.Logs, &order); err != nil {
		return kubernetes.ClusterRecord{}, err
	}
	rec.CreatedAt, rec.LastSeenAt = time.UnixMilli(created), time.UnixMilli(seen)
	if order.Valid {
		n := int(order.Int64)
		rec.SortOrder = &n
	}
	return rec, nil
}
