package proof

// TaskProof is a closed session attached to a task.
type TaskProof struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Summary       string     `json:"summary"`
	ClosedAt      int64      `json:"closedAt"`
	LinkedAt      int64      `json:"linkedAt"`
	Slug          string     `json:"slug"`
	KiraSessionID string     `json:"kiraSessionId"`
	StepID        string     `json:"stepId"`
	StepName      string     `json:"stepName"`
	Rev           int        `json:"rev"`
	Pending       bool       `json:"pending"`
	Steps         []StepMeta `json:"steps"`
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
			Slug: r.Link.Slug, KiraSessionID: r.Link.KiraSessionID, StepID: r.Link.StepID, StepName: r.Link.StepName,
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
