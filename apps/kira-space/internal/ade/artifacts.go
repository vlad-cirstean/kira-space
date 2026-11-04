package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// artifacts.go lists everything a task produced or points at (P409). Detected artifacts come from
// sources that read data the app already has; stored ones are rows an agent or the user reported.

// artifactSourceTimeout bounds each source; a slower one becomes a gap, never an error.
const artifactSourceTimeout = 5 * time.Second

// Artifact sources and kinds (wire values).
const (
	artifactSrcTask   = "task"
	artifactSrcHost   = "host"
	artifactSrcGit    = "git"
	artifactSrcStored = "stored"

	artifactBranch = "branch"
)

// userArtifactKind reports whether the user may file a stored row under kind: the agent-reportable
// kinds plus branch.
func userArtifactKind(kind string) bool {
	return kind == artifactBranch || agentmcp.ReportableArtifactKind(kind)
}

// ArtifactSource is one supplier of a task's artifacts. Built-ins cover task links, repos and stored
// rows; TaskBoardDeps.ArtifactSources appends more. A returned error or a missed deadline becomes a
// gap named Name; artifacts returned beside it are kept.
type ArtifactSource interface {
	Name() string
	Artifacts(ctx context.Context, s ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error)
}

// ArtifactScope is the task a source lists for. Rows holds every branch row of the task, detached ones
// included.
type ArtifactScope struct {
	Task     model.AdeTask
	Rows     []model.AdeTaskBranch
	Archived bool
}

var artifactKindOrder = map[string]int{
	model.AdeArtifactChangeRequest: 0, model.AdeArtifactIssue: 1, artifactBranch: 2,
	model.AdeArtifactDeployment: 3, model.AdeArtifactDoc: 4, model.AdeArtifactLink: 5,
}

// builtinArtifactSources is the registration order: task links, repos, one per issue tracker, stored rows.
func (b *TaskBoard) builtinArtifactSources() []ArtifactSource {
	out := []ArtifactSource{taskLinkSource{b}, repoSource{b}}
	for _, p := range b.deps.Trackers.All() {
		out = append(out, trackerSource{b: b, p: p})
	}
	return append(out, storedSource{b})
}

// TaskArtifacts lists the task's artifacts (archived tasks included), merged, deduped and sorted. A
// source that fails or times out adds a gap; the call itself fails only when the task is unreadable.
func (b *TaskBoard) TaskArtifacts(ctx context.Context, taskID string) (adewire.ArtifactsResult, error) {
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.ArtifactsResult{}, err
	}
	rows, err := b.deps.Tasks.BranchesOfTask(taskID)
	if err != nil {
		return adewire.ArtifactsResult{}, err
	}
	scope := ArtifactScope{Task: task, Rows: rows, Archived: task.ArchivedAt != nil}
	sources := append(b.builtinArtifactSources(), b.deps.ArtifactSources...)
	type result struct {
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	}
	results := make([]result, len(sources))
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for i, src := range sources {
		g.Go(func() error {
			sctx, cancel := context.WithTimeout(ctx, artifactSourceTimeout)
			defer cancel()
			arts, gaps, err := src.Artifacts(sctx, scope)
			if err != nil {
				slog.Warn("ade artifacts: source", "scope", "ade", "source", src.Name(), "task", taskID, "err", err)
				gaps = append(gaps, adewire.ArtifactGap{Source: src.Name()})
			}
			results[i] = result{arts, gaps}
			return nil
		})
	}
	_ = g.Wait() // a source reports failure as a gap
	var all []adewire.Artifact
	gaps := []adewire.ArtifactGap{}
	for _, r := range results {
		all = append(all, r.arts...)
		gaps = append(gaps, r.gaps...)
	}
	return adewire.ArtifactsResult{Artifacts: mergeArtifacts(all), Gaps: gaps}, nil
}

