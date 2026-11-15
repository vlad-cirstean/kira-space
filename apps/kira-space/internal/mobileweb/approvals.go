package mobileweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Approvals is the app's approvals queue (internal/approvals.Queue).
type Approvals interface {
	List() []approvals.Item
	Answer(ids []string, approve bool, options map[string]json.RawMessage) []approvals.Result
}

// maxAnswerIDs bounds one answer call.
const maxAnswerIDs = 50

// phoneApprovalKinds are the Space agent requests a phone sees and answers. Studio kinds carry
// statements, result rows, request bodies and commands, so they stay on the desktop.
var phoneApprovalKinds = map[string]bool{
	approvals.KindTaskCreate:    true,
	approvals.KindBranchRequest: true,
	approvals.KindDetailsEdit:   true,
}

// phoneApprovals keeps the phone kinds and strips absolute paths: Cwd, Location, payload cwd keys,
// and payload worktree paths down to their base name.
func phoneApprovals(items []approvals.Item) ([]approvals.Item, error) {
	out := make([]approvals.Item, 0, len(items))
	for _, it := range items {
		if !phoneApprovalKinds[it.Kind] {
			continue
		}
		it.Origin.Cwd = ""
		it.Target.Location = ""
		if len(it.Payload) > 0 {
			var v any
			if err := json.Unmarshal(it.Payload, &v); err != nil {
				return nil, err
			}
			dropCwd(v)
			baseWorktrees(v)
			raw, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			it.Payload = raw
		}
		out = append(out, it)
	}
	return out, nil
}

func baseWorktrees(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, c := range t {
			if s, ok := c.(string); ok && k == "worktree" {
				t[k] = filepath.Base(s)
				continue
			}
			baseWorktrees(c)
		}
	case []any:
		for _, c := range t {
			baseWorktrees(c)
		}
	}
}

func approvalsEvent(v any) (any, error) {
	items, ok := v.([]approvals.Item)
	if !ok {
		return nil, fmt.Errorf("mobileweb: approvals event payload is %T", v)
	}
	return phoneApprovals(items)
}

func (s *Server) handleApprovals(w http.ResponseWriter, _ *http.Request, _ repos.MobileDeviceRow) {
	if s.cfg.Approvals == nil {
		writeErrorT(w, http.StatusServiceUnavailable, codeUnavailable, "backend.space.mobileweb.approvalsUnavailable.body")
		return
	}
	items, err := phoneApprovals(s.cfg.Approvals.List())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

type approvalAnswerRequest struct {
	IDs     []string                   `json:"ids"`
	Approve bool                       `json:"approve"`
	Options map[string]json.RawMessage `json:"options"`
}

func (s *Server) handleApprovalAnswer(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
	if s.cfg.Approvals == nil {
		writeErrorT(w, http.StatusServiceUnavailable, codeUnavailable, "backend.space.mobileweb.approvalsUnavailable.body")
		return
	}
	var req approvalAnswerRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if len(req.IDs) == 0 {
		writeServiceError(w, ipcerr.Required("ids"))
		return
	}
	if len(req.IDs) > maxAnswerIDs {
		writeServiceError(w, ipcerr.Contract("ids holds at most %d entries", maxAnswerIDs))
		return
	}
	pending := map[string]bool{}
	for _, it := range s.cfg.Approvals.List() {
		if phoneApprovalKinds[it.Kind] {
			pending[it.ID] = true
		}
	}
	var answerable []string
	options := map[string]json.RawMessage{}
	for _, id := range req.IDs {
		auditAdd(r, "approval", id)
		if pending[id] {
			answerable = append(answerable, id)
			if o, ok := req.Options[id]; ok {
				options[id] = o
			}
		}
	}
	var answered []approvals.Result
	if len(answerable) > 0 {
		answered = s.cfg.Approvals.Answer(answerable, req.Approve, options)
	}
	byID := make(map[string]approvals.Result, len(answered))
	for _, res := range answered {
		byID[res.ID] = res
	}
	out := make([]approvals.Result, 0, len(req.IDs))
	for _, id := range req.IDs {
		if res, ok := byID[id]; ok {
			out = append(out, res)
			continue
		}
		e := ipcerr.NotFoundNamed("approval")
		out = append(out, approvals.Result{ID: id, Code: e.Code, Error: e.Message})
	}
	writeJSON(w, http.StatusOK, out)
}
