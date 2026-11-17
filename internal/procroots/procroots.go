// Package procroots records the root pid of every live process tree this process launched, by
// group. Process-wide by nature (pids are); internal/metrics reads it.
//
// A root reaped but not yet untracked can have its pid reused for at most one metrics tick; a
// reused pid outside Kira's tree would count in the group for that tick. Accepted.
package procroots

import "sync"

// Set is one group's ref-counted root pids.
type Set struct {
	mu    sync.Mutex
	roots map[int32]int
}

var (
	Agents  = &Set{roots: map[int32]int{}} // agent CLI trees
	Scripts = &Set{roots: map[int32]int{}} // user script trees: script runs, pre-connect scripts
)

// Track registers pid as a root until the returned func runs. Ref-counted per pid; the returned
// func is idempotent.
func (s *Set) Track(pid int) (untrack func()) {
	p := int32(pid)
	s.mu.Lock()
	s.roots[p]++
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.roots[p] <= 1 {
				delete(s.roots, p)
				return
			}
			s.roots[p]--
		})
	}
}

// Roots returns a snapshot of the tracked root pids, in no particular order.
func (s *Set) Roots() []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int32, 0, len(s.roots))
	for p := range s.roots {
		out = append(out, p)
	}
	return out
}
