package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/updates"
)

// Defaults for UpdatesConfig's zero fields.
const (
	defaultUpdatesMinGap       = 30 * time.Second
	defaultUpdatesInterval     = 5 * time.Minute
	defaultUpdatesJiraInterval = 10 * time.Minute
	updatesIssueTimeout        = 15 * time.Second
	updatesIssueFanOut         = 2
)

// UpdatesConfig tunes the tracked item watcher. Zero durations take the defaults.
type UpdatesConfig struct {
	// Disabled turns off the board-change and timer triggers; Refresh still observes.
	Disabled bool
	// MinGap is the least time between two passes the board-change trigger starts.
	MinGap time.Duration
	// Interval is the timer period.
	Interval time.Duration
	// JiraInterval is the least time between two reads of one task's issue in a non-fresh pass.
	JiraInterval time.Duration
}

func (c UpdatesConfig) withDefaults() UpdatesConfig {
	if c.MinGap <= 0 {
		c.MinGap = defaultUpdatesMinGap
	}
	if c.Interval <= 0 {
		c.Interval = defaultUpdatesInterval
	}
	if c.JiraInterval <= 0 {
		c.JiraInterval = defaultUpdatesJiraInterval
	}
	return c
}

// updateWatcher compares what each tracked item looks like now against what the user last saw,
// and keeps the two snapshots per item in ade_tracked_items.
type updateWatcher struct {
	b   *TaskBoard
	cfg UpdatesConfig

	runMu sync.Mutex // serializes passes
	sf    singleflight.Group

	mu      sync.Mutex // guards the fields below
	lastRun time.Time
	jiraAt  map[string]time.Time // task id -> last issue read
	timer   *time.Timer
	pending *time.Timer // the board-change trigger waiting out MinGap
}

func newUpdateWatcher(b *TaskBoard, cfg UpdatesConfig) *updateWatcher {
	return &updateWatcher{b: b, cfg: cfg.withDefaults(), jiraAt: map[string]time.Time{}}
}

// start runs a first pass and arms the timer.
func (w *updateWatcher) start() {
	if w.cfg.Disabled || w.b.deps.TrackedItems == nil {
		return
	}
	w.b.goTracked(func() { w.run(w.b.ctx, false) })
	w.mu.Lock()
	defer w.mu.Unlock()
	w.timer = time.AfterFunc(w.cfg.Interval, w.tick)
}

func (w *updateWatcher) tick() {
	w.b.runTracked(func() { w.run(w.b.ctx, false) })
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.b.ctx.Err() == nil {
		w.timer = time.AfterFunc(w.cfg.Interval, w.tick)
	}
}

func (w *updateWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	if w.pending != nil {
		w.pending.Stop()
	}
}

// boardChanged schedules a pass after a board change, at most one per MinGap.
func (w *updateWatcher) boardChanged() {
	if w.cfg.Disabled || w.b.deps.TrackedItems == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != nil {
		return
	}
	wait := max(w.cfg.MinGap-time.Since(w.lastRun), 0)
	w.pending = time.AfterFunc(wait, func() {
		w.mu.Lock()
		w.pending = nil
		w.mu.Unlock()
		w.b.runTracked(func() { w.run(w.b.ctx, false) })
	})
}

// run observes once and logs a failure; concurrent passes of one kind share a result.
func (w *updateWatcher) run(ctx context.Context, fresh bool) {
	if err := w.observe(ctx, fresh); err != nil && ctx.Err() == nil {
		slog.Warn("ade updates: observe", "scope", "ade", "err", err)
	}
}

func (w *updateWatcher) observe(ctx context.Context, fresh bool) error {
	key := "observe"
	if fresh {
		key = "observe-fresh"
	}
	_, err, _ := w.sf.Do(key, func() (any, error) {
		w.runMu.Lock()
		defer w.runMu.Unlock()
		err := w.pass(ctx, fresh)
		w.mu.Lock()
		w.lastRun = time.Now()
		w.mu.Unlock()
		return nil, err
	})
	return err
}

// observation is one item's current snapshot; the previous row, if any, is prev.
type observation struct {
	taskID, key, kind string
	snap              string
}

