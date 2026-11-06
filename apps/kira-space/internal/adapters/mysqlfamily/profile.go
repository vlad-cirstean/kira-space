// Package mysqlfamily is the Go analogue of src/engine/adapters/mysql-family/: database/sql plus
// github.com/go-sql-driver/mysql, one *sql.DB per (connection, database) with
// SetMaxOpenConns(1) and one pinned *sql.Conn. File-for-file with the TypeScript it replaces:
// index.ts -> adapter.go, every other file keeps its TS name.
package mysqlfamily

import (
	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// LogFunc is adapters.Deps's own Log signature, named locally so profile.go need not import
// the adapters package just for this one type.
type LogFunc func(level, message string)

// Profile is profile.ts's MysqlFamilyProfile. Three fields, deliberately — "a profile field must
// change observable behaviour, or it does not exist" holds: MySQL's own ApplyEngineOptions is empty
// (a real capability loss, not an oversight — see mysql/profile.go), which is worth a comment
// rather than deleting the field.
type Profile struct {
	// Kind is the adapter's own kind, surfaced as Adapter.Kind() and used in log lines.
	Kind string
	// ServerLabel prefixes ConnectInfo.ServerVersion: "MariaDB"/"MySQL" (D6).
	ServerLabel string
	// ApplyEngineOptions applies engine-specific connection options, after the shared host/port/
	// user/ssl handling. MariaDB's is a no-op; MySQL's used to read allowPublicKeyRetrieval off
	// cfg.Options (P34 D3) — removed under go-sql-driver, see mysql/profile.go (B22).
	ApplyEngineOptions func(cfg *mysql.Config, resolved model.ResolvedConnectionConfig, log LogFunc)
}
