package testsupport

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// MysqlFixture is support/mysql.ts's MysqlFixture — an alias of mysqlFamilyFixture (P107 T2-5),
// shared with MariaFixture.
type MysqlFixture = mysqlFamilyFixture

var mysqlMemo fixture[MysqlFixture]

const (
	mysqlImage        = "mysql:8.4.11"
	mysqlRootPassword = FixturePassword
	mysqlDatabase     = "kira_test"
	mysqlAnalyticsDB  = "kira_analytics"
	mysqlUsername     = "kira"
	mysqlPassword     = FixturePassword
)

// StartMysql is support/mysql.ts's startMysql. See fixture.go's own doc comment for why
// termination is never wired to t.Cleanup (B15).
func StartMysql(t *testing.T) *MysqlFixture {
	return mysqlMemo.start(t, "mysql container", startMysql)
}

// StopMysql terminates the memoized container, if one was ever started. Call once, from the test
// binary's own TestMain, after m.Run() returns — never from an individual test.
func StopMysql() {
	mysqlMemo.stop(func(f *MysqlFixture) { _ = f.container.Terminate(context.Background()) })
}

func startMysql() (*MysqlFixture, error) {
	return startMysqlFamily(mysqlFamilySpec{
		runContainer: func(ctx context.Context) (testcontainers.Container, error) {
			return tcmysql.Run(ctx, ImageFor("mysql", mysqlImage),
				tcmysql.WithDatabase(mysqlDatabase),
				tcmysql.WithUsername(mysqlUsername),
				tcmysql.WithPassword(mysqlPassword),
				testcontainers.WithEnv(map[string]string{"MYSQL_ROOT_PASSWORD": mysqlRootPassword}),
			)
		},
		seedFixture: "0008_mysql_seed.sql",
		dsn:         rootMysqlDSN,
		seedExtras:  seedMysqlExtras,
		database:    mysqlDatabase,
		username:    mysqlUsername,
		password:    mysqlPassword,
		uriScheme:   "mysql",
		configID:    "test-mysql",
		configName:  "Test MySQL",
		configKind:  "mysql",
		configColor: "teal",
		// P34 D5/D26: TLS is available on a stock server (MySQL auto-generates a self-signed
		// certificate at init) — this exercises the real remedy path, not a worked-around one.
		options: map[string]any{"sslmode": "require"},
	})
}

func rootMysqlDSN(host string, port int, database string) string {
	cfg := sqldriver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = "root"
	cfg.Passwd = mysqlRootPassword
	cfg.DBName = database
	cfg.MultiStatements = true
	cfg.AllowNativePasswords = true
	return cfg.FormatDSN()
}

// RootMysqlDSN is rootMysqlDSN, exported for tests that need DDL rights the connected `kira` user
// deliberately lacks (P2 R2) — e.g. creating a probe table in kira_analytics, where kira only ever
// holds the GRANT SELECT seedMysqlExtras gives it above.
func RootMysqlDSN(host string, port int, database string) string {
	return rootMysqlDSN(host, port, database)
}

// seedMysqlExtras is support/mysql.ts's own root-connection block: the second database (mirroring
// MariaDB's kira_analytics) and the big_rows bulk insert. MySQL has no SEQUENCE
// storage engine, so the 1,000,000-row insert uses the conventional numbers-table idiom instead —
// a six-way cross join over a 10-row digits table, chunked into ten 100,000-row statements so any
// single InnoDB transaction stays bounded.
func seedMysqlExtras(ctx context.Context, host string, port int) error {
	db, err := sql.Open("mysql", rootMysqlDSN(host, port, mysqlDatabase))
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+mysqlAnalyticsDB+"`"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `"+mysqlAnalyticsDB+"`.events ("+
		"id INT AUTO_INCREMENT PRIMARY KEY, event_name VARCHAR(255) NOT NULL, "+
		"occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO `"+mysqlAnalyticsDB+"`.events (event_name) VALUES ('signup'), ('login')"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "GRANT SELECT ON `"+mysqlAnalyticsDB+"`.* TO '"+mysqlUsername+"'@'%'"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return err
	}

	// A plain table, not TEMPORARY: MySQL's TEMPORARY tables can't be referenced more than once in
	// the same query, and the six-way self-join below does exactly that. Dropped once done.
	if _, err := db.ExecContext(ctx, "CREATE TABLE digits (d INT PRIMARY KEY)"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO digits (d) VALUES (0),(1),(2),(3),(4),(5),(6),(7),(8),(9)"); err != nil {
		return err
	}
	for outer := 0; outer < 10; outer++ {
		// MySQL 9.x removed the native MD5()/SHA1() functions outright (verified
		// against mysql:9.7 — "FUNCTION kira_test.MD5 does not exist", MySQL error 1305, the same
		// message a missing stored routine gets). SHA2() has existed since MySQL 5.5.3 and is
		// unaffected; LEFT(…, 32) keeps big_rows.payload at its existing CHAR(32) width rather than
		// widening the schema for a value nothing reads back for its hash properties.
		if _, err := db.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO `+"`%s`"+`.big_rows (id, payload)
			 SELECT d1.d*100000 + d2.d*10000 + d3.d*1000 + d4.d*100 + d5.d*10 + d6.d + 1 AS id,
			        LEFT(SHA2(d1.d*100000 + d2.d*10000 + d3.d*1000 + d4.d*100 + d5.d*10 + d6.d + 1, 256), 32) AS payload
			 FROM digits d1, digits d2, digits d3, digits d4, digits d5, digits d6
			 WHERE d1.d = ?`, mysqlDatabase), outer,
		); err != nil {
			return err
		}
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE digits"); err != nil {
		return err
	}
	return analyzeMysqlTables(ctx, db, mysqlDatabase, mysqlAnalyticsDB)
}

func quoteMysqlIdent(name string) string { return "`" + strings.ReplaceAll(name, "`", "``") + "`" }

// analyzeMysqlTables (MySQL and MariaDB) computes statistics for every base table up front. InnoDB recalculates them in
// a background thread, so under load a fixture read could see "~0" for a freshly seeded table,
// and information_schema caches that estimate.
func analyzeMysqlTables(ctx context.Context, db *sql.DB, schemas ...string) error {
	rows, err := db.QueryContext(ctx, "SELECT table_schema, table_name FROM information_schema.tables "+
		"WHERE table_type = 'BASE TABLE' AND table_schema IN (?, ?)", schemas[0], schemas[1])
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			_ = rows.Close()
			return err
		}
		tables = append(tables, quoteMysqlIdent(schema)+"."+quoteMysqlIdent(name))
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, t := range tables {
		res, err := db.QueryContext(ctx, "ANALYZE TABLE "+t)
		if err != nil {
			return err
		}
		if err := res.Close(); err != nil {
			return err
		}
	}
	return nil
}
