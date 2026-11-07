package repos

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// FileHandlerPreviousRepo reads and writes `file_handler_previous`.
type FileHandlerPreviousRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Get returns the saved handler of ext; ok is false when none is saved.
func (r *FileHandlerPreviousRepo) Get(ext string) (appPath string, ok bool, err error) {
	c := kiraschema.FileHandlerPrevious
	sb := r.Dialect.Select(c.AppPath).From(c.Table)
	sb.Where(sb.Equal(c.Ext, ext))
	err = sqlb.QueryRow(r.DB, sb).Scan(&appPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("repos: get file handler %s: %w", ext, err)
	}
	return appPath, true, nil
}

// Put stores the handler, replacing any row for the same extension.
func (r *FileHandlerPreviousRepo) Put(ext, appPath, bundleID string) error {
	c := kiraschema.FileHandlerPrevious
	ib := r.Dialect.InsertInto(c.Table).Cols(c.Ext, c.AppPath, c.BundleID, c.SavedAt).
		Values(ext, appPath, bundleID, time.Now().UnixMilli())
	sqlb.Upsert(ib, []string{c.Ext}, []string{c.AppPath, c.BundleID, c.SavedAt}, "")
	_, err := sqlb.Exec(r.DB, ib)
	if err != nil {
		return fmt.Errorf("repos: put file handler %s: %w", ext, err)
	}
	return nil
}

// Delete drops the row of ext; a missing row is not an error.
func (r *FileHandlerPreviousRepo) Delete(ext string) error {
	c := kiraschema.FileHandlerPrevious
	db := r.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.Ext, ext))
	if _, err := sqlb.Exec(r.DB, db); err != nil {
		return fmt.Errorf("repos: delete file handler %s: %w", ext, err)
	}
	return nil
}