// mergeArtifacts dedups and sorts. The same non-empty URL, or the same (kind, repo, ref) when the URL
// is empty, is one artifact. A detected artifact beats a stored one (its status is fresher). Among
// detected ones the first source wins, unless it has no status and a later one has.
func mergeArtifacts(all []adewire.Artifact) []adewire.Artifact {
	out := make([]adewire.Artifact, 0, len(all))
	index := map[string]int{}
	for _, stored := range []bool{false, true} {
		for _, a := range all {
			if (a.ID != "") != stored {
				continue
			}
			k := artifactKey(a)
			if i, dup := index[k]; dup {
				if !stored && out[i].ID == "" && out[i].Status == "" && a.Status != "" {
					out[i] = a
				}
				continue
			}
			index[k] = len(out)
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ka, kb := artifactKindOrder[a.Kind], artifactKindOrder[b.Kind]; ka != kb {
			return ka < kb
		}
		if a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		return a.Title < b.Title
	})
	return out
}

func artifactKey(a adewire.Artifact) string {
	if a.URL != "" {
		return "u:" + strings.TrimRight(a.URL, "/")
	}
	if a.Ref == "" {
		return strings.Join([]string{"k", a.Kind, a.Repo, "", a.Title}, "\x00")
	}
	return strings.Join([]string{"k", a.Kind, a.Repo, a.Ref}, "\x00")
}

// --- task links ---------------------------------------------------------------------------------

type taskLinkSource struct{ b *TaskBoard }

func (taskLinkSource) Name() string { return artifactSrcTask }

func (s taskLinkSource) Artifacts(_ context.Context, sc ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error) {
	var out []adewire.Artifact
	t := sc.Task
	if t.HostURL != "" {
		a := adewire.Artifact{Kind: model.AdeArtifactLink, Title: t.HostURL, URL: t.HostURL, Source: artifactSrcTask, CreatedAt: t.CreatedAt}
		if l := s.b.hostLink(t.HostURL); l != nil {
			a.Kind, a.Ref = l.Kind, l.Ref
			if l.Ref != "" {
				a.Title = l.Ref
			}
		}
		out = append(out, a)
	}
	return out, nil, nil
}

// --- stored rows --------------------------------------------------------------------------------

type storedSource struct{ b *TaskBoard }

func (storedSource) Name() string { return artifactSrcStored }

func (s storedSource) Artifacts(_ context.Context, sc ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error) {
	rows, err := s.b.deps.Tasks.Artifacts(sc.Task.ID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]adewire.Artifact, len(rows))
	for i, r := range rows {
		out[i] = wireStoredArtifact(r)
	}
	return out, nil, nil
}

func wireStoredArtifact(r model.AdeTaskArtifact) adewire.Artifact {
	return adewire.Artifact{
		ID: r.ID, Kind: r.Kind, Title: r.Title, URL: r.URL, Ref: r.Ref, Source: r.Source, CreatedAt: r.CreatedAt,
		SessionID: r.SessionID,
	}
}

// --- repos: change requests, branches, deployments ----------------------------------------------

type repoSource struct{ b *TaskBoard }

func (repoSource) Name() string { return artifactSrcHost }

func (s repoSource) Artifacts(ctx context.Context, sc ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error) {
	b := s.b
	repoIDs, byRepo := branchesByRepo(sc.Rows)
	nicks, err := b.repoNicks()
	if err != nil {
		return nil, nil, err
	}
	var liveBy map[string][]model.AdeTaskBranch
	var setups map[string]model.AdeWorktreeSetup
	if !sc.Archived {
		live, err := b.deps.Tasks.BranchesLive()
		if err != nil {
			return nil, nil, err
		}
		_, liveBy = branchesByRepo(live)
		if setups, err = b.deps.Tasks.SetupByBranch(); err != nil {
			return nil, nil, err
		}
	}
	var (
		mu   sync.Mutex
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	)
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for _, id := range repoIDs {
		g.Go(func() error {
			a, gp := b.repoArtifacts(ctx, sc, id, nicks[id], byRepo[id], liveBy[id], setups)
			mu.Lock()
			arts, gaps = append(arts, a...), append(gaps, gp...)
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait() // repoArtifacts reports failures as gaps
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].Repo != gaps[j].Repo {
			return gaps[i].Repo < gaps[j].Repo
		}
		return gaps[i].Source < gaps[j].Source
	})
	return arts, gaps, nil
}

