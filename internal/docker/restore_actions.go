package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// healthWait caps the wait for a service_healthy dependency before its dependents start.
var healthWait = 60 * time.Second

// RestoreArgs is RestoreContainers' wire shape.
type RestoreArgs struct {
	EngineID string   `json:"engineId"`
	IDs      []string `json:"ids"`
}

// RestoreDismissArgs is RestoreDismiss' wire shape.
type RestoreDismissArgs struct {
	EngineID string `json:"engineId"`
}

// RestoreResultItem is one container's outcome.
type RestoreResultItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// RestoreResult is RestoreContainers' result.
type RestoreResult struct {
	Items []RestoreResultItem `json:"items"`
}

func noRestore() RestoreState { return RestoreState{State: "none", Items: []RestoreItem{}} }

// stateNow runs one tick and returns the current engine's state; an unreachable engine reads none.
func (r *restoreTracker) stateNow(ctx context.Context) RestoreState {
	if r == nil {
		return noRestore()
	}
	r.tick(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.reachable {
		return noRestore()
	}
	return r.cur
}

// current is the last published state without a tick; unreachable or off reads none.
func (r *restoreTracker) current() RestoreState {
	if r == nil {
		return noRestore()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.reachable {
		return noRestore()
	}
	return r.cur
}

// pendingFor checks the call targets the engine that is pending right now.
func (r *restoreTracker) pendingFor(engineID string) (RestoreSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.snaps[r.curObs.EngineID]
	if !r.reachable || s == nil || r.curObs.EngineID != engineID || s.State != statePending {
		return RestoreSnapshot{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.restore.notPending.body", nil)
	}
	return *s, nil
}

func (r *restoreTracker) restoreContainers(ctx context.Context, args RestoreArgs) (RestoreResult, error) {
	if r == nil {
		return RestoreResult{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.restore.notPending.body", nil)
	}
	r.tick(ctx)
	snap, err := r.pendingFor(args.EngineID)
	if err != nil {
		return RestoreResult{}, err
	}
	if len(args.IDs) == 0 {
		return RestoreResult{}, ipcerr.Required("ids")
	}
	r.mu.Lock()
	open := map[string]bool{}
	for _, it := range buildState(&snap, r.curObs, nowMs(), r.t).Items {
		open[it.ID] = it.Status == itemRestorable || it.Status == itemWaiting
	}
	r.mu.Unlock()
	picked := map[string]bool{}
	for _, id := range args.IDs {
		if !open[id] {
			return RestoreResult{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.core.docker.restore.notRestorable.body", map[string]string{"id": id})
		}
		picked[id] = true
	}
	var sel []RestoreEntry
	for _, e := range snap.Entries {
		if picked[e.ID] {
			sel = append(sel, e)
		}
	}

	res := RestoreResult{Items: make([]RestoreResultItem, 0, len(sel))}
	failed := map[string]bool{}
	for _, e := range orderRestore(sel) {
		it := RestoreResultItem{ID: e.ID, Name: e.Name}
		err := r.waitDependencies(ctx, e, snap.Entries, failed)
		if err == nil {
			err = r.m.start(ctx, e.ID)
		}
		if err != nil {
			it.Error = err.Error()
			failed[e.ID] = true
		} else {
			it.OK = true
		}
		r.mu.Lock()
		if it.OK {
			delete(r.errs, e.ID)
		} else {
			r.errs[e.ID] = it.Error
		}
		r.mu.Unlock()
		res.Items = append(res.Items, it)
	}
	r.tick(ctx)
	return res, nil
}

// dismiss drops the pending restore and starts tracking the running set again.
func (r *restoreTracker) dismiss(ctx context.Context, engineID string) error {
	if r == nil {
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.docker.restore.notPending.body", nil)
	}
	r.tick(ctx)
	if _, err := r.pendingFor(engineID); err != nil {
		return err
	}
	r.tickMu.Lock()
	defer r.tickMu.Unlock()
	r.mu.Lock()
	o := r.curObs
	r.errs = map[string]string{}
	r.mu.Unlock()
	now := nowMs()
	next := rebuildTracking(o, now)
	if r.save(next) {
		r.mu.Lock()
		r.snaps[engineID] = &next
		r.mu.Unlock()
	}
	r.publish(&next, o, now)
	return nil
}

// dependsOn parses the compose depends_on label: "service:condition[:required]" entries, comma separated.
func dependsOn(raw string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		f := strings.Split(strings.TrimSpace(part), ":")
		if f[0] == "" {
			continue
		}
		cond := ""
		if len(f) > 1 {
			cond = f[1]
		}
		out[f[0]] = cond
	}
	return out
}

// orderRestore puts standalone containers first by name, then each compose project with its
// services in depends_on order. A dependency cycle falls back to name order for that project.
func orderRestore(es []RestoreEntry) []RestoreEntry {
	var solo []RestoreEntry
	projects := map[string][]RestoreEntry{}
	for _, e := range es {
		if e.ComposeProject == "" {
			solo = append(solo, e)
		} else {
			projects[e.ComposeProject] = append(projects[e.ComposeProject], e)
		}
	}
	sortEntries(solo)
	names := make([]string, 0, len(projects))
	for p := range projects {
		names = append(names, p)
	}
	sort.Strings(names)
	out := solo
	for _, p := range names {
		out = append(out, orderProject(projects[p])...)
	}
	return out
}

func orderProject(es []RestoreEntry) []RestoreEntry {
	sortEntries(es)
	deps := map[string]map[string]string{}
	for _, e := range es {
		if _, ok := deps[e.ComposeService]; !ok {
			deps[e.ComposeService] = map[string]string{}
		}
		for s, c := range dependsOn(e.ComposeDependsOn) {
			deps[e.ComposeService][s] = c
		}
	}
	rank := map[string]int{}
	remaining := map[string]bool{}
	for s := range deps {
		remaining[s] = true
	}
	for level := 0; len(remaining) > 0; level++ {
		var ready []string
		for s := range remaining {
			ok := true
			for d := range deps[s] {
				if remaining[d] && d != s {
					ok = false
					break
				}
			}
			if ok {
				ready = append(ready, s)
			}
		}
		if len(ready) == 0 {
			return es
		}
		for _, s := range ready {
			rank[s] = level
			delete(remaining, s)
		}
	}
	sort.SliceStable(es, func(i, j int) bool { return rank[es[i].ComposeService] < rank[es[j].ComposeService] })
	return es
}

// waitDependencies fails when a dependency of e failed to start in this restore, then blocks until
// every service_healthy dependency that has a healthcheck is healthy. A dependency outside the
// selection passes through.
func (r *restoreTracker) waitDependencies(ctx context.Context, e RestoreEntry, all []RestoreEntry, failed map[string]bool) error {
	if e.ComposeProject == "" {
		return nil
	}
	for svc, cond := range dependsOn(e.ComposeDependsOn) {
		for _, d := range all {
			if d.ComposeProject != e.ComposeProject || d.ComposeService != svc {
				continue
			}
			if failed[d.ID] {
				return fmt.Errorf("dependency %s did not start", d.Name)
			}
			if cond != "service_healthy" {
				continue
			}
			if err := r.waitHealthy(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *restoreTracker) waitHealthy(ctx context.Context, d RestoreEntry) error {
	cli, _, err := r.m.client()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(healthWait)
	for {
		res, err := cli.ContainerInspect(ctx, d.ID, client.ContainerInspectOptions{})
		if err != nil || res.Container.State == nil || res.Container.State.Health == nil || !res.Container.State.Running {
			return nil
		}
		if res.Container.State.Health.Status == container.Healthy {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dependency %s did not become healthy", d.Name)
		}
		sleepCtx(ctx, 500*time.Millisecond)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}
