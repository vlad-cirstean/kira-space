package gitflow_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpreflight"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func hitShas(res gitrpc.SearchRunResult) []string {
	var out []string
	for _, h := range res.Hits {
		out = append(out, h.SHA)
	}
	sort.Strings(out)
	return out
}

func sortedLines(s string) []string {
	out := lines(s)
	sort.Strings(out)
	return out
}

// Guards search.run against git log --grep/--author on a 2500-commit history.
func TestSearchHistory(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewHistory("hist", flowharness.HistorySpec{Commits: 2500, Branches: 8, MergeEvery: 5})
	repo.Git("commit", "-q", "--allow-empty", "--author=Zed Zebra <zed@example.org>", "-m", "needle in subject", "-m", "haystack in body")
	repo.Git("commit", "-q", "--allow-empty", "--author=Zed Zebra <zed@example.org>", "-m", "second zed work")
	repo.Git("commit", "-q", "--allow-empty", "-m", "plain subject", "-m", "another needle in body")
	id := r.open(repo.Dir).RepoID

	search := func(text string) gitrpc.SearchRunResult {
		res := call[gitrpc.SearchRunResult](t, r.gs, "search.run", gitrpc.SearchRunParams{RepoID: id, Query: gitrpc.SearchQueryParams{Text: text}})
		if res.Kind != "ok" || !res.Complete || res.Truncated {
			t.Fatalf("search %q = kind %q complete %v truncated %v", text, res.Kind, res.Complete, res.Truncated)
		}
		return res
	}
	oracle := func(args ...string) []string {
		return sortedLines(repo.Git(append([]string{"log", "--all", "--format=%H", "-i"}, args...)...))
	}
	equal := func(t *testing.T, got, want []string) {
		t.Helper()
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("hits differ from git log: got %d, want %d", len(got), len(want))
		}
	}

	t.Run("message", func(t *testing.T) {
		equal(t, hitShas(search("commit 12")), oracle("--grep=commit 12"))
		equal(t, hitShas(search("needle")), oracle("--grep=needle"))
	})
	t.Run("author", func(t *testing.T) {
		got := search("zed zebra")
		equal(t, hitShas(got), oracle("--author=Zed Zebra"))
		if got.Total != 2 {
			t.Fatalf("author search total = %d, want 2", got.Total)
		}
		equal(t, hitShas(search("zed@example.org")), oracle("--author=zed@example.org"))
	})
	t.Run("sha prefix", func(t *testing.T) {
		want := repo.RevList("--all")[1234]
		got := search(want[:10])
		if len(got.Hits) != 1 || got.Hits[0].SHA != want {
			t.Fatalf("sha search = %v, want exactly %s", hitShas(got), want)
		}
		if !contains(got.Hits[0].Fields, "sha") {
			t.Fatalf("sha hit fields = %v", got.Hits[0].Fields)
		}
	})
	t.Run("no hits", func(t *testing.T) {
		if got := search("no such text anywhere"); got.Total != 0 {
			t.Fatalf("total = %d, want 0", got.Total)
		}
	})
}

