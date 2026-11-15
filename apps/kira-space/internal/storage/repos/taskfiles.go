package repos

import (
	"database/sql"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// TaskFilesRepo stores the index of each task's files (task_files), the write intents and the
// migration markers.
type TaskFilesRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

var taskFileCols = []string{
	kiraschema.TaskFiles.TaskID, kiraschema.TaskFiles.RelPath, kiraschema.TaskFiles.Kind, kiraschema.TaskFiles.Size,
	kiraschema.TaskFiles.Version, kiraschema.TaskFiles.Sha256, kiraschema.TaskFiles.AcceptedSha256,
	kiraschema.TaskFiles.AcceptedBy, kiraschema.TaskFiles.AcceptedAt, kiraschema.TaskFiles.Review, kiraschema.TaskFiles.Problem,
}

func scanTaskFile(row rowScanner) (model.TaskFile, error) {
	var f model.TaskFile
	err := row.Scan(&f.TaskID, &f.RelPath, &f.Kind, &f.Size, &f.Version, &f.SHA256, &f.AcceptedSHA256,
		&f.AcceptedBy, &f.AcceptedAt, &f.Review, &f.Problem)
	return f, err
}

func (r *TaskFilesRepo) selectFiles() *sqlbuilder.SelectBuilder {
	return r.Dialect.Select(taskFileCols...).From(kiraschema.TaskFiles.Table)
}

// Files lists the task's index rows in path order.
func (r *TaskFilesRepo) Files(taskID string) ([]model.TaskFile, error) {
	c := kiraschema.TaskFiles
	sb := r.selectFiles()
	sb.Where(sb.Equal(c.TaskID, taskID)).OrderBy(c.RelPath)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TaskFile, bool, error) {
		f, err := scanTaskFile(rows)
		return f, true, err
	})
}

// File returns the row, (nil, nil) when the path has none.
func (r *TaskFilesRepo) File(taskID, relPath string) (*model.TaskFile, error) {
	return taskFileOne(r.DB, r.selectFiles(), taskID, relPath)
}

func taskFileOne(q sqlb.Querier, sb *sqlbuilder.SelectBuilder, taskID, relPath string) (*model.TaskFile, error) {
	c := kiraschema.TaskFiles
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.RelPath, relPath))
	query, args := sb.Build()
	return sqlitex.QueryOne(q, func(row *sql.Row) (*model.TaskFile, error) {
		f, err := scanTaskFile(row)
		return &f, err
	}, query, args...)
}

// Pending lists the task's rows waiting on the user or invalid, in path order.
func (r *TaskFilesRepo) Pending(taskID string) ([]model.TaskFile, error) {
	c := kiraschema.TaskFiles
	sb := r.selectFiles()
	sb.Where(sb.Equal(c.TaskID, taskID), sb.NotEqual(c.Review, model.TaskFileAccepted)).OrderBy(c.RelPath)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TaskFile, bool, error) {
		f, err := scanTaskFile(rows)
		return f, true, err
	})
}

// Intents lists every unfinished write intent, oldest first.
func (r *TaskFilesRepo) Intents() ([]model.TaskFileIntent, error) {
	c := kiraschema.TaskFileIntents
	sb := r.Dialect.Select(c.TaskID, c.RelPath, c.Sha256, c.Writer, c.CreatedAt).From(c.Table).OrderBy(c.CreatedAt, c.TaskID, c.RelPath)
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TaskFileIntent, bool, error) {
		var in model.TaskFileIntent
		err := rows.Scan(&in.TaskID, &in.RelPath, &in.SHA256, &in.Writer, &in.CreatedAt)
		return in, true, err
	})
}

// Migration returns the marker of one task part, (nil, nil) when none exists.
func (r *TaskFilesRepo) Migration(taskID, part string) (*model.TaskFilesMigration, error) {
	c := kiraschema.TaskFilesMigration
	sb := r.Dialect.Select(c.TaskID, c.Part, c.State, c.DoneAt, c.VerifyJSON).From(c.Table)
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Equal(c.Part, part))
	return adeOne(r.DB, sb, func(row *sql.Row) (*model.TaskFilesMigration, error) {
		var m model.TaskFilesMigration
		err := row.Scan(&m.TaskID, &m.Part, &m.State, &m.DoneAt, &m.VerifyJSON)
		return &m, err
	})
}

