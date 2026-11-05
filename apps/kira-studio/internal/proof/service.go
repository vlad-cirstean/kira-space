package proof

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// WindowKeyPrefix marks the proof window's key.
const WindowKeyPrefix = "proof-"

// IsWindowKey reports whether key names a proof window.
func IsWindowKey(key string) bool { return strings.HasPrefix(key, WindowKeyPrefix) }

// ChannelChanged is broadcast to every window when sessions change.
const ChannelChanged = "kira:proof:changed"

const draftTTL = 24 * time.Hour

// Deps are the seams Service reads.
type Deps struct {
	Repo *repos.ProofRepo
	// Prompts receives the "ready to review" popup; nil drops it.
	Prompts prompts.Sink
	// Approvals receives a pending edit for review; nil drops it.
	Approvals *approvals.Queue
	// Emit broadcasts an app-wide event; nil drops it.
	Emit func(channel string, payload any)
	Now  func() time.Time
}

// Service owns proof sessions. Safe for concurrent use; the repo serializes writes.
type Service struct{ d Deps }

// New builds a Service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &Service{d: d}
	if d.Approvals != nil {
		d.Approvals.Register(approvals.KindProofReview, reviewOwner{s})
	}
	return s
}

// reviewOwner answers proof.review items: approve accepts the agent's edit, deny rejects it.
type reviewOwner struct{ s *Service }

func (o reviewOwner) Answer(ref string, a approvals.Answer) error {
	if a.Approve {
		return o.s.AcceptEdit(ref)
	}
	return o.s.RejectEdit(ref)
}

// Changed is the kira:proof:changed payload; SessionID is empty when several sessions changed.
type Changed struct {
	SessionID string `json:"sessionId"`
}

func (s *Service) emit(sessionID string) {
	if s.d.Emit != nil {
		s.d.Emit(ChannelChanged, Changed{SessionID: sessionID})
	}
}

func promptID(sessionID string) string { return prompts.ID(prompts.KindProof, sessionID) }

func reviewID(sessionID string) string { return approvals.KindProofReview + ":" + sessionID }

// closePrompt withdraws the session's ready popup and any pending edit review.
func (s *Service) closePrompt(sessionID string) {
	if s.d.Prompts != nil {
		s.d.Prompts.Close(promptID(sessionID))
	}
	if s.d.Approvals != nil {
		s.d.Approvals.Remove(reviewID(sessionID))
	}
}

// openPrompt raises the ready popup of a new session, or the review item of a pending edit.
func (s *Service) openPrompt(id, name string, edit bool) {
	if edit {
		s.putReview(id, name)
		return
	}
	if s.d.Prompts == nil {
		return
	}
	s.d.Prompts.Open(prompts.Prompt{
		ID: promptID(id), Kind: prompts.KindProof, Ref: id,
		Title: i18n.T("backend.prompt.proof.title", map[string]string{"name": name}),
	})
}

type reviewStep struct {
	Ordinal     int    `json:"ordinal"`
	Description string `json:"description"`
	Diff        string `json:"diff"`
}

type reviewPayload struct {
	SessionID string       `json:"sessionId"`
	Name      string       `json:"name"`
	Revision  int          `json:"revision"`
	Reason    string       `json:"reason"`
	Steps     []reviewStep `json:"steps"`
}

func (s *Service) putReview(id, name string) {
	if s.d.Approvals == nil {
		return
	}
	pl := reviewPayload{SessionID: id, Name: name, Steps: []reviewStep{}}
	if d, err := s.Session(id); err == nil && d.Pending != nil {
		pl.Revision, pl.Reason = d.Pending.Rev, d.Pending.Reason
		for _, st := range d.Pending.Steps {
			pl.Steps = append(pl.Steps, reviewStep{Ordinal: st.Ordinal, Description: st.Description, Diff: st.Diff})
		}
	} else if err != nil {
		slog.Warn("proof: read pending edit", "scope", "proof", "session", id, "err", err)
	}
	raw, err := json.Marshal(pl)
	if err != nil {
		slog.Warn("proof: encode pending edit", "scope", "proof", "session", id, "err", err)
		return
	}
	s.d.Approvals.Put(approvals.Item{
		ID: reviewID(id), Kind: approvals.KindProofReview, CreatedAt: s.d.Now().UnixMilli(),
		Target:    approvals.Target{Name: name, Kind: "proof"},
		Operation: approvals.Operation{Class: "review", Tone: "caution"},
		Reason:    "review", Payload: raw,
		Title: i18n.T("backend.prompt.proofEdit.title", map[string]string{"name": name}),
	})
}

