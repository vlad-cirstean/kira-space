//go:build race

package jsengine

// The race detector slows a worker's start and run by an order of magnitude, and hard memory
// exits can pre-empt the positioned soft interrupt. Timing bounds scale; position checks on
// memory errors are skipped.
const (
	timeScale   = 6
	raceEnabled = true
)
