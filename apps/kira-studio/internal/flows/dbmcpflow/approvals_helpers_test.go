package dbmcpflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// pendingReq is one queue item decoded for assertions.
type pendingReq struct {
	Item         approvals.Item
	RequestID    string
	Stage        string
	Class        string
	Statement    string
	ConnectionID string
	Bytes        int
	Cluster      *kubeCluster
}

type kubeCluster struct{ Context, Server, Namespace, Operation string }

func decodeItem(t testing.TB, it approvals.Item) pendingReq {
	t.Helper()
	var pl struct {
		ConnectionID string `json:"connectionId"`
		Statement    string `json:"statement"`
		Bytes        int    `json:"bytes"`
		Context      string `json:"context"`
		Server       string `json:"server"`
		Namespace    string `json:"namespace"`
		Operation    string `json:"operation"`
	}
	if err := json.Unmarshal(it.Payload, &pl); err != nil {
		t.Fatalf("payload of %s: %v", it.ID, err)
	}
	r := pendingReq{
		Item: it, RequestID: strings.TrimPrefix(it.ID, it.Kind+":"), Stage: "query", Class: it.Operation.Class,
		Statement: pl.Statement, ConnectionID: pl.ConnectionID, Bytes: pl.Bytes,
	}
	switch it.Kind {
	case approvals.KindDbResponse:
		r.Stage = "response"
	case approvals.KindKubeRead:
		r.Cluster = &kubeCluster{Context: pl.Context, Server: pl.Server, Namespace: pl.Namespace, Operation: pl.Operation}
	}
	return r
}

func (f *fixture) list(t *testing.T) []approvals.Item {
	t.Helper()
	all, err := f.app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// items waits for n queue items, oldest first.
func (f *fixture) items(t *testing.T, n int) []approvals.Item {
	t.Helper()
	var got []approvals.Item
	testx.WaitUntil(t, 10*time.Second, func() bool {
		got = f.list(t)
		return len(got) == n
	})
	return got
}

// pending waits for the oldest queue item.
func (f *fixture) pending(t *testing.T) pendingReq {
	t.Helper()
	var first approvals.Item
	testx.WaitUntil(t, 10*time.Second, func() bool {
		all := f.list(t)
		if len(all) == 0 {
			return false
		}
		first = all[0]
		return true
	})
	return decodeItem(t, first)
}

// stageNow reports the oldest item's stage, or "" when the queue is empty.
func (f *fixture) stageNow(t *testing.T) string {
	t.Helper()
	all := f.list(t)
	if len(all) == 0 {
		return ""
	}
	return decodeItem(t, all[0]).Stage
}

func (f *fixture) queueLen(t *testing.T) int { t.Helper(); return len(f.list(t)) }

func (f *fixture) answerItems(t *testing.T, approve bool, ids ...string) []approvals.Result {
	t.Helper()
	res, err := f.app.W.ApprovalsSvc.Answer(approvals.AnswerArgs{IDs: ids, Approve: approve})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Error != "" {
			t.Fatalf("answer %s: %+v", r.ID, r)
		}
	}
	return res
}

func (f *fixture) answer(t *testing.T, approve bool, requestID string) {
	t.Helper()
	for _, it := range f.list(t) {
		if strings.HasSuffix(it.ID, ":"+requestID) {
			f.answerItems(t, approve, it.ID)
			return
		}
	}
	t.Fatalf("no queue item for request %s", requestID)
}
