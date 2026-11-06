// Package mysql is the Go analogue of src/engine/adapters/mysql/ (P34 D3/D7): the MySQL profile
// for mysqlfamily.Adapter. Everything else lives once in mysqlfamily/.
package mysql

import (
	sqldriver "github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mysqlfamily"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// applyEngineOptions is client.ts's applyEngineOptions — but for a real reason it can no
// longer do anything under go-sql-driver/mysql: auth.go's public-key retrieval path has no
// AllowPublicKeyRetrieval-style gate at all — it requests the server's RSA public key over
// plaintext unconditionally whenever caching_sha2_password needs it, so this option would be a
// no-op that reads like a real control. Recorded as a capability loss in
// docs/ARCHITECTURE.md's per-engine section and CLAUDE.md's findings, not silently dropped.
func applyEngineOptions(_ *sqldriver.Config, _ model.ResolvedConnectionConfig, _ mysqlfamily.LogFunc) {
}
