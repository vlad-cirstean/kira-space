package oplog

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiratime"
)

// DbEvent is the database host's op event. DbEventSource is consumer-declared, so the payload type
// lives with the consumer, not the producer.
type DbEvent struct {
	Topic   string
	Payload json.RawMessage
}

// Topics the consume loop switches on (packages/shared/protocol/engine-ops.ts opStart/opEnd).
const (
	DbEventOpStart = "op:start"
	DbEventOpEnd   = "op:end"
)

// DbEventSource is the slice of the producer DbSource consumes: adapterhost.Host in production, a
// fake channel in a test.
type DbEventSource interface {
	Subscribe() (<-chan DbEvent, func())
}

const (
	// pruneEveryDbOps bounds the database rows between two prunes.
	pruneEveryDbOps = 500
	// DbHardCapRows caps the database rows kept.
	DbHardCapRows = 20_000

	maxDbCommandBytes = 64 * 1024
	maxDbErrorBytes   = 8 * 1024
	dbStopWait        = 2 * time.Second
)

// opEndStatuses is op:end's status enum, narrower than a record's: op:end is never "running".
var opEndStatuses = map[string]bool{"ok": true, "error": true, "cancelled": true}

const appExitedError = "app exited"

// DbSource turns the database host's op events into records on a Log. An incognito op shows live
// and is never stored.
type DbSource struct {
	src           DbEventSource
	log           *Log
	prune         func(category string, cutoff time.Time, maxRows int) error
	connName      func(id string) string
	retentionDays func() int

	unsubscribe func()
	done        chan struct{}
}

// NewDbSource builds the source. connName resolves a connection's display name for the row's
// subject; retentionDays reads advanced.opLogRetentionDays; prune is the store's PruneCategory.
func NewDbSource(src DbEventSource, log *Log, prune func(string, time.Time, int) error, connName func(string) string, retentionDays func() int) *DbSource {
	return &DbSource{src: src, log: log, prune: prune, connName: connName, retentionDays: retentionDays, done: make(chan struct{})}
}

// Start prunes once, then consumes events on one goroutine until Stop or the source's channel closes.
func (d *DbSource) Start() {
	d.pruneDb()
	events, unsubscribe := d.src.Subscribe()
	d.unsubscribe = unsubscribe
	go d.consume(events)
}

// Stop ends the consumer and waits for it to settle in-flight ops, bounded: a wedged consumer must
// not hang quit.
func (d *DbSource) Stop() {
	if d.unsubscribe != nil {
		d.unsubscribe()
	}
	select {
	case <-d.done:
	case <-time.After(dbStopWait):
		slog.Warn("db op source did not stop in time", "scope", "oplog")
	}
}

func (d *DbSource) pruneDb() {
	if d.prune == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -d.retentionDays())
	if err := d.prune(CategoryDB, cutoff, DbHardCapRows); err != nil {
		slog.Warn("db op prune failed", "scope", "oplog", "err", err)
	}
}

// inFlight is what op:end needs from its op:start. dup marks a reused id: its row in the log stays
// the first op's, so only live updates go out.
type inFlight struct {
	rec       Record
	incognito bool
	dup       bool
}

// consume is the only reader and writer of ops, so the map needs no mutex.
func (d *DbSource) consume(events <-chan DbEvent) {
	defer close(d.done)
	ops := map[string]*inFlight{}
	finished := 0
	for evt := range events {
		switch evt.Topic {
		case DbEventOpStart:
			d.start(evt.Payload, ops)
		case DbEventOpEnd:
			if d.end(evt.Payload, ops) {
				if finished++; finished >= pruneEveryDbOps {
					finished = 0
					d.pruneDb()
				}
			}
		}
	}
	d.settleAll(ops)
}

