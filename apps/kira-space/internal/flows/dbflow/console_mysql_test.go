package dbflow

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"

	sqldriver "github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// mysqlRows runs query on a client of its own against the fixture.
func mysqlRows(t *testing.T, fx *testsupport.MysqlFixture, query string) [][]any {
	t.Helper()
	cfg := sqldriver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", *fx.Config.Host, *fx.Config.Port)
	cfg.User = *fx.Config.Username
	cfg.Passwd = *fx.Config.Password
	cfg.DBName = *fx.Config.Database
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return scanRows(t, query, func() (*sql.Rows, error) { return db.Query(query) })
}

// J-CONS-021 (contract studio-console-script): the MySQL console's script behaviour, pinned. A
// compound CREATE PROCEDURE runs as one statement. DELIMITER is a client directive the engine
// rejects. A CALL that returns several result sets shows the first only (follow-up F-A13). Session
// variables live on the pooled connection and are gone after a reconnect (strip note: F-A3).
func TestConsoleMySQLScript(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMysql(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	t.Cleanup(func() { _, _ = ds.Execute(conn.ID, "", "DROP PROCEDURE IF EXISTS kira_script") })

	_, err := ds.Execute(conn.ID, "", "DELIMITER //")
	var de *flowharness.DataError
	if !errors.As(err, &de) || de.Code != "E_QUERY" {
		t.Fatalf("DELIMITER = %v, want an E_QUERY syntax error", err)
	}
	app.Contract(t, consoleScriptScenario, "mysql#delimiter", map[string]string{"code": de.Code})

	script := []string{
		"DROP PROCEDURE IF EXISTS kira_script",
		"CREATE PROCEDURE kira_script() BEGIN SELECT 1 AS first_set; SELECT 2 AS second_set, 3 AS other; END",
		"CALL kira_script()",
	}
	app.Contract(t, consoleScriptScenario, "mysql#script", script)
	pages, err := ds.Execute(conn.ID, "", script...)
	if err != nil {
		t.Fatalf("procedure script: %v", err)
	}
	if len(pages) != 3 {
		t.Fatalf("%d pages for 3 statements", len(pages))
	}
	app.Contract(t, consoleScriptScenario, "mysql#pages", summaries(pages))
	wantRows(t, "procedure", mysqlRows(t, fx, `select routine_name from information_schema.routines where routine_schema = database() and routine_name = 'kira_script'`),
		[][]any{{"kira_script"}})

	set, err := ds.Execute(conn.ID, "", "SET @kira_probe = 5", "SELECT @kira_probe")
	if err != nil || set[1].Rows[0][0] != "5" {
		t.Fatalf("session variable on the live connection = %+v (%v), want 5", set, err)
	}
	if st, err := app.W.Connections.Disconnect(bridge.ConnectionsIDArgs{ID: conn.ID}); err != nil || st.Status == "connected" {
		t.Fatalf("Disconnect = %+v (%v)", st, err)
	}
	connect(t, app, conn.ID)
	again, err := ds.Execute(conn.ID, "", "SELECT @kira_probe")
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, consoleScriptScenario, "mysql#afterReconnect", summaries(again))
	if again[0].Rows[0][0] != nil {
		t.Fatalf("@kira_probe after a reconnect = %v, want NULL", again[0].Rows[0][0])
	}
}

// errnoOf reads the server error number a failed console run carries in its details.
func errnoOf(t *testing.T, err error) (errno, index int) {
	t.Helper()
	var de *flowharness.DataError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want a data error", err)
	}
	var d struct {
		SqlErrno       int `json:"sqlErrno"`
		StatementIndex int `json:"statementIndex"`
	}
	if jerr := json.Unmarshal([]byte(de.Details), &d); jerr != nil {
		t.Fatalf("details %q: %v", de.Details, jerr)
	}
	return d.SqlErrno, d.StatementIndex
}