// PurgeStale drops drafts an agent abandoned.
func (s *Service) PurgeStale() {
	if _, err := s.d.Repo.PurgeStaleDrafts(s.d.Now().Add(-draftTTL)); err != nil {
		slog.Warn("proof: purge stale drafts", "scope", "proof", "err", err)
	}
}

// RaiseUnreviewed re-raises the popup of every closed session the user has not opened or that
// holds a pending edit. The router persists nothing, so a restart calls this.
func (s *Service) RaiseUnreviewed() {
	list, err := s.d.Repo.Unreviewed()
	if err != nil {
		slog.Warn("proof: list unreviewed", "scope", "proof", "err", err)
		return
	}
	for _, p := range list {
		s.openPrompt(p.ID, p.Name, p.Pending)
	}
}

// StartResult answers proof_start_session and proof_edit_session.
type StartResult struct {
	DraftID string      `json:"draftId"`
	Name    string      `json:"name"`
	Steps   []StepBrief `json:"steps,omitempty"`
}

// StepBrief is a step as the agent sees it.
type StepBrief struct {
	Ordinal     int    `json:"ordinal"`
	Description string `json:"description"`
	View        string `json:"view"`
}

// StartSession opens a draft for a new session.
func (s *Service) StartSession(name, summary string) (StartResult, error) {
	clean, key, err := NormalizeName(name)
	if err != nil {
		return StartResult{}, err
	}
	if err := CheckText("summary", summary, maxSummaryRunes); err != nil {
		return StartResult{}, err
	}
	s.PurgeStale()
	draft := uuid.NewString()
	if err := s.d.Repo.CreateSessionDraft(uuid.NewString(), clean, key, strings.TrimSpace(summary), draft, s.d.Now()); err != nil {
		return StartResult{}, err
	}
	return StartResult{DraftID: draft, Name: clean}, nil
}

// EditSession opens an edit draft on a closed session.
func (s *Service) EditSession(name, reason string) (StartResult, error) {
	_, key, err := NormalizeName(name)
	if err != nil {
		return StartResult{}, err
	}
	if err := CheckText("reason", reason, maxSummaryRunes); err != nil {
		return StartResult{}, err
	}
	s.PurgeStale()
	draft := uuid.NewString()
	sess, steps, err := s.d.Repo.CreateEditDraft(key, draft, strings.TrimSpace(reason), s.d.Now())
	if err != nil {
		return StartResult{}, err
	}
	out := StartResult{DraftID: draft, Name: sess.Name, Steps: make([]StepBrief, len(steps))}
	for i, st := range steps {
		out.Steps[i] = StepBrief{Ordinal: st.Ordinal, Description: st.Description, View: st.View}
	}
	return out, nil
}

// AddStep validates and appends a step; it returns the step's ordinal.
func (s *Service) AddStep(in StepInput) (int, error) {
	if in.DraftID == "" {
		return 0, invalid("draftId is required")
	}
	c, keep, err := in.validate()
	if err != nil {
		return 0, err
	}
	step := repos.ProofNewStep{
		Description: c.Description, Query: c.Query, Language: c.Language, Source: c.Source, View: c.View, Payload: c.Payload,
		Replaces: in.ReplacesOrdinal,
	}
	if keep {
		step.Keep = in.KeepOrdinal
	}
	return s.d.Repo.AddStep(in.DraftID, step)
}

// CloseResult answers proof_close_session.
type CloseResult struct {
	Name   string `json:"name"`
	Steps  int    `json:"steps"`
	Status string `json:"status"`
}

// CloseSession finishes a draft and notifies the user.
func (s *Service) CloseSession(draftID string) (CloseResult, error) {
	if draftID == "" {
		return CloseResult{}, invalid("draftId is required")
	}
	c, err := s.d.Repo.CloseDraft(draftID, s.d.Now())
	if err != nil {
		return CloseResult{}, err
	}
	for _, id := range c.Pruned {
		s.closePrompt(id)
	}
	s.openPrompt(c.SessionID, c.Name, c.Edit)
	s.emit(c.SessionID)
	status := "closed"
	if c.Edit {
		status = "pending_review"
	}
	return CloseResult{Name: c.Name, Steps: c.Steps, Status: status}, nil
}

