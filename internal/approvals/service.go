package approvals

import (
	"encoding/json"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Service is the bound-service surface shared by both apps; each embeds it.
type Service struct{ Q *Queue }

// AnswerArgs answers ids with one decision. Options is keyed by id.
type AnswerArgs struct {
	IDs     []string                   `json:"ids"`
	Approve bool                       `json:"approve"`
	Options map[string]json.RawMessage `json:"options"`
}

// List returns every pending item.
func (s *Service) List() ([]Item, error) { return s.Q.List(), nil }

// Answer answers each id through its owner and reports per-id results.
func (s *Service) Answer(args AnswerArgs) ([]Result, error) {
	if len(args.IDs) == 0 {
		return nil, ipcerr.Required("ids")
	}
	return s.Q.Answer(args.IDs, args.Approve, args.Options), nil
}
