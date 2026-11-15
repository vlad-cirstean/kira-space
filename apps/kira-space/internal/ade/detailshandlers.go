package ade

import (
	"context"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/sqlb"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
)

// detailsHandlers is the one taskfiles.Handler of every Details kind this part owns: notes, drafts,
// handed handovers, links and the README. The cache is the handover and artifact rows plus the task's
// link slots.
type detailsHandlers struct {
	b  *TaskBoard
	df *detailsFiles
}

var handedName = regexp.MustCompile(`^handovers/([0-9]{4})-(.+?)-to-(.+)\.md$`)

func (h *detailsHandlers) cache() repos.DetailsCache { return h.df.cache }

type textBody string

func (h *detailsHandlers) Parse(rel string, c taskfiles.Class, raw []byte) (taskfiles.Parsed, error) {
	if c.Kind == taskfiles.KindLinks {
		return parseLinks(raw, h.b.normalizeArtifact)
	}
	if !utf8.Valid(raw) {
		return nil, &taskfiles.ParseError{Msg: "file must be UTF-8 text"}
	}
	if c.Kind == taskfiles.KindHandover && !handedName.MatchString(rel) {
		return nil, &taskfiles.ParseError{Msg: "a handed handover is named NNNN-<from>-to-<to>.md; write handovers/draft-<step>.md instead"}
	}
	return textBody(raw), nil
}

func (h *detailsHandlers) Diff(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class, parsed taskfiles.Parsed) ([]taskfiles.Change, error) {
	s := detailsSessionOf(ctx)
	switch c.Kind {
	case taskfiles.KindLinks:
		cur, err := h.df.linksIn(tx, taskID)
		if err != nil {
			return nil, err
		}
		var next taskfiles.Links
		if parsed != nil {
			next = parsed.(linksDoc).Links
		}
		changes := taskfiles.DiffLinks(cur.Links, next)
		arts, err := h.cache().Artifacts(tx.Querier(), taskID)
		if err != nil {
			return nil, err
		}
		own := map[string]bool{}
		for _, a := range arts {
			own[entryOfArtifact(a).Key()] = s.key != "" && a.SessionID == s.key
		}
		for i := range changes {
			ch := &changes[i]
			if ch.Slot {
				continue
			}
			ch.Own = ch.Op == taskfiles.OpCreate || own[ch.Key]
		}
		return changes, nil
	case taskfiles.KindReadme:
		want, err := h.readme(ctx, taskID)
		if err != nil || parsed == nil {
			return nil, err
		}
		if string(parsed.(textBody)) == want {
			return nil, nil
		}
		return []taskfiles.Change{{Op: taskfiles.OpEdit, Text: taskfiles.TextDiff(rel, want, string(parsed.(textBody)))}}, nil
	}
	row, err := h.row(ctx, tx, taskID, rel, c)
	if err != nil {
		return nil, err
	}
	switch {
	case parsed == nil && row == nil:
		return nil, nil
	case parsed == nil:
		return []taskfiles.Change{{Op: taskfiles.OpDelete, Own: h.ownRow(row, s), Text: taskfiles.TextDiff(rel, row.Body, "")}}, nil
	case row == nil:
		return []taskfiles.Change{{Op: taskfiles.OpCreate, Own: true, Text: taskfiles.TextDiff(rel, "", string(parsed.(textBody)))}}, nil
	case row.Body != string(parsed.(textBody)):
		return []taskfiles.Change{{Op: taskfiles.OpEdit, Own: h.ownRow(row, s), Text: taskfiles.TextDiff(rel, row.Body, string(parsed.(textBody)))}}, nil
	}
	return nil, nil
}

func (h *detailsHandlers) ownRow(r *model.AdeHandover, s detailsSession) bool {
	return s.key != "" && r.SessionID == s.key
}

// row is the handover row a file holds: by path, and for a draft also by its step, since a row
// written before the file existed has no path yet.
func (h *detailsHandlers) row(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class) (*model.AdeHandover, error) {
	r, err := h.cache().HandoverByRel(tx.Querier(), taskID, rel)
	if err != nil || r != nil || c.Kind != taskfiles.KindDraft {
		return r, err
	}
	return h.cache().DraftOf(tx.Querier(), taskID, h.stepOfSlug(ctx, taskID, c.Step))
}

// stepOfSlug maps the step part of a draft file name back to the step id: the current step when
// its slug matches, else the slug itself.
func (h *detailsHandlers) stepOfSlug(ctx context.Context, taskID, slug string) string {
	if c, err := h.taskOf(ctx, taskID); err == nil {
		if fileSlug(c.step) == slug {
			return c.step
		}
	}
	return slug
}

func (h *detailsHandlers) Apply(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class, parsed taskfiles.Parsed, accepted []taskfiles.Change) error {
	if c.Kind == taskfiles.KindLinks {
		return h.applyLinks(ctx, tx, taskID, parsed, accepted)
	}
	if c.Kind == taskfiles.KindReadme {
		return nil
	}
	if len(accepted) == 0 {
		return nil
	}
	return h.adoptText(ctx, tx, taskID, rel, c, parsed)
}

