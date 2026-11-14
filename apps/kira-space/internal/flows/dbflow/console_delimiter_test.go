package dbflow

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	sqldriver "github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// delimiterScript is the J-CONS-028 script: trigger and function bodies under custom delimiters, a
// `;` and `$$` inside quotes and a comment. delimiterStatements is what the console's splitter
// makes of it (the UI half asserts the same list from the typed text): no DELIMITER line, no `$$`
// or `//` terminator.
const delimiterScript = `-- P636: trigger and function bodies under custom delimiters
DROP TABLE IF EXISTS kira_delim;
DROP FUNCTION IF EXISTS kira_delim_twice;
CREATE TABLE kira_delim (id INT PRIMARY KEY AUTO_INCREMENT, body VARCHAR(64));
DELIMITER $$
CREATE TRIGGER kira_delim_bi BEFORE INSERT ON kira_delim FOR EACH ROW
BEGIN
  IF NEW.body = 'a;b' THEN
    SET NEW.body = CONCAT(NEW.body, '$$'); -- '$$' quoted, no boundary
  END IF;
END$$
DELIMITER //
CREATE FUNCTION kira_delim_twice(x INT) RETURNS INT DETERMINISTIC
BEGIN
  DECLARE y INT;
  SET y = x * 2;
  RETURN y;
END //
DELIMITER ;
INSERT INTO kira_delim (body) VALUES ('a;b'), ('c');
SELECT body, kira_delim_twice(id) AS twice FROM kira_delim ORDER BY id;
SELECT trigger_name FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'kira_delim_bi'`

var delimiterStatements = []string{
	"-- P636: trigger and function bodies under custom delimiters\nDROP TABLE IF EXISTS kira_delim",
	"DROP FUNCTION IF EXISTS kira_delim_twice",
	"CREATE TABLE kira_delim (id INT PRIMARY KEY AUTO_INCREMENT, body VARCHAR(64))",
	"CREATE TRIGGER kira_delim_bi BEFORE INSERT ON kira_delim FOR EACH ROW\nBEGIN\n  IF NEW.body = 'a;b' THEN\n    SET NEW.body = CONCAT(NEW.body, '$$'); -- '$$' quoted, no boundary\n  END IF;\nEND",
	"CREATE FUNCTION kira_delim_twice(x INT) RETURNS INT DETERMINISTIC\nBEGIN\n  DECLARE y INT;\n  SET y = x * 2;\n  RETURN y;\nEND",
	"INSERT INTO kira_delim (body) VALUES ('a;b'), ('c')",
	"SELECT body, kira_delim_twice(id) AS twice FROM kira_delim ORDER BY id",
	"SELECT trigger_name FROM information_schema.triggers WHERE trigger_schema = DATABASE() AND trigger_name = 'kira_delim_bi'",
}

// dumpScript is the trigger section mysqldump and mariadb-dump emit: executable `/*! */` comments,
// `DELIMITER ;;`, a `;;` boundary with the body's inner `;` inside comment spans, a leading banner
// and a closing `-- Dump completed` line. `~` stands for a backtick.
var dumpScript = strings.ReplaceAll(`-- Dump of kira_dump
DROP TABLE IF EXISTS ~kira_dump~;
CREATE TABLE ~kira_dump~ (id INT PRIMARY KEY AUTO_INCREMENT, body VARCHAR(64));
/*!50003 SET @saved_cs_client      = @@character_set_client */ ;
DELIMITER ;;
/*!50003 CREATE*/ /*!50017 DEFINER=CURRENT_USER*/ /*!50003 TRIGGER ~kira_dump_bi~ BEFORE INSERT ON ~kira_dump~ FOR EACH ROW BEGIN
  SET NEW.body = CONCAT(NEW.body, ';');
END */;;
DELIMITER ;
/*!50003 SET character_set_client  = @saved_cs_client */ ;
-- Dump completed on 2026-01-01  0:00:00`, "~", "`")

var dumpStatements = []string{
	strings.ReplaceAll("-- Dump of kira_dump\nDROP TABLE IF EXISTS ~kira_dump~", "~", "`"),
	strings.ReplaceAll("CREATE TABLE ~kira_dump~ (id INT PRIMARY KEY AUTO_INCREMENT, body VARCHAR(64))", "~", "`"),
	"/*!50003 SET @saved_cs_client      = @@character_set_client */",
	strings.ReplaceAll("/*!50003 CREATE*/ /*!50017 DEFINER=CURRENT_USER*/ /*!50003 TRIGGER ~kira_dump_bi~ BEFORE INSERT ON ~kira_dump~ FOR EACH ROW BEGIN\n  SET NEW.body = CONCAT(NEW.body, ';');\nEND */", "~", "`"),
	"/*!50003 SET character_set_client  = @saved_cs_client */",
}

