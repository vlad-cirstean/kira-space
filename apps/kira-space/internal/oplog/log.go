// Package oplog is Kira Space's operation log: every git spawn, agent session and in-app MCP tool
// call, shared by every window. Two in-memory lanes (foreground, background) back the panel;
// foreground rows persist to op_log through one writer goroutine.
package oplog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/notify"
)

const (
	// FgCapacity and BgCapacity bound the two in-memory lanes. A background row never evicts a foreground one.
	FgCapacity = 500
	BgCapacity = 300

	Retention       = 30 * 24 * time.Hour
	MaxRows         = 5000
	DedupeWindow    = 10 * time.Minute
	FlushInterval   = time.Second
	pruneEvery      = 500
	maxSteps        = 64
	maxStderrTail   = 8 << 10
	maxCommandBytes = 16 << 10
)

// Store persists foreground records. Writes happen on the log's writer goroutine only.
type Store interface {
	Upsert(Record) error
	Prune(cutoff time.Time, maxRows int) error
}

// Log is the two-lane ring. A nil *Log is valid and records nothing.
type Log struct {
	mu      sync.Mutex
	fg      []*Record // newest first
	bg      []*Record // newest first
	cancels map[string]func() bool

	emit      notify.Emitter[Record]
	batchEmit notify.Emitter[[]Record]
	pending   map[string]Record
	pendOrder []string
	flushing  bool
	flushIn   time.Duration

	dedupe     map[string]dedupeEntry
	finishedFg int

	store Store
	w     *writer

	idPrefix string
	closers  []func()
}

type dedupeEntry struct {
	rec *Record
	at  time.Time
}

// Option configures New.
type Option func(*Log)

// WithStore persists foreground records and failed background records to s.
func WithStore(s Store) Option { return func(l *Log) { l.store = s } }

func New(opts ...Option) *Log {
	l := &Log{
		cancels: make(map[string]func() bool),
		pending: make(map[string]Record),
		dedupe:  make(map[string]dedupeEntry),
		flushIn: FlushInterval,
	}
	for _, o := range opts {
		o(l)
	}
	if l.store != nil {
		l.w = newWriter()
	}
	return l
}

// Seed fills the foreground lane from persisted rows (newest first). Call before any window opens.
func (l *Log) Seed(recs []Record) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range recs {
		r := recs[i]
		if r.Steps == nil {
			r.Steps = []Step{}
		}
		lane, capacity := &l.fg, FgCapacity
		if r.Background {
			lane, capacity = &l.bg, BgCapacity
		}
		if len(*lane) < capacity {
			*lane = append(*lane, &r)
		}
	}
}

// Close drains pending persistence writes, bounded.
func (l *Log) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	closers := l.closers
	l.closers = nil
	l.mu.Unlock()
	for _, c := range closers {
		c()
	}
	if l.w == nil {
		return
	}
	l.w.close(2 * time.Second)
}

// OnUpdate subscribes to every foreground record change and returns its unsubscribe func.
func (l *Log) OnUpdate(fn func(Record)) func() { return l.emit.Subscribe(fn) }

// OnBatch subscribes to coalesced background record changes (last state per id, at most once a second).
func (l *Log) OnBatch(fn func([]Record)) func() { return l.batchEmit.Subscribe(fn) }

// Op is a handle to one running operation. A nil *Op is valid and does nothing.
type Op struct {
	log      *Log
	rec      *Record
	started  time.Time
	bg       bool
	cmdFixed bool
	cmd      string
	cmdFull  bool
}

