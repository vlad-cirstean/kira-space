package postgres

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// caps is caps.ts's postgresCaps, literally (§4.2).
// adapters.RelationalCaps (P107 I2-37) is that shared value.
var caps = adapters.RelationalCaps