// SetMigration stores the marker, replacing the task part's previous one.
func (r *TaskFilesRepo) SetMigration(m model.TaskFilesMigration) error {
	c := kiraschema.TaskFilesMigration
	ib := r.Dialect.InsertInto(c.Table).Cols(c.TaskID, c.Part, c.State, c.DoneAt, c.VerifyJSON).
		Values(m.TaskID, m.Part, m.State, m.DoneAt, m.VerifyJSON)
	sqlb.UpsertSet(ib, []string{c.TaskID, c.Part}, []string{
		c.State + " = excluded." + c.State, c.DoneAt + " = excluded." + c.DoneAt, c.VerifyJSON + " = excluded." + c.VerifyJSON,
	}, "")
	if _, err := sqlb.Exec(r.DB, ib); err != nil {
		return fmt.Errorf("repos: set task files migration: %w", err)
	}
	return nil
}

// TaskFilesTx is one transaction over the index, with the raw handle for a content cache write that
// must commit with it.
type TaskFilesTx struct {
	tx      *sql.Tx
	dialect sqlb.Dialect
}

// Querier is the transaction itself, for cache writes that commit or roll back with the index.
func (t *TaskFilesTx) Querier() sqlb.Querier { return t.tx }

// File returns the row, (nil, nil) when the path has none.
func (t *TaskFilesTx) File(taskID, relPath string) (*model.TaskFile, error) {
	return taskFileOne(t.tx, t.dialect.Select(taskFileCols...).From(kiraschema.TaskFiles.Table), taskID, relPath)
}

// FilesUnder lists the rows whose path starts with prefix, in path order. The prefix must hold no LIKE wildcard.
func (t *TaskFilesTx) FilesUnder(taskID, prefix string) ([]model.TaskFile, error) {
	c := kiraschema.TaskFiles
	sb := t.dialect.Select(taskFileCols...).From(c.Table)
	sb.Where(sb.Equal(c.TaskID, taskID), sb.Like(c.RelPath, prefix+"%")).OrderBy(c.RelPath)
	rows, err := sqlb.Query(t.tx, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.TaskFile, bool, error) {
		f, err := scanTaskFile(rows)
		return f, true, err
	})
}

// PutFile inserts or replaces the row.
func (t *TaskFilesTx) PutFile(f model.TaskFile) error {
	c := kiraschema.TaskFiles
	ib := t.dialect.InsertInto(c.Table).Cols(taskFileCols...).
		Values(f.TaskID, f.RelPath, f.Kind, f.Size, f.Version, f.SHA256, f.AcceptedSHA256, f.AcceptedBy, f.AcceptedAt, f.Review, f.Problem)
	sets := make([]string, 0, len(taskFileCols)-2)
	for _, col := range taskFileCols[2:] {
		sets = append(sets, col+" = excluded."+col)
	}
	sqlb.UpsertSet(ib, []string{c.TaskID, c.RelPath}, sets, "")
	if _, err := sqlb.Exec(t.tx, ib); err != nil {
		return fmt.Errorf("repos: put task file: %w", err)
	}
	return nil
}

// DeleteFile removes the row; a missing one is not an error.
func (t *TaskFilesTx) DeleteFile(taskID, relPath string) error {
	c := kiraschema.TaskFiles
	db := t.dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.TaskID, taskID), db.Equal(c.RelPath, relPath))
	if _, err := sqlb.Exec(t.tx, db); err != nil {
		return fmt.Errorf("repos: delete task file: %w", err)
	}
	return nil
}

// PutIntent records a write about to happen; a second intent for the path replaces the first.
func (t *TaskFilesTx) PutIntent(in model.TaskFileIntent) error {
	c := kiraschema.TaskFileIntents
	ib := t.dialect.InsertInto(c.Table).Cols(c.TaskID, c.RelPath, c.Sha256, c.Writer, c.CreatedAt).
		Values(in.TaskID, in.RelPath, in.SHA256, in.Writer, in.CreatedAt)
	sqlb.UpsertSet(ib, []string{c.TaskID, c.RelPath}, []string{
		c.Sha256 + " = excluded." + c.Sha256, c.Writer + " = excluded." + c.Writer, c.CreatedAt + " = excluded." + c.CreatedAt,
	}, "")
	if _, err := sqlb.Exec(t.tx, ib); err != nil {
		return fmt.Errorf("repos: put task file intent: %w", err)
	}
	return nil
}

// DeleteIntent removes the intent of a path; a missing one is not an error.
func (t *TaskFilesTx) DeleteIntent(taskID, relPath string) error {
	c := kiraschema.TaskFileIntents
	db := t.dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.TaskID, taskID), db.Equal(c.RelPath, relPath))
	if _, err := sqlb.Exec(t.tx, db); err != nil {
		return fmt.Errorf("repos: delete task file intent: %w", err)
	}
	return nil
}

// InTx runs fn in one transaction and commits when it returns nil.
func (r *TaskFilesRepo) InTx(fn func(*TaskFilesTx) error) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("repos: begin task files tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if err := fn(&TaskFilesTx{tx: tx, dialect: r.Dialect}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("repos: commit task files tx: %w", err)
	}
	return nil
}
