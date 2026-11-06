// Package dbfold folds a Kira Studio database (<home>/studio.db) into the Kira Space database once (P544).
//
// Run is idempotent and resumable: a marker row in the Space settings records progress, both databases
// are backed up before anything is written, and every step checks its own end state. The legacy
// migrations bring an old studio.db to its last schema before its rows are imported in one transaction.
package dbfold

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbfold/legacy"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/homefold"
	"github.com/kirathecat/kira-studio/internal/appstorage"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

const markerKey = "dbfold.p544"

const (
	stateImporting = "importing"
	stateDone      = "done"
)

// Options names the Space database and home. Ephemeral reports window keys (host views, captures,
// opened files, proofs) that never carry user tabs. Step and Now are test seams.
type Options struct {
	Home      string
	Space     *sql.DB
	Ephemeral func(key string) bool
	Step      func(name string) error // nil in production; returns an error to interrupt
	Now       func() time.Time
}

type marker struct {
	State    string `json:"state"`
	Fresh    bool   `json:"fresh"`
	Backup   string `json:"backup,omitempty"`
	Imported bool   `json:"imported,omitempty"`
}

func (o *Options) step(name string) error {
	if o.Step == nil {
		return nil
	}
	return o.Step(name)
}

func (o *Options) now() time.Time {
	if o.Now == nil {
		return time.Now()
	}
	return o.Now()
}

// Run folds studio.db into the Space database. A failed run keeps its marker; the next run resumes.
func Run(o Options) error {
	m, found, err := readMarker(o.Space)
	if err != nil {
		return err
	}
	if found && m.State == stateDone {
		return nil
	}
	path := config.ModuleDbPathAt(o.Home)
	exists := fileExists(path)
	if !found {
		if !exists {
			if err := o.foldRunsOnly(); err != nil {
				return err
			}
			return writeMarker(o.Space, marker{State: stateDone, Fresh: true})
		}
		m = marker{
			State:  stateImporting,
			Backup: filepath.Join(o.Home, "backups", "p544-"+o.now().UTC().Format("20060102T150405Z")),
		}
		if err := writeMarker(o.Space, m); err != nil {
			return err
		}
	}
	if exists && !m.Imported {
		if err := o.importStudio(path, &m); err != nil {
			return err
		}
	}
	if !exists {
		if err := o.foldRunsOnly(); err != nil {
			return err
		}
	}
	if m.Imported {
		if err := fixLegacyConnections(o.Space); err != nil {
			return err
		}
	}
	if exists {
		if err := o.step("remove"); err != nil {
			return err
		}
		if err := removeDB(path); err != nil {
			return err
		}
	} else if !m.Imported {
		slog.Warn("dbfold: studio.db is gone before its import finished", "scope", "dbfold")
	}
	if err := o.step("done"); err != nil {
		return err
	}
	m.State = stateDone
	if err := writeMarker(o.Space, m); err != nil {
		return err
	}
	slog.Info("database fold done", "scope", "startup", "backup", m.Backup)
	return nil
}

// foldRunsOnly finishes the home fold's runs step when there is no studio.db to read rows from;
// otherwise the legacy run files and automation folders would never reach Space.
func (o *Options) foldRunsOnly() error {
	return homefold.Runs(homefold.Options{Space: o.Space, SpaceHome: o.Home})
}

func (o *Options) importStudio(path string, m *marker) error {
	if err := os.MkdirAll(m.Backup, 0o700); err != nil {
		return fmt.Errorf("dbfold: backup dir: %w", err)
	}
	if err := o.step("backup"); err != nil {
		return err
	}
	if err := backup(o.Space, path, m.Backup); err != nil {
		return err
	}
	if err := o.step("upgrade"); err != nil {
		return err
	}
	steps, err := legacy.All()
	if err != nil {
		return fmt.Errorf("dbfold: load legacy migrations: %w", err)
	}
	// A *sqlitex.SchemaTooNewError reaches the caller as-is (startupfail.Classify).
	studio, err := appstorage.OpenAt(path, steps)
	if err != nil {
		return err
	}
	defer func() { _ = studio.Close() }()
	if err := o.step("runs"); err != nil {
		return err
	}
	if err := homefold.Runs(homefold.Options{Space: o.Space, SpaceHome: o.Home, Studio: studio}); err != nil {
		return err
	}
	if err := o.step("import"); err != nil {
		return err
	}
	if err := importRows(o, path, m); err != nil {
		return fmt.Errorf("dbfold: import: %w", err)
	}
	m.Imported = true
	return studio.Close()
}

