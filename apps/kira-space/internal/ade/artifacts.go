package ade

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/ids"
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
	artifactSrcNotes  = "notes"

	artifactBranch = model.AdeArtifactBranch
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
	// Quick asks for local data only: no host or git call, so the list is fast and has no
	// gaps. The agent's finish reply uses it; the UI panel does not.
	Quick bool
}

var artifactKindOrder = map[string]int{
	model.AdeArtifactChangeRequest: 0, model.AdeArtifactIssue: 1, artifactBranch: 2,
	model.AdeArtifactDeployment: 3, model.AdeArtifactDoc: 4, model.AdeArtifactLink: 5,
}

// builtinArtifactSources is the registration order: task links, repos, stored rows, links found in notes.
func (b *TaskBoard) builtinArtifactSources() []ArtifactSource {
	return []ArtifactSource{taskLinkSource{b}, repoSource{b}, storedSource{b}, notesSource{b}}
}

// TaskArtifacts lists the task's artifacts (archived tasks included), merged, deduped and sorted. A
// source that fails or times out adds a gap; the call itself fails only when the task is unreadable.
func (b *TaskBoard) TaskArtifacts(ctx context.Context, taskID string) (adewire.ArtifactsResult, error) {
	return b.taskArtifacts(ctx, taskID, false)
}

func (b *TaskBoard) taskArtifacts(ctx context.Context, taskID string, quick bool) (adewire.ArtifactsResult, error) {
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return adewire.ArtifactsResult{}, err
	}
	rows, err := b.deps.Tasks.BranchesOfTask(taskID)
	if err != nil {
		return adewire.ArtifactsResult{}, err
	}
	scope := ArtifactScope{Task: task, Rows: rows, Archived: task.ArchivedAt != nil, Quick: quick}
	sources := append(b.builtinArtifactSources(), b.deps.ArtifactSources...)
	type result struct {
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	}
	results := make([]result, len(sources))
	took := make([]time.Duration, len(sources))
	begun := time.Now()
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for i, src := range sources {
		g.Go(func() error {
			sctx, cancel := context.WithTimeout(ctx, artifactSourceTimeout)
			defer cancel()
			arts, gaps, err := src.Artifacts(sctx, scope)
			took[i] = time.Since(begun)
			if err != nil {
				slog.Warn("ade artifacts: source", "scope", "ade", "source", src.Name(), "task", taskID, "err", err)
				gaps = append(gaps, adewire.ArtifactGap{Source: src.Name()})
			}
			results[i] = result{arts, gaps}
			return nil
		})
	}
	_ = g.Wait() // a source reports failure as a gap
	if total := time.Since(begun); !quick && total > artifactSlowLog {
		attrs := []any{"scope", "ade", "task", taskID, "ms", total.Milliseconds()}
		for i, src := range sources {
			attrs = append(attrs, src.Name()+"Ms", took[i].Milliseconds())
		}
		slog.Info("ade artifacts: slow", attrs...)
	}
	var all []adewire.Artifact
	gaps := []adewire.ArtifactGap{}
	for _, r := range results {
		all = append(all, r.arts...)
		gaps = append(gaps, r.gaps...)
	}
	return adewire.ArtifactsResult{Artifacts: mergeArtifacts(all), Gaps: gaps}, nil
}

// mergeArtifacts dedups and sorts (see artifactLess). The same non-empty URL, or the same (kind, repo, ref) when the URL
// is empty, is one artifact. A detected artifact beats a stored one (its status is fresher), and both
// beat a found link. Among
// detected ones the first source wins, unless it has no status and a later one has.
func mergeArtifacts(all []adewire.Artifact) []adewire.Artifact {
	out := make([]adewire.Artifact, 0, len(all))
	index := map[string]int{}
	for pass := range 3 {
		for _, a := range all {
			if artifactPass(a) != pass {
				continue
			}
			k := artifactKey(a)
			if i, dup := index[k]; dup {
				if pass == 0 && out[i].ID == "" && out[i].Status == "" && a.Status != "" {
					out[i] = a
				}
				continue
			}
			index[k] = len(out)
			out = append(out, a)
		}
	}
	sort.SliceStable(out, artifactLess(out))
	return out
}