func (h *detailsHandlers) Adopt(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class, parsed taskfiles.Parsed) error {
	switch c.Kind {
	case taskfiles.KindLinks:
		cur, err := h.df.linksIn(tx, taskID)
		if err != nil {
			return err
		}
		return h.applyLinks(ctx, tx, taskID, parsed, taskfiles.DiffLinks(cur.Links, parsed.(linksDoc).Links))
	case taskfiles.KindReadme:
		return nil
	}
	return h.adoptText(ctx, tx, taskID, rel, c, parsed)
}

// adoptText makes the row of a text file equal its content, creating the row for a new file.
func (h *detailsHandlers) adoptText(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class, parsed taskfiles.Parsed) error {
	body := string(parsed.(textBody))
	now := h.b.deps.Now().UnixMilli()
	row, err := h.row(ctx, tx, taskID, rel, c)
	if err != nil {
		return err
	}
	q := tx.Querier()
	if row != nil {
		if row.RelPath != rel {
			if err := h.cache().SetHandoverRel(q, row.ID, rel); err != nil {
				return err
			}
		}
		if row.Body == body {
			return nil
		}
		// "edited" marks a hand change; an agent revising its own draft is not one.
		return h.cache().SetHandoverBody(q, row.ID, body, row.Edited || detailsSessionOf(ctx).role == taskfiles.RoleUser, now)
	}
	s := detailsSessionOf(ctx)
	call, err := h.taskOf(ctx, taskID)
	if err != nil {
		return err
	}
	tc := call.tc
	n := model.AdeHandover{ID: ids.NewID(), TaskID: taskID, Body: body, RelPath: rel, CreatedAt: now, UpdatedAt: now, SessionID: s.key,
		Source: model.AdeHandoverSourceAgent}
	if s.role == taskfiles.RoleUser {
		n.Source = model.AdeHandoverSourceUser
	}
	switch c.Kind {
	case taskfiles.KindNote:
		cur := call.step
		n.State, n.FromStepID, n.FromStepName = model.AdeHandoverNote, cur, stepName(tc, cur)
	case taskfiles.KindDraft:
		step := h.stepOfSlug(ctx, taskID, c.Step)
		n.State, n.FromStepID, n.FromStepName = model.AdeHandoverDraft, step, stepName(tc, step)
		if s.role == taskfiles.RoleRun {
			n.Source = model.AdeHandoverSourceRun
		}
	case taskfiles.KindHandover:
		from, to := handedEnds(rel)
		n.State, n.Finished, n.FromStepID, n.ToStepID = model.AdeHandoverHanded, true, from, to
		n.FromStepName, n.ToStepName = stepName(tc, from), stepName(tc, to)
	}
	return h.cache().InsertHandover(q, n)
}

func (h *detailsHandlers) Remove(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class) error {
	q := tx.Querier()
	switch c.Kind {
	case taskfiles.KindLinks:
		cur, err := h.df.linksIn(tx, taskID)
		if err != nil {
			return err
		}
		return h.applyLinks(ctx, tx, taskID, linksDoc{}, taskfiles.DiffLinks(cur.Links, taskfiles.Links{}))
	case taskfiles.KindReadme:
		return nil
	}
	row, err := h.row(ctx, tx, taskID, rel, c)
	if err != nil || row == nil {
		return err
	}
	return h.cache().DeleteHandover(q, row.ID)
}

func (h *detailsHandlers) Render(ctx context.Context, tx taskfiles.Tx, taskID, rel string, c taskfiles.Class) ([]byte, bool, error) {
	switch c.Kind {
	case taskfiles.KindLinks:
		cur, err := h.df.linksIn(tx, taskID)
		if err != nil || cur.empty() {
			return nil, false, err
		}
		return renderLinks(cur), true, nil
	case taskfiles.KindReadme:
		s, err := h.readme(ctx, taskID)
		return []byte(s), err == nil, nil
	case taskfiles.KindProof, taskfiles.KindProofData, taskfiles.KindProofImage:
		return h.proofAccepted(tx, taskID, rel)
	}
	row, err := h.cache().HandoverByRel(tx.Querier(), taskID, rel)
	if err != nil || row == nil {
		return nil, false, err
	}
	return []byte(row.Body), true, nil
}

func (h *detailsHandlers) readme(ctx context.Context, taskID string) (string, error) {
	call, err := h.taskOf(ctx, taskID)
	if err != nil {
		return "", err
	}
	return h.df.readmeText(call.tc, call.step), nil
}

