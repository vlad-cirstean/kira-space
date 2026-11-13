package harness

import (
	"log/slog"
	"sync"
)

const eventQueueCap = 1024

// eventQueue runs fn for every enqueued event, in arrival order per terminal. Each terminal with
// events waiting has its own lane drained by one goroutine that exits when the lane is empty, so
// a slow handler (an OS notification, a probe of the main thread) delays only its own terminal's
// later events. A full lane blocks enqueue: backpressure on the listener's HTTP handler.
type eventQueue struct {
	fn func(AgentEvent)

	mu    sync.Mutex
	space *sync.Cond // a lane shrank
	lanes map[string]*lane
	// closed is guarded by mu. senders counts enqueue/barrier calls past the closed check, so
	// close never races a send; workers counts running lane goroutines.
	closed  bool
	senders sync.WaitGroup
	workers sync.WaitGroup
}

// lane is one terminal's pending items. A lane stays registered while its worker runs, including
// while the worker handles the last popped item, so a barrier sees events still in flight.
type lane struct {
	items []queueItem
}

type queueItem struct {
	ev      AgentEvent
	barrier chan struct{}
}

func newEventQueue(fn func(AgentEvent)) *eventQueue {
	q := &eventQueue{fn: fn, lanes: map[string]*lane{}}
	q.space = sync.NewCond(&q.mu)
	return q
}

// laneLocked returns the terminal's lane, starting its worker when it has none. The caller holds mu.
func (q *eventQueue) laneLocked(terminalID string) *lane {
	l := q.lanes[terminalID]
	if l == nil {
		l = &lane{}
		q.lanes[terminalID] = l
		q.workers.Add(1)
		go q.run(terminalID, l)
	}
	return l
}

func (q *eventQueue) run(terminalID string, l *lane) {
	defer q.workers.Done()
	for {
		q.mu.Lock()
		if len(l.items) == 0 {
			delete(q.lanes, terminalID)
			q.mu.Unlock()
			return
		}
		it := l.items[0]
		l.items[0] = queueItem{}
		l.items = l.items[1:]
		q.space.Broadcast()
		q.mu.Unlock()
		if it.barrier != nil {
			close(it.barrier)
			continue
		}
		q.call(it.ev)
	}
}

// call recovers a panic so one bad event cannot kill the worker.
func (q *eventQueue) call(ev AgentEvent) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("agent hooks: event handler panic", "scope", "ade", "event", string(ev.Event), "panic", r)
		}
	}()
	q.fn(ev)
}

// acquire registers a sender; false once the queue is closed.
func (q *eventQueue) acquire() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	q.senders.Add(1)
	return true
}

func (q *eventQueue) enqueue(ev AgentEvent) {
	if !q.acquire() {
		return
	}
	defer q.senders.Done()
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		l := q.laneLocked(ev.TerminalID)
		if len(l.items) < eventQueueCap {
			l.items = append(l.items, queueItem{ev: ev})
			return
		}
		q.space.Wait()
	}
}

// barrier returns once every event of the terminal enqueued before it has been handled.
func (q *eventQueue) barrier(terminalID string) {
	if !q.acquire() {
		return
	}
	defer q.senders.Done()
	q.mu.Lock()
	l := q.lanes[terminalID]
	if l == nil {
		q.mu.Unlock()
		return
	}
	ch := make(chan struct{})
	l.items = append(l.items, queueItem{barrier: ch})
	q.mu.Unlock()
	<-ch
}

// barrierAll returns once every event of every terminal enqueued before it has been handled.
func (q *eventQueue) barrierAll() {
	if !q.acquire() {
		return
	}
	defer q.senders.Done()
	q.mu.Lock()
	var chans []chan struct{}
	for _, l := range q.lanes {
		ch := make(chan struct{})
		l.items = append(l.items, queueItem{barrier: ch})
		chans = append(chans, ch)
	}
	q.mu.Unlock()
	for _, ch := range chans {
		<-ch
	}
}

// close stops intake, drains what is queued, then waits for the lane workers to exit.
func (q *eventQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.senders.Wait()
	q.workers.Wait()
}
