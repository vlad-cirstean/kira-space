package mysql

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// caps is caps.ts's mysqlCaps — identical values to mariadbCaps (P34 D10).
// adapters.RelationalCaps (P107 I2-37) is that shared value.
var caps = adapters.RelationalCaps
