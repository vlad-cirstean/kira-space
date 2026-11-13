package ade

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/leases"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/harness"
)

// LeaseQueueStore keeps the lease queue names across restarts. Holds and waits stay in memory:
// the PTYs die with the app.
type LeaseQueueStore interface {
	List() ([]string, error)
	Save(names []string) error
}

const (
	leaseSweepEvery = 30 * time.Second
	runKeyPrefix    = "run:"
	// leaseToolCeiling x the TTL bounds how long one silent tool call keeps a holder alive, so an
	// interrupted tool (no end event) cannot hold a lease for good.
	leaseToolCeiling = 3
	// leaseOrderFresh is how long a computed task order stays current with no board change, for the
	// settings it reads that never announce one.
	leaseOrderFresh = 5 * time.Second
)

// leaseState is the board's lease bookkeeping around the leases.Book.
type leaseState struct {
	book *leases.Book
	// mu guards pending: lease messages not yet delivered, by owner. It is taken before the
	// tracker's lock, never after.
	mu      sync.Mutex
	pending map[leases.Key][]string
	// saveMu serializes SaveLeaseQueues so the stored names and the book end in the same order.
	saveMu sync.Mutex
	// order caches the task order a waiter is ranked by. A board change marks it stale without waiting.
	order orderCache
	// runs holds the owners of live headless runs, which emit no hook events: a live run is alive.
	runs map[leases.Key]struct{}
	// flush holds the delivery in progress per owner, guarded by mu (the entry is dropped by endLeases).
	flush map[leases.Key]*leaseFlush
}

// leaseFlush serializes one owner's deliveries. A flush takes the pending messages, writes them to
// the PTY without holding leaseState.mu, and puts them back when the session was busy. mu spans
// that whole sequence, so a later flush or notice read waits and then sees the re-queued messages
// in order. The other fields are guarded by leaseState.mu.
type leaseFlush struct {
	mu    sync.Mutex
	msgs  []string // taken and not yet sent or re-queued: still pending for the snapshot
	ended bool     // endLeases ran meanwhile: do not re-queue
}

// flushOf returns the owner's flush state, creating it. The caller holds lease.mu.
func (s *leaseState) flushOf(key leases.Key) *leaseFlush {
	f := s.flush[key]
	if f == nil {
		f = &leaseFlush{}
		s.flush[key] = f
	}
	return f
}

func (b *TaskBoard) initLeases() {
	b.lease = &leaseState{pending: map[leases.Key][]string{}, runs: map[leases.Key]struct{}{}, flush: map[leases.Key]*leaseFlush{}}
	b.lease.book = leases.New(leases.Options{
		Now: b.deps.Now, TTL: b.deps.LeaseTTL, Order: b.taskListOrder,
		OnGrant: b.onLeaseGrant, OnChange: b.leasesChanged,
	})
	if b.deps.LeaseQueues != nil {
		names, err := b.deps.LeaseQueues.List()
		if err == nil {
			err = b.lease.book.SetQueues(names)
		}
		if err != nil {
			slog.Warn("ade: load lease queues", "scope", "ade", "err", err)
		}
	}
	if tr := b.deps.Tracker; tr != nil {
		tr.SetHookReplyHandler(b.leaseHookReply)
		tr.SetHookReplyFailedHandler(b.requeueLeaseReply)
		tr.SetIdleHandler(b.flushLease)
		tr.SetActivityHandler(func(recordID string) { b.lease.book.Touch(leases.Key(recordID)) })
	}
}

func (b *TaskBoard) leasesChanged() {
	if b.deps.OnLeases != nil {
		b.deps.OnLeases()
	}
}

// leaseKeyOf is the lease owner of a caller: its session record, else its headless run.
func leaseKeyOf(sessionID, runID string) leases.Key {
	if sessionID != "" {
		return leases.Key(sessionID)
	}
	return leases.Key(runKeyPrefix + runID)
}

// taskListOrder is the order the Tasks UI shows: the rank of a lease waiter. Once built it never
// runs Board on the caller: a stale order is returned while one background build refreshes it.
// A failed first read ranks everyone by arrival.
func (b *TaskBoard) taskListOrder() []string {
	now := b.deps.Now()
	// The UI reads the machine's local date, so the order does too.
	day := now.Local().Format("2006-01-02")
	return b.lease.order.get(now, day, func() ([]string, bool) {
		board, err := b.Board(b.ctx)
		if err != nil {
			slog.Warn("ade: lease task order", "scope", "ade", "err", err)
			return nil, false
		}
		s := model.DefaultAdeSettings()
		if b.deps.AdeSettings != nil {
			s = b.deps.AdeSettings()
		}
		return TaskListOrder(board, s, day), true
	}, func(fn func()) { b.goTracked(fn) })
}