// backup writes a VACUUM INTO copy of the Space database and of the not yet migrated studio.db; a
// copy already on disk (an interrupted run) is kept.
func backup(space *sql.DB, studioPath, dir string) error {
	studio, err := sqlitex.Open(studioPath)
	if err != nil {
		return fmt.Errorf("dbfold: open studio.db for backup: %w", err)
	}
	defer func() { _ = studio.Close() }()
	for name, db := range map[string]*sql.DB{"space.db": space, "studio.db": studio} {
		dst := filepath.Join(dir, name)
		if fileExists(dst) {
			continue
		}
		tmp := dst + ".tmp"
		if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("dbfold: backup %s: %w", name, err)
		}
		if _, err := db.Exec(`VACUUM INTO ?`, tmp); err != nil {
			return fmt.Errorf("dbfold: backup %s: %w", name, err)
		}
		if err := os.Rename(tmp, dst); err != nil {
			return fmt.Errorf("dbfold: backup %s: %w", name, err)
		}
	}
	return nil
}

// fixLegacyConnections drops the superseded connection columns and tables when the imported database had
// already exported its connections to files; otherwise connfiles exports them on its first load.
func fixLegacyConnections(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'config.migrated.connections'`).Scan(&n); err != nil {
		return fmt.Errorf("dbfold: read connections marker: %w", err)
	}
	if n == 0 {
		return nil
	}
	return connfiles.DropLegacy(db)
}

func removeDB(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("dbfold: remove %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func readMarker(db *sql.DB) (m marker, found bool, err error) {
	var raw string
	switch err = db.QueryRow(`SELECT value FROM settings WHERE key = ?`, markerKey).Scan(&raw); {
	case errors.Is(err, sql.ErrNoRows):
		return marker{}, false, nil
	case err != nil:
		return marker{}, false, fmt.Errorf("dbfold: read marker: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return marker{}, false, fmt.Errorf("dbfold: marker: %w", err)
	}
	return m, true, nil
}

func writeMarker(db interface {
	Exec(query string, args ...any) (sql.Result, error)
}, m marker) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, markerKey, string(raw)); err != nil {
		return fmt.Errorf("dbfold: write marker: %w", err)
	}
	return nil
}

// attachURI is path as a file: URI; the connection runs with URI parsing on.
func attachURI(path string) string {
	path = strings.ReplaceAll(path, "%", "%25")
	path = strings.ReplaceAll(path, "?", "%3F")
	path = strings.ReplaceAll(path, "#", "%23")
	return "file:" + path
}

// tx runs on the one pinned connection of the Space pool, so nothing else may use o.Space meanwhile.
type tx struct {
	ctx  context.Context
	conn *sql.Conn
}

func (t tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.conn.ExecContext(t.ctx, query, args...)
}

func (t tx) queryRow(query string, args ...any) *sql.Row {
	return t.conn.QueryRowContext(t.ctx, query, args...)
}

func importRows(o *Options, path string, m *marker) (err error) {
	ctx := context.Background()
	conn, err := o.Space.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	t := tx{ctx: ctx, conn: conn}
	if _, err := t.Exec(`ATTACH DATABASE ? AS studio`, attachURI(path)); err != nil {
		return fmt.Errorf("attach studio.db: %w", err)
	}
	defer func() {
		if _, derr := t.Exec(`DETACH DATABASE studio`); derr != nil && err == nil {
			err = fmt.Errorf("detach studio.db: %w", derr)
		}
	}()
	if _, err := t.Exec(`BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = t.Exec(`ROLLBACK`)
		}
	}()
	// Every parent row is in before the commit; deferring keeps the table order free.
	if _, err := t.Exec(`PRAGMA defer_foreign_keys = ON`); err != nil {
		return err
	}
	freshSettings, err := spaceSettingsFresh(t)
	if err != nil {
		return err
	}
	if err := copyModuleTables(t); err != nil {
		return err
	}
	if err := copySettings(t, freshSettings); err != nil {
		return err
	}
	if err := copyOps(t); err != nil {
		return err
	}
	if err := copyKeyed(t); err != nil {
		return err
	}
	if err := copyLayout(t); err != nil {
		return err
	}
	if err := copyTabs(t, o.Ephemeral); err != nil {
		return fmt.Errorf("tabs: %w", err)
	}
	m.Imported = true
	if err := writeMarker(t, *m); err != nil {
		m.Imported = false
		return err
	}
	if _, err := t.Exec(`COMMIT`); err != nil {
		m.Imported = false
		return err
	}
	committed = true
	return nil
}