// repoNicks maps a code repo id to the name the task shows for it.
func (b *TaskBoard) repoNicks() (map[string]string, error) {
	cfgs, err := b.deps.RepoConfig.List()
	if err != nil {
		return nil, err
	}
	nick := make(map[string]string, len(cfgs))
	for _, c := range cfgs {
		nick[c.CodeRepoID] = cmpNonEmpty(c.Nickname, c.Name)
	}
	return nick, nil
}

func namedBranches(rows []model.AdeTaskBranch) []model.AdeTaskBranch {
	var named []model.AdeTaskBranch
	for _, r := range rows {
		if r.Name != "" {
			named = append(named, r)
		}
	}
	return named
}

// repoArtifacts is one repo's share: the task's change requests (live from the host, else the archive
// snapshot), its branches and, for a live task, the deployments git facts report. rows are the task's
// rows in the repo; liveRows every live row of the repo, the input the board gives repoFacts.
func (b *TaskBoard) repoArtifacts(ctx context.Context, sc ArtifactScope, repoID, nick string, rows, liveRows []model.AdeTaskBranch, setups map[string]model.AdeWorktreeSetup) ([]adewire.Artifact, []adewire.ArtifactGap) {
	named := namedBranches(rows)
	if len(named) == 0 {
		return nil, nil
	}
	var (
		arts  []adewire.Artifact
		gaps  []adewire.ArtifactGap
		found map[string]adewire.ChangeRequest
		urls  map[string]string
	)
	entry, err := b.openRepo(ctx, repoID)
	if err != nil {
		slog.Warn("ade artifacts: open repo", "scope", "ade", "repo", repoID, "err", err)
		gaps = append(gaps, adewire.ArtifactGap{Source: artifactSrcHost, Repo: nick}, adewire.ArtifactGap{Source: artifactSrcGit, Repo: nick})
	} else {
		var kind string
		kind, _, found, urls = resolveRepoHost(ctx, entry, named)
		if kind == "unavailable" || (ctx.Err() != nil && kind != "disabled") {
			gaps = append(gaps, adewire.ArtifactGap{Source: artifactSrcHost, Repo: nick})
		}
	}
	var facts *repoResult
	if err == nil && !sc.Archived {
		facts = b.repoFacts(ctx, repoID, liveRows, setups)
	}
	for _, r := range named {
		cr, ok := found[r.ID]
		if !ok && r.ChangeRequest != nil {
			cr, ok = adewire.ChangeRequest{Number: r.ChangeRequest.Number, Title: r.ChangeRequest.Title, State: r.ChangeRequest.State, URL: r.ChangeRequest.URL}, true
		}
		if ok {
			arts = append(arts, adewire.Artifact{
				Kind: model.AdeArtifactChangeRequest, Title: cr.Title, URL: cr.URL, Ref: fmt.Sprintf("#%d", cr.Number),
				Status: cr.State, Repo: nick, Source: artifactSrcHost,
			})
		}
		if !artifactBranchVisible(sc, r) {
			continue
		}
		br := adewire.Artifact{
			Kind: artifactBranch, Title: r.Name, URL: urls[r.ID], Repo: nick, Source: artifactSrcGit, CreatedAt: r.AddedAt,
			Status: "local",
		}
		if r.MergedAt != nil {
			br.Status = "merged"
		}
		if wb, ok := facts.branch(r.ID); ok {
			br.Ref = wb.Worktree
			switch {
			case wb.MergedIntoMain:
				br.Status = "merged"
			case br.Status == "local" && wb.Upstream != "":
				br.Status = "pushed"
			}
			if r.Kind == model.AdeBranchKindMine {
				arts = append(arts, deploymentArtifacts(wb, nick)...)
			}
		}
		arts = append(arts, br)
	}
	return arts, gaps
}

// branch is the board's fact row for a branch id; ok is false when facts were not computed.
func (r *repoResult) branch(id string) (adewire.Branch, bool) {
	if r == nil {
		return adewire.Branch{}, false
	}
	wb, ok := r.branches[id]
	return wb, ok
}

// artifactBranchVisible reports whether a row is a branch of the task as it stands: live for a live
// task, archived with the task for an archived one. Detached rows keep only their change request.
func artifactBranchVisible(sc ArtifactScope, r model.AdeTaskBranch) bool {
	if sc.Archived {
		return r.ArchivedAt != nil && sc.Task.ArchivedAt != nil && *r.ArchivedAt == *sc.Task.ArchivedAt
	}
	return r.ArchivedAt == nil
}

