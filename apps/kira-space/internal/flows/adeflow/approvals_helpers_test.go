package adeflow_test

import (
	"encoding/json"
	"fmt"
	"sort"
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

// orderedBranchItem is branchItem for a request built by parallel calls: they join in any order and
// number their items by call, so the payload's items are sorted by repo and branch and renumbered.
func orderedBranchItem(t *testing.T, app *flowharness.App, id string) approvals.Item {
	t.Helper()
	it := branchItem(app, id)
	if it == nil {
		t.Fatalf("no approval item for request %s", id)
	}
	var pl map[string]json.RawMessage
	var items []map[string]any
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(pl["items"], &items); err != nil {
		t.Fatal(err)
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := fmt.Sprint(items[i]["repo"], "/", items[i]["branch"]), fmt.Sprint(items[j]["repo"], "/", items[j]["branch"])
		return a < b
	})
	for i := range items {
		items[i]["id"] = fmt.Sprintf("item%d", i)
	}
	var err error
	if pl["items"], err = json.Marshal(items); err != nil {
		t.Fatal(err)
	}
	if it.Payload, err = json.Marshal(pl); err != nil {
		t.Fatal(err)
	}
	return *it
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
