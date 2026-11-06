package adapters

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// DefaultConsoleCap bounds one console batch. 10,000 rows matches the data tab's largest page
// size. The byte budget is shared by every statement of a batch (RemainingCap) and sits below the
// response limit adapterhost enforces: 56 MiB x 17/16 (encoding overhead) = 59.5 MiB, leaving the
// rest of the 64 MiB data frame for the envelope and per-cell offsets.
var DefaultConsoleCap = page.ResultCap{Rows: 10_000, Bytes: 56 << 20}

// ConsoleCapFor resolves req's cap: a zero field takes the default, a field above the default is
// clamped down to it.
func ConsoleCapFor(req model.ConsoleRequest) page.ResultCap {
	c := req.Cap
	if c.Rows <= 0 || c.Rows > DefaultConsoleCap.Rows {
		c.Rows = DefaultConsoleCap.Rows
	}
	if c.Bytes <= 0 || c.Bytes > DefaultConsoleCap.Bytes {
		c.Bytes = DefaultConsoleCap.Bytes
	}
	return c
}

// RemainingCap charges used bytes against c's byte budget for the next statement of the batch.
// The floor is 1, since a zero field means "no limit": an exhausted budget still admits one row
// and then reports the statement truncated.
func RemainingCap(c page.ResultCap, used int) page.ResultCap {
	if c.Bytes > 0 {
		c.Bytes = max(c.Bytes-used, 1)
	}
	return c
}
