// Package mariadb is the Go analogue of src/engine/adapters/mariadb/ (P34 D7/D9): the MariaDB
// profile for mysqlfamily.Adapter. Everything else lives once in mysqlfamily/.
package mariadb

import (
	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/mysqlfamily"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// applyEngineOptions is client.ts's applyEngineOptions — MariaDB has no auth plugin that needs an
// engine-specific connection option, so this is a no-op (compare mysql/client.go's own, which is
// also empty after B22, for a different reason).
func applyEngineOptions(_ *mysql.Config, _ model.ResolvedConnectionConfig, _ mysqlfamily.LogFunc) {}
