package testsupport

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	sqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmariadb "github.com/testcontainers/testcontainers-go/modules/mariadb"
)

// MariaFixture is support/mariadb.ts's MariaFixture — an alias of mysqlFamilyFixture (P107
// T2-5), shared with MysqlFixture.
type MariaFixture = mysqlFamilyFixture

var mariaMemo fixture[MariaFixture]

const (
	mariaImage        = "mariadb:11.4.13"
	mariaRootPassword = FixturePassword
	mariaDatabase     = "kira_test"
	mariaAnalyticsDB  = "kira_analytics"
	mariaUsername     = "kira"
	mariaPassword     = FixturePassword
	mariaBigRowsCount = 1_000_000
)

// StartMariadb is support/mariadb.ts's startMariadb. See fixture.go's own doc comment for why
// termination is never wired to t.Cleanup (B15).
func StartMariadb(t *testing.T) *MariaFixture {
	return mariaMemo.start(t, "mariadb container", startMariadb)
}

// StopMariadb terminates the memoized container, if one was ever started. Call once, from the test
// binary's own TestMain, after m.Run() returns — never from an individual test.
func StopMariadb() {
	mariaMemo.stop(func(f *MariaFixture) { _ = f.container.Terminate(context.Background()) })
}

func startMariadb() (*MariaFixture, error) {
	return startMysqlFamily(mysqlFamilySpec{
		runContainer: func(ctx context.Context) (testcontainers.Container, error) {
			// performance_schema is off by default on MariaDB (unlike MySQL) — needed so a real
			// "attribute is gone after disconnect" acceptance case can query
			// performance_schema.SESSION_CONNECT_ATTRS (B23).
			return tcmariadb.Run(ctx, ImageFor("mariadb", mariaImage),
				tcmariadb.WithDatabase(mariaDatabase),
				tcmariadb.WithUsername(mariaUsername),
				tcmariadb.WithPassword(mariaPassword),
				testcontainers.WithEnv(map[string]string{"MARIADB_ROOT_PASSWORD": mariaRootPassword}),
				testcontainers.WithCmdArgs("--performance-schema=ON"),
			)
		},
		seedFixture: "0002_mariadb_seed.sql",
		dsn:         rootMariaDSN,
		seedExtras:  seedMariadbExtras,
		database:    mariaDatabase,
		username:    mariaUsername,
		password:    mariaPassword,
		uriScheme:   "mariadb",
		configID:    "test-mariadb",
		configName:  "Test MariaDB",
		configKind:  "mariadb",
		configColor: "blue",
		options:     map[string]any{},
	})
}

func rootMariaDSN(host string, port int, database string) string {
	cfg := sqldriver.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = "root"
	cfg.Passwd = mariaRootPassword
	cfg.DBName = database
	cfg.MultiStatements = true
	return cfg.FormatDSN()
}

// RootMariaDSN is rootMariaDSN, exported for tests that need DDL rights the connected `kira` user
// deliberately lacks (P2 R2) — e.g. creating a probe table in kira_analytics, where kira only ever
// holds the GRANT SELECT seedMariadbExtras gives it below.
func RootMariaDSN(host string, port int, database string) string {
	return rootMariaDSN(host, port, database)
}

// seedMariadbExtras is support/mariadb.ts's own root-connection block: the second database
// (mirroring Postgres's analytics schema — MariaDB has no schema level, §6d), the GRANT, and the
// big_rows bulk insert via the SEQUENCE engine's own seq_1_to_N pseudo-table, then ANALYZE TABLE on every table.
func seedMariadbExtras(ctx context.Context, host string, port int) error {
	db, err := sql.Open("mysql", rootMariaDSN(host, port, mariaDatabase))
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+mariaAnalyticsDB+"`"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `"+mariaAnalyticsDB+"`.events ("+
		"id INT AUTO_INCREMENT PRIMARY KEY, event_name VARCHAR(255) NOT NULL, "+
		"occurred_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO `"+mariaAnalyticsDB+"`.events (event_name) VALUES ('signup'), ('login')"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "GRANT SELECT ON `"+mariaAnalyticsDB+"`.* TO '"+mariaUsername+"'@'%'"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "FLUSH PRIVILEGES"); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO `%s`.big_rows (id, payload) SELECT seq, MD5(seq) FROM seq_1_to_%d", mariaDatabase, mariaBigRowsCount),
	); err != nil {
		return err
	}
	return analyzeMysqlTables(ctx, db, mariaDatabase, mariaAnalyticsDB)
}