// rootDB opens a client of its own as root. Binary logging makes CREATE TRIGGER and CREATE FUNCTION
// need SUPER for the fixture user (server rule), so the script runs as root too; the shared
// fixture's log_bin_trust_function_creators stays as it is.
func rootDB(t *testing.T, fx *testsupport.MysqlFixture) *sql.DB {
	t.Helper()
	cfg := sqldriver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", *fx.Config.Host, *fx.Config.Port)
	cfg.User = "root"
	cfg.Passwd = testsupport.FixturePassword
	cfg.DBName = *fx.Config.Database
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func rootRows(t *testing.T, db *sql.DB, query string) [][]any {
	t.Helper()
	return scanRows(t, query, func() (*sql.Rows, error) { return db.Query(query) })
}

// J-CONS-028 (contract studio-console-script): a script with DELIMITER lines runs as the statement
// list the console's splitter makes of it, and a second client proves the trigger and function are
// real. The UI half types the script and asserts the list it sends equals this one. MariaDB runs
// the same list; its contract keys are MySQL's.
func TestConsoleMySQLDelimiter(t *testing.T) {
	flowharness.RequireDocker(t)
	runConsoleDelimiter(t, testsupport.StartMysql(t), true)
}

func TestConsoleMariaDBDelimiter(t *testing.T) {
	flowharness.RequireDocker(t)
	runConsoleDelimiter(t, testsupport.StartMariadb(t), false)
}

func runConsoleDelimiter(t *testing.T, fx *testsupport.MysqlFixture, writeContract bool) {
	root := rootDB(t, fx)
	t.Cleanup(func() {
		for _, q := range []string{"DROP TABLE IF EXISTS kira_delim, kira_dump", "DROP FUNCTION IF EXISTS kira_delim_twice"} {
			_, _ = root.Exec(q)
		}
	})
	cfg := fx.Config
	user, pass := "root", testsupport.FixturePassword
	cfg.Username, cfg.Password = &user, &pass
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(cfg))
	connect(t, app, conn.ID)
	ds := app.DataStream(t)
	contract := func(key string, v any) {
		if writeContract {
			app.Contract(t, consoleScriptScenario, key, v)
		}
	}

	contract("mysql#delimiterScript", delimiterScript)
	contract("mysql#delimiterStatements", delimiterStatements)
	pages, err := ds.Execute(conn.ID, "", delimiterStatements...)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(pages) != len(delimiterStatements) {
		t.Fatalf("%d pages for %d statements", len(pages), len(delimiterStatements))
	}
	contract("mysql#delimiterPages", summaries(pages))

	// The trigger fired and kept its `$$`, the function runs, and both are real objects.
	rows := rootRows(t, root, `select body, kira_delim_twice(id) from kira_delim order by id`)
	wantRows(t, "kira_delim", rows, [][]any{{"a;b$$", "2"}, {"c", "4"}})
	body := rootRows(t, root, `select action_statement from information_schema.triggers where trigger_schema = database() and trigger_name = 'kira_delim_bi'`)
	if len(body) != 1 || !strings.Contains(body[0][0].(string), "IF NEW.body") {
		t.Fatalf("trigger kira_delim_bi = %v, want its whole body", body)
	}
	wantRows(t, "function", rootRows(t, root, `select routine_name from information_schema.routines where routine_schema = database() and routine_name = 'kira_delim_twice'`), [][]any{{"kira_delim_twice"}})
	contract("mysql#delimiterEffects", rows)

	t.Run("dump", func(t *testing.T) {
		contract("mysql#dumpScript", dumpScript)
		contract("mysql#dumpStatements", dumpStatements)
		pages, err := ds.Execute(conn.ID, "", dumpStatements...)
		if err != nil {
			t.Fatalf("Execute dump: %v", err)
		}
		if len(pages) != len(dumpStatements) {
			t.Fatalf("%d pages for %d statements", len(pages), len(dumpStatements))
		}
		if _, err := root.Exec("INSERT INTO kira_dump (body) VALUES ('x')"); err != nil {
			t.Fatal(err)
		}
		wantRows(t, "kira_dump", rootRows(t, root, `select body from kira_dump`), [][]any{{"x;"}})
		wantRows(t, "dump trigger", rootRows(t, root, `select trigger_name from information_schema.triggers where trigger_schema = database() and trigger_name = 'kira_dump_bi'`), [][]any{{"kira_dump_bi"}})
	})
}
