package testsupport

import (
	"context"
	"database/sql"

	_ "github.com/go-sql-driver/mysql"
)

// seedMysqlFamilyDatabase runs seedSQL (one of the repo's own packages/db-fixtures/fixtures/*.sql
// files, unmodified) against dsn, shared by StartMariadb/StartMysql. MultiStatements=true (already
// set on dsn's own Config, mariadb.go's/mysql.go's) lets the whole seed file run as one Exec call,
// the same way Postgres's simple protocol runs a semicolon-separated body in one message.
func seedMysqlFamilyDatabase(ctx context.Context, dsn, seedSQL string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, seedSQL)
	return err
}
