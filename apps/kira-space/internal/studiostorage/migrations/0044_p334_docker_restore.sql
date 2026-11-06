-- P334: last known running containers per Docker engine (Info.ID), kept across engine restarts.
CREATE TABLE docker_restore_engines (
  engine_id   TEXT PRIMARY KEY,
  context     TEXT NOT NULL,
  host        TEXT NOT NULL,
  boot_id     TEXT NOT NULL,
  boot_at     INTEGER NOT NULL,
  state       TEXT NOT NULL CHECK (state IN ('tracking', 'pending')),
  detected_at INTEGER,
  updated_at  INTEGER NOT NULL
);
CREATE TABLE docker_restore_containers (
  engine_id          TEXT NOT NULL REFERENCES docker_restore_engines(engine_id) ON DELETE CASCADE,
  container_id       TEXT NOT NULL,
  name               TEXT NOT NULL,
  image              TEXT NOT NULL,
  compose_project    TEXT NOT NULL DEFAULT '',
  compose_service    TEXT NOT NULL DEFAULT '',
  compose_depends_on TEXT NOT NULL DEFAULT '',
  restart_policy     TEXT NOT NULL DEFAULT '',
  added_at           INTEGER NOT NULL,
  stopped_seen_at    INTEGER,
  PRIMARY KEY (engine_id, container_id)
);
