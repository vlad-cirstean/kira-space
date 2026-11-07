package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// MaskReviewRepo reads connection_mask_reviewed, the projection of each connection file's
// mask_review.columns (P534).
type MaskReviewRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// ListForConnection returns the reviewed column entries of connectionID ordered by entry, never nil.
func (r *MaskReviewRepo) ListForConnection(connectionID string) ([]string, error) {
	c := kiraschema.ConnectionMaskReviewed
	sb := r.Dialect.Select(c.ColumnRef).From(c.Table)
	sb.Where(sb.Equal(c.ConnectionID, connectionID)).OrderBy(c.ColumnRef)
	rows, err := sqlb.Query(r.DB, sb)
	if err != nil {
		return nil, fmt.Errorf("repos: list reviewed columns: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("repos: scan reviewed column: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
