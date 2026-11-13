package ade

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// historyMemoTTL bounds how long the history wire is reused. Writes the engine makes to archived
// rows bump the gen at once; the TTL covers rows changed behind its back (a repo removed elsewhere).
const historyMemoTTL = 30 * time.Second

// historyMemo caches the board's history entries: archived rows change only on archive, restore and
// task edits, yet every board build read and mapped them all.
type historyMemo struct {
	gen atomic.Uint64

	mu      sync.Mutex
	built   uint64
	at      time.Time
	entries []adewire.HistoryEntry
	ok      bool
}

// touchHistory drops the memo; call it after any write that can change an archived task or its branches.
func (b *TaskBoard) touchHistory() { b.history.gen.Add(1) }

// historyEntries returns the history wire, reading the archived rows only on a memo miss. The slice
// is shared read-only, like the board snapshot that carries it.
func (b *TaskBoard) historyEntries() ([]adewire.HistoryEntry, error) {
	h := &b.history
	gen := h.gen.Load()
	h.mu.Lock()
	if h.ok && h.built == gen && time.Since(h.at) < historyMemoTTL {
		out := h.entries
		h.mu.Unlock()
		return out, nil
	}
	h.mu.Unlock()
	archived, err := b.deps.Tasks.ListArchived()
	if err != nil {
		return nil, err
	}
	branches, err := b.deps.Tasks.BranchesArchived()
	if err != nil {
		return nil, err
	}
	out := toWireHistory(archived, branches)
	h.mu.Lock()
	// Keep the entry only if no write landed while it was read.
	if h.gen.Load() == gen {
		h.built, h.at, h.entries, h.ok = gen, time.Now(), out, true
	}
	h.mu.Unlock()
	return out, nil
}
