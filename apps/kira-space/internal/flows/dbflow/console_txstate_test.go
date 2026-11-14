package dbflow

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// Decision 8, backend half of the console's transaction tracking: where the engine knows whether a
// transaction is open, the execute response (and a failed run's error details) says so, and the
// console takes that over reading the statements. One real server per engine. MySQL 8.4 has no
// such state; TestConsoleMySQLTransactionEnds asserts it reports none.

type txStep struct {
	label string
	stmts []string
	want  adapters.TxState
	// fails: the run errors at stmts[failIndex] and its details carry inTransaction == want.
	fails     bool
	failIndex int
}

func runTxSteps(t *testing.T, ds *flowharness.DataStream, connID string, steps []txStep) {
	t.Helper()
	for _, st := range steps {
		_, tx, err := ds.ExecuteTx(connID, "", st.stmts...)
		if !st.fails {
			if err != nil {
				t.Fatalf("%s: %v", st.label, err)
			}
			if tx != st.want {
				t.Fatalf("%s: transaction = %v, want %v", st.label, tx, st.want)
			}
			continue
		}
		var de *flowharness.DataError
		if !errors.As(err, &de) {
			t.Fatalf("%s: err = %v, want a data error", st.label, err)
		}
		var d struct {
			InTransaction  *bool `json:"inTransaction"`
			StatementIndex int   `json:"statementIndex"`
		}
		if jerr := json.Unmarshal([]byte(de.Details), &d); jerr != nil {
			t.Fatalf("%s: details %q: %v", st.label, de.Details, jerr)
		}
		if d.InTransaction == nil || *d.InTransaction != (st.want == adapters.TxOpen) || d.StatementIndex != st.failIndex {
			t.Fatalf("%s: details %s, want inTransaction=%v at statement %d", st.label, de.Details, st.want == adapters.TxOpen, st.failIndex)
		}
	}
}

func TestConsolePostgresTransactionState(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartPostgres(t)
	pgExec(t, fx.URI, `drop table if exists app.tx_state`, `create table app.tx_state (n int unique deferrable initially deferred)`)
	app := flowharness.New(t)
	conn := pgFixtureConn(t, app, fx)
	ds := app.DataStream(t)
	runTxSteps(t, ds, conn.ID, []txStep{
		{label: "plain statement", stmts: []string{"SELECT 1"}, want: adapters.TxIdle},
		{label: "BEGIN", stmts: []string{"BEGIN"}, want: adapters.TxOpen},
		{label: "statement inside", stmts: []string{"INSERT INTO app.tx_state VALUES (1)"}, want: adapters.TxOpen},
		{label: "COMMIT", stmts: []string{"COMMIT"}, want: adapters.TxIdle},
		{label: "BEGIN with a comment in front", stmts: []string{"/* a /* nested */ b */ BEGIN"}, want: adapters.TxOpen},
		{label: "ROLLBACK", stmts: []string{"ROLLBACK"}, want: adapters.TxIdle},
		{label: "a failing statement aborts the transaction", stmts: []string{"BEGIN", "SELECT 1/0"}, want: adapters.TxOpen, fails: true, failIndex: 1},
		{label: "ROLLBACK of the aborted transaction", stmts: []string{"ROLLBACK"}, want: adapters.TxIdle},
		// A deferred constraint fails at COMMIT, which ends the transaction anyway.
		{label: "a failed COMMIT", stmts: []string{"BEGIN", "INSERT INTO app.tx_state VALUES (2), (2)", "COMMIT"}, want: adapters.TxIdle, fails: true, failIndex: 2},
		{label: "savepoint inside a transaction", stmts: []string{"BEGIN", "SAVEPOINT s1", "ROLLBACK TO s1"}, want: adapters.TxOpen},
		{label: "end", stmts: []string{"END"}, want: adapters.TxIdle},
	})
}

func TestConsoleMariaDBTransactionState(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartMariadb(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	if _, err := ds.Execute(conn.ID, "", "DROP TABLE IF EXISTS kira_tx", "CREATE TABLE kira_tx (n INT) ENGINE=InnoDB"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = ds.Execute(conn.ID, "", "ROLLBACK", "SET autocommit=1", "DROP TABLE IF EXISTS kira_tx") })
	runTxSteps(t, ds, conn.ID, []txStep{
		{label: "plain statement", stmts: []string{"SELECT 1"}, want: adapters.TxIdle},
		{label: "BEGIN", stmts: []string{"BEGIN"}, want: adapters.TxOpen},
		{label: "COMMIT", stmts: []string{"INSERT INTO kira_tx VALUES (1)", "COMMIT"}, want: adapters.TxIdle},
		{label: "BEGIN after a # comment", stmts: []string{"# note\nBEGIN"}, want: adapters.TxOpen},
		{label: "a syntax-error DDL does not commit", stmts: []string{"CREATE TABL kira_tx_x (i INT)"}, want: adapters.TxOpen, fails: true, failIndex: 0},
		{label: "DDL that parses commits", stmts: []string{"CREATE TABLE kira_tx_ok (i INT)"}, want: adapters.TxIdle},
		{label: "DROP", stmts: []string{"BEGIN", "DROP TABLE kira_tx_ok"}, want: adapters.TxIdle},
		{label: "CHECK TABLE", stmts: []string{"BEGIN", "CHECK TABLE kira_tx"}, want: adapters.TxIdle},
		{label: "COMMIT AND CHAIN", stmts: []string{"BEGIN", "COMMIT AND CHAIN"}, want: adapters.TxOpen},
		{label: "ROLLBACK", stmts: []string{"ROLLBACK"}, want: adapters.TxIdle},
		{label: "autocommit off then COMMIT then a write", stmts: []string{"SET autocommit=0", "COMMIT", "INSERT INTO kira_tx VALUES (2)"}, want: adapters.TxOpen},
		{label: "rollback of it", stmts: []string{"ROLLBACK"}, want: adapters.TxIdle},
		{label: "autocommit back on", stmts: []string{"SET autocommit=1"}, want: adapters.TxIdle},
	})
}

func TestConsoleSQLiteTransactionState(t *testing.T) {
	app := flowharness.New(t)
	path := filepath.Join(t.TempDir(), "tx.db")
	sqliteExec(t, path, "create table seed (id integer primary key)")
	conn := createConn(t, app, sqliteInput("tx", path))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	runTxSteps(t, ds, conn.ID, []txStep{
		{label: "plain statement", stmts: []string{"SELECT 1"}, want: adapters.TxIdle},
		{label: "BEGIN", stmts: []string{"BEGIN"}, want: adapters.TxOpen},
		{label: "COMMIT", stmts: []string{"COMMIT"}, want: adapters.TxIdle},
		{label: "SAVEPOINT outside a transaction opens one", stmts: []string{"SAVEPOINT s1"}, want: adapters.TxOpen},
		{label: "RELEASE closes it", stmts: []string{"RELEASE s1"}, want: adapters.TxIdle},
		{label: "a failing statement keeps the transaction", stmts: []string{"BEGIN", "INSERT INTO seed VALUES (1)", "INSERT INTO seed VALUES (1)"}, want: adapters.TxOpen, fails: true, failIndex: 2},
		{label: "ROLLBACK", stmts: []string{"ROLLBACK"}, want: adapters.TxIdle},
	})
}