func TestRepoSettingsAffectStream(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewHistory("hist", flowharness.HistorySpec{Commits: 450, Branches: 2})
	id := r.open(repo.Dir).RepoID

	firstPage := func(gs *flowharness.GitStream) int {
		n := 0
		for _, c := range gs.Stream("graph.stream", m{"repoId": id}, 1).Drain() {
			_, rows := c.Graph(t)
			n += len(rows)
		}
		return n
	}
	if got := firstPage(r.gs); got != 450 {
		t.Fatalf("default page size loads %d rows of 450", got)
	}
	size := 150
	snap := r.setSettings(id, gitrpc.RepoSettingsPatchWire{GraphPageSize: &size})
	if snap.GraphPageSize != 150 {
		t.Fatalf("settings snapshot = %+v", snap)
	}
	fresh := r.app.OpenGitStream()
	openOn(t, fresh, repo.Dir)
	if got := firstPage(fresh); got != 150 {
		t.Fatalf("first page after the setting = %d rows, want 150", got)
	}
	r.gs.WaitEvent("repoSettings.changed", nil, wait)

	script, base := "echo hi", "/tmp/elsewhere"
	for _, patch := range []gitrpc.RepoSettingsPatchWire{{WorktreePrepareScript: &script}, {WorktreeBasePath: &base}} {
		if we := wireErr(t, r.gs, "repoSettings.set", gitrpc.RepoSettingsSetParams{RepoID: id, Patch: patch}); we.Code != "E_READ_ONLY" {
			t.Fatalf("restricted patch %+v code = %q, want E_READ_ONLY", patch, we.Code)
		}
	}
	tiny := 5
	if we := wireErr(t, r.gs, "repoSettings.set", gitrpc.RepoSettingsSetParams{RepoID: id, Patch: gitrpc.RepoSettingsPatchWire{GraphPageSize: &tiny}}); we.Code == "" {
		t.Fatal("out-of-range page size accepted")
	}
	if got := call[gitrpc.RepoSettingsSnapshot](t, r.gs, "repoSettings.get", gitrpc.RepoSettingsGetParams{RepoID: id}); got.GraphPageSize != 150 || got.WorktreePrepareScript != "" {
		t.Fatalf("settings after refused patches = %+v", got)
	}
}

