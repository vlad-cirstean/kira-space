package dbflow

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

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

// Decision 8 (contract studio-console-script): what ends a MySQL transaction, pinned on the real
// server because the console reads the state off the statements it ran. DDL commits implicitly,
// COMMIT AND CHAIN keeps the next transaction open, and a statement that fails leaves the
// transaction open with the statements before it applied and its index in the error details.
func TestConsoleMySQLTransactionEnds(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMysql(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	exec := func(stmts ...string) {
		t.Helper()
		if _, err := ds.Execute(conn.ID, "", stmts...); err != nil {
			t.Fatalf("%v: %v", stmts, err)
		}
	}
	count := func() string {
		t.Helper()
		p, err := ds.Execute(conn.ID, "", "SELECT count(*) FROM kira_tx")
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(p[0].Rows[0][0])
	}
	exec("DROP TABLE IF EXISTS kira_tx", "DROP TABLE IF EXISTS kira_tx_ddl", "CREATE TABLE kira_tx (n INT) ENGINE=InnoDB")
	t.Cleanup(func() { _, _ = ds.Execute(conn.ID, "", "DROP TABLE IF EXISTS kira_tx", "DROP TABLE IF EXISTS kira_tx_ddl") })

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
	var de *flowharness.DataError
	if !errors.As(err, &de) || de.Details != `{"statementIndex":2}` {
		t.Fatalf("failing script = %v, want a data error at statement 2", err)
	}
	if got := count(); got != "3" {
		t.Fatalf("rows inside the transaction the failed script left open = %s, want 3", got)
	}
	exec("ROLLBACK")
	if got := count(); got != "2" {
		t.Fatalf("rows after ROLLBACK = %s, want 2", got)
	}
}