func (w *updateWatcher) pass(ctx context.Context, fresh bool) error {
	b := w.b
	store := b.deps.TrackedItems
	if store == nil {
		return nil
	}
	tasks, err := b.deps.Tasks.ListLive()
	if err != nil {
		return err
	}
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return err
	}
	existing, err := store.List()
	if err != nil {
		return err
	}
	prev := make(map[model.AdeTrackedKey]model.AdeTrackedItem, len(existing))
	for _, it := range existing {
		prev[model.AdeTrackedKey{TaskID: it.TaskID, ItemKey: it.ItemKey}] = it
	}

	live := make(map[string]model.AdeTask, len(tasks))
	for _, t := range tasks {
		live[t.ID] = t
	}
	created := make([]model.AdeTaskBranch, 0, len(branches))
	for _, br := range branches {
		if _, ok := live[br.TaskID]; ok && br.Name != "" {
			created = append(created, br)
		}
	}

	// wanted holds every item the watcher should keep; observed the subset it could read this pass.
	wanted := map[model.AdeTrackedKey]bool{}
	for _, br := range created {
		wanted[model.AdeTrackedKey{TaskID: br.TaskID, ItemKey: "cr:" + br.ID}] = true
		wanted[model.AdeTrackedKey{TaskID: br.TaskID, ItemKey: "branch:" + br.ID}] = true
	}
	var observed []observation
	observed = append(observed, w.branchObservations(ctx, created, prev)...)
	observed = append(observed, w.crObservations(ctx, created, prev)...)

	issueTasks := w.issueTasks(live)
	for _, t := range issueTasks {
		wanted[model.AdeTrackedKey{TaskID: t.task.ID, ItemKey: "issue"}] = true
	}
	observed = append(observed, w.issueObservations(ctx, issueTasks, prev, fresh)...)

	now := b.deps.Now().UnixMilli()
	var upserts []model.AdeTrackedItem
	for _, o := range observed {
		k := model.AdeTrackedKey{TaskID: o.taskID, ItemKey: o.key}
		old, had := prev[k]
		switch {
		case !had:
			upserts = append(upserts, model.AdeTrackedItem{
				TaskID: o.taskID, ItemKey: o.key, Kind: o.kind, SeenJSON: o.snap, CurrentJSON: o.snap, SeenAt: now, ObservedAt: now,
			})
		case old.CurrentJSON != o.snap:
			it := old
			it.CurrentJSON, it.ObservedAt = o.snap, now
			ch := updates.Diff(o.kind, old.SeenJSON, o.snap)
			switch {
			case ch.Reset:
				it.SeenJSON, it.SeenAt, it.ChangedAt = o.snap, now, 0
			case len(ch.Concepts) == 0:
				it.ChangedAt = 0
			default:
				it.ChangedAt = now
			}
			upserts = append(upserts, it)
		}
	}
	var deletes []model.AdeTrackedKey
	for k := range prev {
		if !wanted[k] {
			deletes = append(deletes, k)
		}
	}
	if err := store.Apply(upserts, deletes); err != nil {
		return err
	}
	if len(upserts) > 0 || len(deletes) > 0 {
		b.notifyUpdates()
	}
	return nil
}

func (b *TaskBoard) notifyUpdates() {
	if b.deps.OnUpdates != nil {
		b.deps.OnUpdates()
	}
}

// branchObservations reads each created branch's facts off the board. A conflict check still
// running keeps the branch's previous conflict value.
func (w *updateWatcher) branchObservations(ctx context.Context, created []model.AdeTaskBranch, prev map[model.AdeTrackedKey]model.AdeTrackedItem) []observation {
	if len(created) == 0 {
		return nil
	}
	board, err := w.b.Board(ctx)
	if err != nil {
		slog.Warn("ade updates: board", "scope", "ade", "err", err)
		return nil
	}
	byID := make(map[string]adewire.Branch, len(board.Branches))
	for _, br := range board.Branches {
		byID[br.ID] = br
	}
	var out []observation
	for _, tb := range created {
		br, ok := byID[tb.ID]
		if !ok {
			continue
		}
		snap := updates.BranchSnap{
			Tip: br.Tip, Behind: br.Behind, UpstreamBehind: br.UpstreamBehind, Merged: br.MergedIntoMain,
			Deployments: make(map[string]string, len(br.Deployments)),
		}
		for _, d := range br.Deployments {
			snap.Deployments[d.Env] = d.Status
		}
		if br.ConflictCheck == "done" {
			snap.ConflictsKnown, snap.Conflicts = true, slices.Clone(br.ConflictsIfRebased)
		} else if old, ok := prev[model.AdeTrackedKey{TaskID: tb.TaskID, ItemKey: "branch:" + tb.ID}]; ok {
			var was updates.BranchSnap
			if updates.DecodeBranch(old.CurrentJSON, &was) {
				snap.ConflictsKnown, snap.Conflicts = was.ConflictsKnown, was.Conflicts
			}
		}
		out = append(out, observation{taskID: tb.TaskID, key: "branch:" + tb.ID, kind: updates.KindBranch, snap: snap.Encode()})
	}
	return out
}

