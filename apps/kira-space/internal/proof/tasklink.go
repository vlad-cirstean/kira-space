package proof

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// Sources of a task link: the agent called attach_proof, or the user picked the session.
const (
	LinkSourceAgent = "agent"
	LinkSourceUser  = "user"
)

// TaskLink says which task a session attaches to, who attached it and the stage it happened in.
type TaskLink struct {
	TaskID, Source, KiraSessionID, StepID, StepName string
}

// TaskProof is a closed session attached to a task.
type TaskProof struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Summary       string     `json:"summary"`
	ClosedAt      int64      `json:"closedAt"`
	LinkedAt      int64      `json:"linkedAt"`
	Source        string     `json:"source"`
	KiraSessionID string     `json:"kiraSessionId"`
	StepID        string     `json:"stepId"`
	StepName      string     `json:"stepName"`
	Rev           int        `json:"rev"`
	Pending       bool       `json:"pending"`
	Steps         []StepMeta `json:"steps"`
}

func repoLink(l TaskLink) repos.ProofTaskLink {
	return repos.ProofTaskLink{TaskID: l.TaskID, Source: l.Source, KiraSessionID: l.KiraSessionID, StepID: l.StepID, StepName: l.StepName}
}

// AttachByName attaches the closed session with that name to a task.
func (s *Service) AttachByName(name string, l TaskLink) (TaskProof, error) {
	_, key, err := NormalizeName(name)
	if err != nil {
		return TaskProof{}, err
	}
	return s.attach(key, true, l)
}

// AttachByID attaches the closed session with that id to a task.
func (s *Service) AttachByID(id string, l TaskLink) (TaskProof, error) {
	return s.attach(id, false, l)
}

func (s *Service) attach(idOrKey string, byKey bool, l TaskLink) (TaskProof, error) {
	row, err := s.d.Repo.LinkTask(idOrKey, byKey, repoLink(l), s.d.Now())
	if err != nil {
		return TaskProof{}, err
	}
	s.emit(row.ID)
	out := TaskProof{ID: row.ID, Name: row.Name, Summary: row.Summary, Source: l.Source, StepID: l.StepID, StepName: l.StepName,
		Pending: row.PendingRev != nil, Steps: []StepMeta{}}
	if row.ClosedAt != nil {
		out.ClosedAt = *row.ClosedAt
	}
	if row.CurrentRev != nil {
		out.Rev = *row.CurrentRev
		list, err := s.d.Repo.Steps(row.ID, out.Rev)
		if err != nil {
			return TaskProof{}, err
		}
		out.Steps = meta(list)
	}
	return out, nil
}

// Detach removes a session from a task; the session stays in the proof window.
func (s *Service) Detach(id, taskID string) error {
	if err := s.d.Repo.UnlinkTask(id, taskID); err != nil {
		return err
	}
	s.emit(id)
	return nil
}

// TaskProofs lists a task's sessions, newest closed first.
func (s *Service) TaskProofs(taskID string) ([]TaskProof, error) {
	rows, err := s.d.Repo.TaskSessions(taskID)
	if err != nil {
		return nil, err
	}
	out := make([]TaskProof, len(rows))
	for i, r := range rows {
		p := TaskProof{ID: r.Session.ID, Name: r.Session.Name, Summary: r.Session.Summary, LinkedAt: r.LinkedAt,
			Source: r.Link.Source, KiraSessionID: r.Link.KiraSessionID, StepID: r.Link.StepID, StepName: r.Link.StepName,
			Pending: r.Session.PendingRev != nil, Steps: meta(r.Steps)}
		if r.Session.ClosedAt != nil {
			p.ClosedAt = *r.Session.ClosedAt
		}
		if r.Session.CurrentRev != nil {
			p.Rev = *r.Session.CurrentRev
		}
		out[i] = p
	}
	return out, nil
}

// Unattached lists closed sessions not attached to any task, newest first.
func (s *Service) Unattached() ([]Summary, error) {
	list, err := s.d.Repo.UnattachedSessions()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, len(list))
	for i, p := range list {
		out[i] = Summary{ID: p.ID, Name: p.Name, ClosedAt: p.ClosedAt, Steps: p.Steps, Pending: p.Pending, Seen: p.Seen}
	}
	return out, nil
}
