package sqlite

import (
	"database/sql"
	"database/sql/driver"
	"errors"

	sqlitedriver "modernc.org/sqlite"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
)

// SQLite's own primary result codes (sqlite3.h) — an extended code (e.g. 2067 for
// SQLITE_CONSTRAINT_UNIQUE) always has the primary code in its low byte, exactly as
// errors.ts's own mapError already assumed; modernc.org/sqlite's *Error.Code() is re-derived here
// against the real driver, not assumed to still return the same shape node:sqlite did.
const (
	cantOpen = 14
	notADB   = 26
	busy     = 5
	locked   = 6
	readOnly = 8
)

// mapError is errors.ts's mapError, the discipline unchanged: classify by the driver's own
// numeric code, never by sniffing message text. CONSTRAINT (19) and plain ERROR (1) fall through
// to E_QUERY with SQLite's own message verbatim (Adapter rule 4) — its constraint messages
// ("UNIQUE constraint failed: t.a") are already better than anything a wrapper would compose.
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var ae *adapters.Error
	if errors.As(err, &ae) {
		return ae
	}

	var se *sqlitedriver.Error
	if errors.As(err, &se) {
		switch se.Code() & 0xff {
		case cantOpen, notADB:
			return adapters.New(adapters.CodeConnect, se.Error(), err)
		case busy, locked:
			return adapters.New(adapters.CodeTimeout, se.Error(), err)
		case readOnly:
			return adapters.New(adapters.CodeUnsupported, se.Error(), adapters.RefusedWrite(err))
		}
		return adapters.New(adapters.CodeQuery, se.Error(), err)
	}

	// database/sql's own "the handle/conn/tx is already gone" sentinels — the Go analogue of
	// node:sqlite's ERR_INVALID_STATE (a statement used after the database closed).
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrTxDone) || errors.Is(err, driver.ErrBadConn) {
		return adapters.New(adapters.CodeConnect, err.Error(), err)
	}

	return adapters.New(adapters.CodeQuery, err.Error(), err)
}
