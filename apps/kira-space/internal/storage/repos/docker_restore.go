package repos

import (
	"database/sql"
	"errors"

	"github.com/kirathecat/kira-studio/internal/docker"
)

// DockerRestoreRepo reads and writes docker_restore_engines / docker_restore_containers (P334).
type DockerRestoreRepo struct {
	DB *sql.DB
}

var _ docker.RestoreStore = (*DockerRestoreRepo)(nil)

func (r *DockerRestoreRepo) LoadRestore(engineID string) (docker.RestoreSnapshot, bool, error) {
	s := docker.RestoreSnapshot{EngineID: engineID, Entries: []docker.RestoreEntry{}}
	var detected sql.NullInt64
	err := r.DB.QueryRow(
		`SELECT context, host, boot_id, boot_at, state, detected_at, updated_at FROM docker_restore_engines WHERE engine_id = ?`,
		engineID,
	).Scan(&s.Context, &s.Host, &s.BootID, &s.BootAt, &s.State, &detected, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return docker.RestoreSnapshot{}, false, nil
	}
	if err != nil {
		return docker.RestoreSnapshot{}, false, err
	}
	s.DetectedAt = detected.Int64
	rows, err := r.DB.Query(
		`SELECT container_id, name, image, compose_project, compose_service, compose_depends_on, restart_policy, added_at, stopped_seen_at
		 FROM docker_restore_containers WHERE engine_id = ? ORDER BY name, container_id`, engineID)
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
	if _, err := tx.Exec(
		`INSERT INTO docker_restore_engines (engine_id, context, host, boot_id, boot_at, state, detected_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(engine_id) DO UPDATE SET context = excluded.context, host = excluded.host, boot_id = excluded.boot_id,
		   boot_at = excluded.boot_at, state = excluded.state, detected_at = excluded.detected_at, updated_at = excluded.updated_at`,
		s.EngineID, s.Context, s.Host, s.BootID, s.BootAt, s.State, detected, s.UpdatedAt,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM docker_restore_containers WHERE engine_id = ?`, s.EngineID); err != nil {
		return err
	}
	for _, e := range s.Entries {
		var stopped any
		if e.StoppedSeenAt != 0 {
			stopped = e.StoppedSeenAt
		}
		if _, err := tx.Exec(
			`INSERT INTO docker_restore_containers
			 (engine_id, container_id, name, image, compose_project, compose_service, compose_depends_on, restart_policy, added_at, stopped_seen_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.EngineID, e.ID, e.Name, e.Image, e.ComposeProject, e.ComposeService, e.ComposeDependsOn, e.RestartPolicy, e.AddedAt, stopped,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *DockerRestoreRepo) PruneRestore(updatedBefore int64) error {
	_, err := r.DB.Exec(`DELETE FROM docker_restore_engines WHERE updated_at < ?`, updatedBefore)
	return err
}
