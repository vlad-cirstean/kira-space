package oplog

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/filelock"
	"github.com/kirathecat/kira-studio/internal/ids"
)

// ModuleMemory is the module of memory MCP server ops, written by a process the app does not own.
const ModuleMemory = "memory"

// ForeignStoppedMsg is the error of a foreign row whose writer died while it ran.
const ForeignStoppedMsg = "Memory MCP server stopped while this ran."

// FinishCall ends op from an MCP tools/call verdict: a canceled ctx is cancelled, a Go error or an
// IsError result (with its text) is error, else ok.
func FinishCall(op *Op, isError bool, text string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		op.Finish(StatusCancelled, "")
	case err != nil:
		op.Finish(StatusError, err.Error())
	case isError:
		op.Finish(StatusError, text)
	default:
		op.Finish(StatusOK, "")
	}
}

// WithIDPrefix prefixes every minted op id, marking the rows of an out-of-process writer.
func WithIDPrefix(p string) Option { return func(l *Log) { l.idPrefix = p } }

// Owner is a process that writes op_log from outside the app. It holds <dir>/<token>.lock for its
// life. Take it before the first row is written and release it only after Log.Close drained the
// writer: a running foreign row whose owner lock is free is then truly orphaned.
type Owner struct {
	token string
	path  string
	file  *os.File
}

// ClaimOwner creates dir if missing and locks a fresh owner file in it.
func ClaimOwner(dir string) (*Owner, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	token := ids.NewID()
	path := filepath.Join(dir, token+".lock")
	f, ok, err := filelock.TryLock(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("oplog: owner lock already held")
	}
	return &Owner{token: token, path: path, file: f}, nil
}

// Token is the id prefix owner rows use: pass Token()+"." to WithIDPrefix.
func (o *Owner) Token() string { return o.token }

// Release unlocks and removes the owner file.
func (o *Owner) Release() {
	if o == nil || o.file == nil {
		return
	}
	_ = os.Remove(o.path)
	_ = o.file.Close()
	o.file = nil
}

// OwnerToken returns the owner token of a foreign row id. Owner tokens are UUIDs (older ones base32), so a '.' marks one.
func OwnerToken(id string) (string, bool) {
	t, _, ok := strings.Cut(id, ".")
	return t, ok && t != ""
}

// OwnerAlive reports whether the owner still holds its lock. A free or missing lock means dead; the
// file is then removed. An unreadable dir counts as alive: never fail a live writer's rows.
func OwnerAlive(dir, token string) bool {
	path := filepath.Join(dir, token+".lock")
	f, ok, err := filelock.TryLock(path)
	switch {
	case err != nil:
		return !errors.Is(err, fs.ErrNotExist)
	case !ok:
		return true
	}
	_ = os.Remove(path)
	_ = f.Close()
	return false
}

// ForeignStore reads and fails the rows other processes wrote. Rowid is the cursor: SQLite assigns
// it under the write lock, so rowid order is commit order across processes, and an upsert keeps it.
type ForeignStore interface {
	// DataVersion moves only when another connection commits.
	DataVersion() (int64, error)
	MaxRowID() (int64, error)
	// ForeignAfter returns foreign rows with a rowid past rowid or an id in ids, and the new max rowid.
	ForeignAfter(rowid int64, ids []string) ([]Record, int64, error)
	RunningForeignIDs() ([]string, error)
	// FailRunning turns the still-running rows among ids into errors carrying msg.
	FailRunning(ids []string, msg string) error
}

// ForeignCursor is where a watcher resumes. Take it before Seed reads the table, so a row
// committed in between is read twice (Ingest is idempotent) and never missed.
type ForeignCursor struct{ Version, RowID int64 }

// NewForeignCursor reads the current cursor.
func NewForeignCursor(s ForeignStore) (ForeignCursor, error) {
	v, err := s.DataVersion()
	if err != nil {
		return ForeignCursor{}, err
	}
	r, err := s.MaxRowID()
	return ForeignCursor{Version: v, RowID: r}, err
}

// SweepDeadOwners fails every running foreign row whose owner is gone.
func SweepDeadOwners(s ForeignStore, ownersDir string) error {
	ids, err := s.RunningForeignIDs()
	if err != nil {
		return err
	}
	_, err = failDead(s, ownersDir, ids)
	return err
}

// failDead fails the rows among ids whose owner is dead and returns their ids.
func failDead(s ForeignStore, ownersDir string, ids []string) ([]string, error) {
	alive := map[string]bool{}
	var dead []string
	for _, id := range ids {
		tok, _ := OwnerToken(id)
		a, seen := alive[tok]
		if !seen {
			a = OwnerAlive(ownersDir, tok)
			alive[tok] = a
		}
		if !a {
			dead = append(dead, id)
		}
	}
	if len(dead) == 0 {
		return nil, nil
	}
	return dead, s.FailRunning(dead, ForeignStoppedMsg)
}

