package mysqlfamily

import (
	"errors"
	"strconv"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// mapError is errors.ts's mapError. B22 removes the two RSA-key branches (errno 45044/45063):
// go-sql-driver has no allowPublicKeyRetrieval gate to fail this specific way (§1.11).
func mapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	message := err.Error()

	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		switch myErr.Number {
		case 1045:
			return withErrno(adapters.New(adapters.CodeAuth, message, err), myErr.Number)
		case 1317:
			return withErrno(adapters.New(adapters.CodeCancelled, message, err), myErr.Number)
		case 1792, 1290:
			return withErrno(adapters.New(adapters.CodeQuery, message, adapters.RefusedWrite(err)), myErr.Number)
		}
		return withErrno(adapters.New(adapters.CodeQuery, message, err), myErr.Number)
	}

	if ne := adapters.ClassifyNetError(err); ne.DNS || ne.Refused || ne.NetTimeout {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}

// withErrno records the server error number as the `sqlErrno` detail. The console reads it to tell
// a statement the parser rejected (1064: no implicit commit ran) from one that failed later.
func withErrno(e *adapters.Error, number uint16) *adapters.Error {
	return e.WithDetails(`{"sqlErrno":` + strconv.Itoa(int(number)) + `}`)
}
