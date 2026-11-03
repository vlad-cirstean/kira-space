package gitflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

const gitEventChanged = bridge.GitEventPrefix + "repo.changed"

func boundCode(t *testing.T, err error) (string, string) {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("error %v is not an ipcerr.Error", err)
	}
	return ie.Code, ie.Message
}

// GitService carries the read methods of a window's connection: the same results as the stream,
// events addressed to the window, a refused restricted setting, release on window close.
func TestGitServiceBound(t *testing.T) {
	const scenario = "git-bound"
	app := flowharness.New(t)
	svc := app.W.GitSvc
	ctx := context.Background()
	mask := flowharness.Mask("sha", "parents", "timestamp", "authorTimeSeconds", "committerDate", "objectId")

	repo := app.NewRepo("proj")
	repo.Commit("first", map[string]string{"f.txt": "one\ntwo\n"})
	sha := repo.Commit("second", map[string]string{"f.txt": "one\ntwo\nthree\n"})
	repo.Write("f.txt", "one\ntwo\nthree\nfour\n")

	open, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: repo.Dir}})
	if err != nil || open.Kind != "ok" || open.Repo == nil {
		t.Fatalf("RepoOpen = %+v, %v", open, err)
	}
	id := open.Repo.RepoID

	init, err := svc.AppInit(ctx, bridge.GitAppInitArgs{WindowKey: "w1"})
	if err != nil || init.Git.Kind == "" {
		t.Fatalf("AppInit = %+v, %v", init, err)
	}
	status, err := svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: "w1", Params: gitrpc.StatusGetParams{RepoID: id}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "git:status.get#bound", status, mask)
	refs, err := svc.RefsList(ctx, bridge.GitRefsListArgs{WindowKey: "w1", Params: gitrpc.RefsListParams{RepoID: id}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "git:refs.list#bound", refs, mask)
	detail, err := svc.CommitDetail(ctx, bridge.GitCommitDetailArgs{WindowKey: "w1", Params: gitrpc.CommitDetailParams{RepoID: id, SHA: sha}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "git:commit.detail#bound", detail, mask)
	if _, err := svc.CommitFileDiff(ctx, bridge.GitCommitFileDiffArgs{WindowKey: "w1", Params: gitrpc.CommitFileDiffParams{RepoID: id, SHA: sha, Path: "f.txt"}}); err != nil {
		t.Fatal(err)
	}
	working, err := svc.WorkingDetail(ctx, bridge.GitWorkingDetailArgs{WindowKey: "w1", Params: gitrpc.WorkingDetailParams{RepoID: id}})
	if err != nil || len(working.Files) != 1 {
		t.Fatalf("WorkingDetail = %+v, %v", working, err)
	}
	file, err := svc.FileRead(ctx, bridge.GitFileReadArgs{WindowKey: "w1", Params: gitrpc.FileReadParams{RepoID: id, Rev: sha, Path: "f.txt"}})
	if err != nil || file.Kind != "found" {
		t.Fatalf("FileRead = %+v, %v", file, err)
	}
	app.Contract(t, scenario, "git:file.read#bound", file)
	if _, err := svc.FileGoToTarget(ctx, bridge.GitFileGoToTargetArgs{WindowKey: "w1", Params: gitrpc.FileGoToTargetParams{RepoID: id, Rev: sha, Path: "f.txt"}}); err != nil {
		t.Fatal(err)
	}
	blame, err := svc.BlameLine(ctx, bridge.GitBlameLineArgs{WindowKey: "w1", Params: gitrpc.BlameLineParams{RepoID: id, Path: "f.txt", Line: 1}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "git:blame.line#bound", blame, mask)
	hits, err := svc.SearchRun(ctx, bridge.GitSearchRunArgs{WindowKey: "w1", Params: gitrpc.SearchRunParams{RepoID: id, Query: gitrpc.SearchQueryParams{Text: "second"}}})
	if err != nil || hits.Kind != "ok" || len(hits.Hits) != 1 {
		t.Fatalf("SearchRun = %+v, %v", hits, err)
	}
	settings, err := svc.RepoSettingsGet(ctx, bridge.GitRepoSettingsGetArgs{WindowKey: "w1", Params: gitrpc.RepoSettingsGetParams{RepoID: id}})
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "git:repoSettings.get#bound", settings)

	// The window's connection holds the repo for a stream of the same window too.
	gs := app.OpenGitStreamFor("w1")
	var graph gitrpc.GraphStatusResult
	gs.MustRequest("graph.status", gitrpc.GraphStatusParams{RepoID: id}, &graph)

	// Events go to the window that holds the repo only.
	repo.Commit("third", map[string]string{"g.txt": "g\n"})
	ev := app.Events.Wait(t, gitEventChanged, func(e flowharness.Event) bool { return e.Window == "w1" }, wait)
	app.Contract(t, scenario, "event:"+gitEventChanged, ev.Data)
	for _, e := range app.Events.Since(0, gitEventChanged) {
		if e.Window != "w1" {
			t.Fatalf("repo.changed went to window %q, want w1 only", e.Window)
		}
	}

	// A restricted setting is refused before the handler.
	script := "rm -rf /"
	_, err = svc.RepoSettingsSet(ctx, bridge.GitRepoSettingsSetArgs{WindowKey: "w1", Params: gitrpc.RepoSettingsSetParams{
		RepoID: id, Patch: gitrpc.RepoSettingsPatchWire{WorktreePrepareScript: &script},
	}})
	code, msg := boundCode(t, err)
	if code != string(ipcerr.CodeReadOnly) {
		t.Fatalf("restricted set = %s %s, want E_READ_ONLY", code, msg)
	}
	size := 500
	snap, err := svc.RepoSettingsSet(ctx, bridge.GitRepoSettingsSetArgs{WindowKey: "w1", Params: gitrpc.RepoSettingsSetParams{
		RepoID: id, Patch: gitrpc.RepoSettingsPatchWire{GraphPageSize: &size},
	}})
	if err != nil || snap.GraphPageSize != size {
		t.Fatalf("RepoSettingsSet = %+v, %v", snap, err)
	}

	// An argument-injection ref is refused.
	_, err = svc.CommitDetail(ctx, bridge.GitCommitDetailArgs{WindowKey: "w1", Params: gitrpc.CommitDetailParams{RepoID: id, SHA: "--output=x"}})
	if code, _ := boundCode(t, err); code != string(ipcerr.CodeContract) {
		t.Fatalf("injection ref = %s, want E_CONTRACT", code)
	}

	// Closing a window releases its holds and leaves the others alone.
	if _, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w2", Params: gitrpc.RepoOpenParams{Path: repo.Dir}}); err != nil {
		t.Fatal(err)
	}
	gs.Close()
	app.W.GitWindows.Close("w1")
	_, err = svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: "w1", Params: gitrpc.StatusGetParams{RepoID: id}})
	if code, _ := boundCode(t, err); code != string(ipcerr.CodeConflict) {
		t.Fatalf("StatusGet after close = %s, want E_CONFLICT", code)
	}
	// A late open on the closed window is refused and creates no Conn; a page boot revives the key.
	_, err = svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: repo.Dir}})
	if code, _ := boundCode(t, err); code != string(ipcerr.CodeConflict) {
		t.Fatalf("RepoOpen after close = %s, want E_CONFLICT", code)
	}
	if _, err := svc.ResetWindow(ctx, bridge.GitResetWindowArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RepoOpen(ctx, bridge.GitRepoOpenArgs{WindowKey: "w1", Params: gitrpc.RepoOpenParams{Path: repo.Dir}}); err != nil {
		t.Fatalf("RepoOpen after ResetWindow: %v", err)
	}
	app.W.GitWindows.Close("w1")
	if _, err := svc.StatusGet(ctx, bridge.GitStatusGetArgs{WindowKey: "w2", Params: gitrpc.StatusGetParams{RepoID: id}}); err != nil {
		t.Fatalf("w2 after w1 closed: %v", err)
	}
	if _, err := svc.RepoClose(ctx, bridge.GitRepoCloseArgs{WindowKey: "w2", Params: gitrpc.RepoCloseParams{RepoID: id}}); err != nil {
		t.Fatal(err)
	}
}