// artifactLess is a total order, independent of source completion order: kind, then creation time,
// then title, then the remaining fields. A deployment's CreatedAt is its last check time, not a
// creation time, so it varies run to run; deployments order by title (env) instead.
func artifactLess(out []adewire.Artifact) func(i, j int) bool {
	return func(i, j int) bool {
		a, b := out[i], out[j]
		if ka, kb := artifactKindOrder[a.Kind], artifactKindOrder[b.Kind]; ka != kb {
			return ka < kb
		}
		if a.Kind != model.AdeArtifactDeployment && a.CreatedAt != b.CreatedAt {
			return a.CreatedAt < b.CreatedAt
		}
		for _, p := range [][2]string{{a.Title, b.Title}, {a.Repo, b.Repo}, {a.Ref, b.Ref}, {a.URL, b.URL}, {a.Source, b.Source}, {a.ID, b.ID}} {
			if p[0] != p[1] {
				return p[0] < p[1]
			}
		}
		return false
	}
}

// artifactPass orders the merge: detected first, then stored, then links found in notes, which never
// replace or demote anything.
func artifactPass(a adewire.Artifact) int {
	switch {
	case a.Source == artifactSrcNotes:
		return 2
	case a.ID != "":
		return 1
	}
	return 0
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
	if sc.Quick {
		var arts []adewire.Artifact
		for _, id := range repoIDs {
			arts = append(arts, b.quickRepoArtifacts(ctx, sc, id, nicks[id], byRepo[id])...)
		}
		return arts, nil, nil
	}
	set := repoShareSet{repoIDs: repoIDs, byRepo: byRepo, nicks: nicks}
	facts := b.artifactFacts(ctx, sc)
	arts, gaps := b.repoShares(ctx, sc, set, facts, false)
	if len(repoIDs) > 0 {
		b.confirmArtifacts(sc, set, facts, arts, gaps)
	}
	return arts, gaps, nil
}

// repoShares is every repo's share of the task, in repo id order. resolve asks the host for each
// change request (it may wait); without it the share reads only caches, the stored rows and facts.
func (b *TaskBoard) repoShares(ctx context.Context, sc ArtifactScope, set repoShareSet, facts *artifactFacts, resolve bool) ([]adewire.Artifact, []adewire.ArtifactGap) {
	repoIDs, byRepo, nicks := set.repoIDs, set.byRepo, set.nicks
	type share struct {
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	}
	shares := make([]share, len(repoIDs))
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for i, id := range repoIDs {
		g.Go(func() error {
			shares[i].arts, shares[i].gaps = b.repoArtifacts(ctx, sc, id, nicks[id], byRepo[id], facts, resolve)
			return nil
		})
	}
	_ = g.Wait() // repoArtifacts reports failures as gaps
	var (
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	)
	for _, sh := range shares {
		arts, gaps = append(arts, sh.arts...), append(gaps, sh.gaps...)
	}
	sort.SliceStable(gaps, func(i, j int) bool {
		if gaps[i].Repo != gaps[j].Repo {
			return gaps[i].Repo < gaps[j].Repo
		}
		return gaps[i].Source < gaps[j].Source
	})
	return arts, gaps
}

// artifactFacts is the branch facts of a live task's rows, read from the board snapshot.
type artifactFacts struct {
	byID map[string]adewire.Branch
	// current is false when the snapshot predates a board invalidation.
	current bool
}

// artifactFacts reads the task's rows' facts from the kept board snapshot, never recomputing them
// (the board already holds the same facts). Waits for a build only when none exists yet. nil for an
// archived task or when no board can be had; the rows then decide.
func (b *TaskBoard) artifactFacts(ctx context.Context, sc ArtifactScope) *artifactFacts {
	if sc.Archived {
		return nil
	}
	board, current, ok := b.boardLatest()
	if !ok {
		var err error
		if board, err = b.Board(ctx); err != nil {
			slog.Warn("ade artifacts: board", "scope", "ade", "task", sc.Task.ID, "err", err)
			return nil
		}
		current = true
	}
	return factsOfBoard(board, sc.Rows, current)
}