// Start records a new running operation. Origin and enclosing op come from ctx.
func (l *Log) Start(ctx context.Context, m Meta) *Op {
	if l == nil {
		return nil
	}
	o, _ := OriginFrom(ctx)
	if o.Module == "" {
		o.Module = ModuleApp
	}
	if o.Trigger == "" {
		o.Trigger = TriggerAuto
	}
	cat := m.Category
	if cat == "" {
		cat = CategoryGit
	}
	source := m.Source
	if source == "" {
		source = o.Label
	}
	rec := &Record{
		ID:         l.idPrefix + ids.NewID(),
		StartedAt:  kiratime.NowISO(),
		Category:   cat,
		Kind:       m.Kind,
		Status:     StatusRunning,
		Module:     o.Module,
		Trigger:    o.Trigger,
		Source:     source,
		Subject:    m.Subject,
		RepoRoot:   m.RepoRoot,
		RepoName:   m.RepoName,
		TaskID:     o.TaskID,
		Background: m.Background || o.Background,
		Repeat:     1,
		Steps:      []Step{},
	}
	if parent := OpFrom(ctx); parent != nil {
		rec.ParentID = parent.rec.ID
	}
	op := &Op{log: l, rec: rec, started: time.Now(), bg: rec.Background}
	l.mu.Lock()
	if op.bg {
		l.bg = insertFront(l.bg, rec, BgCapacity)
	} else {
		l.fg = insertFront(l.fg, rec, FgCapacity)
	}
	snap := rec.clone()
	l.mu.Unlock()
	l.publish(snap, !op.bg, !op.bg)
	return op
}

// Record logs one already-finished operation.
func (l *Log) Record(ctx context.Context, m Meta, status, errMsg string) {
	l.Start(ctx, m).Finish(status, errMsg)
}

func insertFront(lane []*Record, rec *Record, capacity int) []*Record {
	lane = append(lane, nil)
	copy(lane[1:], lane)
	lane[0] = rec
	if len(lane) > capacity {
		clear(lane[capacity:])
		lane = lane[:capacity]
	}
	return lane
}