func TestRepoSettingsLiveInFile(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("filed")
	repo.Commit("init", map[string]string{"a.txt": "a\n"})
	id := r.open(repo.Dir).RepoID
	path := filepath.Join(r.app.SpaceHome, "repos", "filed.yaml")
	read := func() string {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	size := 150
	r.setSettings(id, gitrpc.RepoSettingsPatchWire{GraphPageSize: &size})
	if text := read(); !strings.Contains(text, "page_size: 150") || !strings.Contains(text, "path: "+id) {
		t.Fatalf("file after the first patch:\n%s", text)
	}

	if err := os.WriteFile(path, []byte(read()+"# kept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	on := true
	r.setSettings(id, gitrpc.RepoSettingsPatchWire{StashIncludeUntracked: &on})
	if text := read(); !strings.Contains(text, "# kept") || !strings.Contains(text, "include_untracked: true") {
		t.Fatalf("file after the second patch lost the comment or the leaf:\n%s", text)
	}

	edited := strings.Replace(read(), "page_size: 150", "page_size: 300", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	r.gs.WaitEvent("repoSettings.changed", func(raw json.RawMessage) bool {
		var p gitrpc.RepoSettingsChangedPayload
		return json.Unmarshal(raw, &p) == nil && p.RepoID == id && p.Settings.GraphPageSize == 300
	}, wait)

	mark := r.app.Events.Mark()
	if err := os.WriteFile(path, []byte("path: "+id+"\nbogus: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.app.Events.WaitAfter(t, mark, bridge.ChannelRepoConfigChanged, nil, wait)
	next := 200
	if we := wireErr(t, r.gs, "repoSettings.set", gitrpc.RepoSettingsSetParams{RepoID: id, Patch: gitrpc.RepoSettingsPatchWire{GraphPageSize: &next}}); we.Code == "" {
		t.Fatal("repoSettings.set wrote into a broken file")
	}
	if got := call[gitrpc.RepoSettingsSnapshot](t, r.gs, "repoSettings.get", gitrpc.RepoSettingsGetParams{RepoID: id}); got.GraphPageSize != 300 {
		t.Fatalf("settings with a broken file = %+v, want the last good page size 300", got)
	}
}

func TestReviewSession(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	repo.Commit("base", map[string]string{"keep.txt": "keep\n", "edit.txt": "one\ntwo\nthree\n"})
	repo.Git("checkout", "-q", "-b", "feat")
	repo.Commit("add a", map[string]string{"a.txt": "a1\na2\na3\n"})
	repo.Commit("edit", map[string]string{"edit.txt": "one\nTWO\nthree\n"})
	tip := repo.Commit("add b", map[string]string{"dir/b.txt": "b\n"})
	repo.Checkout("main")
	id := r.open(repo.Dir).RepoID

	base := call[gitreview.BaseResolution](t, r.gs, "review.resolveBase", gitrpc.ReviewResolveBaseParams{RepoID: id, Branch: "feat"})
	if base.Base == nil || *base.Base != "main" || base.Range.Kind != "ready" || base.Range.CommitCount == nil || *base.Range.CommitCount != 3 {
		t.Fatalf("review.resolveBase = %+v, want main with 3 commits ready", base)
	}
	files := call[gitsession.RangeFilesResult](t, r.gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"})
	var got []string
	for _, f := range files.Files {
		got = append(got, f.Change.Path)
	}
	sort.Strings(got)
	if want := sortedLines(repo.Git("diff", "--name-only", "main...feat")); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("review.files = %v, git diff main...feat: %v", got, want)
	}
	if files.BranchTip != tip || files.MergeBase != repo.Git("merge-base", "main", "feat") {
		t.Fatalf("review.files tip/mergeBase = %s/%s", files.BranchTip, files.MergeBase)
	}

	mark := call[gitrpc.ReviewMarkResult](t, r.gs, "review.mark", gitrpc.ReviewMarkParams{RepoID: id, Branch: "feat", Path: "a.txt", Reviewed: true})
	if mark.Review.Kind != "full" {
		t.Fatalf("review.mark = %+v, want full", mark.Review)
	}
	files = call[gitsession.RangeFilesResult](t, r.gs, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"})
	for _, f := range files.Files {
		if want := f.Change.Path == "a.txt"; (f.Review.Kind == "full") != want {
			t.Fatalf("%s review kind = %q", f.Change.Path, f.Review.Kind)
		}
	}

	add := call[gitrpc.ReviewCommentAddResult](t, r.gs, "review.comment.add", gitrpc.ReviewCommentAddParams{
		RepoID: id, Branch: "feat", Path: "a.txt", At: tip, Range: gitreview.LineRange{Start: 2, End: 3}, Body: "why a2?",
	})
	if add.Comment.ID == 0 || add.Comment.Anchor != gitreview.AnchorExact {
		t.Fatalf("comment add = %+v", add.Comment)
	}
	export := call[gitrpc.ReviewCommentExportResult](t, r.gs, "review.comment.export", gitrpc.ReviewCommentExportParams{RepoID: id, Branch: "feat"})
	if !strings.Contains(export.Text, "why a2?") || !strings.Contains(export.Text, "a.txt") {
		t.Fatalf("export = %q", export.Text)
	}

	// A new connection is a window reopened: the review database holds the comment and the mark.
	r.gs.Close()
	again := r.app.OpenGitStream()
	id = openOn(t, again, repo.Dir).RepoID
	list := call[gitsession.CommentListResult](t, again, "review.comment.list", gitrpc.ReviewCommentListParams{RepoID: id, Branch: "feat"})
	if len(list.Comments) != 1 || list.Comments[0].Body != "why a2?" || list.Comments[0].ID != add.Comment.ID {
		t.Fatalf("comments after reopen = %+v", list.Comments)
	}
	files = call[gitsession.RangeFilesResult](t, again, "review.files", gitrpc.ReviewFilesParams{RepoID: id, Branch: "feat", Base: "main"})
	reviewed := 0
	for _, f := range files.Files {
		if f.Review.Kind == "full" {
			reviewed++
		}
	}
	if reviewed != 1 {
		t.Fatalf("reviewed files after reopen = %d, want 1", reviewed)
	}
	rm := call[gitrpc.ReviewCommentRemoveResult](t, again, "review.comment.remove", gitrpc.ReviewCommentRemoveParams{RepoID: id, Branch: "feat", ID: add.Comment.ID})
	if !rm.Removed {
		t.Fatal("comment remove reported nothing removed")
	}
	if list = call[gitsession.CommentListResult](t, again, "review.comment.list", gitrpc.ReviewCommentListParams{RepoID: id, Branch: "feat"}); len(list.Comments) != 0 {
		t.Fatalf("comments after remove = %+v", list.Comments)
	}
}

// Guards the no-remote path: preflights answer with data, not errors, and PR lookups stay quiet.
func TestNoRemoteAndPr(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewRepo("proj")
	sha := repo.Commit("first", map[string]string{"a.txt": "a\n"})
	id := r.open(repo.Dir).RepoID

	pull := call[m](t, r.gs, "remote.pullPreflight", gitrpc.RemotePullPreflightParams{RepoID: id, Branch: "main"})
	if pull["upstream"] != nil {
		t.Fatalf("pull preflight upstream = %v, want none", pull["upstream"])
	}
	push := call[m](t, r.gs, "remote.pushPreflight", gitrpc.RemotePushPreflightParams{RepoID: id, Branch: "main", Remote: "origin"})
	if push["upstream"] != nil || push["wouldSetUpstream"] != true {
		t.Fatalf("push preflight = %v, want no upstream", push)
	}
	if res := r.remote(id, gitsession.RemoteOpParams{Kind: "fetch", Remote: "origin"}); res.OK || res.Error == nil {
		t.Fatalf("fetch without a remote = %+v, want a classified error", res)
	}

	for name, res := range map[string]gitrpc.ChangeRequestLookup{
		"commit": call[gitrpc.ChangeRequestLookup](t, r.gs, "commit.resolveChangeRequests", gitrpc.CommitResolveChangeRequestsParams{RepoID: id, SHA: sha}),
		"branch": call[gitrpc.ChangeRequestLookup](t, r.gs, "branch.resolveChangeRequest", gitrpc.BranchResolveChangeRequestParams{RepoID: id, Branch: "main"}),
	} {
		if len(res.ChangeRequests) != 0 {
			t.Fatalf("%s.resolveChangeRequests = %+v, want no PRs", name, res)
		}
	}

	if info := call[gitrpc.GitHostInfoResult](t, r.gs, "gitHost.info", gitrpc.GitHostInfoParams{RepoID: id}); info.Host != nil {
		t.Fatalf("gitHost.info without a remote = %+v, want null", info.Host)
	}
	if p := call[githost.Providers](t, r.gs, "gitHost.providers", struct{}{}); len(p.Providers) != 2 || p.Providers[0].ID != "github" || p.Providers[1].ID != "azure" {
		t.Fatalf("gitHost.providers = %+v, want github, azure", p)
	}

	repo.AddRemote("https://gitlab.example.com/team/proj.git")
	url := call[gitrpc.ChangeRequestBrowserUrlResult](t, r.gs, "changeRequest.browserUrl", gitrpc.ChangeRequestBrowserUrlParams{RepoID: id, Number: 7})
	if url.URL != nil {
		t.Fatalf("changeRequest.browserUrl for a non-GitHub remote = %q, want null", *url.URL)
	}
}

// Two windows on one repo: closing one leaves the other streaming, and both heard the change.
func TestTwoConnectionsShareRepo(t *testing.T) {
	r := newRig(t)
	repo := r.app.NewHistory("hist", flowharness.HistorySpec{Commits: 40, Branches: 2})
	idA := r.open(repo.Dir).RepoID
	second := r.app.OpenGitStream()
	idB := openOn(t, second, repo.Dir).RepoID
	if idA != idB {
		t.Fatalf("repo ids differ: %s vs %s", idA, idB)
	}

	r.external(func() { repo.Commit("while two are open", map[string]string{"n.txt": "n\n"}) })
	testx.WaitUntil(t, wait, func() bool { return len(second.Events("repo.changed")) > 0 })

	if err := r.gs.Request("repo.close", gitrpc.RepoCloseParams{RepoID: idA}, nil); err != nil {
		t.Fatalf("repo.close: %v", err)
	}
	var rows int
	for _, c := range second.Stream("graph.stream", m{"repoId": idB}, 1).Drain() {
		_, got := c.Graph(t)
		rows += len(got)
	}
	if rows != 41 {
		t.Fatalf("second connection streams %d rows after the first closed, want 41", rows)
	}
	if st := call[gitpreflight.StatusSummary](t, second, "status.get", gitrpc.StatusGetParams{RepoID: idB}); st.Head.Name != "main" {
		t.Fatalf("status on the surviving connection = %+v", st.Head)
	}
}