// AgentSession is one row of proof_list_sessions.
type AgentSession struct {
	Name        string `json:"name"`
	ClosedAt    string `json:"closedAt"`
	Steps       int    `json:"steps"`
	PendingEdit bool   `json:"pendingEdit"`
}

// ListForAgent lists closed sessions, newest first.
func (s *Service) ListForAgent() ([]AgentSession, error) {
	list, err := s.d.Repo.Sessions()
	if err != nil {
		return nil, err
	}
	out := make([]AgentSession, len(list))
	for i, p := range list {
		out[i] = AgentSession{Name: p.Name, ClosedAt: time.UnixMilli(p.ClosedAt).UTC().Format(time.RFC3339), Steps: p.Steps, PendingEdit: p.Pending}
	}
	return out, nil
}

// IsUserError reports whether err is a caller-correctable failure (validation or repo conflict).
func IsUserError(err error) bool {
	var inv *InvalidError
	var pe *repos.ProofError
	return errors.As(err, &inv) || errors.As(err, &pe)
}

// Summary is one history row.
type Summary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ClosedAt int64  `json:"closedAt"`
	Steps    int    `json:"steps"`
	Pending  bool   `json:"pending"`
	Seen     bool   `json:"seen"`
}

// Sessions lists closed sessions, newest first.
func (s *Service) Sessions() ([]Summary, error) {
	list, err := s.d.Repo.Sessions()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, len(list))
	for i, p := range list {
		out[i] = Summary{ID: p.ID, Name: p.Name, ClosedAt: p.ClosedAt, Steps: p.Steps, Pending: p.Pending, Seen: p.Seen}
	}
	return out, nil
}

// StepMeta is a step without its data. Diff is relative to the current revision for a pending one.
type StepMeta struct {
	Ordinal       int    `json:"ordinal"`
	Description   string `json:"description"`
	Language      string `json:"language"`
	Source        string `json:"source"`
	View          string `json:"view"`
	Bytes         int    `json:"bytes"`
	Diff          string `json:"diff"`
	OriginOrdinal *int   `json:"originOrdinal"`
}

// Revision is one revision's step list.
type Revision struct {
	Rev       int        `json:"rev"`
	Reason    string     `json:"reason"`
	CreatedAt int64      `json:"createdAt"`
	Steps     []StepMeta `json:"steps"`
}

// Detail is a session with its current revision and, when the agent proposed an edit, the pending
// one plus the old steps the edit drops.
type Detail struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Summary  string     `json:"summary"`
	ClosedAt int64      `json:"closedAt"`
	Current  Revision   `json:"current"`
	Pending  *Revision  `json:"pending"`
	Removed  []StepMeta `json:"removed"`
}

func notFound() error {
	return &repos.ProofError{Code: repos.ProofNotFound, Msg: "no such proof session"}
}

func meta(rows []repos.ProofStepRow) []StepMeta {
	out := make([]StepMeta, len(rows))
	for i, r := range rows {
		out[i] = StepMeta{Ordinal: r.Ordinal, Description: r.Description, Language: r.Language, Source: r.Source, View: r.View,
			Bytes: r.PayloadBytes, Diff: r.Diff, OriginOrdinal: r.OriginOrdinal}
	}
	return out
}

func (s *Service) revision(id string, rev int) (Revision, error) {
	row, ok, err := s.d.Repo.Revision(id, rev)
	if err != nil {
		return Revision{}, err
	}
	if !ok {
		return Revision{}, notFound()
	}
	steps, err := s.d.Repo.Steps(id, rev)
	if err != nil {
		return Revision{}, err
	}
	return Revision{Rev: rev, Reason: row.Reason, CreatedAt: row.CreatedAt, Steps: meta(steps)}, nil
}