// crObservations resolves each created branch's change request per repo. A repo whose host lookup
// is not ok keeps its branches' previous snapshots.
func (w *updateWatcher) crObservations(ctx context.Context, created []model.AdeTaskBranch, prev map[model.AdeTrackedKey]model.AdeTrackedItem) []observation {
	repoIDs, byRepo := branchesByRepo(created)
	var mu sync.Mutex
	var out []observation
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(boardFanOut)
	for _, id := range repoIDs {
		g.Go(func() error {
			entry, err := w.b.openRepo(gctx, id)
			if err != nil {
				slog.Warn("ade updates: open repo", "scope", "ade", "repo", id, "err", err)
				return nil
			}
			var rows []observation
			for _, br := range byRepo[id] {
				res := entry.ResolveBranchChangeRequest(gctx, br.Name)
				if res.Kind != "ok" {
					continue
				}
				snap := updates.CRSnap{}
				if len(res.ChangeRequests) > 0 {
					cr := res.ChangeRequests[0]
					snap = updates.CRSnap{
						Present: true, Number: cr.Number, State: cr.State, Title: cr.Title, HeadSha: cr.HeadSha,
						ReviewDecision: cr.ReviewDecision, ChecksState: cr.ChecksState,
						CommentCount: cr.CommentCount, Assignees: slices.Clone(cr.Assignees),
					}
					// The per-branch fallback carries no counts: keep the last known ones.
					if old, ok := prev[model.AdeTrackedKey{TaskID: br.TaskID, ItemKey: "cr:" + br.ID}]; ok {
						var was updates.CRSnap
						if updates.DecodeCR(old.CurrentJSON, &was) && was.Present && was.Number == snap.Number {
							if snap.CommentCount == nil {
								snap.CommentCount = was.CommentCount
							}
							if snap.Assignees == nil {
								snap.Assignees = was.Assignees
							}
						}
					}
				}
				rows = append(rows, observation{taskID: br.TaskID, key: "cr:" + br.ID, kind: updates.KindChangeRequest, snap: snap.Encode()})
			}
			mu.Lock()
			out = append(out, rows...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // per-repo failures keep the previous snapshots
	return out
}

type issueTask struct {
	task     model.AdeTask
	provider issuetracker.Provider
	ref      issuetracker.Ref
}

// issueTasks lists the live tasks whose linked issue a tracker with details can read.
func (w *updateWatcher) issueTasks(live map[string]model.AdeTask) []issueTask {
	var out []issueTask
	for _, t := range live {
		for _, p := range w.b.deps.Trackers.All() {
			if _, ok := p.(issuetracker.IssueDetails); !ok {
				continue
			}
			if ref, ok := trackerRef(p, t); ok {
				out = append(out, issueTask{task: t, provider: p, ref: ref})
				break
			}
		}
	}
	return out
}

func (w *updateWatcher) issueObservations(ctx context.Context, tasks []issueTask, prev map[model.AdeTrackedKey]model.AdeTrackedItem, fresh bool) []observation {
	var mu sync.Mutex
	var out []observation
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(updatesIssueFanOut)
	for _, it := range tasks {
		w.mu.Lock()
		last := w.jiraAt[it.task.ID]
		w.mu.Unlock()
		_, had := prev[model.AdeTrackedKey{TaskID: it.task.ID, ItemKey: "issue"}]
		if !fresh && had && time.Since(last) < w.cfg.JiraInterval {
			continue
		}
		g.Go(func() error {
			d, err := w.readIssue(gctx, it, fresh)
			if err != nil {
				slog.Debug("ade updates: issue", "scope", "ade", "task", it.task.ID, "err", err)
				return nil
			}
			w.mu.Lock()
			w.jiraAt[it.task.ID] = time.Now()
			w.mu.Unlock()
			snap := updates.IssueSnap{Key: d.Ref.Key, Title: d.Title, Status: d.Status, Priority: d.Priority}
			if d.Assignee != nil {
				snap.Assignee = d.Assignee.ID
			}
			if !d.CommentsOmitted {
				n := d.CommentsTotal
				snap.Comments = &n
			}
			if d.Summary.Known {
				n := d.Summary.Count
				snap.Linked = &n
			}
			mu.Lock()
			out = append(out, observation{taskID: it.task.ID, key: "issue", kind: updates.KindIssue, snap: snap.Encode()})
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // a tracker failure keeps the previous snapshot
	return out
}

// readIssue reads the saved copy and falls back to a fresh read when it is older than JiraInterval.
func (w *updateWatcher) readIssue(ctx context.Context, it issueTask, fresh bool) (issuetracker.IssueDetail, error) {
	det := it.provider.(issuetracker.IssueDetails)
	ctx, cancel := context.WithTimeout(ctx, updatesIssueTimeout)
	defer cancel()
	d, err := det.IssueDetail(ctx, it.ref, fresh)
	if err != nil {
		return d, err
	}
	if !fresh && time.Since(time.UnixMilli(d.FetchedAt)) > w.cfg.JiraInterval {
		return det.IssueDetail(ctx, it.ref, true)
	}
	return d, nil
}

// TrackedUpdates returns every tracked item whose current snapshot differs from the seen one. The
// concepts are unfiltered: which count as meaningful is the user's setting, applied by the renderer.
func (b *TaskBoard) TrackedUpdates(_ context.Context) (adewire.TrackedUpdatesResult, error) {
	out := adewire.TrackedUpdatesResult{Updates: []adewire.TrackedUpdate{}}
	if b.deps.TrackedItems == nil {
		return out, nil
	}
	rows, err := b.deps.TrackedItems.List()
	if err != nil {
		return out, err
	}
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		return out, err
	}
	names := make(map[string]model.AdeTaskBranch, len(branches))
	for _, br := range branches {
		names[br.ID] = br
	}
	for _, row := range rows {
		if row.SeenJSON == row.CurrentJSON {
			continue
		}
		ch := updates.Diff(row.Kind, row.SeenJSON, row.CurrentJSON)
		if len(ch.Concepts) == 0 {
			continue
		}
		u := adewire.TrackedUpdate{
			TaskID: row.TaskID, ItemKey: row.ItemKey, Kind: row.Kind, Concepts: ch.Concepts,
			Counts: map[string]int{}, ChangedAt: row.ChangedAt,
		}
		maps.Copy(u.Counts, ch.Counts)
		switch row.Kind {
		case updates.KindChangeRequest:
			var s updates.CRSnap
			updates.DecodeCR(row.CurrentJSON, &s)
			u.BranchID = strings.TrimPrefix(row.ItemKey, "cr:")
			u.Label = "#" + strconv.Itoa(s.Number)
		case updates.KindBranch:
			u.BranchID = strings.TrimPrefix(row.ItemKey, "branch:")
			br, ok := names[u.BranchID]
			if !ok {
				continue
			}
			u.Label = br.Name
		case updates.KindIssue:
			var s updates.IssueSnap
			updates.DecodeIssue(row.CurrentJSON, &s)
			u.Label = s.Key
		}
		out.Updates = append(out.Updates, u)
	}
	return out, nil
}

// MarkTrackedSeen marks the given items of a task (none = all of them) as seen.
func (b *TaskBoard) MarkTrackedSeen(_ context.Context, args adewire.MarkTrackedSeenArgs) error {
	if b.deps.TrackedItems == nil {
		return errors.New("ade: tracked items store is not configured")
	}
	changed, err := b.deps.TrackedItems.MarkSeen(args.TaskID, args.ItemKeys, b.deps.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("ade: mark tracked items seen: %w", err)
	}
	if changed {
		b.notifyUpdates()
	}
	return nil
}