func factsOfBoard(board adewire.Board, rows []model.AdeTaskBranch, current bool) *artifactFacts {
	mine := make(map[string]bool, len(rows))
	for _, r := range rows {
		mine[r.ID] = true
	}
	f := &artifactFacts{byID: make(map[string]adewire.Branch, len(rows)), current: current}
	for _, wb := range board.Branches {
		if mine[wb.ID] {
			f.byID[wb.ID] = wb
		}
	}
	return f
}

func (f *artifactFacts) branch(id string) (adewire.Branch, bool) {
	if f == nil {
		return adewire.Branch{}, false
	}
	wb, ok := f.byID[id]
	return wb, ok
}

// quickRepoArtifacts is one repo's share from local data alone: each row's branch and its change
// request, no deployments. The change request is the one the repo entry already caches, else the
// stored row's (set when the task is archived). A branch is merged when its row says so, else local.
func (b *TaskBoard) quickRepoArtifacts(ctx context.Context, sc ArtifactScope, repoID, nick string, rows []model.AdeTaskBranch) []adewire.Artifact {
	named := namedBranches(rows)
	if len(named) == 0 {
		return nil
	}
	entry, err := b.openRepo(ctx, repoID)
	if err != nil {
		slog.Warn("ade artifacts: open repo", "scope", "ade", "repo", repoID, "err", err)
		entry = nil
	}
	var arts []adewire.Artifact
	for _, r := range named {
		var cr *adewire.ChangeRequest
		if entry != nil {
			if c, ok := entry.CachedBranchChangeRequest(ctx, r.Name); ok {
				cr = &adewire.ChangeRequest{Number: c.Number, Title: c.Title, State: c.State, URL: c.URL}
			}
		}
		if cr == nil && r.ChangeRequest != nil {
			cr = &adewire.ChangeRequest{Number: r.ChangeRequest.Number, Title: r.ChangeRequest.Title, State: r.ChangeRequest.State, URL: r.ChangeRequest.URL}
		}
		if cr != nil {
			arts = append(arts, adewire.Artifact{
				Kind: model.AdeArtifactChangeRequest, Title: cr.Title, URL: cr.URL, Ref: fmt.Sprintf("#%d", cr.Number),
				Status: cr.State, Repo: nick, Source: artifactSrcHost,
			})
		}
		if !artifactBranchVisible(sc, r) {
			continue
		}
		br := adewire.Artifact{Kind: artifactBranch, Title: r.Name, Repo: nick, Source: artifactSrcGit, CreatedAt: r.AddedAt, Status: "local"}
		if r.MergedAt != nil {
			br.Status = "merged"
		}
		arts = append(arts, br)
	}
	return arts
}

