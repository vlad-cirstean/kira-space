package repos

// rowScanner is *sql.Row and *sql.Rows' shared Scan surface, so one scan function serves both a
// QueryRow result and a Query row-by-row loop.
type rowScanner interface {
	Scan(dest ...any) error
}
