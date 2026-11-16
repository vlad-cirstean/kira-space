// Package agentproc records the root pid of every live agent process tree this process launched.
// Process-wide by nature (pids are), so one package-level set; internal/metrics reads it.
//
// A root reaped but not yet untracked can have its pid reused for at most one metrics tick; a
// reused pid outside Kira's tree would count as an agent for that tick. Accepted.
package agentproc

import (
	"sync"
)

var (
	mu    sync.Mutex
	roots = map[int32]int{}
)

// Track registers pid as an agent root until the returned func runs. Ref-counted per pid; the
// returned func is idempotent.
func Track(pid int) (untrack func()) {
	p := int32(pid)
	mu.Lock()
	roots[p]++
	mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()
			if roots[p] <= 1 {
				delete(roots, p)
				return
			}
			roots[p]--
		})
	}
}

// Roots returns a snapshot of the tracked root pids, in no particular order.
func Roots() []int32 {
	mu.Lock()
	defer mu.Unlock()
	out := make([]int32, 0, len(roots))
	for p := range roots {
		out = append(out, p)
	}
	return out
}
