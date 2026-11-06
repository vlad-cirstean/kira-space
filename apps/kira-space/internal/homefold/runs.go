package homefold

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/fsmove"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

var runFileRE = regexp.MustCompile(`^([a-z0-9][a-z0-9_-]{0,79})\.yaml$`)

const maxIDLen = 80

// Runs copies the Studio runs rows, run files and automation folders into the Space stack, then
// deletes the copied Studio rows. It runs after Files. A nil o.Studio folds the run files and
// automation folders only (a legacy home without a Studio database).
func Runs(o Options) error {
	m, found, err := readMarker(o.Space)
	if err != nil {
		return err
	}
	if !found || (m.State != stateFiles && m.State != stateRuns) {
		return nil
	}
	src := filepath.Join(m.Backup, "kira-studio")

	if err := o.step("space-backup"); err != nil {
		return err
	}
	if err := backupSpaceDB(o, m.Backup); err != nil {
		return err
	}
	if err := o.step("map"); err != nil {
		return err
	}
	if m.State == stateFiles {
		if m.Scripts, m.Runs, err = buildMaps(o, src); err != nil {
			return fmt.Errorf("homefold: map ids: %w", err)
		}
		m.State = stateRuns
		if err := writeMarker(o.Space, m); err != nil {
			return err
		}
	}
	if err := o.step("rows"); err != nil {
		return err
	}
	if !m.RowsCopied {
		if err := copyRows(o, &m); err != nil {
			return fmt.Errorf("homefold: copy runs rows: %w", err)
		}
	}
	if err := o.step("run-files"); err != nil {
		return err
	}
	if err := copyRunFiles(o, m, src); err != nil {
		return fmt.Errorf("homefold: copy run files: %w", err)
	}
	if err := o.step("automations"); err != nil {
		return err
	}
	if err := copyAutomations(o, m, src); err != nil {
		return fmt.Errorf("homefold: copy automations: %w", err)
	}
	if err := o.step("studio-delete"); err != nil {
		return err
	}
	if err := deleteStudioRows(o.Studio); err != nil {
		return fmt.Errorf("homefold: clear Studio runs rows: %w", err)
	}
	if err := o.step("done"); err != nil {
		return err
	}
	m.State = stateDone
	slog.Info("homefold: Studio home folded", "scope", "homefold", "backup", m.Backup)
	return writeMarker(o.Space, m)
}

