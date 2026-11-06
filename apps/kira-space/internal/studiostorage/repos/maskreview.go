package repos

import (
	"database/sql"
	"fmt"
)

// MaskReviewRepo reads connection_mask_reviewed, the projection of each connection file's
// mask_review.columns (P534).
type MaskReviewRepo struct {
	DB *sql.DB
}

// ListForConnection returns the reviewed column entries of connectionID ordered by entry, never nil.
func (r *MaskReviewRepo) ListForConnection(connectionID string) ([]string, error) {
	rows, err := r.DB.Query(`SELECT column_ref FROM connection_mask_reviewed WHERE connection_id = ? ORDER BY column_ref`, connectionID)
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