// dropLeaseOrder forgets the cached task order after a board change. It never waits for a build.
func (b *TaskBoard) dropLeaseOrder() { b.lease.order.drop() }

// sweepLeases frees holders that went silent; the tracker says who is mid-turn.
func (b *TaskBoard) sweepLeases() {
	every := min(leaseSweepEvery, b.lease.book.TTL()/4)
	t := time.NewTicker(max(every, 10*time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-t.C:
			b.lease.book.Sweep(b.leaseAlive)
		}
	}
}

// leaseAlive is true for a live headless run and for a session inside a tool call; a session that
// went quiet between tool calls expires like any other silent holder.
func (b *TaskBoard) leaseAlive(k leases.Key) bool {
	b.lease.mu.Lock()
	_, run := b.lease.runs[k]
	b.lease.mu.Unlock()
	if run {
		return true
	}
	return b.deps.Tracker != nil && !strings.HasPrefix(string(k), runKeyPrefix) &&
		b.deps.Tracker.Working(string(k), leaseToolCeiling*b.lease.book.TTL())
}

// beginLeaseRun marks key as a live headless run until endLeases(key).
func (b *TaskBoard) beginLeaseRun(key leases.Key) {
	b.lease.mu.Lock()
	b.lease.runs[key] = struct{}{}
	b.lease.mu.Unlock()
}

// --- delivery ------------------------------------------------------------------------------------

// ttlText words a TTL for an agent: whole minutes when it is, else seconds.
func ttlText(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		if n := int(d / time.Minute); n > 1 {
			return strconv.Itoa(n) + " minutes"
		}
		return "1 minute"
	}
	return strconv.Itoa(int(d.Round(time.Second)/time.Second)) + " seconds"
}

func leaseMessage(g leases.Grant, ttl time.Duration) string {
	switch g.Reason {
	case leases.ReasonRevoked:
		return "Kira: the user revoked your lease `" + g.Queue + "`. Stop using the resource; call acquire_lease to queue again."
	case leases.ReasonRemoved:
		return "Kira: the user removed lease queue `" + g.Queue + "`; you no longer wait for it. Call acquire_lease again if you still need a lease."
	case leases.ReasonExpired:
		return "Kira: your lease `" + g.Queue + "` expired after " + ttlText(ttl) + " of silence. Stop using the resource; call acquire_lease to queue again."
	}
	return "Kira: you now hold lease `" + g.Queue + "`. Continue the work that waited for it. Call release_lease when done."
}

func (b *TaskBoard) onLeaseGrant(g leases.Grant) {
	b.deliverLease(g.Key, leaseMessage(g, b.lease.book.TTL()))
}

// deliverLease queues msg for key, then sends what waits if the session is idle now. Otherwise the
// next Stop or UserPromptSubmit hook reply, the idle handler or the owner's next lease tool call
// carries it.
func (b *TaskBoard) deliverLease(key leases.Key, msg string) {
	b.lease.mu.Lock()
	b.lease.pending[key] = append(b.lease.pending[key], msg)
	b.lease.mu.Unlock()
	b.flushLease(string(key))
	b.leasesChanged()
}

// flushLease types the pending messages of a session record into its idle terminal.
func (b *TaskBoard) flushLease(recordID string) {
	if b.deps.Tracker == nil || strings.HasPrefix(recordID, runKeyPrefix) {
		return
	}
	key := leases.Key(recordID)
	b.lease.mu.Lock()
	f := b.lease.flushOf(key)
	b.lease.mu.Unlock()
	f.mu.Lock()
	defer f.mu.Unlock()
	b.lease.mu.Lock()
	msgs := b.lease.pending[key]
	delete(b.lease.pending, key)
	f.msgs, f.ended = msgs, false
	b.lease.mu.Unlock()
	if len(msgs) == 0 {
		return
	}
	sent, err := b.deps.Tracker.SendIfIdle(recordID, strings.Join(msgs, "\n"))
	b.lease.mu.Lock()
	f.msgs = nil
	if !sent && !f.ended {
		// Back first in line: messages queued meanwhile stay behind them. An owner that ended
		// meanwhile has nobody to deliver to.
		b.lease.pending[key] = append(msgs, b.lease.pending[key]...)
	}
	b.lease.mu.Unlock()
	if err != nil {
		slog.Warn("ade: lease message", "scope", "ade", "recordId", recordID, "err", err)
	}
	if sent {
		b.leasesChanged()
	}
}