func backupSpaceDB(o Options, backup string) error {
	dst := filepath.Join(backup, "space.db")
	if fsmove.Exists(dst) {
		return nil
	}
	tmp := dst + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if _, err := o.Space.Exec(`VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("homefold: back up Space database: %w", err)
	}
	return os.Rename(tmp, dst)
}

// buildMaps assigns Space ids to every Studio script and run whose id would collide; an unmapped id
// keeps itself. Space wins its own script ids.
func buildMaps(o Options, src string) (scripts, runs map[string]string, err error) {
	studioIDs, err := scriptIDs(o.Studio, "custom_scripts", filepath.Join(src, "runs"))
	if err != nil {
		return nil, nil, err
	}
	taken, err := scriptIDs(o.Space, "scripts", runsDir(o.SpaceHome))
	if err != nil {
		return nil, nil, err
	}
	scripts = map[string]string{}
	for _, id := range sortedKeys(studioIDs) {
		if !taken[id] {
			taken[id] = true
			continue
		}
		fresh := freeID(id, taken)
		taken[fresh] = true
		scripts[id] = fresh
	}
	spaceRuns, err := columnSet(o.Space, `SELECT id FROM script_runs`)
	if err != nil {
		return nil, nil, err
	}
	studioRuns, err := columnSet(o.Studio, `SELECT id FROM script_runs`)
	if err != nil {
		return nil, nil, err
	}
	runs = map[string]string{}
	for _, id := range sortedKeys(studioRuns) {
		if spaceRuns[id] {
			runs[id] = ids.NewID()
		}
	}
	return scripts, runs, nil
}

// freeID suffixes base with -studio, -studio-2, ... keeping the stem within the run-file name pattern.
func freeID(base string, taken map[string]bool) string {
	for n := 1; ; n++ {
		suffix := "-studio"
		if n > 1 {
			suffix += "-" + strconv.Itoa(n)
		}
		stem := base
		if len(stem)+len(suffix) > maxIDLen {
			stem = stem[:maxIDLen-len(suffix)]
		}
		if id := stem + suffix; !taken[id] {
			return id
		}
	}
}

// scriptIDs is a database's script ids (table) plus the stems of the run files in dir.
func scriptIDs(db *sql.DB, table, dir string) (map[string]bool, error) {
	set, err := columnSet(db, `SELECT id FROM `+table)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if sm := runFileRE.FindStringSubmatch(e.Name()); sm != nil && !e.IsDir() {
			set[sm[1]] = true
		}
	}
	return set, nil
}

func columnSet(db *sql.DB, query string) (map[string]bool, error) {
	if db == nil {
		return map[string]bool{}, nil
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	set := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		set[v] = true
	}
	return set, rows.Err()
}

func mapped(m map[string]string, id string) string {
	if to, ok := m[id]; ok {
		return to
	}
	return id
}

// tableRows reads every row of table from db over cols, as column-name keyed values.
func tableRows(db *sql.DB, table string, cols []string, where string, args ...any) ([]map[string]any, error) {
	rows, err := db.Query(`SELECT `+strings.Join(cols, ", ")+` FROM `+table+where, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			row[c] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// sharedColumns is the columns both databases hold for a table, in the Studio order, so a column
// added later on one side cannot make the copy drop or fail. src and dst name the table in each.
func sharedColumns(a, b *sql.DB, src, dst string) ([]string, error) {
	studio, err := tableColumns(a, src)
	if err != nil {
		return nil, err
	}
	space, err := tableColumns(b, dst)
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, c := range space {
		have[c] = true
	}
	var out []string
	for _, c := range studio {
		if have[c] {
			out = append(out, c)
		}
	}
	return out, nil
}

func tableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
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

// insertRows returns how many rows INSERT OR IGNORE dropped on a key clash.
func insertRows(tx *sql.Tx, table string, cols []string, rows []map[string]any) (int, error) {
	q := `INSERT OR IGNORE INTO ` + table + ` (` + strings.Join(cols, ", ") + `) VALUES (` +
		strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + `)`
	stmt, err := tx.Prepare(q)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stmt.Close() }()
	ignored := 0
	for _, r := range rows {
		args := make([]any, len(cols))
		for i, c := range cols {
			args[i] = r[c]
		}
		res, err := stmt.Exec(args...)
		if err != nil {
			return 0, err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			ignored++
		}
	}
	return ignored, nil
}

// runTable maps a Studio table to its Space name.
type runTable struct{ src, dst string }

var runTables = []runTable{
	{"custom_script_collections", "script_collections"},
	{"custom_scripts", "scripts"},
	{"script_runs", "script_runs"},
	{"script_run_logs", "script_run_logs"},
	{"secret_values", "secret_values"},
}

// tableData is one table's shared columns and Studio rows.
type tableData struct {
	cols []string
	rows []map[string]any
}

// readStudioTables reads every folded table up front: the Space database is one connection, held by
// the transaction that writes.
func readStudioTables(o Options) (map[string]tableData, error) {
	if o.Studio == nil {
		return map[string]tableData{}, nil
	}
	where := map[string]string{"secret_values": ` WHERE scope = '` + string(secrets.ScopeScriptParam) + `'`}
	out := map[string]tableData{}
	for _, t := range runTables {
		cols, err := sharedColumns(o.Studio, o.Space, t.src, t.dst)
		if err != nil {
			return nil, err
		}
		rows, err := tableRows(o.Studio, t.src, cols, where[t.dst])
		if err != nil {
			return nil, err
		}
		out[t.dst] = tableData{cols, rows}
	}
	return out, nil
}

// remapRows rewrites ids and paths into the Space stack. Collections and scripts sort after Space's own.
func remapRows(o Options, m marker, data map[string]tableData) (needExport bool) {
	legacyAuto := filepath.Join(m.Legacy, "automations") + string(filepath.Separator)
	spaceAuto := filepath.Join(o.SpaceHome, "automations") + string(filepath.Separator)
	for _, r := range data["scripts"].rows {
		r["id"] = mapped(m.Scripts, asString(r["id"]))
		needExport = needExport || asString(r["config_json"]) == ""
	}
	for _, r := range data["script_runs"].rows {
		old := asString(r["script_id"])
		r["id"] = mapped(m.Runs, asString(r["id"]))
		r["script_id"] = mapped(m.Scripts, old)
		if cwd := asString(r["cwd"]); strings.HasPrefix(cwd, legacyAuto+old) {
			r["cwd"] = spaceAuto + mapped(m.Scripts, old) + strings.TrimPrefix(cwd, legacyAuto+old)
		}
	}
	for _, r := range data["script_run_logs"].rows {
		r["run_id"] = mapped(m.Runs, asString(r["run_id"]))
	}
	for _, r := range data["secret_values"].rows {
		r["owner"] = mapped(m.Scripts, asString(r["owner"]))
	}
	return needExport
}

// copyRows copies the Studio runs tables into the Space database in one transaction, which also
// records the copy in the marker so a resume does not recount its own rows as clashes.
func copyRows(o Options, m *marker) error {
	data, err := readStudioTables(o)
	if err != nil {
		return err
	}
	needExport := remapRows(o, *m, data)

	tx, err := o.Space.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	runIDs := map[string]bool{}
	for _, r := range data["script_runs"].rows {
		runIDs[asString(r["id"])] = true
	}
	for _, t := range runTables {
		table := t.dst
		d := data[table]
		if table == "script_run_logs" {
			kept := d.rows[:0]
			for _, r := range d.rows {
				if runIDs[asString(r["run_id"])] {
					kept = append(kept, r)
				}
			}
			if len(kept) < len(d.rows) {
				slog.Warn("homefold: Studio run logs without a run dropped", "scope", "homefold", "dropped", len(d.rows)-len(kept))
			}
			d.rows = kept
		}
		if table == "script_collections" || table == "scripts" {
			var offset int64
			if err := tx.QueryRow(`SELECT COALESCE(MAX(sort_order) + 1, 0) FROM ` + table).Scan(&offset); err != nil {
				return err
			}
			for _, r := range d.rows {
				if n, ok := r["sort_order"].(int64); ok {
					r["sort_order"] = n + offset
				}
			}
		}
		ignored, err := insertRows(tx, table, d.cols, d.rows)
		if err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
		if ignored > 0 {
			slog.Warn("homefold: Studio rows dropped on a key clash with Space", "scope", "homefold", "table", table, "read", len(d.rows), "dropped", ignored)
		}
	}
	if needExport {
		if _, err := tx.Exec(`DELETE FROM settings WHERE key = 'config.migrated.runs'`); err != nil {
			return err
		}
	}
	m.RowsCopied = true
	if err := writeMarker(tx, *m); err != nil {
		m.RowsCopied = false
		return err
	}
	if err := tx.Commit(); err != nil {
		m.RowsCopied = false
		return err
	}
	return nil
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

func copyRunFiles(o Options, m marker, src string) error {
	dir := filepath.Join(src, "runs")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runsDir(o.SpaceHome), 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		sm := runFileRE.FindStringSubmatch(e.Name())
		if sm == nil || e.IsDir() {
			slog.Warn("homefold: run folder entry left in backup", "scope", "homefold", "name", e.Name())
			continue
		}
		dst := filepath.Join(runsDir(o.SpaceHome), mapped(m.Scripts, sm[1])+".yaml")
		if fsmove.Exists(dst) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if err := renameio.WriteFile(dst, b, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyAutomations(o Options, m marker, src string) error {
	dir := filepath.Join(src, "automations")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(o.SpaceHome, "automations"), 0o700); err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dst := filepath.Join(o.SpaceHome, "automations", mapped(m.Scripts, e.Name()))
		if fsmove.Exists(dst) {
			continue
		}
		tmp := dst + ".p539tmp"
		if err := os.RemoveAll(tmp); err != nil {
			return err
		}
		if err := fsmove.CopyTree(filepath.Join(dir, e.Name()), tmp); err != nil {
			return err
		}
		if err := os.Rename(tmp, dst); err != nil {
			return err
		}
	}
	return nil
}

// deleteStudioRows drops the copied rows from the Studio database in one transaction, so a second
// import can never see them.
func deleteStudioRows(db *sql.DB) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM script_run_logs`,
		`DELETE FROM script_runs`,
		`DELETE FROM custom_scripts`,
		`DELETE FROM custom_script_collections`,
		`DELETE FROM secret_values WHERE scope = '` + string(secrets.ScopeScriptParam) + `'`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
