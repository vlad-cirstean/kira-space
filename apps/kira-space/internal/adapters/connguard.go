package adapters

import (
	"context"
	"sync"
	"time"
)

// ConnGuard serialises use of one pinned database connection for postgres and mysqlfamily: a
// 1-slot semaphore (acquired with a ctx, so a queued op can be cancelled), the count of
// RunWithAbortRace goroutines still touching the connection, and the count of server-side cancels
// still aimed at it. The zero value is ready to use.
type ConnGuard struct {
	once sync.Once
	sem  chan struct{}

	m        sync.Mutex
	cond     *sync.Cond
	inFlight int
	cancels  int
}

func (g *ConnGuard) init() {
	g.once.Do(func() {
		g.sem = make(chan struct{}, 1)
		g.cond = sync.NewCond(&g.m)
	})
}

// Lock takes the connection, or fails with E_CANCELLED once ctx is done while queued.
func (g *ConnGuard) Lock(ctx context.Context) error {
	g.init()
	select {
	case g.sem <- struct{}{}:
		return nil
	default:
	}
	select {
	case g.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return CheckCancelled(ctx)
	}
}

// Unlock frees the connection. Prefer Release from an op's own cleanup.
func (g *ConnGuard) Unlock() {
	g.init()
	<-g.sem
}

// Track registers one RunWithAbortRace goroutine touching the connection. Call synchronously,
// before the goroutine starts; the returned done runs once it settles.
func (g *ConnGuard) Track() (done func()) {
	g.init()
	g.m.Lock()
	g.inFlight++
	g.m.Unlock()
	return func() {
		g.m.Lock()
		g.inFlight--
		if g.inFlight == 0 {
			g.cond.Broadcast()
		}
		g.m.Unlock()
	}
}

// WaitInFlight blocks until every tracked goroutine has settled.
func (g *ConnGuard) WaitInFlight() {
	g.init()
	g.m.Lock()
	for g.inFlight > 0 {
		g.cond.Wait()
	}
	g.m.Unlock()
}

// BeginCancel records a server-side cancel about to be aimed at the query running on this
// connection. Call it in the same locked step that pops the running query, so the op's Release
// cannot slip past it.
func (g *ConnGuard) BeginCancel() {
	g.init()
	g.m.Lock()
	g.cancels++
	g.m.Unlock()
}

// EndCancel pairs BeginCancel once the cancel has been delivered or abandoned.
func (g *ConnGuard) EndCancel() {
	g.init()
	g.m.Lock()
	g.cancels--
	if g.cancels == 0 {
		g.cond.Broadcast()
	}
	g.m.Unlock()
}

func (g *ConnGuard) waitIdle() {
	g.m.Lock()
	for g.inFlight > 0 || g.cancels > 0 {
		g.cond.Wait()
	}
	g.m.Unlock()
}

// Release ends an op's use of the connection. The connection stays held until abandoned background
// queries and any cancel still in flight have finished, or that cancel could land on the next op's
// query on the same backend. When something is still pending the wait runs in the background, so
// the op itself (a Stop on it, a Disconnect) never blocks on a hung query; later ops queue on Lock.
func (g *ConnGuard) Release() {
	g.init()
	g.m.Lock()
	busy := g.inFlight > 0 || g.cancels > 0
	g.m.Unlock()
	if !busy {
		g.Unlock()
		return
	}
	go func() {
		g.waitIdle()
		g.Unlock()
	}()
}

// CloseWithin runs graceful once the connection is free, or force when ctx ends or grace passes
// first. force must unblock a stuck reader (close the socket); the holder's op then fails and
// releases on its own.
func (g *ConnGuard) CloseWithin(ctx context.Context, grace time.Duration, graceful, force func()) {
	g.init()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
		graceful()
	case <-ctx.Done():
		force()
	case <-timer.C:
		force()
	}
}