// Decision 8 (contract studio-console-script): what ends a MySQL transaction, pinned on the real
// server because MySQL 8.4 reports no transaction state and the console reads it off the statements
// it ran. DDL commits implicitly unless the parser rejects it (1064), COMMIT AND CHAIN keeps the
// next transaction open, autocommit=0 reopens one after every COMMIT, XA ends through XA END and
// XA ROLLBACK, and a failed statement leaves the transaction open with its index (and the server
// error number) in the error details. The engine reports no state here, which the test asserts.
func TestConsoleMySQLTransactionEnds(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMysql(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	exec := func(stmts ...string) {
		t.Helper()
		_, tx, err := ds.ExecuteTx(conn.ID, "", stmts...)
		if err != nil {
			t.Fatalf("%v: %v", stmts, err)
		}
		if tx != adapters.TxUnknown {
			t.Fatalf("MySQL reported transaction state %v, want none (8.4 has no in_transaction)", tx)
		}
	}
	// count is the session's own view (inside its transaction); other is what a second client sees.
	count := func() string {
		t.Helper()
		p, err := ds.Execute(conn.ID, "", "SELECT count(*) FROM kira_tx")
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(p[0].Rows[0][0])
	}
	other := func() string {
		t.Helper()
		return fmt.Sprint(mysqlRows(t, fx, "select count(*) from kira_tx")[0][0])
	}
	autocommit := func() string {
		t.Helper()
		p, err := ds.Execute(conn.ID, "", "SELECT @@autocommit")
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(p[0].Rows[0][0])
	}
	exec("DROP TABLE IF EXISTS kira_tx", "DROP TABLE IF EXISTS kira_tx_ddl", "DROP PROCEDURE IF EXISTS kira_commit", "CREATE TABLE kira_tx (n INT) ENGINE=InnoDB")
	t.Cleanup(func() {
		_, _ = ds.Execute(conn.ID, "", "ROLLBACK", "SET autocommit=1", "DROP TABLE IF EXISTS kira_tx", "DROP TABLE IF EXISTS kira_tx_ddl", "DROP PROCEDURE IF EXISTS kira_commit")
	})

	implicit := []string{"BEGIN", "INSERT INTO kira_tx VALUES (1)", "CREATE TABLE kira_tx_ddl (i INT)", "ROLLBACK"}
	app.Contract(t, consoleScriptScenario, "mysql#implicitCommit", implicit)
	exec(implicit...)
	if got := count(); got != "1" {
		t.Fatalf("rows after DDL then ROLLBACK = %s, want 1: the DDL committed the insert", got)
	}

	chain := []string{"BEGIN", "INSERT INTO kira_tx VALUES (2)", "COMMIT AND CHAIN", "INSERT INTO kira_tx VALUES (3)"}
	app.Contract(t, consoleScriptScenario, "mysql#chain", chain)
	exec(chain...)
	exec("ROLLBACK")
	if got := count(); got != "2" {
		t.Fatalf("rows after COMMIT AND CHAIN then ROLLBACK = %s, want 2: the chain kept a transaction open", got)
	}

	failing := []string{"BEGIN", "INSERT INTO kira_tx VALUES (4)", "INSERT INTO kira_tx VALUES ('x')", "COMMIT"}
	app.Contract(t, consoleScriptScenario, "mysql#failing", failing)
	_, err := ds.Execute(conn.ID, "", failing...)
	if errno, index := errnoOf(t, err); errno != 1366 || index != 2 {
		t.Fatalf("failing script = %v, want errno 1366 at statement 2", err)
	}
	if got := count(); got != "3" {
		t.Fatalf("rows inside the transaction the failed script left open = %s, want 3", got)
	}
	exec("ROLLBACK")
	if got := count(); got != "2" {
		t.Fatalf("rows after ROLLBACK = %s, want 2", got)
	}

	// A syntax error never reaches the implicit commit; a DDL that parses and then fails does.
	parseErr := []string{"BEGIN", "INSERT INTO kira_tx VALUES (5)", "CREATE TABL kira_tx_x (i INT)"}
	app.Contract(t, consoleScriptScenario, "mysql#parseErrorDDL", parseErr)
	_, err = ds.Execute(conn.ID, "", parseErr...)
	if errno, index := errnoOf(t, err); errno != 1064 || index != 2 {
		t.Fatalf("syntax-error DDL = %v, want errno 1064 at statement 2", err)
	}
	if got := count(); got != "3" || other() != "2" {
		t.Fatalf("after a syntax-error DDL the session sees %s rows and a second client %s, want 3 and 2: the transaction stays open", got, other())
	}
	exec("ROLLBACK")
	if got := count(); got != "2" {
		t.Fatalf("rows after ROLLBACK = %s, want 2", got)
	}
	runtimeErr := []string{"BEGIN", "INSERT INTO kira_tx VALUES (6)", "CREATE TABLE kira_tx (i INT)"}
	app.Contract(t, consoleScriptScenario, "mysql#runtimeErrorDDL", runtimeErr)
	_, err = ds.Execute(conn.ID, "", runtimeErr...)
	if errno, index := errnoOf(t, err); errno != 1050 || index != 2 {
		t.Fatalf("existing-table DDL = %v, want errno 1050 at statement 2", err)
	}
	if got := other(); got != "3" {
		t.Fatalf("a second client sees %s rows after a DDL that failed at run time, want 3: it committed first", got)
	}
	exec("ROLLBACK")

	// Statements that commit implicitly beyond DDL. RESET needs the RELOAD privilege the fixture user
	// lacks: it fails with 1227 and has committed all the same, since the commit runs before the
	// privilege check.
	for key, c := range map[string]struct {
		stmt  string
		errno int
	}{
		"mysql#checkTable": {"CHECK TABLE kira_tx", 0},
		"mysql#reset":      {"RESET BINARY LOGS AND GTIDS", 1227},
	} {
		script := []string{"BEGIN", "INSERT INTO kira_tx VALUES (7)", c.stmt, "ROLLBACK"}
		app.Contract(t, consoleScriptScenario, key, script)
		before := other()
		_, err := ds.Execute(conn.ID, "", script...)
		if c.errno == 0 && err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if c.errno != 0 {
			if errno, index := errnoOf(t, err); errno != c.errno || index != 2 {
				t.Fatalf("%s: %v, want errno %d at statement 2", key, err, c.errno)
			}
		}
		if after := other(); after == before {
			t.Fatalf("%s: a second client still sees %s rows, want the insert committed by %q", key, after, c.stmt)
		}
		exec("ROLLBACK")
	}

	// A procedure may commit or leave a transaction open; the statement text cannot say.
	exec("CREATE PROCEDURE kira_commit() BEGIN COMMIT; END")
	call := []string{"BEGIN", "INSERT INTO kira_tx VALUES (8)", "CALL kira_commit()"}
	app.Contract(t, consoleScriptScenario, "mysql#call", call)
	before := other()
	exec(call...)
	if after := other(); after == before {
		t.Fatalf("a second client still sees %s rows, want the insert committed by the procedure", after)
	}

	// autocommit=0 keeps a transaction open after COMMIT; the close must switch autocommit back on.
	off := []string{"SET autocommit=0", "COMMIT", "INSERT INTO kira_tx VALUES (9)"}
	app.Contract(t, consoleScriptScenario, "mysql#autocommitOff", off)
	before = other()
	exec(off...)
	if got, second := count(), other(); got == second || second != before {
		t.Fatalf("session sees %s rows, a second client %s (was %s): the insert should be uncommitted", got, second, before)
	}
	closeOff := []string{"ROLLBACK", "SET autocommit=1"}
	app.Contract(t, consoleScriptScenario, "mysql#autocommitClose", closeOff)
	exec(closeOff...)
	if got := count(); got != before || autocommit() != "1" {
		t.Fatalf("after the close statements: %s rows (want %s), autocommit %s (want 1)", got, before, autocommit())
	}
	exec("SET autocommit=0", "ROLLBACK")
	if got := autocommit(); got != "0" {
		t.Fatalf("autocommit after ROLLBACK alone = %s, want 0: the restore statement is needed", got)
	}
	exec("SET autocommit=1")

	// XA: ROLLBACK is refused inside an active XA transaction; XA END then XA ROLLBACK ends it.
	xa := []string{"XA START 'kira_xa'", "INSERT INTO kira_tx VALUES (10)"}
	app.Contract(t, consoleScriptScenario, "mysql#xa", xa)
	exec(xa...)
	if _, err := ds.Execute(conn.ID, "", "ROLLBACK"); err == nil {
		t.Fatal("ROLLBACK inside an active XA transaction succeeded, want an XA error")
	}
	exec("XA END 'kira_xa'", "XA ROLLBACK 'kira_xa'")
	if got := count(); got != other() {
		t.Fatalf("session sees %s rows, a second client %s after the XA rollback, want equal", got, other())
	}
}