// takeLeaseNotices removes and returns the messages waiting for key.
func (b *TaskBoard) takeLeaseNotices(key leases.Key) []string {
	// Wait for a delivery in flight: its messages are neither pending nor sent yet.
	b.lease.mu.Lock()
	f := b.lease.flush[key]
	b.lease.mu.Unlock()
	if f != nil {
		f.mu.Lock()
		defer f.mu.Unlock()
	}
	b.lease.mu.Lock()
	msgs := b.lease.pending[key]
	delete(b.lease.pending, key)
	b.lease.mu.Unlock()
	if len(msgs) > 0 {
		b.leasesChanged()
	}
	return msgs
}

// leaseHookReply answers a session's Stop and UserPromptSubmit hooks with the messages waiting for
// it, once.
func (b *TaskBoard) leaseHookReply(recordID string, ev harness.AgentEvent) harness.HookReply {
	msgs := b.takeLeaseNotices(leases.Key(recordID))
	if len(msgs) == 0 {
		return harness.HookReply{}
	}
	text := strings.Join(msgs, "\n")
	if ev.Event == harness.EventTurnEnd {
		return harness.HookReply{Block: text}
	}
	return harness.HookReply{Context: text}
}

// requeueLeaseReply puts the messages of a hook reply that never arrived back first in line.
func (b *TaskBoard) requeueLeaseReply(recordID string, reply harness.HookReply) {
	text := reply.Block + reply.Context
	if text == "" {
		return
	}
	key := leases.Key(recordID)
	b.lease.mu.Lock()
	b.lease.pending[key] = append([]string{text}, b.lease.pending[key]...)
	b.lease.mu.Unlock()
	b.leasesChanged()
}

// endLeases frees everything a finished session or run holds or waits for.
func (b *TaskBoard) endLeases(key leases.Key) {
	b.lease.mu.Lock()
	delete(b.lease.pending, key)
	delete(b.lease.runs, key)
	if f := b.lease.flush[key]; f != nil {
		f.ended = true
		delete(b.lease.flush, key)
	}
	b.lease.mu.Unlock()
	b.lease.book.ReleaseSession(key)
}

// --- agent tools ---------------------------------------------------------------------------------

// leaseToolError turns a book refusal into text the agent can act on.
func (b *TaskBoard) leaseToolError(err error) error {
	if len(b.lease.book.Names()) == 0 {
		return agentmcp.ToolError("No lease queues exist. The user creates them in Agents, Leases.")
	}
	if errors.Is(err, leases.ErrUnknownQueue) {
		return agentmcp.ToolError(err.Error())
	}
	return err
}

// leaseNext tells the agent how it will learn of a grant.
func (b *TaskBoard) leaseNext(call agentmcp.SpaceCall, state string) string {
	if state == agentmcp.LeaseHeld {
		return "You hold the lease. Call release_lease when done."
	}
	if call.SessionID != "" && b.deps.Tracker != nil {
		if _, live := b.deps.Tracker.TerminalOf(call.SessionID); live {
			return "Kira messages you in this session when the lease is granted. Do other work meanwhile."
		}
	}
	return "Call acquire_lease or lease_status again later to check."
}

func (b *TaskBoard) AcquireLease(_ context.Context, call agentmcp.SpaceCall, args agentmcp.AcquireLeaseArgs) (agentmcp.AcquireLeaseResult, error) {
	key := leaseKeyOf(call.SessionID, call.RunID)
	st, err := b.lease.book.Acquire(key, call.TaskID, args.Queue)
	if err != nil {
		return agentmcp.AcquireLeaseResult{}, b.leaseToolError(err)
	}
	return agentmcp.AcquireLeaseResult{
		Queue: st.Queue, State: st.State, Position: st.Position, HolderTask: b.leaseTaskName(st.HolderTask),
		TTLSeconds: st.TTLSeconds, Next: b.leaseNext(call, st.State), Notices: b.takeLeaseNotices(key),
	}, nil
}

func (b *TaskBoard) ReleaseLease(_ context.Context, call agentmcp.SpaceCall, args agentmcp.ReleaseLeaseArgs) (agentmcp.ReleaseLeaseResult, error) {
	key := leaseKeyOf(call.SessionID, call.RunID)
	if args.Queue != "" && !slices.Contains(b.lease.book.Names(), args.Queue) {
		return agentmcp.ReleaseLeaseResult{}, b.leaseToolError(leases.ErrUnknownQueue)
	}
	released := b.lease.book.Release(key, args.Queue)
	return agentmcp.ReleaseLeaseResult{Released: released, Notices: b.takeLeaseNotices(key)}, nil
}

