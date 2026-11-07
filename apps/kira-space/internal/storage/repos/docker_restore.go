package repos

import (
	"database/sql"
	"errors"

	"github.com/kirathecat/kira-studio/internal/docker"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// DockerRestoreRepo reads and writes docker_restore_engines / docker_restore_containers (P334).
type DockerRestoreRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

var _ docker.RestoreStore = (*DockerRestoreRepo)(nil)

func (r *DockerRestoreRepo) LoadRestore(engineID string) (docker.RestoreSnapshot, bool, error) {
	s := docker.RestoreSnapshot{EngineID: engineID, Entries: []docker.RestoreEntry{}}
	var detected sql.NullInt64
	e := kiraschema.DockerRestoreEngines
	eb := r.Dialect.Select(e.Context, e.Host, e.BootID, e.BootAt, e.State, e.DetectedAt, e.UpdatedAt).From(e.Table)
	eb.Where(eb.Equal(e.EngineID, engineID))
	err := sqlb.QueryRow(r.DB, eb).Scan(&s.Context, &s.Host, &s.BootID, &s.BootAt, &s.State, &detected, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return docker.RestoreSnapshot{}, false, nil
	}
	if err != nil {
		return docker.RestoreSnapshot{}, false, err
	}
	s.DetectedAt = detected.Int64
	k := kiraschema.DockerRestoreContainers
	kb := r.Dialect.Select(k.ContainerID, k.Name, k.Image, k.ComposeProject, k.ComposeService, k.ComposeDependsOn, k.RestartPolicy, k.AddedAt, k.StoppedSeenAt).
		From(k.Table)
	kb.Where(kb.Equal(k.EngineID, engineID)).OrderBy(k.Name, k.ContainerID)
	rows, err := sqlb.Query(r.DB, kb)
	if err != nil {
		return docker.RestoreSnapshot{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			e       docker.RestoreEntry
			stopped sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &e.Name, &e.Image, &e.ComposeProject, &e.ComposeService, &e.ComposeDependsOn, &e.RestartPolicy, &e.AddedAt, &stopped); err != nil {
			return docker.RestoreSnapshot{}, false, err
		}
		e.StoppedSeenAt = stopped.Int64
		s.Entries = append(s.Entries, e)
	}
	return s, true, rows.Err()
}

func (r *DockerRestoreRepo) SaveRestore(s docker.RestoreSnapshot) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var detected any
	if s.DetectedAt != 0 {
		detected = s.DetectedAt
	}
	e := kiraschema.DockerRestoreEngines
	ib := r.Dialect.InsertInto(e.Table).Cols(e.EngineID, e.Context, e.Host, e.BootID, e.BootAt, e.State, e.DetectedAt, e.UpdatedAt).
		Values(s.EngineID, s.Context, s.Host, s.BootID, s.BootAt, s.State, detected, s.UpdatedAt)
	sqlb.Upsert(ib, []string{e.EngineID}, []string{e.Context, e.Host, e.BootID, e.BootAt, e.State, e.DetectedAt, e.UpdatedAt}, "")
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return err
	}
	k := kiraschema.DockerRestoreContainers
	del := r.Dialect.DeleteFrom(k.Table)
	del.Where(del.Equal(k.EngineID, s.EngineID))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return err
	}
	for _, e := range s.Entries {
		var stopped any
		if e.StoppedSeenAt != 0 {
			stopped = e.StoppedSeenAt
		}
		cb := r.Dialect.InsertInto(k.Table).
			Cols(k.EngineID, k.ContainerID, k.Name, k.Image, k.ComposeProject, k.ComposeService, k.ComposeDependsOn, k.RestartPolicy, k.AddedAt, k.StoppedSeenAt).
			Values(s.EngineID, e.ID, e.Name, e.Image, e.ComposeProject, e.ComposeService, e.ComposeDependsOn, e.RestartPolicy, e.AddedAt, stopped)
		if _, err := sqlb.Exec(tx, cb); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *DockerRestoreRepo) PruneRestore(updatedBefore int64) error {
	e := kiraschema.DockerRestoreEngines
	db := r.Dialect.DeleteFrom(e.Table)
	db.Where(db.LessThan(e.UpdatedAt, updatedBefore))
	_, err := sqlb.Exec(r.DB, db)
	return err
}