func (df *detailsFiles) readmeText(tc *taskCtx, step string) string {
	var proofs []taskfiles.ReadmeProof
	if df.proofIndex != nil {
		proofs = df.proofIndex(tc.task.ID)
	}
	return taskfiles.Readme(taskfiles.ReadmeInput{
		TaskID: tc.task.ID, Title: taskTitle(tc.task, tc.branches), Step: fileSlug(step), Proofs: proofs,
	})
}

// linksApply carries the state of one applyLinks pass.
type linksApply struct {
	h         *detailsHandlers
	ctx       context.Context
	q         sqlb.Querier
	taskID    string
	next      linksDoc
	slots     repos.TaskSlots
	arts      []model.AdeTaskArtifact
	byKey     map[string]model.AdeTaskArtifact
	nextByKey map[string]taskfiles.LinkEntry
	now       int64
}

// applyLinks applies the accepted changes of a links.yaml to the slots and artifact rows.
func (h *detailsHandlers) applyLinks(ctx context.Context, tx taskfiles.Tx, taskID string, parsed taskfiles.Parsed, accepted []taskfiles.Change) error {
	la := &linksApply{h: h, ctx: ctx, q: tx.Querier(), taskID: taskID, now: h.b.deps.Now().UnixMilli(),
		byKey: map[string]model.AdeTaskArtifact{}, nextByKey: map[string]taskfiles.LinkEntry{}}
	if parsed != nil {
		la.next = parsed.(linksDoc)
	}
	var err error
	if la.slots, err = h.cache().Slots(la.q, taskID); err != nil {
		return err
	}
	if la.arts, err = h.cache().Artifacts(la.q, taskID); err != nil {
		return err
	}
	for _, a := range la.arts {
		la.byKey[entryOfArtifact(a).Key()] = a
	}
	for _, e := range la.next.Entries {
		la.nextByKey[e.Key()] = e
	}
	slotsChanged := false
	for _, ch := range accepted {
		if ch.Slot {
			slotsChanged = la.slot(ch) || slotsChanged
			continue
		}
		if err := la.entry(ch); err != nil {
			return err
		}
	}
	if slotsChanged {
		return h.cache().SetSlots(la.q, taskID, la.slots)
	}
	return nil
}

// slot applies a jira or pr slot change; reports whether a slot changed.
func (la *linksApply) slot(ch taskfiles.Change) bool {
	switch ch.Key {
	case "jira":
		la.slots.JiraKey, la.slots.JiraURL = la.next.JiraKey, la.next.JiraURL
	case "pr":
		la.slots.HostURL = la.next.PR
	default:
		return false
	}
	return true
}

// entry applies one artifact create, edit or delete.
func (la *linksApply) entry(ch taskfiles.Change) error {
	h := la.h
	switch ch.Op {
	case taskfiles.OpDelete:
		if a, ok := la.byKey[ch.Key]; ok {
			return h.cache().DeleteArtifact(la.q, a.ID)
		}
	case taskfiles.OpEdit:
		e, a := la.nextByKey[ch.Key], la.byKey[ch.Key]
		if a.ID != "" {
			return h.cache().UpdateArtifact(la.q, a.ID, e.Kind, e.Title, e.URL, e.Ref, la.now)
		}
	case taskfiles.OpCreate:
		e := la.nextByKey[ch.Key]
		if len(la.arts) >= agentmcp.MaxArtifactsPerTask {
			return fmt.Errorf("a task holds at most %d links", agentmcp.MaxArtifactsPerTask)
		}
		s := detailsSessionOf(la.ctx)
		src := model.AdeArtifactSourceAgent
		if s.role == taskfiles.RoleUser {
			src = model.AdeArtifactSourceUser
		}
		a := model.AdeTaskArtifact{ID: ids.NewID(), TaskID: la.taskID, Kind: e.Kind, Title: e.Title, URL: e.URL, Ref: e.Ref, Source: src,
			RunID: s.run, SessionID: s.key, CreatedAt: la.now, UpdatedAt: la.now}
		if c, err := h.taskOf(la.ctx, la.taskID); err == nil {
			a.StepID = c.step
		}
		if err := h.cache().InsertArtifact(la.q, a); err != nil {
			return err
		}
		la.arts = append(la.arts, a)
	}
	return nil
}

// handedEnds reads the from and to step slugs of a handed file name.
func handedEnds(rel string) (from, to string) {
	m := handedName.FindStringSubmatch(rel)
	if m == nil {
		return "", ""
	}
	from, to = m[2], m[3]
	if from == "task" {
		from = ""
	}
	return from, to
}

// proofAccepted returns the accepted bytes of a proof file from the blob store.
func (h *detailsHandlers) proofAccepted(tx taskfiles.Tx, taskID, rel string) ([]byte, bool, error) {
	if h.df.blobs == nil {
		return nil, false, nil
	}
	row, err := tx.File(taskID, rel)
	if err != nil || row == nil || row.AcceptedSHA256 == "" {
		return nil, false, err
	}
	return h.df.blobs.Get(row.AcceptedSHA256)
}
