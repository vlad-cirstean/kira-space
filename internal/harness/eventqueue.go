package harness

import (
	"log/slog"
	"sync"
)

const eventQueueCap = 1024

// eventQueue runs fn for every enqueued event on one goroutine, in arrival order. A full queue
// blocks enqueue: backpressure on the listener's HTTP handler.
type eventQueue struct {
	fn    func(AgentEvent)
	items chan queueItem

	mu     sync.Mutex
	closed bool
	// senders counts enqueue/barrier calls past the closed check, so close never races a send.
	senders sync.WaitGroup
	done    chan struct{}
}

type queueItem struct {
	ev      AgentEvent
	barrier chan struct{}
}

func newEventQueue(fn func(AgentEvent)) *eventQueue {
	q := &eventQueue{fn: fn, items: make(chan queueItem, eventQueueCap), done: make(chan struct{})}
	go q.run()
	return q
}

func (q *eventQueue) run() {
	defer close(q.done)
	for it := range q.items {
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
	q.items <- queueItem{ev: ev}
}

// barrier returns once every event enqueued before it has been handled.
func (q *eventQueue) barrier() {
	if !q.acquire() {
		return
	}
	ch := make(chan struct{})
	q.items <- queueItem{barrier: ch}
	q.senders.Done()
	<-ch
}

// close stops intake, drains what is queued, then waits for the worker to exit.
func (q *eventQueue) close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		<-q.done
		return
	}
	q.closed = true
	q.mu.Unlock()
	q.senders.Wait()
	close(q.items)
	<-q.done
}