func deploymentArtifacts(wb adewire.Branch, nick string) []adewire.Artifact {
	var out []adewire.Artifact
	for _, d := range wb.Deployments {
		if d.Status != "deployed" && d.Status != "stale" {
			continue
		}
		sha := d.DeployedSha
		if len(sha) > 7 {
			sha = sha[:7]
		}
		out = append(out, adewire.Artifact{
			Kind: model.AdeArtifactDeployment, Title: d.Env, Ref: sha, Status: d.Status, Repo: nick,
			Source: artifactSrcGit, CreatedAt: d.CheckedAt,
		})
	}
	return out
}

// --- writes -------------------------------------------------------------------------------------

// normalizeArtifact gives a URL a registered provider recognises the provider's kind and ref.
func (b *TaskBoard) normalizeArtifact(in agentmcp.ArtifactInput) agentmcp.ArtifactInput {
	if in.URL == "" {
		return in
	}
	if l := b.hostLink(in.URL); l != nil {
		in.Kind = l.Kind
		if l.Ref != "" {
			in.Ref = l.Ref
		}
	}
	return in
}

// storeArtifacts validates and upserts a batch under the task lock; nothing is written when any item
// or the task's cap refuses. The refusal is an ErrInvalidInput.
func (b *TaskBoard) storeArtifacts(taskID, source, stepID, runID, sessionID string, kindOK func(string) bool, in []agentmcp.ArtifactInput) ([]model.AdeTaskArtifact, error) {
	if n := len(in); n == 0 || n > agentmcp.MaxArtifactsPerCall {
		return nil, invalid("artifacts needs 1 to %d entries.", agentmcp.MaxArtifactsPerCall)
	}
	clean := make([]agentmcp.ArtifactInput, len(in))
	for i, a := range in {
		c, msg := agentmcp.CleanArtifactAs(a, kindOK)
		if msg != "" {
			return nil, invalid("artifacts[%d]: %s", i, msg)
		}
		clean[i] = b.normalizeArtifact(c)
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return nil, err
	}
	existing, err := b.deps.Tasks.Artifacts(taskID)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(existing)+len(clean))
	for _, e := range existing {
		if e.URL != "" {
			known[e.URL] = true
		}
	}
	added := 0
	for _, c := range clean {
		if c.URL == "" || !known[c.URL] {
			added++
		}
		if c.URL != "" {
			known[c.URL] = true
		}
	}
	if len(existing)+added > agentmcp.MaxArtifactsPerTask {
		return nil, invalid("a task holds at most %d artifacts; remove some first.", agentmcp.MaxArtifactsPerTask)
	}
	if stepID == "" {
		stepID = b.currentStepID(tc)
	}
	now := b.deps.Now().UnixMilli()
	out := make([]model.AdeTaskArtifact, 0, len(clean))
	for _, c := range clean {
		row, err := b.deps.Tasks.UpsertArtifact(model.AdeTaskArtifact{
			ID: b.newID(), TaskID: taskID, Kind: c.Kind, Title: c.Title, URL: c.URL, Ref: c.Ref, Source: source,
			StepID: stepID, RunID: runID, SessionID: sessionID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	b.notifyBoard()
	return out, nil
}

// ReportArtifacts records the artifacts an agent reports for its task.
func (b *TaskBoard) ReportArtifacts(_ context.Context, call agentmcp.SpaceCall, in []agentmcp.ArtifactInput) (agentmcp.ReportArtifactsResult, error) {
	rows, err := b.storeArtifacts(call.TaskID, model.AdeArtifactSourceAgent, call.StepID, call.RunID, call.SessionID, agentmcp.ReportableArtifactKind, in)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return agentmcp.ReportArtifactsResult{}, agentmcp.ToolError(trimInvalid(err))
		}
		return agentmcp.ReportArtifactsResult{}, err
	}
	return agentmcp.ReportArtifactsResult{Recorded: len(rows)}, nil
}