// Ingest merges rows another process already persisted into the lanes and publishes real changes.
// It never persists and never dedupes: dedupe is a writer-side rule.
func (l *Log) Ingest(recs []Record) {
	if l == nil {
		return
	}
	for i := range recs {
		r := recs[i]
		r.Cancellable = false
		if r.Steps == nil {
			r.Steps = []Step{}
		}
		l.mu.Lock()
		lane, capacity := &l.fg, FgCapacity
		if r.Background {
			lane, capacity = &l.bg, BgCapacity
		}
		changed := true
		if idx := laneIndex(*lane, r.ID); idx >= 0 {
			changed = !sameRecord((*lane)[idx], &r)
			if changed {
				(*lane)[idx] = &r
			}
		} else {
			*lane = insertByStart(*lane, &r, capacity)
		}
		snap := r.clone()
		l.mu.Unlock()
		if changed {
			l.publish(snap, !r.Background, false)
		}
	}
}

func laneIndex(lane []*Record, id string) int {
	for i, x := range lane {
		if x.ID == id {
			return i
		}
	}
	return -1
}

// insertByStart places rec newest first by StartedAt, dropping the oldest past capacity.
func insertByStart(lane []*Record, rec *Record, capacity int) []*Record {
	at := len(lane)
	for i, x := range lane {
		if rec.StartedAt >= x.StartedAt {
			at = i
			break
		}
	}
	if at >= capacity {
		return lane
	}
	lane = append(lane, nil)
	copy(lane[at+1:], lane[at:])
	lane[at] = rec
	if len(lane) > capacity {
		clear(lane[capacity:])
		lane = lane[:capacity]
	}
	return lane
}

func sameRecord(a, b *Record) bool {
	return a.Status == b.Status && a.Repeat == b.Repeat && a.Source == b.Source && a.Subject == b.Subject &&
		derefStr(a.Error) == derefStr(b.Error) && derefStr(a.Command) == derefStr(b.Command) &&
		(a.DurationMs == nil) == (b.DurationMs == nil) && (a.DurationMs == nil || *a.DurationMs == *b.DurationMs)
}

// trackedForeignIDs lists the foreign ids whose rows can still change: running ones, and background
// ones, whose failure rows gain a repeat count.
func (l *Log) trackedForeignIDs() (tracked, running []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, lane := range [][]*Record{l.fg, l.bg} {
		for _, r := range lane {
			if _, ok := OwnerToken(r.ID); !ok {
				continue
			}
			if r.Status == StatusRunning {
				running = append(running, r.ID)
			}
			if r.Status == StatusRunning || r.Background {
				tracked = append(tracked, r.ID)
			}
		}
	}
	return tracked, running
}

// WatchForeign ingests foreign rows into l: each tick re-reads when data_version moved, and fails
// running rows whose owner died. It stops with l.Close.
func WatchForeign(l *Log, s ForeignStore, ownersDir string, every time.Duration, cur ForeignCursor) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	l.mu.Lock()
	l.closers = append(l.closers, func() { cancel(); <-done })
	l.mu.Unlock()
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cur = tickForeign(l, s, ownersDir, cur)
			}
		}
	}()
}

func tickForeign(l *Log, s ForeignStore, ownersDir string, cur ForeignCursor) ForeignCursor {
	tracked, running := l.trackedForeignIDs()
	v, err := s.DataVersion()
	if err != nil {
		slog.Warn("oplog: data_version", "scope", "oplog", "err", err)
		return cur
	}
	if v != cur.Version {
		recs, top, err := s.ForeignAfter(cur.RowID, tracked)
		if err != nil {
			slog.Warn("oplog: read foreign", "scope", "oplog", "err", err)
			return cur
		}
		l.Ingest(recs)
		cur = ForeignCursor{Version: v, RowID: top}
	}
	if len(running) == 0 {
		return cur
	}
	dead, err := failDead(s, ownersDir, running)
	if err != nil {
		slog.Warn("oplog: fail orphaned", "scope", "oplog", "err", err)
		return cur
	}
	if len(dead) > 0 {
		recs, _, err := s.ForeignAfter(cur.RowID, dead)
		if err != nil {
			slog.Warn("oplog: read foreign", "scope", "oplog", "err", err)
			return cur
		}
		l.Ingest(recs)
	}
	return cur
}
