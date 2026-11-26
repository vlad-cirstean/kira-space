//go:build race

package dbmcpflow

import "time"

// Race detector slows pure-Go SQLite more than 10x (8M-row CTE: about 2 min). Client waits scale;
// queueHoldCTE and slowRows shrink so the queued read stays inside throttleMaxWait (30 s).
const (
	clientTimeout = 5 * time.Minute
	queueHoldCTE  = 1_000_000
	slowRows      = 250_000 // the throttle test's slow query, about 1 s without the detector
)
