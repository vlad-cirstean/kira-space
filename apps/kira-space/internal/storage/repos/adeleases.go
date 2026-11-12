package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// AdeLeaseQueueRepo stores the resource lease queue names (ade_lease_queues) in display order.
type AdeLeaseQueueRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// List returns the queue names in position order.
func (r *AdeLeaseQueueRepo) List() ([]string, error) {
	c := kiraschema.AdeLeaseQueues
	rows, err := sqlb.Query(r.DB, r.Dialect.Select(c.Name).From(c.Table).OrderBy(c.Position))
	if err != nil {
		return nil, fmt.Errorf("repos: query ade lease queues: %w", err)
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("repos: scan ade lease queue: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos: ade lease queues rows: %w", err)
	}
	return out, nil
}

// Save replaces every row with names, in order, in one transaction.
func (r *AdeLeaseQueueRepo) Save(names []string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin save ade lease queues: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	c := kiraschema.AdeLeaseQueues
	if _, err := sqlb.Exec(tx, r.Dialect.DeleteFrom(c.Table)); err != nil {
		return fmt.Errorf("repos: clear ade lease queues: %w", err)
	}
	for i, n := range names {
		ib := r.Dialect.InsertInto(c.Table).Cols(c.Name, c.Position).Values(n, i)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("repos: write ade lease queue %s: %w", n, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit save ade lease queues: %w", err)
	}
	return nil
}