// Session returns one closed session.
func (s *Service) Session(id string) (Detail, error) {
	row, ok, err := s.d.Repo.Session(id)
	if err != nil {
		return Detail{}, err
	}
	if !ok || row.CurrentRev == nil || row.ClosedAt == nil {
		return Detail{}, notFound()
	}
	cur, err := s.revision(id, *row.CurrentRev)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{ID: row.ID, Name: row.Name, Summary: row.Summary, ClosedAt: *row.ClosedAt, Current: cur, Removed: []StepMeta{}}
	if row.PendingRev == nil {
		return d, nil
	}
	pend, err := s.revision(id, *row.PendingRev)
	if err != nil {
		return Detail{}, err
	}
	d.Pending = &pend
	referenced := map[int]bool{}
	for _, st := range pend.Steps {
		if st.OriginOrdinal != nil {
			referenced[*st.OriginOrdinal] = true
		}
	}
	for _, st := range cur.Steps {
		if !referenced[st.Ordinal] {
			st.Diff = "removed"
			d.Removed = append(d.Removed, st)
		}
	}
	return d, nil
}

// StepDetail is one step with its query and data.
type StepDetail struct {
	StepMeta
	Query string          `json:"query"`
	Data  json.RawMessage `json:"data"`
}

// Step returns one step of a closed or pending revision.
func (s *Service) Step(id string, rev, ordinal int) (StepDetail, error) {
	r, ok, err := s.d.Repo.Step(id, rev, ordinal)
	if err != nil {
		return StepDetail{}, err
	}
	if !ok {
		return StepDetail{}, &repos.ProofError{Code: repos.ProofNotFound, Msg: "no such proof step"}
	}
	return StepDetail{StepMeta: meta([]repos.ProofStepRow{r})[0], Query: r.Query, Data: json.RawMessage(r.Payload)}, nil
}

// ViewStateEntry is one step's saved view state.
type ViewStateEntry struct {
	Ordinal int       `json:"ordinal"`
	State   ViewState `json:"state"`
}

// ViewStates returns the saved view state of one revision's steps.
func (s *Service) ViewStates(id string, rev int) ([]ViewStateEntry, error) {
	rows, err := s.d.Repo.ViewStates(id, rev)
	if err != nil {
		return nil, err
	}
	out := make([]ViewStateEntry, 0, len(rows))
	for _, r := range rows {
		e := ViewStateEntry{Ordinal: r.Ordinal}
		if err := json.Unmarshal([]byte(r.State), &e.State); err != nil {
			slog.Warn("proof: drop unreadable view state", "scope", "proof", "session", id, "ordinal", r.Ordinal, "err", err)
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// SetViewState saves one step's view state after checking it.
func (s *Service) SetViewState(id string, rev, ordinal int, state ViewState) error {
	if err := state.Check(); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(raw) > maxViewStateBytes {
		return invalid("view state is %d KiB; the limit is %d KiB", len(raw)>>10, maxViewStateBytes>>10)
	}
	return s.d.Repo.SetViewState(id, rev, ordinal, string(raw), s.d.Now())
}

// AcceptEdit makes the agent's pending revision current.
func (s *Service) AcceptEdit(id string) error {
	if err := s.d.Repo.AcceptEdit(id, s.d.Now()); err != nil {
		return err
	}
	s.seen(id)
	s.emit(id)
	return nil
}

// RejectEdit drops the agent's pending revision.
func (s *Service) RejectEdit(id string) error {
	if err := s.d.Repo.RejectEdit(id, s.d.Now()); err != nil {
		return err
	}
	s.seen(id)
	s.emit(id)
	return nil
}

// Delete removes a closed session and withdraws its popup.
func (s *Service) Delete(id string) error {
	ok, err := s.d.Repo.Delete(id)
	if err != nil {
		return err
	}
	if !ok {
		return notFound()
	}
	s.closePrompt(id)
	s.emit(id)
	return nil
}

// MarkSeen records that the user opened the session and withdraws its popup, unless an edit still
// waits for review.
func (s *Service) MarkSeen(id string) error {
	if err := s.d.Repo.MarkSeen(id, s.d.Now()); err != nil {
		return err
	}
	if row, ok, err := s.d.Repo.Session(id); err == nil && ok && row.PendingRev == nil {
		s.closePrompt(id)
	}
	return nil
}

func (s *Service) seen(id string) {
	if err := s.d.Repo.MarkSeen(id, s.d.Now()); err != nil {
		slog.Warn("proof: mark seen", "scope", "proof", "err", err)
	}
	s.closePrompt(id)
}