// moduleTable is one table the Studio schema adds, in parent-first order.
type moduleTable struct{ src, dst string }

var moduleTables = []moduleTable{
	{"connections", "connections"},
	{"connection_tree_filters", "connection_tree_filters"},
	{"connection_ddl", "connection_ddl"},
	{"saved_queries", "connection_saved_queries"},
	{"filter_history", "connection_filter_history"},
	{"metadata_cache", "connection_metadata_cache"},
	{"api_collections", "api_collections"},
	{"api_items", "api_items"},
	{"api_environments", "api_environments"},
	{"api_variables", "api_variables"},
	{"api_variable_history", "api_variable_history"},
	{"api_response_history", "api_response_history"},
	{"grpc_call_history", "api_grpc_history"},
	{"connection_mask_rules", "connection_mask_rules"},
	{"connection_mask_reviewed", "connection_mask_reviewed"},
	{"docker_restore_engines", "docker_restore_engines"},
	{"docker_restore_containers", "docker_restore_containers"},
	{"kube_clusters", "kube_clusters"},
	{"proof_sessions", "proof_sessions"},
	{"proof_revisions", "proof_revisions"},
	{"proof_steps", "proof_steps"},
	{"proof_view_state", "proof_view_state"},
	{"proof_task_links", "proof_task_links"},
}

// copyOps maps Studio's op_log rows into the merged op_log, the same mapping migration 0080 applies to
// a db_op_log already in the Space database.
func copyOps(t tx) error {
	has, err := studioHasTable(t, "op_log")
	if err != nil || !has {
		return err
	}
	_, err = t.Exec(`INSERT OR IGNORE INTO main.op_log (id, started_ms, duration_ms, category, kind, status, module, trigger_,
			source, subject, repo_root, repo_name, task_id, parent_id, command, error, steps_json, steps_dropped, background,
			repeat_count, connection_id, tab_id, rows, command_truncated, path)
		SELECT d.id, COALESCE(CAST(unixepoch(d.started_at, 'subsec') * 1000 AS INTEGER), 0), d.duration_ms, 'db', d.kind, d.status,
			'database', d.source, d.source, COALESCE(c.name, ''), '', '', '', '', d.command, d.error, '[]', 0, 0, 1,
			d.connection_id, d.tab_id, d.rows, d.command_truncated, d.path
		FROM studio.op_log d LEFT JOIN main.connections c ON c.id = d.connection_id`)
	if err != nil {
		return fmt.Errorf("copy op_log: %w", err)
	}
	return nil
}

// instantCols are the destination columns that hold unix milliseconds while the Studio source holds an ISO
// TEXT instant. A nullable one maps an unparsable value to NULL, a required one to 0.
var instantCols = map[string]struct{ required, nullable []string }{
	"connections":               {required: []string{"created_at", "updated_at"}},
	"connection_mask_rules":     {required: []string{"created_at", "updated_at"}},
	"connection_saved_queries":  {required: []string{"created_at"}, nullable: []string{"used_at"}},
	"connection_filter_history": {required: []string{"used_at"}},
	"connection_metadata_cache": {required: []string{"fetched_at"}},
	"api_collections":           {required: []string{"created_at", "updated_at"}},
	"api_environments":          {required: []string{"created_at", "updated_at"}},
	"api_items":                 {required: []string{"created_at", "updated_at"}},
	"api_variables":             {required: []string{"created_at", "updated_at"}},
	"api_variable_history":      {required: []string{"recorded_at"}},
	"api_response_history":      {required: []string{"sent_at"}},
	"api_grpc_history":          {required: []string{"called_at"}},
}

