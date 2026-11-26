//go:build !race

package dbmcpflow

import "time"

const (
	clientTimeout = 30 * time.Second
	queueHoldCTE  = mediumCTE
	slowRows      = 2_500_000
)
