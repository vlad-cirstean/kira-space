package ade

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// orderCache holds the task order, tagged with the board generation it was built from. drop is a
// generation bump, so a board change never waits behind a build. A stale order is returned at once
// while one background build refreshes it; only the first call, with no order yet, waits.
type orderCache struct {
	gen atomic.Uint64
	// mu guards the fields below and is never held during a build.
	mu    sync.Mutex
	have  bool
	order []string
	at    time.Time
	day   string
	atGen uint64
	// flight is closed when the build that every blocked caller waits on ends.
	flight chan struct{}
	// refreshing is set while a background rebuild of a stale order runs.
	refreshing bool
}

func (c *orderCache) drop() { c.gen.Add(1) }

// get returns the order for day. A current order (this gen, under leaseOrderFresh old, same day) is
// returned as is. A stale one from the same day is returned too while spawn runs one rebuild. With
// no usable order the caller waits for a build. build reports false for a failure, which is neither
// kept nor retried here.
func (c *orderCache) get(now time.Time, day string, build func() ([]string, bool), spawn func(func())) []string {
	for {
		c.mu.Lock()
		gen := c.gen.Load()
		if c.have && c.day == day {
			if c.atGen == gen && now.Sub(c.at) < leaseOrderFresh {
				out := slices.Clone(c.order)
				c.mu.Unlock()
				return out
			}
			out := slices.Clone(c.order)
			if !c.refreshing {
				c.refreshing = true
				c.mu.Unlock()
				spawn(func() {
					order, ok := build()
					c.mu.Lock()
					c.refreshing = false
					if ok {
						c.store(order, now, day, gen)
					}
					c.mu.Unlock()
				})
				return out
			}
			c.mu.Unlock()
			return out
		}
		if c.flight != nil {
			wait := c.flight
			c.mu.Unlock()
			<-wait
			continue
		}
		c.flight = make(chan struct{})
		flight := c.flight
		c.mu.Unlock()
		order, ok := build()
		c.mu.Lock()
		if ok {
			c.store(order, now, day, gen)
		}
		c.flight = nil
		close(flight)
		c.mu.Unlock()
		if !ok {
			return nil
		}
		return slices.Clone(order)
	}
}

// store keeps order unless a newer generation's is already kept. The caller holds mu.
func (c *orderCache) store(order []string, now time.Time, day string, gen uint64) {
	if c.have && c.day == day && gen < c.atGen {
		return
	}
	c.order, c.at, c.day, c.atGen, c.have = order, now, day, gen, true
}