// instantExpr converts one ISO TEXT source column to unix milliseconds; a value already numeric is kept.
func instantExpr(col string, required bool) string {
	q := `"` + col + `"`
	e := `CASE WHEN typeof(` + q + `) IN ('integer', 'real') THEN CAST(` + q + ` AS INTEGER)
		ELSE CAST(ROUND(unixepoch(` + q + `, 'subsec') * 1000) AS INTEGER) END`
	e = `MAX(0, ` + e + `)`
	if required {
		return `COALESCE(` + e + `, 0)`
	}
	return e
}

// selectList is the source select expression of every shared column, converting instants.
func selectList(dst string, cols []string) string {
	conv := map[string]string{}
	for _, c := range instantCols[dst].required {
		conv[c] = instantExpr(c, true)
	}
	for _, c := range instantCols[dst].nullable {
		conv[c] = instantExpr(c, false)
	}
	out := make([]string, len(cols))
	for i, c := range cols {
		if e, ok := conv[c]; ok {
			out[i] = e
		} else {
			out[i] = `"` + c + `"`
		}
	}
	return strings.Join(out, ", ")
}

// ensureLegacyConnections gives the Space connections table the superseded columns and tables a Studio
// database holds when it has not exported its connections to files yet, so connfiles exports them on its
// first load. A fresh Space schema does not carry them.
func ensureLegacyConnections(t tx) error {
	has, err := studioHasTable(t, "connections")
	if err != nil || !has {
		return err
	}
	var n int
	if err := t.queryRow(`SELECT COUNT(*) FROM studio.settings WHERE key = 'config.migrated.connections' AND value = 'true'`).Scan(&n); err != nil {
		return fmt.Errorf("read studio connections marker: %w", err)
	}
	if n > 0 {
		return nil
	}
	have, err := columns(t, "main", "connections")
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, c := range have {
		present[c] = true
	}
	for _, c := range []string{"password", "uri", "mcp_review_results"} {
		if !present[c] {
			def := ""
			if c == "mcp_review_results" {
				def = " NOT NULL DEFAULT 'inherit'"
			}
			if _, err := t.Exec(`ALTER TABLE main.connections ADD COLUMN ` + c + ` TEXT` + def); err != nil {
				return fmt.Errorf("add legacy column %s: %w", c, err)
			}
		}
	}
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS main.connection_tree_filters (
			connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
			scope TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY (connection_id, scope, value))`,
		`CREATE TABLE IF NOT EXISTS main.connection_ddl (
			connection_id TEXT PRIMARY KEY REFERENCES connections(id) ON DELETE CASCADE,
			ddl TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	} {
		if _, err := t.Exec(ddl); err != nil {
			return fmt.Errorf("create legacy table: %w", err)
		}
	}
	return nil
}

func copyModuleTables(t tx) error {
	if err := ensureLegacyConnections(t); err != nil {
		return err
	}
	for _, mt := range moduleTables {
		has, err := studioHasTable(t, mt.src)
		if err != nil {
			return err
		}
		if !has {
			continue
		}
		cols, err := sharedColumns(t, mt.src, mt.dst)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			continue
		}
		list := strings.Join(quoteAll(cols), ", ")
		where := ""
		if mt.dst == "proof_task_links" {
			where = ` WHERE task_id IN (SELECT id FROM main.ade_tasks)`
		}
		res, err := t.Exec(`INSERT OR IGNORE INTO main.` + mt.dst + ` (` + list + `) SELECT ` + selectList(mt.dst, cols) + ` FROM studio.` + mt.src + where)
		if err != nil {
			return fmt.Errorf("copy %s: %w", mt.src, err)
		}
		var total int
		if err := t.queryRow(`SELECT COUNT(*) FROM studio.` + mt.src).Scan(&total); err != nil {
			return err
		}
		copied, _ := res.RowsAffected()
		if dropped := total - int(copied); dropped > 0 {
			slog.Warn("dbfold: rows not imported", "scope", "dbfold", "table", mt.dst, "count", dropped)
		}
	}
	return nil
}

