package repos

import (
	"database/sql"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// AdeReviewWindow is one row of `ade_review_windows`: an ephemeral review window and what it reviews.
type AdeReviewWindow struct {
	WindowKey string
	TaskID    string
	BranchID  string
	CreatedAt int64
}

// AdeReviewWindowsRepo reads and writes `ade_review_windows`. Its window rows live in `windows`,
// which cascades the delete.
type AdeReviewWindowsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

func (r *AdeReviewWindowsRepo) selectWindow() *sqlbuilder.SelectBuilder {
	c := kiraschema.AdeReviewWindows
	return r.Dialect.Select(c.WindowKey, c.TaskID, c.BranchID, c.CreatedAt).From(c.Table)
}

// Insert adds the row; the caller created the `windows` row first.
func (r *AdeReviewWindowsRepo) Insert(w AdeReviewWindow) error {
	c := kiraschema.AdeReviewWindows
	ib := r.Dialect.InsertInto(c.Table).Cols(c.WindowKey, c.TaskID, c.BranchID, c.CreatedAt).
		Values(w.WindowKey, w.TaskID, w.BranchID, w.CreatedAt)
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: insert review window %s: %w", w.WindowKey, err)
	}
	return nil
}

func scanReviewWindow(row rowScanner) (AdeReviewWindow, error) {
	var w AdeReviewWindow
	err := row.Scan(&w.WindowKey, &w.TaskID, &w.BranchID, &w.CreatedAt)
	return w, err
}

// ByBranch returns the branch's review window, (nil, nil) when none is open.
func (r *AdeReviewWindowsRepo) ByBranch(branchID string) (*AdeReviewWindow, error) {
	sb := r.selectWindow()
	sb.Where(sb.Equal(kiraschema.AdeReviewWindows.BranchID, branchID))
	return adeOne(r.DB, sb, func(row *sql.Row) (*AdeReviewWindow, error) {
		w, err := scanReviewWindow(row)
		return &w, err
	})
}

// ByKey returns the review window with this key, (nil, nil) when it is not one.
func (r *AdeReviewWindowsRepo) ByKey(key string) (*AdeReviewWindow, error) {
	sb := r.selectWindow()
	sb.Where(sb.Equal(kiraschema.AdeReviewWindows.WindowKey, key))
	return adeOne(r.DB, sb, func(row *sql.Row) (*AdeReviewWindow, error) {
		w, err := scanReviewWindow(row)
		return &w, err
	})
}

// KeysByTask lists the task's review window keys.
func (r *AdeReviewWindowsRepo) KeysByTask(taskID string) ([]string, error) {
	c := kiraschema.AdeReviewWindows
	sb := r.Dialect.Select(c.WindowKey).From(c.Table)
	sb.Where(sb.Equal(c.TaskID, taskID))
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (string, bool, error) {
		var k string
		err := rows.Scan(&k)
		return k, true, err
	})
}

// IsReviewKey reports whether key names a review window.
func (r *AdeReviewWindowsRepo) IsReviewKey(key string) (bool, error) {
	w, err := r.ByKey(key)
	return w != nil, err
}

// PurgeAll deletes every review window's `windows` row (cascading its review row): review windows
// are never restored on relaunch.
func (r *AdeReviewWindowsRepo) PurgeAll() error {
	c, w := kiraschema.AdeReviewWindows, kiraschema.Windows
	keys := r.Dialect.Select(c.WindowKey).From(c.Table)
	del := r.Dialect.DeleteFrom(w.Table)
	del.Where(del.In(w.Key, keys))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos: purge review windows: %w", err)
	}
	return nil
}

// DeleteByTask drops the task's review rows without touching `windows`.
func (r *AdeReviewWindowsRepo) DeleteByTask(taskID string) error {
	c := kiraschema.AdeReviewWindows
	del := r.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.TaskID, taskID))
	if _, err := sqlb.Exec(r.DB, del); err != nil {
		return fmt.Errorf("repos: delete review windows of task %s: %w", taskID, err)
	}
	return nil
}
