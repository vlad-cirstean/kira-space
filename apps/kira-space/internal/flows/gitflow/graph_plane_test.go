package gitflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// The graph rows ride the gitgraph data plane; every other graph call is a GitService method on
// the same window Conn. Covers the bound graph methods, credit, cancel, plane close, ResetWindow
// and the plane's refusals.
func TestGitGraphPlane(t *testing.T) {
	const scenario = "git-graph-plane"
	app := flowharness.New(t)
	svc := app.W.GitSvc
	ctx := context.Background()
	gs := app.OpenGitStreamFor("w1")
	key := gs.Key()
	maskIDs := flowharness.Mask("repoId")

	// 1200 rows over ChunkRows=500: three chunks.
	repo := app.NewHistory("hist", flowharness.HistorySpec{Commits: 1200, Branches: 1})
	id := openOn(t, gs, repo.Dir).RepoID

	// Bound graph methods and the plane share one walk.
	page := 600
	call[gitrpc.RepoSettingsSnapshot](t, gs, "repoSettings.set", gitrpc.RepoSettingsSetParams{RepoID: id, Patch: gitrpc.RepoSettingsPatchWire{GraphPageSize: &page}})
	if got := len(gs.Stream("graph.stream", m{"repoId": id}, 10).Drain()); got != 2 {
		t.Fatalf("first page = %d chunks, want 2", got)
	}
	loaded, err := svc.GraphLoadMore(ctx, bridge.GitGraphLoadMoreArgs{WindowKey: key, Params: gitrpc.GraphLoadMoreParams{RepoID: id}})
	if err != nil || !loaded.Started {
		t.Fatalf("GraphLoadMore = %+v, %v", loaded, err)
	}
	app.Contract(t, scenario, "git:graph.loadMore#bound", loaded)
	status, err := svc.GraphStatus(ctx, bridge.GitGraphStatusArgs{WindowKey: key, Params: gitrpc.GraphStatusParams{RepoID: id}})
	if err != nil || status.Loaded != 1200 || !status.Exhausted {
		t.Fatalf("GraphStatus = %+v, %v", status, err)
	}
	app.Contract(t, scenario, "git:graph.status#bound", status)
	metas := chunkMetas(t, gs.Stream("graph.stream", m{"repoId": id, "resumeThroughRow": 500}, 10).Drain())
	if len(metas) != 2 || metas[0].From != 500 || metas[1].To != 1200 {
		t.Fatalf("resumed chunks = %+v, want rows 500 to 1200", metas)
	}
	app.Contract(t, scenario, "graph.stream#plane", metas[0], maskIDs)
	refreshed, err := svc.GraphRefresh(ctx, bridge.GitGraphRefreshArgs{WindowKey: key, Params: gitrpc.GraphRefreshParams{RepoID: id}})
	if err != nil || !refreshed.Restarted {
		t.Fatalf("GraphRefresh = %+v, %v", refreshed, err)
	}
	app.Contract(t, scenario, "git:graph.refresh#bound", refreshed)
	if _, err := svc.GraphReportFailure(ctx, bridge.GitGraphReportFailureArgs{WindowKey: key, Params: gitrpc.GraphReportFailureParams{RepoID: id, Reason: "corrupted"}}); err != nil {
		t.Fatalf("GraphReportFailure: %v", err)
	}

	// Credit 1: one chunk, the next held until a credit arrives.
	held := gs.Stream("graph.stream", m{"repoId": id}, 1)
	if _, ok, err := held.Next(); err != nil || !ok {
		t.Fatalf("first chunk: ok=%v err=%v", ok, err)
	}
	if !held.Quiet(300 * time.Millisecond) {
		t.Fatal("second chunk arrived without credit")
	}
	held.Credit(1)
	if _, ok, err := held.Next(); err != nil || !ok {
		t.Fatalf("second chunk after credit: ok=%v err=%v", ok, err)
	}

	// Cancel mid-stream releases the walk lock: the next open and a bound status both answer.
	held.Cancel()
	if got := len(gs.Stream("graph.stream", m{"repoId": id}, 10).Drain()); got != 3 {
		t.Fatalf("stream after cancel = %d chunks, want 3", got)
	}
	blocked := gs.Stream("graph.stream", m{"repoId": id}, 1)
	if _, ok, err := blocked.Next(); err != nil || !ok {
		t.Fatalf("chunk before cancel: ok=%v err=%v", ok, err)
	}
	blocked.Cancel()
	if _, err := svc.GraphStatus(ctx, bridge.GitGraphStatusArgs{WindowKey: key, Params: gitrpc.GraphStatusParams{RepoID: id}}); err != nil {
		t.Fatalf("GraphStatus after cancel: %v", err)
	}

	// Plane close cancels the stream and keeps the hold.
	gs.Stream("graph.stream", m{"repoId": id}, 1).Next()
	gs.ClosePlane()
	if _, err := svc.GraphStatus(ctx, bridge.GitGraphStatusArgs{WindowKey: key, Params: gitrpc.GraphStatusParams{RepoID: id}}); err != nil {
		t.Fatalf("GraphStatus after plane close: %v", err)
	}
	if _, err := svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: key, Params: gitrpc.StatusGetParams{RepoID: id}}); err != nil {
		t.Fatalf("StatusGet after plane close: %v", err)
	}
	if got := len(gs.Stream("graph.stream", m{"repoId": id}, 10).Drain()); got != 3 {
		t.Fatalf("stream after redial = %d chunks, want 3", got)
	}

	// Refusals: bad params end the stream with an error; a duplicate live id leaves the first alone.
	_, ok, err := gs.Stream("graph.stream", m{"repoId": ""}, 1).Next()
	if ok || flowharness.ErrorCode(err) != string(ipcerr.CodeContract) {
		t.Fatalf("empty repoId: ok=%v err=%v, want %s", ok, err, ipcerr.CodeContract)
	}
	first := gs.Stream("graph.stream", m{"repoId": id}, 1)
	if _, ok, err := first.Next(); err != nil || !ok {
		t.Fatalf("first chunk: ok=%v err=%v", ok, err)
	}
	if err := first.OpenAgain(m{"repoId": id}); flowharness.ErrorCode(err) != string(ipcerr.CodeContract) {
		t.Fatalf("duplicate open error = %v", err)
	}
	first.Credit(1)
	if _, ok, err := first.Next(); err != nil || !ok {
		t.Fatalf("first stream after the duplicate open: ok=%v err=%v", ok, err)
	}
	first.Cancel()

	// ResetWindow drops the window's holds; another window's hold keeps the repo watched.
	other := app.OpenGitStreamFor("w2")
	openOn(t, other, repo.Dir)
	mark := app.Events.Mark()
	if _, err := svc.ResetWindow(ctx, bridge.GitResetWindowArgs{WindowKey: key}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: key, Params: gitrpc.StatusGetParams{RepoID: id}})
	if code, _ := boundCode(t, err); code != string(ipcerr.CodeConflict) {
		t.Fatalf("StatusGet after ResetWindow = %s, want %s", code, ipcerr.CodeConflict)
	}
	repo.Commit("after reset", map[string]string{"after.txt": "x\n"})
	app.Events.WaitAfter(t, mark, gitEventChanged, func(e flowharness.Event) bool { return e.Window == "w2" }, wait)
	for _, e := range app.Events.Since(mark, gitEventChanged) {
		if e.Window == key {
			t.Fatalf("repo.changed reached the reset window %q", key)
		}
	}
	if got := openOn(t, gs, repo.Dir); got.RepoID != id {
		t.Fatalf("reopen after reset = %s, want %s", got.RepoID, id)
	}
	if _, err := svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: key, Params: gitrpc.StatusGetParams{RepoID: id}}); err != nil {
		t.Fatalf("StatusGet after reopen: %v", err)
	}
}