func studioHasTable(t tx, name string) (bool, error) {
	var n int
	err := t.queryRow(`SELECT COUNT(*) FROM studio.sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0, err
}

// sharedColumns is the columns both sides hold, in the Studio order, so a column one side lacks cannot
// make the copy drop or fail.
func sharedColumns(t tx, src, dst string) ([]string, error) {
	have, err := columns(t, "studio", src)
	if err != nil {
		return nil, err
	}
	want, err := columns(t, "main", dst)
	if err != nil {
		return nil, err
	}
	in := make(map[string]bool, len(want))
	for _, c := range want {
		in[c] = true
	}
	var out []string
	for _, c := range have {
		if in[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

func columns(t tx, schema, table string) ([]string, error) {
	rows, err := t.conn.QueryContext(t.ctx, `SELECT name FROM pragma_table_info(?, ?) ORDER BY cid`, table, schema)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func quoteAll(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = `"` + c + `"`
	}
	return out
}

// studioSettingKeys are the settings rows the Studio modules own. config.migrated.runs stays out:
// the runs fold forces its own re-export.
var studioSettingKeys = []string{
	`data.%`, `cache.%`, `api.%`, `dbMcp.%`,
	`advanced.opLogRetentionDays`, `advanced.expensiveQueryRows`,
	`config.migrated.connections`, `config.migrated.api`,
}

// settingRenames maps a Studio leaf to the Space leaf it feeds when Space has no settings of its own.
var settingRenames = map[string]string{
	"appearance.fontFamily":  "appearance.fontFamily",
	"appearance.fontSize":    "appearance.fontSize",
	"appearance.rowDensity":  "appearance.rowDensity",
	"appearance.wordWrap":    "appearance.wordWrap",
	"appearance.rowColoring": "appearance.rowColoring",
	"agent.provider":         "agent.provider",
	"agent.hermetic":         "agent.hermetic",
	"advanced.notifyPrompts": "advanced.notifyPrompts",
	"advanced.logLevel":      "advanced.gitLogLevel",
}

func spaceSettingsFresh(t tx) (bool, error) {
	var n int
	if err := t.queryRow(`SELECT COUNT(*) FROM main.settings WHERE key LIKE 'appearance.%'`).Scan(&n); err != nil {
		return false, fmt.Errorf("count appearance settings: %w", err)
	}
	return n == 0, nil
}

func copySettings(t tx, fresh bool) error {
	where := make([]string, len(studioSettingKeys))
	args := make([]any, len(studioSettingKeys))
	for i, k := range studioSettingKeys {
		where[i] = `key LIKE ?`
		args[i] = k
	}
	if _, err := t.Exec(`INSERT OR IGNORE INTO main.settings (key, value)
		SELECT key, value FROM studio.settings WHERE `+strings.Join(where, ` OR `), args...); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if !fresh {
		return nil
	}
	for from, to := range settingRenames {
		if _, err := t.Exec(`INSERT OR IGNORE INTO main.settings (key, value)
			SELECT ?, value FROM studio.settings WHERE key = ?`, to, from); err != nil {
			return fmt.Errorf("settings %s: %w", from, err)
		}
	}
	return nil
}

// copyKeyed copies the config snapshots and sealed secrets; a row Space already holds wins. Script
// parameters were already moved and deleted by the runs fold.
func copyKeyed(t tx) error {
	for _, q := range []string{
		`INSERT OR IGNORE INTO main.config_last_valid (folder, file, value_json, recorded_at)
		 SELECT folder, file, value_json, recorded_at FROM studio.config_last_valid`,
		`INSERT OR IGNORE INTO main.secret_values (scope, owner, name, value, updated_ms)
		 SELECT scope, owner, name, value, updated_ms FROM studio.secret_values WHERE scope <> 'script-param'`,
	} {
		if _, err := t.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// layoutKeys are the panel sizes the Studio modes own, plus the cell editor height (P538).
var layoutKeys = []string{
	`modes.studio.%`, `modes.api.%`, `modes.docker.%`, `modes.kubernetes.%`, `panel.cellEditor.height`,
}

func copyLayout(t tx) error {
	where := make([]string, len(layoutKeys))
	args := make([]any, len(layoutKeys))
	for i, k := range layoutKeys {
		where[i] = `key LIKE ?`
		args[i] = k
	}
	_, err := t.Exec(`INSERT OR IGNORE INTO main.ui_layout (key, value)
		SELECT key, value FROM studio.ui_layout WHERE `+strings.Join(where, ` OR `), args...)
	return err
}

type studioTab struct {
	id, connectionID, windowKey, path, kind, state string
	workspaceID                                    sql.NullString
	connNull                                       bool
	order                                          int
	active                                         bool
}

// copyTabs adds the Studio tabs to the tab table. A tab whose window the Space database does not know moves
// onto Space's primary window, appended after every tab it holds; Space creates that window when it has
// none. A tab id Space already holds gets a fresh id.
func copyTabs(t tx, ephemeral func(string) bool) error {
	if ephemeral == nil {
		ephemeral = func(string) bool { return false }
	}
	has, err := studioHasTable(t, "tabs")
	if err != nil || !has {
		return err
	}
	rows, err := t.conn.QueryContext(t.ctx, `SELECT t.id, t.connection_id, t.window_key, t.path, t.kind, t.state_json,
		t."order", t.active, t.workspace_id FROM studio.tabs t
		LEFT JOIN studio.windows w ON w.key = t.window_key ORDER BY COALESCE(w."order", 0), t."order"`)
	if err != nil {
		return err
	}
	var tabs []studioTab
	for rows.Next() {
		var s studioTab
		var conn sql.NullString
		if err := rows.Scan(&s.id, &conn, &s.windowKey, &s.path, &s.kind, &s.state, &s.order, &s.active, &s.workspaceID); err != nil {
			_ = rows.Close()
			return err
		}
		s.connectionID, s.connNull = conn.String, !conn.Valid
		tabs = append(tabs, s)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(tabs) == 0 {
		return nil
	}
	known, primary, err := spaceWindows(t, ephemeral)
	if err != nil {
		return err
	}
	for _, s := range tabs {
		if !known[s.windowKey] {
			if primary == "" {
				primary = ids.NewID()
				if _, err := t.Exec(`INSERT INTO main.windows (key, sort_order) VALUES (?, (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM main.windows))`, primary); err != nil {
					return err
				}
				known[primary] = true
			}
			if err := appendTab(t, primary, s); err != nil {
				return err
			}
			continue
		}
		if err := insertTab(t, s.windowKey, s.order, s.active, s); err != nil {
			return err
		}
	}
	return nil
}

func spaceWindows(t tx, ephemeral func(string) bool) (known map[string]bool, primary string, err error) {
	rows, err := t.conn.QueryContext(t.ctx, `SELECT key FROM main.windows ORDER BY sort_order ASC`)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = rows.Close() }()
	known = map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, "", err
		}
		known[k] = true
		if primary == "" && !ephemeral(k) {
			primary = k
		}
	}
	return known, primary, rows.Err()
}

// appendTab puts an orphan tab after the last tab of window; it is active only when no tab there is.
func appendTab(t tx, window string, s studioTab) error {
	var next int
	var activeTaken bool
	if err := t.queryRow(`SELECT COALESCE(MAX(sort_order), -1) + 1, COALESCE(MAX(active), 0) FROM main.tabs WHERE window_key = ?`,
		window).Scan(&next, &activeTaken); err != nil {
		return err
	}
	return insertTab(t, window, next, s.active && !activeTaken, s)
}

func insertTab(t tx, window string, order int, active bool, s studioTab) error {
	id := s.id
	var taken int
	if err := t.queryRow(`SELECT COUNT(*) FROM main.tabs WHERE id = ?`, id).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		id = ids.NewID()
	}
	var conn any
	if !s.connNull {
		conn = s.connectionID
	}
	_, err := t.Exec(`INSERT INTO main.tabs (id, connection_id, path, kind, state_json, sort_order, active, window_key, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, conn, s.path, s.kind, s.state, order, active, window, s.workspaceID)
	return err
}
