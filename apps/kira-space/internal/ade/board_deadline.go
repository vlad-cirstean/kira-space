package ade

import (
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// repoFactsDeadline is how long a board build waits for one repo's git facts once it has an
// earlier answer to show instead.
const repoFactsDeadline = 1 * time.Second

// repoLast is a repo's last finished facts. rows holds each row's hash so an overlay keeps only the
// facts of rows that did not change. late marks a result that landed after its build gave up and
// still matches the repo's change counter: builds with the same rows and counter use it as is until
// it is boardSnapshotFresh old.
type repoLast struct {
	seq   uint64
	sig   string
	rows  map[string]string
	res   *repoResult
	late  bool
	moved uint64
	at    time.Time
}

// repoFlight is one running facts computation, joined by every build of the same rows and the same
// change counter of the repo (the board-wide gen moves for every unrelated write and is no
// signal). A build that sees a newer counter waits for the flight and then starts the one
// successor, so a repo never runs two computations of the same rows at once.
type repoFlight struct {
	sig   string
	moved uint64
	began time.Time
	done  chan struct{}
	res   *repoResult
	// abandoned is set, under boardFacts.mu, by a build that stopped waiting.
	abandoned bool
}

// boardFacts is the per-repo state behind repoFactsDeadlined.
type boardFacts struct {
	mu     sync.Mutex
	seq    uint64
	last   map[string]*repoLast
	flight map[string]*repoFlight
	// moved counts, per repo, the changes that can alter its facts (a ref, worktree or change
	// request event, a Refresh, a finished conflict check); all counts those of every repo.
	moved map[string]uint64
	all   uint64
	// hook runs at the start of every computation; tests stall one with it.
	hook func(codeRepoID string)
	// deadline overrides repoFactsDeadline in tests.
	deadline time.Duration
}

// repoMoved records a change that can alter the facts of one repo. Call it before the board
// invalidation that announces the change, so a build of the new gen sees the new counter.
func (b *TaskBoard) repoMoved(id string) {
	f := &b.facts
	f.mu.Lock()
	if f.moved == nil {
		f.moved = map[string]uint64{}
	}
	f.moved[id]++
	f.mu.Unlock()
}

// allReposMoved is repoMoved for a change every repo sees (repo list or config).
func (b *TaskBoard) allReposMoved() {
	f := &b.facts
	f.mu.Lock()
	f.all++
	f.mu.Unlock()
}

// movedOf is the repo's change counter. The caller holds f.mu.
func (f *boardFacts) movedOf(id string) uint64 { return f.moved[id] + f.all }

func rowHash(sb model.AdeTaskBranch) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", sb.ID, sb.Name, sb.Base, sb.BaseBranchID, sb.Kind)
}

