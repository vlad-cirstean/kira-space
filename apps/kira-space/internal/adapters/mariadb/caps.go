package mariadb

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// caps is caps.ts's mariadbCaps, literally — identical to Postgres's and to mysql's own (P34 D10:
// stated per engine rather than shared, so a future divergence has somewhere honest to be said).
// adapters.RelationalCaps (P107 I2-37) is that shared value.
var caps = adapters.RelationalCaps
