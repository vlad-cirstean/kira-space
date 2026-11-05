package adeflow_test

import (
	"encoding/json"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// brAnswer says whether an answer reached a request still open.
type brAnswer struct{ Resolved bool }

const branchItemPrefix = approvals.KindBranchRequest + ":"

// branchItem is the task's branch.request approvals item, nil when none is open.
func branchItem(app *flowharness.App, id string) *approvals.Item {
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		return nil
	}
	for i := range all {
		if all[i].ID == branchItemPrefix+id {
			return &all[i]
		}
	}
	return nil
}

// branchRequest reads an open request from its item's payload; E_NOT_FOUND once it is answered.
func branchRequest(app *flowharness.App, id string) (adewire.BranchRequest, error) {
	it := branchItem(app, id)
	if it == nil {
		return adewire.BranchRequest{}, ipcerr.NotFoundNamed("approval")
	}
	req := adewire.BranchRequest{ID: id, CreatedAt: it.CreatedAt, ExpiresAt: it.ExpiresAt}
	var pl struct {
		TaskID    string                      `json:"taskId"`
		TaskTitle string                      `json:"taskTitle"`
		Items     []adewire.BranchRequestItem `json:"items"`
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		return req, err
	}
	req.TaskID, req.TaskTitle, req.Items = pl.TaskID, pl.TaskTitle, pl.Items
	return req, nil
}

// answerBR answers one request. A request already gone reports Resolved false, never an error.
func answerBR(app *flowharness.App, id string, approve bool, opts any) (brAnswer, error) {
	args := approvals.AnswerArgs{IDs: []string{branchItemPrefix + id}, Approve: approve}
	if opts != nil {
		raw, err := json.Marshal(opts)
		if err != nil {
			return brAnswer{}, err
		}
		args.Options = map[string]json.RawMessage{branchItemPrefix + id: raw}
	}
	res, err := app.W.ApprovalsSvc.Answer(args)
	if err != nil {
		return brAnswer{}, err
	}
	switch {
	case res[0].Code == string(ipcerr.CodeNotFound):
		return brAnswer{}, nil
	case res[0].Error != "":
		return brAnswer{}, &ipcerr.Error{Code: res[0].Code, Message: res[0].Error}
	}
	return brAnswer{Resolved: true}, nil
}

type brOptions struct {
	ItemIDs       []string `json:"itemIds"`
	RemoteItemIDs []string `json:"remoteItemIds"`
	ShownItemIDs  []string `json:"shownItemIds,omitempty"`
}

func approveBR(app *flowharness.App, id string, itemIDs, remoteIDs []string) (brAnswer, error) {
	return answerBR(app, id, true, brOptions{ItemIDs: itemIDs, RemoteItemIDs: remoteIDs})
}

func denyBR(app *flowharness.App, id string) (brAnswer, error) { return answerBR(app, id, false, nil) }

func mustApprove(t *testing.T, app *flowharness.App, id string, itemIDs, remoteIDs []string) {
	t.Helper()
	if a, err := approveBR(app, id, itemIDs, remoteIDs); err != nil || !a.Resolved {
		t.Fatalf("approve = %+v, %v; want it to reach the open request", a, err)
	}
}

func mustDeny(t *testing.T, app *flowharness.App, id string) {
	t.Helper()
	if a, err := denyBR(app, id); err != nil || !a.Resolved {
		t.Fatalf("deny = %+v, %v; want it to reach the open request", a, err)
	}
}

func itemIDsOf(req adewire.BranchRequest) []string {
	ids := make([]string, len(req.Items))
	for i, it := range req.Items {
		ids[i] = it.ID
	}
	return ids
}