// rowsSig hashes the rows and their worktree setups: a setup change restarts the computation.
func rowsSig(rows []model.AdeTaskBranch, setups map[string]model.AdeWorktreeSetup) (string, map[string]string) {
	h := fnv.New64a()
	per := make(map[string]string, len(rows))
	for _, sb := range rows {
		s := rowHash(sb)
		per[sb.ID] = s
		_, _ = h.Write([]byte(s + "\x01"))
		if su, ok := setups[sb.ID]; ok {
			_, _ = fmt.Fprintf(h, "%s\x00%d\x00%v\x00%v\x00%s\x01", su.State, su.StartedAt, su.FinishedAt != nil, su.ExitCode != nil, su.Note)
		}
	}
	return fmt.Sprintf("%x", h.Sum64()), per
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func cloneRepoResult(r *repoResult) *repoResult {
	c := *r
	c.branches = maps.Clone(r.branches)
	c.pairs = append([]adewire.Pair(nil), r.pairs...)
	c.jobs = maps.Clone(r.jobs)
	c.merged = maps.Clone(r.merged)
	return &c
}

// repoFactsDeadlined is repoFactsWith for a board build: the first load of a repo waits for its
// facts, later builds wait at most repoFactsDeadline and then show the last good facts with
// RepoState.Updating set. The computation always runs to the end and pushes the board when it
// lands late.
func (b *TaskBoard) repoFactsDeadlined(ctx context.Context, id string, rows []model.AdeTaskBranch, setups map[string]model.AdeWorktreeSetup, shared *boardShared) *repoResult {
	f := &b.facts
	sig, per := rowsSig(rows, setups)

	wait := repoFactsDeadline
	if f.deadline > 0 {
		wait = f.deadline
	}
	buildStart := time.Now()
	for {
		f.mu.Lock()
		cur := f.movedOf(id)
		if l := f.last[id]; l != nil && l.late && l.sig == sig && l.moved == cur && time.Since(l.at) < boardSnapshotFresh {
			res := cloneRepoResult(l.res)
			f.mu.Unlock()
			return res
		}
		fl := f.flight[id]
		if fl == nil || fl.sig != sig {
			fl = &repoFlight{sig: sig, moved: cur, began: time.Now(), done: make(chan struct{})}
			f.flight[id] = fl
			f.seq++
			b.startFacts(id, fl, f.seq, per, rows, setups, shared)
		}
		behind := fl.moved != cur // the flight read its tips before a change this build must show
		last := f.last[id]
		f.mu.Unlock()

		if last == nil {
			select {
			case <-fl.done:
				if behind {
					continue
				}
				return cloneRepoResult(fl.res)
			case <-ctx.Done():
				return b.repoFactsWith(ctx, id, rows, setups, shared)
			}
		}
		t := time.NewTimer(max(0, wait-time.Since(earlier(fl.began, buildStart))))
		select {
		case <-fl.done:
			t.Stop()
			if behind {
				continue
			}
			return cloneRepoResult(fl.res)
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
		}
		f.mu.Lock()
		select {
		case <-fl.done: // landed between the timer and the lock
			f.mu.Unlock()
			if behind {
				continue
			}
			return cloneRepoResult(fl.res)
		default:
		}
		fl.abandoned = true
		if l := f.last[id]; l != nil {
			last = l
		}
		f.mu.Unlock()
		return overlayFacts(last, rows, per, setups)
	}
}

// startFacts runs the computation on the board's context, tracked so Close waits for it. The
// caller holds f.mu.
func (b *TaskBoard) startFacts(id string, fl *repoFlight, seq uint64, per map[string]string, rows []model.AdeTaskBranch, setups map[string]model.AdeWorktreeSetup, shared *boardShared) {
	f := &b.facts
	started := b.track()
	if !started {
		fl.res = &repoResult{}
		close(fl.done)
		return
	}
	go func() {
		defer b.wg.Done()
		if f.hook != nil {
			f.hook(id)
		}
		res := b.repoFactsWith(b.ctx, id, rows, setups, shared)
		f.mu.Lock()
		fl.res = res
		if f.flight[id] == fl {
			delete(f.flight, id)
		}
		// A result of a repo that changed since the flight read its tips still feeds overlays but
		// is no answer; its landing schedules a build that starts the successor.
		late := fl.abandoned
		fresh := late && fl.moved == f.movedOf(id)
		if l := f.last[id]; l == nil || l.seq < seq {
			f.last[id] = &repoLast{seq: seq, sig: fl.sig, rows: per, res: cloneRepoResult(res), late: fresh, moved: fl.moved, at: time.Now()}
		}
		close(fl.done)
		f.mu.Unlock()
		if late {
			b.scheduleBoard()
		}
	}()
}

// overlayFacts is the last good facts under the current rows: a row that did not change keeps its
// facts, any other row shows stored meta only. It queues no jobs and writes no marks.
func overlayFacts(last *repoLast, rows []model.AdeTaskBranch, per map[string]string, setups map[string]model.AdeWorktreeSetup) *repoResult {
	res := &repoResult{
		state:    last.res.state,
		branches: make(map[string]adewire.Branch, len(rows)),
		pairs:    []adewire.Pair{}, merged: map[string]int64{}, jobs: map[string]rebaseJob{},
	}
	res.state.Updating = true
	for _, sb := range rows {
		var setup *adewire.WorktreeSetup
		if s, ok := setups[sb.ID]; ok {
			setup = toWireSetup(s)
		}
		if wb, ok := last.res.branches[sb.ID]; ok && last.rows[sb.ID] == per[sb.ID] {
			wb.Setup = setup
			if wb.Worktree == "" && wb.Setup != nil && wb.Setup.State == "ready" {
				wb.Setup = nil // same stale-setup rule as computeBranch
			}
			res.branches[sb.ID] = wb
			continue
		}
		res.branches[sb.ID] = zeroBranch(sb, sb.Base, setup)
	}
	for _, p := range last.res.pairs {
		if _, a := res.branches[p.A]; !a {
			continue
		}
		if _, c := res.branches[p.B]; c && last.rows[p.A] == per[p.A] && last.rows[p.B] == per[p.B] {
			res.pairs = append(res.pairs, p)
		}
	}
	return res
}
