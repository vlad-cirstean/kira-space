package adeflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/fakehost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/repocfg"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeagent"
	"github.com/kirathecat/kira-studio/internal/shell"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// P409 task artifacts. Contract ade-artifacts: tests/ui/ade-v2-artifacts.spec.ts reads these.

const (
	artBranch  = "feat/api-work"
	artPRURL   = "https://forge.test/acme/api/-/merge_requests/7"
	artIssue   = "https://forge.test/acme/api/-/issues/3"
	artPreview = "https://preview.example/1"
)

var artMask = flowharness.Mask("id", "createdAt", "ref", "taskId", "sessionId")

func reportCall(items ...map[string]any) fakeagent.MCPCall {
	arr := make([]any, len(items))
	for i, it := range items {
		arr[i] = it
	}
	return fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "report_artifacts", Tolerate: true, Args: map[string]any{"artifacts": arr}}
}

func artItem(kind, title, url, ref string) map[string]any {
	return map[string]any{"kind": kind, "title": title, "url": url, "ref": ref}
}

// mcpReply reads a recorded tool result: the isError text, or the listing a finishing call appended.
func mcpReply(t *testing.T, app *flowharness.App, name string) (text string, list agentmcp.TaskArtifacts) {
	t.Helper()
	var raw []byte
	testx.WaitUntil(t, waitFor, func() bool {
		var err error
		raw, err = os.ReadFile(filepath.Join(app.FakeDir, name))
		return err == nil && json.Valid(raw)
	})
	var errRes struct {
		IsError bool   `json:"isError"`
		Text    string `json:"text"`
	}
	if json.Unmarshal(raw, &errRes) == nil && errRes.IsError {
		return errRes.Text, list
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	for _, b := range blocks {
		if _, body, ok := strings.Cut(b.Text, "\n"); ok && strings.HasPrefix(body, "{") {
			if err := json.Unmarshal([]byte(body), &list); err != nil {
				t.Fatalf("%s: listing: %v", name, err)
			}
			return b.Text, list
		}
	}
	return string(raw), list
}

func artifactsOf(t *testing.T, app *flowharness.App, taskID string) adewire.ArtifactsResult {
	t.Helper()
	res, err := app.W.AdeTask.Artifacts(ctx, adewire.TaskArgs{TaskID: taskID})
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	return res
}

func findArtifact(res adewire.ArtifactsResult, kind, title string) (adewire.Artifact, bool) {
	for _, a := range res.Artifacts {
		if a.Kind == kind && a.Title == title {
			return a, true
		}
	}
	return adewire.Artifact{}, false
}

func TestTaskArtifacts(t *testing.T) {
	dir := t.TempDir()
	ready, hold1, hold2 := filepath.Join(dir, "ready"), filepath.Join(dir, "hold1"), filepath.Join(dir, "hold2")
	finish := fakeagent.MCPCall{Server: agentmcp.ServerName, Tool: "finish_step", Args: map[string]any{"status": "done", "summary": "all done"}}
	tooMany := make([]map[string]any, 21)
	for i := range tooMany {
		tooMany[i] = artItem("link", "l", "https://x.example/"+string(rune('a'+i)), "")
	}
	forge := fakehost.New()
	forge.AddChangeRequest(githost.ChangeRequest{
		Number: 7, Title: "Fix login", State: "open", HeadRef: artBranch, HeadOwner: "acme", BaseRef: "main", URL: artPRURL,
	})
	f := newRunFixtureWith(t, []fakeagent.Action{
		{Sh: rvImplement + " && until [ -f " + ready + " ]; do sleep 0.1; done", MCP: []fakeagent.MCPCall{
			reportCall(
				artItem("doc", "Design notes", "", "docs/design.md"),
				artItem("deployment", "Preview", artPreview, ""),
				artItem("link", "The PR", artPRURL, ""),
			),
			reportCall(artItem("deployment", "Preview env", artPreview, "")),
			reportCall(artItem("link", "bad", "javascript:alert(1)", "")),
			reportCall(artItem("branch", "b", "", "b")),
			reportCall(artItem("doc", "nothing", "", "")),
			reportCall(tooMany...),
			finish,
		}, Name: "nofinish"},
		{MCP: []fakeagent.MCPCall{handoverCall("Signed off.", true)}, WaitFile: hold1},
		{MCP: []fakeagent.MCPCall{handoverCall("Signed off again.", true)}, WaitFile: hold2},
	}, agentStep("one", "")+manualStep, flowharness.WithGitHost(forge), flowharness.WithoutGh(), flowharness.WithTrackerGrace(time.Second))
	app := f.app
	gitOut(t, f.repo.Dir, "remote", "set-url", "origin", "https://forge.test/acme/api.git")

	hostURL := artIssue
	if _, err := app.W.AdeTask.UpdateTask(ctx, adewire.UpdateTaskArgs{TaskID: f.taskID, Patch: adewire.TaskPatch{
		HostURL: &hostURL, Jira: &adewire.Jira{Key: "KIRA-12", URL: "https://jira.test/browse/KIRA-12"},
	}}); err != nil {
		t.Fatal(err)
	}
	f.start(t, artBranch)
	testx.WaitUntil(t, waitFor, func() bool {
		return branchOf(t, board(t, app), f.taskID, f.repoID).Ahead == 1
	})
	// main holds the branch tip, so the prod script's HEAD contains it.
	gitOut(t, f.repo.Dir, "merge", "-q", "--ff-only", artBranch)
	setRepoFile(t, app, f.repoID, func(rf *repocfg.File) {
		rf.Environments = []repocfg.Environment{{Name: "prod", DeployedSHAScript: "git rev-parse HEAD"}}
	})
	testx.WaitUntil(t, waitFor, func() bool {
		br := branchOf(t, board(t, app), f.taskID, f.repoID)
		return len(br.Deployments) == 1 && br.Deployments[0].Status == "deployed"
	})
	if err := os.WriteFile(ready, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitRun(t, app, f.taskID, "one", "done")

	// 1. The finishing reply lists everything; the reported PR folds into the detected one.
	_, list := mcpReply(t, app, "mcp-finish_step-1.json")
	kinds := map[string]int{}
	for _, a := range list.Artifacts {
		kinds[a.Kind]++
	}
	if kinds["changeRequest"] != 1 || kinds["issue"] != 2 || kinds["branch"] != 1 || kinds["deployment"] != 2 || kinds["doc"] != 1 || kinds["link"] != 0 || len(list.Artifacts) != 7 {
		t.Fatalf("finish_step artifacts = %+v", list.Artifacts)
	}
	if len(list.Gaps) != 1 || list.Gaps[0].Source != "jira" || list.Gaps[0].Reason != "unavailable" {
		t.Fatalf("gaps = %+v, want the jira gap only (the harness has no acli)", list.Gaps)
	}
	res := artifactsOf(t, app, f.taskID)
	pr, ok := findArtifact(res, "changeRequest", "Fix login")
	if !ok || pr.Source != "host" || pr.Status != "open" || pr.URL != artPRURL || pr.Ref != "#7" || pr.ID != "" {
		t.Fatalf("change request = %+v, want the detected PR #7 once", pr)
	}
	if a, ok := findArtifact(res, "branch", artBranch); !ok || a.Source != "git" || a.Ref == "" || a.Status != "pushed" {
		t.Fatalf("branch = %+v", a)
	}
	if a, ok := findArtifact(res, "deployment", "prod"); !ok || a.Source != "git" || a.Status != "deployed" || len(a.Ref) != 7 {
		t.Fatalf("deployment = %+v", a)
	}
	if a, ok := findArtifact(res, "issue", "KIRA-12"); !ok || a.Source != "jira" || a.Status != "" {
		t.Fatalf("jira = %+v", a)
	}
	if a, ok := findArtifact(res, "issue", "api#3"); !ok || a.Source != "task" || a.URL != artIssue {
		t.Fatalf("host link = %+v", a)
	}
	doc, ok := findArtifact(res, "doc", "Design notes")
	if !ok || doc.Source != "agent" || doc.Ref != "docs/design.md" || doc.ID == "" {
		t.Fatalf("doc = %+v", doc)
	}
	app.Contract(t, "ade-artifacts", "AdeTaskService.Artifacts", res, artMask)

	// 2. Reporting the same URL again updates the row.
	preview := 0
	for _, a := range res.Artifacts {
		if a.URL == artPreview {
			preview++
			if a.Title != "Preview env" || a.Source != "agent" {
				t.Fatalf("preview = %+v, want the new title", a)
			}
		}
	}
	if preview != 1 {
		t.Fatalf("preview rows = %d, want 1", preview)
	}

	// 3. Refusals are tool errors and store nothing.
	for n, want := range map[string]string{
		"mcp-report_artifacts-3.json": "http or https",
		"mcp-report_artifacts-4.json": "unknown kind",
		"mcp-report_artifacts-5.json": "url or a ref",
		"mcp-report_artifacts-6.json": "1 to 20",
	} {
		if text, _ := mcpReply(t, app, n); !strings.Contains(text, want) {
			t.Fatalf("%s = %q, want %q", n, text, want)
		}
	}
	stored := 0
	for _, a := range res.Artifacts {
		if a.ID != "" {
			stored++
		}
	}
	if stored != 2 {
		t.Fatalf("stored rows = %d, want the doc and the preview", stored)
	}

	// 4. The bridge adds a user row, removes one, and refuses a detected id.
	add := adewire.AddLinkArgs{TaskID: f.taskID, Text: "Runbook https://wiki.example/runbook"}
	app.Contract(t, "ade-artifacts", "args:AdeTaskService.AddLink", add)
	added, err := app.W.AdeTask.AddLink(ctx, add)
	if err != nil || added.Artifact == nil || added.Artifact.ID == "" || added.Artifact.Source != "user" {
		t.Fatalf("AddLink = %+v, %v", added, err)
	}
	app.Contract(t, "ade-artifacts", "AdeTaskService.AddLink", added, artMask)
	for _, bad := range []string{"plain words", "ftp://x.example/f", "https://a.example https://b.example"} {
		if _, err := app.W.AdeTask.AddLink(ctx, adewire.AddLinkArgs{TaskID: f.taskID, Text: bad}); err == nil {
			t.Fatalf("AddLink accepted %q", bad)
		}
	}
	rm := adewire.RemoveArtifactArgs{TaskID: f.taskID, ID: doc.ID}
	app.Contract(t, "ade-artifacts", "args:AdeTaskService.RemoveArtifact", rm)
	if err := app.W.AdeTask.RemoveArtifact(ctx, rm); err != nil {
		t.Fatal(err)
	}
	br := branchOf(t, board(t, app), f.taskID, f.repoID)
	if err := app.W.AdeTask.RemoveArtifact(ctx, adewire.RemoveArtifactArgs{TaskID: f.taskID, ID: br.ID}); err == nil {
		t.Fatal("RemoveArtifact accepted a detected id")
	}
	after := artifactsOf(t, app, f.taskID)
	if _, ok := findArtifact(after, "doc", "Design notes"); ok {
		t.Fatal("removed doc still listed")
	}
	if _, ok := findArtifact(after, "link", "Runbook"); !ok || len(after.Artifacts) != 7 {
		t.Fatalf("artifacts after add and remove = %+v", after.Artifacts)
	}

	// 5. A finished handover from an interactive session lists the artifacts too.
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	launch, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, launch)
	if text, list := mcpReply(t, app, "mcp-handover-1.json"); len(list.Artifacts) != 7 || !strings.Contains(text, "Runbook") {
		t.Fatalf("handover reply = %q, %+v", text, list)
	}
	if err := os.WriteFile(hold1, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, launch.SessionID, "stopped")

	// 6. A host that fails leaves a gap; the call still succeeds.
	app.Restart()
	forge.SetOffline(true)
	app.WindowMgr.OpenWindow(shell.WindowRecord{Key: sessionWindow})
	second, err := app.W.AdeTask.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatal(err)
	}
	openLaunch(t, app, second)
	_, list = mcpReply(t, app, "mcp-handover-2.json")
	if len(list.Gaps) != 2 || list.Gaps[0].Source != "host" || list.Gaps[0].Repo != "api" || list.Gaps[1].Source != "jira" {
		t.Fatalf("gaps = %+v, want a host gap for api and the jira gap", list.Gaps)
	}
	if err := os.WriteFile(hold2, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitSession(t, app, second.SessionID, "stopped")
	app.Contract(t, "ade-artifacts", "AdeTaskService.Artifacts#gap", artifactsOf(t, app, f.taskID), artMask)

	// 7. An archived task keeps its PR from the archive snapshot, no deployment, and takes no new row.
	forge.SetOffline(false)
	if err := app.W.AdeTask.ArchiveTask(ctx, adewire.TaskArgs{TaskID: f.taskID}); err != nil {
		t.Fatal(err)
	}
	app.Restart()
	forge.SetOffline(true)
	arch := artifactsOf(t, app, f.taskID)
	if a, ok := findArtifact(arch, "changeRequest", "Fix login"); !ok || a.Status != "open" || a.URL != artPRURL {
		t.Fatalf("archived PR = %+v in %+v", a, arch.Artifacts)
	}
	if _, ok := findArtifact(arch, "deployment", "prod"); ok {
		t.Fatal("archived task lists a deployment")
	}
	if _, ok := findArtifact(arch, "link", "Runbook"); !ok {
		t.Fatal("archived task lost its stored rows")
	}
	if _, err := app.W.AdeTask.AddLink(ctx, add); err == nil {
		t.Fatal("AddLink accepted an archived task")
	}
	app.Contract(t, "ade-artifacts", "AdeTaskService.Artifacts#archived", arch, artMask)
}