func (d *DbSource) start(payload json.RawMessage, ops map[string]*inFlight) {
	var evt struct {
		OpID         string  `json:"opId"`
		ConnectionID *string `json:"connectionId"`
		TabID        *string `json:"tabId"`
		Kind         string  `json:"kind"`
		StartedAt    string  `json:"startedAt"`
		Incognito    bool    `json:"incognito"`
		DelayMs      int     `json:"delayMs"`
		Source       string  `json:"source"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil || !model.ValidOpKind(evt.Kind) {
		return
	}
	if !model.ValidOpSource(evt.Source) {
		evt.Source = "user"
	}
	rec := Record{
		ID: evt.OpID, StartedAt: evt.StartedAt, Category: CategoryDB, Kind: evt.Kind, Status: StatusRunning,
		Module: ModuleDB, Trigger: evt.Source, Source: evt.Source, Repeat: 1, Steps: []Step{}, Cancellable: true,
		ConnectionID: evt.ConnectionID, TabID: evt.TabID,
	}
	if evt.ConnectionID != nil && d.connName != nil {
		rec.Subject = d.connName(*evt.ConnectionID)
	}
	if evt.DelayMs > 0 {
		rec.DelayMs = &evt.DelayMs
	}
	op := &inFlight{rec: rec, incognito: evt.Incognito}
	if !d.log.Admit(rec, !evt.Incognito) {
		op.dup = true
		d.log.Announce(rec)
	}
	ops[evt.OpID] = op
}

// end reports whether a stored op finished, for the prune counter.
func (d *DbSource) end(payload json.RawMessage, ops map[string]*inFlight) bool {
	var evt struct {
		OpID       string  `json:"opId"`
		Status     string  `json:"status"`
		DurationMs int     `json:"durationMs"`
		Rows       *int    `json:"rows"`
		Command    *string `json:"command"`
		Error      *string `json:"error"`
		Path       *string `json:"path"`
	}
	if err := json.Unmarshal(payload, &evt); err != nil || !opEndStatuses[evt.Status] {
		return false
	}
	op, ok := ops[evt.OpID]
	delete(ops, evt.OpID)
	if !ok {
		// An op:end with no op:start still shows once, as a "test" op, so the panel never loses a result.
		op = &inFlight{rec: Record{
			ID: evt.OpID, StartedAt: kiratime.NowISO(), Category: CategoryDB, Kind: "test", Module: ModuleDB,
			Trigger: "user", Source: "user", Repeat: 1, Steps: []Step{}, Status: StatusRunning,
		}, dup: true}
	}
	apply := func(r *Record) {
		r.Status = evt.Status
		r.DurationMs = &evt.DurationMs
		r.Rows = evt.Rows
		r.Path = evt.Path
		r.Cancellable = false
		r.Command, r.CommandTruncated = capCommand(evt.Command)
		r.Error = capError(evt.Error)
	}
	if op.dup {
		rec := op.rec
		apply(&rec)
		d.log.Announce(rec)
		return false
	}
	_, settled := d.log.Settle(evt.OpID, apply)
	return settled && !op.incognito
}

// settleAll finishes whatever never reached op:end when the source closed.
func (d *DbSource) settleAll(ops map[string]*inFlight) {
	now := time.Now()
	msg := appExitedError
	for id, op := range ops {
		delete(ops, id)
		if op.dup {
			continue
		}
		ms := 0
		if started, err := kiratime.ParseISO(op.rec.StartedAt); err == nil {
			ms = max(int(now.Sub(started).Milliseconds()), 0)
		}
		d.log.Settle(id, func(r *Record) {
			r.Status = StatusError
			r.DurationMs = &ms
			r.Error = &msg
			r.Cancellable = false
		})
	}
}

func capCommand(s *string) (*string, bool) {
	if s == nil || len(*s) <= maxDbCommandBytes {
		return s, false
	}
	c := truncateRunes(*s, maxDbCommandBytes)
	return &c, true
}

func capError(s *string) *string {
	if s == nil || len(*s) <= maxDbErrorBytes {
		return s
	}
	c := truncateRunes(*s, maxDbErrorBytes)
	return &c
}