func (b *TaskBoard) LeaseStatus(_ context.Context, call agentmcp.SpaceCall, args agentmcp.LeaseStatusArgs) (agentmcp.LeaseStatusResult, error) {
	key := leaseKeyOf(call.SessionID, call.RunID)
	names := b.lease.book.Names()
	if args.Queue != "" && !slices.Contains(names, args.Queue) {
		return agentmcp.LeaseStatusResult{}, b.leaseToolError(leases.ErrUnknownQueue)
	}
	out := agentmcp.LeaseStatusResult{Queues: []agentmcp.LeaseQueueInfo{}}
	for _, q := range b.lease.book.Snapshot() {
		if args.Queue != "" && q.Name != args.Queue {
			continue
		}
		info := agentmcp.LeaseQueueInfo{Name: q.Name, Waiters: []agentmcp.LeaseEntryInfo{}}
		if q.Holder != nil {
			info.Holder = &agentmcp.LeaseEntryInfo{TaskID: q.Holder.TaskID, TaskTitle: b.leaseTaskName(q.Holder.TaskID), Mine: q.Holder.Key == key}
		}
		for i, w := range q.Waiters {
			info.Waiters = append(info.Waiters, agentmcp.LeaseEntryInfo{TaskID: w.TaskID, TaskTitle: b.leaseTaskName(w.TaskID), Position: i + 1, Mine: w.Key == key})
		}
		out.Queues = append(out.Queues, info)
	}
	out.Notices = b.takeLeaseNotices(key)
	return out, nil
}

// leaseTaskName is a task's title (or Jira key), else its id; "" for no task.
func (b *TaskBoard) leaseTaskName(taskID string) string {
	if taskID == "" {
		return ""
	}
	if s := b.opSubject(taskID); s != "" {
		return s
	}
	return taskID
}

// --- bridge --------------------------------------------------------------------------------------

// LeaseQueues lists the configured queue names in order.
func (b *TaskBoard) LeaseQueues() []string { return b.lease.book.Names() }

// SaveLeaseQueues validates and stores the queue names, then applies them. A removed queue revokes
// its holder.
func (b *TaskBoard) SaveLeaseQueues(names []string) error {
	names, err := leases.ValidateNames(names)
	if err != nil {
		return invalid("%v", err)
	}
	b.lease.saveMu.Lock()
	defer b.lease.saveMu.Unlock()
	if b.deps.LeaseQueues != nil {
		if err := b.deps.LeaseQueues.Save(names); err != nil {
			return err
		}
	}
	return b.lease.book.SetQueues(names)
}

// LeaseSnapshot lists every queue with its holder and waiters.
func (b *TaskBoard) LeaseSnapshot() adewire.LeaseSnapshot {
	ttl := b.lease.book.TTL()
	entry := func(e leases.Entry, position int) adewire.LeaseEntry {
		b.lease.mu.Lock()
		noticed := len(b.lease.pending[e.Key]) > 0
		if f := b.lease.flush[e.Key]; f != nil && len(f.msgs) > 0 {
			noticed = true // taken by a delivery in flight
		}
		b.lease.mu.Unlock()
		out := adewire.LeaseEntry{
			SessionID: string(e.Key), TaskID: e.TaskID, TaskTitle: b.leaseTaskName(e.TaskID),
			Position: position, ReleaseAsked: e.ReleaseAsked, NoticePending: noticed,
		}
		if position == 0 {
			out.Since, out.ExpiresAt = e.Since.UnixMilli(), e.Seen.Add(ttl).UnixMilli()
		}
		return out
	}
	snap := adewire.LeaseSnapshot{Queues: []adewire.LeaseQueue{}}
	for _, q := range b.lease.book.Snapshot() {
		out := adewire.LeaseQueue{Name: q.Name, Waiters: []adewire.LeaseEntry{}}
		if q.Holder != nil {
			h := entry(*q.Holder, 0)
			out.Holder = &h
		}
		for i, w := range q.Waiters {
			out.Waiters = append(out.Waiters, entry(w, i+1))
		}
		snap.Queues = append(snap.Queues, out)
	}
	return snap
}

// RequestLeaseRelease asks the holder to finish up and release.
func (b *TaskBoard) RequestLeaseRelease(queue string) error {
	key, err := b.lease.book.RequestRelease(queue)
	if err != nil {
		return invalid("%v", err)
	}
	b.deliverLease(key, "Kira: the user asks you to release lease `"+queue+"`: finish the current step, then call release_lease.")
	return nil
}

// ForceLeaseRelease revokes the holder and grants the next waiter.
func (b *TaskBoard) ForceLeaseRelease(queue string) error {
	if _, err := b.lease.book.ForceRelease(queue); err != nil {
		return invalid("%v", err)
	}
	return nil
}
