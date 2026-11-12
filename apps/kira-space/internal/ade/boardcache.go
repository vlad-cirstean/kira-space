package ade

import (
	"context"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
)

// boardCache holds the shared Board snapshot, the in-flight builds and the push coalescer.
//
// gen counts board invalidations. A snapshot or build belongs to the gen read when it started, so a
// reader arriving after an invalidation never sees state older than the write that caused it.
type boardCache struct {
	gen atomic.Uint64

	mu      sync.Mutex
	snap    *boardSnapshot
	flights map[uint64]*boardFlight

	pushMu    sync.Mutex
	push      boardChanges
	pushTimer *time.Timer

	// afterLoad runs inside a build after the store read; tests stall a build with it.
	afterLoad func()
}

type boardSnapshot struct {
	gen   uint64
	at    time.Time
	board adewire.Board
}

// boardFlight is one build every caller of its gen joins.
type boardFlight struct {
	done  chan struct{}
	board adewire.Board
	err   error
}

const (
	// boardSnapshotFresh bounds how long an unchanged snapshot is served: git moves that no write of
	// ours announced (an external commit the watcher is slow to report) show up within it.
	boardSnapshotFresh = 2 * time.Second
	// boardPushWindow coalesces the board-changed pushes of a burst of writes into one.
	boardPushWindow = 30 * time.Millisecond
)

// allChanged is the bare notification: refs may have changed and any task may have.
var allChanged = adewire.BoardChangedEvent{Refs: true}

// boardChanges accumulates BoardChangedEvents; the zero value holds nothing.
type boardChanges struct {
	any   bool
	refs  bool
	all   bool // some merged change had unknown tasks
	tasks map[string]struct{}
}

func (c *boardChanges) merge(ev adewire.BoardChangedEvent) {
	c.any = true
	c.refs = c.refs || ev.Refs
	if len(ev.TaskIDs) == 0 {
		c.all = true
		return
	}
	if c.tasks == nil {
		c.tasks = make(map[string]struct{}, len(ev.TaskIDs))
	}
	for _, id := range ev.TaskIDs {
		c.tasks[id] = struct{}{}
	}
}

// event is the merged payload; TaskIDs is nil when any merged change had unknown tasks.
func (c *boardChanges) event() adewire.BoardChangedEvent {
	ev := adewire.BoardChangedEvent{Refs: c.refs}
	if !c.all {
		ev.TaskIDs = make([]string, 0, len(c.tasks))
		for id := range c.tasks {
			ev.TaskIDs = append(ev.TaskIDs, id)
		}
		sort.Strings(ev.TaskIDs)
	}
	return ev
}

// taskChanged is the notification of a change that touches only these tasks' own rows: no branch
// row, ref or change request.
func taskChanged(ids ...string) adewire.BoardChangedEvent {
	return adewire.BoardChangedEvent{TaskIDs: slices.Clone(ids)}
}

// invalidateBoard makes every snapshot and in-flight build older than now unusable.
func (b *TaskBoard) invalidateBoard() {
	b.bcache.gen.Add(1)
	b.dropLeaseOrder()
}

// Board returns the shared snapshot while it is current and fresh, else joins or starts the one
// build of the current gen. A caller's cancel only ends its own wait: the build runs on the board's
// context and its result is kept for the next caller. The snapshot is shared read-only.
func (b *TaskBoard) Board(ctx context.Context) (adewire.Board, error) {
	c := &b.bcache
	gen := c.gen.Load()
	c.mu.Lock()
	if s := c.snap; s != nil && s.gen == gen && time.Since(s.at) < boardSnapshotFresh {
		board := s.board
		c.mu.Unlock()
		return board, nil
	}
	fl := c.flights[gen]
	start := fl == nil
	if start {
		fl = &boardFlight{done: make(chan struct{})}
		if c.flights == nil {
			c.flights = map[uint64]*boardFlight{}
		}
		c.flights[gen] = fl
	}
	c.mu.Unlock()
	if start {
		began := time.Now()
		if !b.track() {
			b.finishBoardFlight(gen, fl, began, adewire.Board{}, errBoardClosed)
		} else {
			go func() {
				defer b.wg.Done()
				board, err := b.buildBoard(b.ctx)
				b.finishBoardFlight(gen, fl, began, board, err)
			}()
		}
	}
	select {
	case <-fl.done:
		return fl.board, fl.err
	case <-ctx.Done():
		return adewire.Board{}, ctx.Err()
	}
}

// finishBoardFlight publishes a build's result to its joiners and keeps it as the snapshot unless a
// newer gen's is already kept.
func (b *TaskBoard) finishBoardFlight(gen uint64, fl *boardFlight, began time.Time, board adewire.Board, err error) {
	c := &b.bcache
	c.mu.Lock()
	delete(c.flights, gen)
	if err == nil && (c.snap == nil || gen >= c.snap.gen) {
		c.snap = &boardSnapshot{gen: gen, at: began, board: board}
	}
	c.mu.Unlock()
	fl.board, fl.err = board, err
	close(fl.done)
}

// notifyBoard announces a change of unknown scope.
func (b *TaskBoard) notifyBoard() { b.notifyBoardWith(allChanged) }

// notifyBoardWith invalidates the snapshot at once and pushes ch, merged with every change that
// lands within boardPushWindow, as one board-changed event.
func (b *TaskBoard) notifyBoardWith(ch adewire.BoardChangedEvent) {
	b.invalidateBoard()
	c := &b.bcache
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	c.push.merge(ch)
	if c.pushTimer == nil {
		c.pushTimer = time.AfterFunc(boardPushWindow, b.flushBoardPush)
	}
}

// notifyBoardNow is notifyBoardWith that pushes before it returns, for a caller that raises
// another push which reads the board it announces.
func (b *TaskBoard) notifyBoardNow(ch adewire.BoardChangedEvent) {
	b.invalidateBoard()
	c := &b.bcache
	c.pushMu.Lock()
	c.push.merge(ch)
	if c.pushTimer != nil {
		c.pushTimer.Stop()
	}
	ev, _ := b.takeBoardPush()
	c.pushMu.Unlock()
	b.emitBoard(ev)
}

func (b *TaskBoard) flushBoardPush() {
	c := &b.bcache
	c.pushMu.Lock()
	ev, ok := b.takeBoardPush()
	c.pushMu.Unlock()
	if ok { // a notifyBoardNow that stopped this timer too late already took the change
		b.runTracked(func() { b.emitBoard(ev) })
	}
}

// takeBoardPush empties the coalescer; the caller holds pushMu.
func (b *TaskBoard) takeBoardPush() (adewire.BoardChangedEvent, bool) {
	c := &b.bcache
	if !c.push.any {
		return adewire.BoardChangedEvent{}, false
	}
	ev := c.push.event()
	c.push, c.pushTimer = boardChanges{}, nil
	return ev, true
}

func (b *TaskBoard) emitBoard(ev adewire.BoardChangedEvent) {
	if b.deps.OnBoard != nil {
		b.deps.OnBoard(ev)
	}
	b.updates.boardChanged()
}

func (b *TaskBoard) stopBoardPush() {
	c := &b.bcache
	c.pushMu.Lock()
	defer c.pushMu.Unlock()
	if c.pushTimer != nil {
		c.pushTimer.Stop()
	}
}
