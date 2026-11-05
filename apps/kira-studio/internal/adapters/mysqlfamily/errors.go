package mysqlfamily

import (
	"errors"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
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
			return adapters.New(adapters.CodeAuth, message, err)
		case 1317:
			return adapters.New(adapters.CodeCancelled, message, err)
		case 1792, 1290:
			return adapters.New(adapters.CodeQuery, message, adapters.RefusedWrite(err))
		}
		return adapters.New(adapters.CodeQuery, message, err)
	}

	if ne := adapters.ClassifyNetError(err); ne.DNS || ne.Refused || ne.NetTimeout {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}
