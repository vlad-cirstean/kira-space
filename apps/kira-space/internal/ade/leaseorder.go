package ade

import (
	"sync"
	"sync/atomic"
	"time"
)

// orderCache holds the task order for leaseOrderFresh. drop is a generation bump, so a board change
// never waits behind a build; a build that a drop overtook is returned to its caller but not kept.
type orderCache struct {
	gen atomic.Uint64
	// mu collapses concurrent builds and guards the entry.
	mu    sync.Mutex
	order []string
	at    time.Time
	day   string
	atGen uint64
}

func (c *orderCache) drop() { c.gen.Add(1) }

// get returns the kept order when it is fresh, from today and of the current generation, else the
// order build makes. build reports false for a failure, which is neither kept nor retried here.
func (c *orderCache) get(now time.Time, day string, build func() ([]string, bool)) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	gen := c.gen.Load()
	if c.atGen == gen && c.day == day && !c.at.IsZero() && now.Sub(c.at) < leaseOrderFresh {
		return c.order
	}
	order, ok := build()
	if !ok {
		return nil
	}
	if c.gen.Load() == gen {
		c.order, c.at, c.day, c.atGen = order, now, day, gen
	}
	return order
}
