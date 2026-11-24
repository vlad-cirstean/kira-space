package proof

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles/store"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// WindowKeyPrefix marks the proof window's key.
const WindowKeyPrefix = "proof-"

// WindowWidth and WindowHeight: proof window default, a step above the 1024x640 window floor, room
// for fitted images, screenshots still small (P749).
const (
	WindowWidth  = 1152
	WindowHeight = 720
)

// IsWindowKey reports whether key names a proof window.
func IsWindowKey(key string) bool { return strings.HasPrefix(key, WindowKeyPrefix) }

// ChannelChanged is broadcast to every window when sessions change.
const ChannelChanged = "kira:proof:changed"

// Deps are the seams Service reads.
type Deps struct {
	Repo *repos.ProofRepo
	// Blobs holds the accepted and proposed bytes of proof files; image steps read from it.
	Blobs *store.Blobs
	// OnEditResolved runs after the user accepts or rejects a pending edit, so the files follow the decision.
	OnEditResolved func(sessionID string, accepted bool) error
	// DeleteFiles removes the folder of a proof and its rows; Delete calls it for a proof with a folder.
	DeleteFiles func(taskID, slug string) error
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

// NotifyChanged announces an accepted change of the proof's files.
func (s *Service) NotifyChanged(id string) { s.emit(id) }

func promptID(sessionID string) string { return prompts.ID(prompts.KindProof, sessionID) }

func reviewID(sessionID string) string { return approvals.KindProofReview + ":" + sessionID }

// closeReadyPrompt withdraws the session's ready popup.
func (s *Service) closeReadyPrompt(sessionID string) {
	if s.d.Prompts != nil {
		s.d.Prompts.Close(promptID(sessionID))
	}
}

// closePrompt withdraws the session's ready popup and any pending edit review.
func (s *Service) closePrompt(sessionID string) {
	s.closeReadyPrompt(sessionID)
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
	TaskID    string       `json:"taskId"`
	Slug      string       `json:"slug"`
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
		pl.TaskID, pl.Slug = d.TaskID, d.Slug
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

// IsUserError reports whether err is a caller-correctable failure (validation or repo conflict).
func IsUserError(err error) bool {
	var inv *InvalidError
	var pe *repos.ProofError
	return errors.As(err, &inv) || errors.As(err, &pe)
}

// StepMeta is a step without its data. Diff is relative to the current revision for a pending one.
type StepMeta struct {
	Ordinal       int    `json:"ordinal"`
	ID            string `json:"id"`
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
	ID string `json:"id"`
	// TaskID and Slug name the proof's folder: tasks/<TaskID>/proofs/<Slug>.
	TaskID string `json:"taskId"`
	// TaskTitle is the title of the task holding the proof.
	TaskTitle string     `json:"taskTitle"`
	Slug      string     `json:"slug"`
	Name      string     `json:"name"`
	Summary   string     `json:"summary"`
	ClosedAt  int64      `json:"closedAt"`
	Current   Revision   `json:"current"`
	Pending   *Revision  `json:"pending"`
	Removed   []StepMeta `json:"removed"`
	// Version changes with every content write; a window compares it to tell its copy is stale.
	Version string `json:"version"`
}

func notFound() error {
	return &repos.ProofError{Code: repos.ProofNotFound, Msg: "no such proof session"}
}

func meta(rows []repos.ProofStepRow) []StepMeta {
	out := make([]StepMeta, len(rows))
	for i, r := range rows {
		out[i] = StepMeta{Ordinal: r.Ordinal, ID: r.StepKey, Description: r.Description, Language: r.Language, Source: r.Source, View: r.View,
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

func version(row repos.ProofSessionRow) string {
	var pend int
	if row.PendingRev != nil {
		pend = *row.PendingRev
	}
	return fmt.Sprintf("%d.%d.%d", *row.CurrentRev, pend, row.UpdatedAt)
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
	taskID, slug, _, err := s.d.Repo.LinkOf(id)
	if err != nil {
		return Detail{}, err
	}
	title, err := s.d.Repo.TaskTitleOf(id)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{ID: row.ID, TaskTitle: title, TaskID: taskID, Slug: slug, Name: row.Name, Summary: row.Summary, ClosedAt: *row.ClosedAt, Current: cur, Removed: []StepMeta{}, Version: version(row)}
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
	data := json.RawMessage(r.Payload)
	if r.View == ViewImage {
		if data, err = s.imageData(r.Payload); err != nil {
			return StepDetail{}, err
		}
	}
	return StepDetail{StepMeta: meta([]repos.ProofStepRow{r})[0], Query: r.Query, Data: data}, nil
}

// ImageData is the wire payload of an image step: the file's bytes beside its metadata.
type ImageData struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
	File   string `json:"file"`
	Base64 string `json:"base64"`
}

// imageData resolves a stored ImageMeta against the blob store.
func (s *Service) imageData(payload []byte) (json.RawMessage, error) {
	var m ImageMeta
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("proof: unreadable image step: %w", err)
	}
	var raw []byte
	if s.d.Blobs != nil {
		b, ok, err := s.d.Blobs.Get(m.SHA256)
		if err != nil {
			return nil, err
		}
		if ok {
			raw = b
		}
	}
	if raw == nil {
		return nil, &repos.ProofError{Code: repos.ProofNotFound, Msg: "the image file of this step is missing"}
	}
	return json.Marshal(ImageData{Format: m.Format, Width: m.Width, Height: m.Height, Bytes: m.Bytes, File: m.File, Base64: base64.StdEncoding.EncodeToString(raw)})
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
	if s.d.OnEditResolved != nil {
		if err := s.d.OnEditResolved(id, true); err != nil {
			return err
		}
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
	if s.d.OnEditResolved != nil {
		if err := s.d.OnEditResolved(id, false); err != nil {
			return err
		}
	}
	s.seen(id)
	s.emit(id)
	return nil
}

// Delete removes a closed session and withdraws its popup.
func (s *Service) Delete(id string) error {
	if taskID, slug, ok, err := s.d.Repo.LinkOf(id); err != nil {
		return err
	} else if ok && s.d.DeleteFiles != nil {
		if err := s.d.DeleteFiles(taskID, slug); err != nil {
			return err
		}
		s.closePrompt(id)
		s.emit(id)
		return nil
	}
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

// MarkSeen records that the user opened the session and withdraws its popup. The edit review stays
// while an edit still waits.
func (s *Service) MarkSeen(id string) error {
	if err := s.d.Repo.MarkSeen(id, s.d.Now()); err != nil {
		return err
	}
	s.closeReadyPrompt(id)
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

// Blobs is the store holding the bytes of proof files.
func (s *Service) Blobs() *store.Blobs { return s.d.Blobs }

// SetOnEditResolved registers the hook that makes the files follow an accepted or rejected edit. The
// task board owns the files and exists after the service, so it registers itself here.
func (s *Service) SetOnEditResolved(fn func(sessionID string, accepted bool) error) {
	s.d.OnEditResolved = fn
}

// SessionOf returns the proof session of a task folder.
func (s *Service) SessionOf(taskID, slug string) (id string, ok bool, err error) {
	u, err := s.d.Repo.ByTaskSlug(taskID, slug)
	if err != nil || u == nil {
		return "", false, err
	}
	return u.Session.ID, true, nil
}

// RaiseReady opens the "ready to review" popup of a new proof.
func (s *Service) RaiseReady(id string) {
	if row, ok, err := s.d.Repo.Session(id); err == nil && ok {
		s.openPrompt(id, row.Name, false)
	}
	s.emit(id)
}

// RaiseReview puts the review of a pending edit in the approvals queue, replacing an earlier one.
func (s *Service) RaiseReview(id string) {
	if row, ok, err := s.d.Repo.Session(id); err == nil && ok {
		s.putReview(id, row.Name)
	}
	s.emit(id)
}

// WithdrawReview removes the review of an edit that no longer exists.
func (s *Service) WithdrawReview(id string) {
	s.closePrompt(id)
	s.emit(id)
}

// LinkOf returns the task and proof folder of a session.
func (s *Service) LinkOf(id string) (taskID, slug string, ok bool, err error) {
	return s.d.Repo.LinkOf(id)
}

// SetOnDelete registers the hook that deletes a proof's folder with its rows.
func (s *Service) SetOnDelete(fn func(taskID, slug string) error) { s.d.DeleteFiles = fn }

// LegacyProofs lists the sessions of a task whose files the migration has not written yet.
func (s *Service) LegacyProofs(taskID string) ([]repos.LegacyProof, error) {
	return s.d.Repo.LegacyProofs(taskID)
}

// PurgeTaskless deletes the sessions no task holds, one log line each: unattached sessions have no
// entry point since proofs became task files.
func (s *Service) PurgeTaskless() {
	list, err := s.d.Repo.TasklessSessions()
	if err != nil {
		slog.Warn("proof: list taskless sessions", "scope", "proof", "err", err)
		return
	}
	for _, p := range list {
		var closed int64
		if p.ClosedAt != nil {
			closed = *p.ClosedAt
		}
		slog.Info("proof: delete taskless session", "scope", "proof", "session", p.ID, "name", p.Name, "closedAt", closed)
		if err := s.d.Repo.DeleteSession(p.ID); err != nil {
			slog.Warn("proof: delete taskless session", "scope", "proof", "session", p.ID, "err", err)
		}
	}
}