// ListArtifacts is the task's artifacts as an agent reads them.
func (b *TaskBoard) ListArtifacts(ctx context.Context, taskID string) (agentmcp.TaskArtifacts, error) {
	res, err := b.TaskArtifacts(ctx, taskID)
	if err != nil {
		return agentmcp.TaskArtifacts{}, err
	}
	return agentArtifacts(res), nil
}

func agentArtifacts(res adewire.ArtifactsResult) agentmcp.TaskArtifacts {
	out := agentmcp.TaskArtifacts{
		Artifacts: make([]agentmcp.Artifact, len(res.Artifacts)), Gaps: make([]agentmcp.ArtifactGap, len(res.Gaps)),
	}
	for i, a := range res.Artifacts {
		out.Artifacts[i] = agentmcp.Artifact{Kind: a.Kind, Title: a.Title, URL: a.URL, Ref: a.Ref, Status: a.Status, Repo: a.Repo, Source: a.Source}
	}
	for i, g := range res.Gaps {
		out.Gaps[i] = agentmcp.ArtifactGap{Source: g.Source, Repo: g.Repo, Reason: g.Reason}
	}
	return out
}

// AddArtifact records an artifact the user adds to a live task.
func (b *TaskBoard) AddArtifact(_ context.Context, args adewire.AddArtifactArgs) (adewire.Artifact, error) {
	title := strings.TrimSpace(args.Title)
	if title == "" {
		title = b.pastedArtifactTitle(strings.TrimSpace(args.URL))
	}
	rows, err := b.storeArtifacts(args.TaskID, model.AdeArtifactSourceUser, "", "", "", agentmcp.ReportableArtifactKind, []agentmcp.ArtifactInput{
		{Kind: args.Kind, Title: title, URL: args.URL, Ref: args.Ref},
	})
	if err != nil {
		return adewire.Artifact{}, err
	}
	return wireStoredArtifact(rows[0]), nil
}

// pastedArtifactTitle names a pasted link: the host link ref when a provider claims it, else the URL's
// host and path. "" when url is empty or unparsable, which leaves the title check to refuse it.
func (b *TaskBoard) pastedArtifactTitle(raw string) string {
	if raw == "" {
		return ""
	}
	if l := b.hostLink(raw); l != nil && l.Ref != "" {
		return l.Ref
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	t := strings.TrimRight(u.Host+u.Path, "/")
	if len(t) > agentmcp.MaxArtifactTitle {
		t = strings.ToValidUTF8(t[:agentmcp.MaxArtifactTitle], "")
	}
	return t
}

// UpdateArtifact renames a stored artifact of a live task. Detected artifacts have no id to rename.
func (b *TaskBoard) UpdateArtifact(_ context.Context, args adewire.UpdateArtifactArgs) (adewire.Artifact, error) {
	title := strings.TrimSpace(args.Title)
	if title == "" || len(title) > agentmcp.MaxArtifactTitle {
		return adewire.Artifact{}, invalid("%s", i18n.T("backend.space.ade.artifactTitle.body", nil))
	}
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if _, err := b.loadTaskCtx(args.TaskID); err != nil {
		return adewire.Artifact{}, err
	}
	row, err := b.deps.Tasks.GetArtifact(args.ID)
	if err != nil {
		return adewire.Artifact{}, err
	}
	if row == nil || row.TaskID != args.TaskID {
		return adewire.Artifact{}, invalid("artifact %s is not stored for this task", args.ID)
	}
	row2, err := b.deps.Tasks.UpdateArtifactTitle(args.TaskID, args.ID, title, b.deps.Now().UnixMilli())
	if err != nil {
		return adewire.Artifact{}, err
	}
	b.notifyBoard()
	return wireStoredArtifact(row2), nil
}

// RemoveArtifact deletes a stored artifact of a live task. Detected artifacts have no id to remove.
func (b *TaskBoard) RemoveArtifact(_ context.Context, args adewire.RemoveArtifactArgs) error {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if _, err := b.loadTaskCtx(args.TaskID); err != nil {
		return err
	}
	row, err := b.deps.Tasks.GetArtifact(args.ID)
	if err != nil {
		return err
	}
	if row == nil || row.TaskID != args.TaskID {
		return invalid("artifact %s is not stored for this task", args.ID)
	}
	if err := b.deps.Tasks.DeleteArtifact(args.ID); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}
