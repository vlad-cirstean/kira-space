package dbmcpflow

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// Helpers of the API side e2e scenarios (P686 Stream A).

// e2eOpts is the normalisation every API scenario contract shares: volatile fields masked and the test
// server's address replaced.
func (f *fixture) e2eOpts(srvURL string, extra ...flowharness.ContractOption) []flowharness.ContractOption {
	opts := []flowharness.ContractOption{
		flowharness.Mask("createdAt", "expiresAt", "cwd", "environment.id", "expiresInSeconds"),
		flowharness.Replace(srvURL, "http://127.0.0.1:<port>"), flowharness.Replace(strings.TrimPrefix(srvURL, "http://"), "127.0.0.1:<port>"),
		flowharness.Replace(f.dbPath, "/data/mcp.db"),
	}
	return append(opts, extra...)
}

// setDbMcp applies one dbMcp settings patch and records the call under scenario/key for the UI half.
func (f *fixture) setDbMcp(t *testing.T, scenario, key string, patch *model.DbMcpPatch) {
	t.Helper()
	args := bridge.SettingsSetArgs{Patch: model.SettingsPatch{DbMcp: patch}}
	if _, err := f.app.W.Settings.Set(args); err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, scenario, "args:SettingsService.Set#"+key, args)
}

// answerWith answers one item with options, failing on a refusal.
func (f *fixture) answerWith(t *testing.T, approve bool, id string, opts map[string]string) {
	t.Helper()
	args := approvals.AnswerArgs{IDs: []string{id}, Approve: approve}
	if len(opts) > 0 {
		args.Options = map[string]json.RawMessage{}
		for k, v := range opts {
			args.Options[k] = json.RawMessage(v)
		}
	}
	res, err := f.app.W.ApprovalsSvc.Answer(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Error != "" {
			t.Fatalf("answer %s: %+v", r.ID, r)
		}
	}
}

// heldCall is an api_request whose response waits for the user's review.
type heldCall struct {
	pending chan callOutcome
	req     pendingReq
	held    bridge.DbMcpPendingResponse
}

// startHeld calls api_request and waits until its response item shows.
func (f *fixture) startHeld(t *testing.T, sess *mcp.ClientSession, args map[string]any) heldCall {
	t.Helper()
	pending := callToolAsync(sess, "api_request", args)
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	req := f.pending(t)
	return heldCall{pending: pending, req: req, held: f.pendingResponse(t, req.RequestID)}
}

// finish answers the held response and returns the agent's result.
func (f *fixture) finish(t *testing.T, h heldCall, send bool) *mcp.CallToolResult {
	t.Helper()
	f.answerItems(t, send, h.req.Item.ID)
	return wait(t, h.pending)
}

// keptNames lists the live kept variable names of scope.
func (f *fixture) keptNames(scope string) []string {
	var names []string
	for _, m := range f.app.W.Kept.List(scope) {
		names = append(names, m.Name)
	}
	return names
}

// studioGrant opens a client on the per-session Studio server and returns release alone: the server side of
// the session ends (its kept variables are cleared, its parked approvals abandoned) while the client stays
// open to read the answer of a call that was waiting.
func (f *fixture) studioGrant(t *testing.T, id string) (*mcp.ClientSession, func()) {
	t.Helper()
	entry, release, err := f.app.W.DbMcpSessions.Grant(dbmcp.SessionIdentity{SessionID: id, Cwd: f.app.Work})
	if err != nil {
		t.Fatalf("grant session %s: %v", id, err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "flow-studio-client", Version: "1"}, nil)
	sess, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   entry.URL,
		HTTPClient: &http.Client{Transport: headers(entry.Headers), Timeout: 30 * time.Second},
	}, nil)
	if err != nil {
		release()
		t.Fatalf("connect session %s: %v", id, err)
	}
	t.Cleanup(func() { _ = sess.Close(); release() })
	return sess, release
}

// startHeldAfterCard runs api_request through its approval card and returns it with the response held.
func (f *fixture) startHeldAfterCard(t *testing.T, sess *mcp.ClientSession, args map[string]any) heldCall {
	t.Helper()
	pending := callToolAsync(sess, "api_request", args)
	f.answerItems(t, true, f.pending(t).Item.ID)
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	req := f.pending(t)
	return heldCall{pending: pending, req: req, held: f.pendingResponse(t, req.RequestID)}
}

// startHeldFromQueue waits for the oldest response item and returns it with its held response.
func (f *fixture) startHeldFromQueue(t *testing.T) heldCall {
	t.Helper()
	testx.WaitUntil(t, 10*time.Second, func() bool { return f.stageNow(t) == "response" })
	req := f.pending(t)
	return heldCall{req: req, held: f.pendingResponse(t, req.RequestID)}
}