// Recent returns up to limit foreground plus up to limit background records, newest first.
func (l *Log) Recent(limit int) []Record {
	if l == nil || limit <= 0 {
		return []Record{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fg, bg := l.fg[:min(limit, len(l.fg))], l.bg[:min(limit, len(l.bg))]
	out := make([]Record, 0, len(fg)+len(bg))
	i, j := 0, 0
	for i < len(fg) || j < len(bg) {
		if j >= len(bg) || (i < len(fg) && fg[i].StartedAt >= bg[j].StartedAt) {
			out = append(out, fg[i].clone())
			i++
		} else {
			out = append(out, bg[j].clone())
			j++
		}
	}
	return out
}

// CancellableRunning returns the running foreground rows of taskID that accept a cancel, newest first.
func (l *Log) CancellableRunning(taskID string) []Record {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Record
	for _, r := range l.fg {
		if r.TaskID == taskID && r.Status == StatusRunning && r.Cancellable {
			out = append(out, r.clone())
		}
	}
	return out
}

// Cancel calls the registered cancel func under the log lock, so an op that unregistered
// before releasing its slot is never cancelled after the slot changes hands. It reports
// whether a func was registered and accepted the cancel. A cancel func must only signal; it
// must never call back into the Log.
func (l *Log) Cancel(id string) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	fn := l.cancels[id]
	return fn != nil && fn()
}

// publish delivers snap: foreground immediately, background coalesced. persist enqueues the upsert.
func (l *Log) publish(snap Record, foreground, persist bool) {
	if persist {
		l.persist(snap)
	}
	if foreground {
		l.emit.Emit(snap)
		return
	}
	l.queueBatch(snap)
}

func (l *Log) persist(snap Record) {
	if l.w == nil {
		return
	}
	l.w.submit(func() {
		if err := l.store.Upsert(snap); err != nil {
			slog.Warn("oplog: persist", "scope", "oplog", "id", snap.ID, "err", err)
		}
	})
}

func (l *Log) queueBatch(snap Record) {
	l.mu.Lock()
	if _, ok := l.pending[snap.ID]; !ok {
		l.pendOrder = append(l.pendOrder, snap.ID)
	}
	l.pending[snap.ID] = snap
	if !l.flushing {
		l.flushing = true
		time.AfterFunc(l.flushIn, l.flush)
	}
	l.mu.Unlock()
}

func (l *Log) flush() {
	l.mu.Lock()
	out := make([]Record, 0, len(l.pendOrder))
	for _, id := range l.pendOrder {
		if r, ok := l.pending[id]; ok {
			out = append(out, r)
		}
	}
	clear(l.pending)
	l.pendOrder = l.pendOrder[:0]
	l.flushing = false
	l.mu.Unlock()
	if len(out) > 0 {
		l.batchEmit.Emit(out)
	}
}

func (l *Log) dropPendingLocked(id string) { delete(l.pending, id) }

// SetCommand fixes the displayed command (session and MCP ops); step recording no longer derives it.
func (o *Op) SetCommand(cmd string) {
	if o == nil {
		return
	}
	o.log.mu.Lock()
	o.cmdFixed = true
	c := truncateRunes(cmd, maxCommandBytes)
	o.rec.Command = &c
	snap := o.rec.clone()
	o.log.mu.Unlock()
	o.log.publish(snap, !o.bg, false)
}

// ID returns the record id, or "" for a nil op.
func (o *Op) ID() string {
	if o == nil {
		return ""
	}
	return o.rec.ID
}

// SetCancel registers fn as the op's cancel handler.
func (o *Op) SetCancel(fn func() bool) {
	if o == nil {
		return
	}
	o.log.mu.Lock()
	if o.rec.Status != StatusRunning {
		o.log.mu.Unlock()
		return
	}
	o.log.cancels[o.rec.ID] = fn
	o.rec.Cancellable = true
	snap := o.rec.clone()
	o.log.mu.Unlock()
	o.log.publish(snap, !o.bg, false)
}

// ClearCancel unregisters the cancel handler. Call it before releasing the op's slot.
func (o *Op) ClearCancel() {
	if o == nil {
		return
	}
	o.log.mu.Lock()
	if !o.clearCancelLocked() {
		o.log.mu.Unlock()
		return
	}
	snap := o.rec.clone()
	o.log.mu.Unlock()
	o.log.publish(snap, !o.bg, false)
}

// Finish records the outcome. Later calls are ignored. A failing step's stderr tail is appended to errMsg.
func (o *Op) Finish(status, errMsg string) {
	if o == nil {
		return
	}
	l := o.log
	l.mu.Lock()
	if o.rec.Status != StatusRunning {
		l.mu.Unlock()
		return
	}
	o.clearCancelLocked()
	ms := int(time.Since(o.started).Milliseconds())
	o.rec.DurationMs = &ms
	o.rec.Status = status
	for i := range o.rec.Steps {
		if o.rec.Steps[i].Status == StatusRunning {
			o.rec.Steps[i].Status = StatusCancelled
		}
	}
	if status == StatusError {
		msg := RedactText(errMsg)
		if st := lastFailedStderr(o.rec.Steps); st != "" && !strings.Contains(msg, st) {
			if msg != "" {
				msg += "\n\n"
			}
			msg += st
		}
		if msg != "" {
			o.rec.Error = &msg
		}
	} else if errMsg != "" {
		msg := RedactText(errMsg)
		o.rec.Error = &msg
	}

	rec := o.rec
	persist := !o.bg
	switch {
	case o.bg && status == StatusError:
		if existing := l.dedupeLocked(rec); existing != nil {
			existing.Repeat++
			l.bg = removeRec(l.bg, rec)
			l.dropPendingLocked(rec.ID)
			snap := existing.clone()
			l.mu.Unlock()
			l.publish(snap, false, true)
			return
		}
		persist = true
	case !o.bg:
		l.finishedFg++
	}
	prune := !o.bg && l.finishedFg%pruneEvery == 0
	snap := rec.clone()
	l.mu.Unlock()
	l.publish(snap, !o.bg, persist)
	if prune && l.w != nil {
		l.w.submit(func() {
			if err := l.store.Prune(time.Now().Add(-Retention), MaxRows); err != nil {
				slog.Warn("oplog: prune", "scope", "oplog", "err", err)
			}
		})
	}
}

// dedupeLocked returns the live earlier record a background failure folds into, or registers rec as the new anchor.
func (l *Log) dedupeLocked(rec *Record) *Record {
	now := time.Now()
	for k, e := range l.dedupe {
		if now.Sub(e.at) > DedupeWindow {
			delete(l.dedupe, k)
		}
	}
	first, _, _ := strings.Cut(derefStr(rec.Error), "\n")
	key := rec.Module + "\x00" + rec.RepoRoot + "\x00" + rec.Kind + "\x00" + first
	if e, ok := l.dedupe[key]; ok && e.rec != rec && containsRec(l.bg, e.rec) {
		e.at = now
		l.dedupe[key] = e
		return e.rec
	}
	l.dedupe[key] = dedupeEntry{rec: rec, at: now}
	return nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func containsRec(lane []*Record, r *Record) bool {
	for _, x := range lane {
		if x == r {
			return true
		}
	}
	return false
}

func removeRec(lane []*Record, r *Record) []*Record {
	for i, x := range lane {
		if x == r {
			return append(lane[:i], lane[i+1:]...)
		}
	}
	return lane
}

func lastFailedStderr(steps []Step) string {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Status == StatusError && steps[i].Stderr != nil {
			return *steps[i].Stderr
		}
	}
	return ""
}

func (o *Op) clearCancelLocked() bool {
	if _, ok := o.log.cancels[o.rec.ID]; !ok {
		return false
	}
	delete(o.log.cancels, o.rec.ID)
	o.rec.Cancellable = false
	return true
}

// StepRef is one in-flight step of an op. A nil *StepRef does nothing.
type StepRef struct {
	op      *Op
	idx     int // -1: dropped past the step cap
	started time.Time
	write   bool
}

// BeginStep records a git spawn as a step of the op and, for a write, adds it to the command line.
// It returns nil when the op already finished: the caller records its own row instead.
func (o *Op) BeginStep(argv []string, readOnly bool) *StepRef {
	if o == nil {
		return nil
	}
	rendered := renderCommand(argv)
	o.log.mu.Lock()
	if o.rec.Status != StatusRunning {
		o.log.mu.Unlock()
		return nil
	}
	ref := &StepRef{op: o, idx: -1, started: time.Now(), write: !readOnly}
	if len(o.rec.Steps) < maxSteps {
		o.rec.Steps = append(o.rec.Steps, Step{Argv: rendered, ReadOnly: readOnly, Status: StatusRunning})
		ref.idx = len(o.rec.Steps) - 1
	} else {
		o.rec.StepsDropped++
	}
	if !readOnly && !o.cmdFixed && !o.cmdFull {
		if o.cmd != "" {
			o.cmd += " && "
		}
		o.cmd += rendered
		if len(o.cmd) > maxCommandBytes {
			o.cmd = truncateRunes(o.cmd, maxCommandBytes)
			o.cmdFull = true
		}
		c := o.cmd
		o.rec.Command = &c
	}
	var snap Record
	emit := !readOnly
	if emit {
		snap = o.rec.clone()
	}
	o.log.mu.Unlock()
	if emit {
		o.log.publish(snap, !o.bg, false)
	}
	return ref
}

// End finishes the step. exit is nil when the process never produced one.
func (s *StepRef) End(status string, exit *int, stderr string) {
	if s == nil {
		return
	}
	o := s.op
	o.log.mu.Lock()
	if s.idx >= 0 && s.idx < len(o.rec.Steps) {
		st := &o.rec.Steps[s.idx]
		if st.Status != StatusRunning {
			o.log.mu.Unlock()
			return
		}
		ms := int(time.Since(s.started).Milliseconds())
		st.DurationMs = &ms
		st.Status = status
		st.ExitCode = exit
		if status == StatusError && stderr != "" {
			t := RedactText(tail(stderr, maxStderrTail))
			st.Stderr = &t
		}
	}
	emit := (s.write || status == StatusError) && o.rec.Status == StatusRunning
	var snap Record
	if emit {
		snap = o.rec.clone()
	}
	o.log.mu.Unlock()
	if emit {
		o.log.publish(snap, !o.bg, false)
	}
}

func renderCommand(argv []string) string { return renderTool("git", argv) }

func renderTool(name string, argv []string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range argv {
		b.WriteByte(' ')
		b.WriteString(quoteArg(a))
	}
	return b.String()
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsFunc(s, unsafeRune) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func unsafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return false
	}
	return !strings.ContainsRune("_@%+=:,./-", r)
}
