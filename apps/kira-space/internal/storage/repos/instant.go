package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// isoScan scans an INTEGER unix-ms column into an ISO string; NULL gives "".
type isoScan struct{ dst *string }

func (s isoScan) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		*s.dst = kiratime.MsToISO(v)
	case nil:
		*s.dst = ""
	default:
		return fmt.Errorf("repos: instant column holds %T, want INTEGER", src)
	}
	return nil
}

func msISO(dst *string) sql.Scanner { return isoScan{dst} }