// repoNicks maps a code repo id to the name the task shows for it.
func (b *TaskBoard) repoNicks() (map[string]string, error) {
	cfgs, err := b.repoConfigs()
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
// snapshot), its branches and, for a live task, the deployments the board's facts report. rows are the
// task's rows in the repo. Without resolve the change requests come from the repo entry's caches (any
// age, no host call) and the host gap from the last lookup kind; with it the host is asked and the
// kind recorded.
func (b *TaskBoard) repoArtifacts(ctx context.Context, sc ArtifactScope, repoID, nick string, rows []model.AdeTaskBranch, facts *artifactFacts, resolve bool) ([]adewire.Artifact, []adewire.ArtifactGap) {
	named := namedBranches(rows)
	if len(named) == 0 {
		return nil, nil
	}
	in := repoShareInput{facts: facts}
	entry, err := b.openRepo(ctx, repoID)
	if err != nil {
		slog.Warn("ade artifacts: open repo", "scope", "ade", "repo", repoID, "err", err)
		in.openFailed, in.facts = true, nil
	} else if resolve {
		var kind string
		kind, _, in.crs, in.urls = resolveRepoHost(ctx, entry, named)
		if ctx.Err() == nil { // a cancelled lookup says nothing about the host
			b.artHost.setKind(repoID, kind)
		}
		in.hostGap = kind == "unavailable"
	} else {
		in.crs, in.urls = cachedRepoHost(ctx, entry, named)
		in.hostGap = b.artHost.lastKind(repoID) == "unavailable"
	}
	return repoShare(sc, nick, named, in)
}

// cachedRepoHost is resolveRepoHost from caches alone: each branch's change request as the entry
// already knows it, of any age, and its web page composed locally. It never calls the host.
func cachedRepoHost(ctx context.Context, entry *gitsession.RepoEntry, named []model.AdeTaskBranch) (crs map[string]adewire.ChangeRequest, urls map[string]string) {
	crs, urls = map[string]adewire.ChangeRequest{}, map[string]string{}
	for _, r := range named {
		if c, ok := entry.CachedBranchChangeRequest(ctx, r.Name); ok {
			crs[r.ID] = wireChangeRequest(c)
		}
		if u, ok := entry.BranchURL(ctx, r.Name); ok {
			urls[r.ID] = u
		}
	}
	return crs, urls
}

// repoShareInput is everything repoShare builds from; it reads no store, git or host.
type repoShareInput struct {
	crs  map[string]adewire.ChangeRequest // by row id; absent when the host knows none
	urls map[string]string                // branch web page by row id
	// facts is nil when no facts were read: the stored rows decide each branch's status.
	facts *artifactFacts
	// hostGap marks the host source failed for the repo; openFailed also fails the git source.
	hostGap, openFailed bool
}

// repoShare is the pure builder of one repo's share. The served answer and the background confirm
// both call it, so their results compare like for like.
func repoShare(sc ArtifactScope, nick string, named []model.AdeTaskBranch, in repoShareInput) ([]adewire.Artifact, []adewire.ArtifactGap) {
	var (
		arts []adewire.Artifact
		gaps []adewire.ArtifactGap
	)
	if in.openFailed {
		gaps = append(gaps, adewire.ArtifactGap{Source: artifactSrcHost, Repo: nick}, adewire.ArtifactGap{Source: artifactSrcGit, Repo: nick})
	} else if in.hostGap {
		gaps = append(gaps, adewire.ArtifactGap{Source: artifactSrcHost, Repo: nick})
	}
	for _, r := range named {
		cr, ok := in.crs[r.ID]
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
			Kind: artifactBranch, Title: r.Name, URL: in.urls[r.ID], Repo: nick, Source: artifactSrcGit, CreatedAt: r.AddedAt,
			Status: "local",
		}
		wb, hasFacts := in.facts.branch(r.ID)
		if r.MergedAt != nil && !hasFacts {
			br.Status = "merged" // no facts for this row: the stored record decides
		}
		if hasFacts {
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

// storeCleanLocked upserts validated artifacts. The caller holds the task lock and loaded tc under it.
func (b *TaskBoard) storeCleanLocked(tc *taskCtx, source, stepID, runID, sessionID string, clean []agentmcp.ArtifactInput) ([]model.AdeTaskArtifact, error) {
	taskID := tc.task.ID
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
			ID: ids.NewID(), TaskID: taskID, Kind: c.Kind, Title: c.Title, URL: c.URL, Ref: c.Ref, Source: source,
			StepID: stepID, RunID: runID, SessionID: sessionID, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	b.mirrorTask(taskID, userEdit(true))
	b.notifyBoard()
	return out, nil
}

// ListArtifacts is the task's artifacts as an agent reads them, from local data only (Quick): its
// one caller is the finish reply, which must not wait on a host, git or tracker call.
func (b *TaskBoard) ListArtifacts(ctx context.Context, taskID string) (agentmcp.TaskArtifacts, error) {
	res, err := b.taskArtifacts(ctx, taskID, true)
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
		out.Gaps[i] = agentmcp.ArtifactGap{Source: g.Source, Repo: g.Repo}
	}
	return out
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
	b.recordArtifact(args.TaskID, row.ID, artifactText(*row), "")
	b.mirrorTask(args.TaskID, userEdit(true))
	b.notifyBoard()
	return nil
}
