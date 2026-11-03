package gitflow_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/internal/flowtest/notifysink"
	"github.com/kirathecat/kira-studio/internal/prompts"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// credentialEntry waits for the one git-credential popup and returns it.
func credentialEntry(t *testing.T, app *flowharness.App) prompts.Routed {
	t.Helper()
	var got prompts.Routed
	testx.WaitUntil(t, wait, func() bool {
		all, err := app.W.PromptsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range all {
			if p.Kind == prompts.KindGitCredential {
				got = p
				return true
			}
		}
		return false
	})
	return got
}

func noCredentialEntry(t *testing.T, app *flowharness.App) {
	t.Helper()
	testx.WaitUntil(t, wait, func() bool {
		all, err := app.W.PromptsSvc.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range all {
			if p.Kind == prompts.KindGitCredential {
				return false
			}
		}
		return true
	})
}

func TestCredentialRoutes(t *testing.T) {
	app := flowharness.New(t)
	sink := notifysink.New()
	app.W.Prompts.SetSink(sink)
	app.W.Windows.Add("w1", 0, nil, func() {})
	app.W.Windows.Add("w2", 1, nil, func() {})

	seed := app.NewRepo("seed")
	seed.Commit("base", map[string]string{"a.txt": "a\n"})
	bare := app.NewBare("remote")
	bare.PushFrom(seed)
	url, _ := smartHTTP(t, app.Work, "ana", "s3cret")
	local := bare.Clone(filepath.Join(app.Work, "local"))
	// The user name is in the URL, so git asks for the password only.
	local.Git("remote", "set-url", "origin", strings.Replace(url, "http://", "http://ana@", 1)+"/remote.git")
	rec, err := app.W.CodeWorkspace.ImportRepo(context.Background(), bridge.CodeWorkspaceImportArgs{Path: local.Dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.CreateTask(context.Background(), adewire.CreateTaskArgs{Title: "Fix login", CodeRepoIDs: []string{rec.ID}}); err != nil {
		t.Fatal(err)
	}

	// The ADE board has no window behind it: the popup goes to the main window.
	done := refreshOnce(app)
	e := credentialEntry(t, app)
	if e.Origin != "" || e.Target != "w1" || !strings.Contains(e.Title, "Git needs a credential") {
		t.Fatalf("board entry = %+v, want generic origin targeted at w1", e)
	}
	testx.WaitUntil(t, wait, func() bool { _, ok := sink.Shown("prompt:git-credential"); return ok })
	app.Contract(t, "git-credential-route", "PromptsService.List#credential", e, flowharness.Mask("createdAt", "id", "ref"))
	answerLogin(t, app, "ana", "s3cret")
	<-done
	noCredentialEntry(t, app)
	testx.WaitUntil(t, wait, func() bool { _, ok := sink.Shown("prompt:git-credential"); return !ok })

	// A native stream opened by window w2 raises the popup in w2, wherever the main window is.
	s := app.OpenGitStreamFor("w2")
	id := openOn(t, s, local.Dir).RepoID
	s.Fire("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp})
	e = credentialEntry(t, app)
	if e.Origin != "w2" || e.Target != "w2" {
		t.Fatalf("stream entry = %+v, want origin and target w2", e)
	}
	// With w2 gone the popup falls back to the main window.
	app.W.Windows.RemoveAndCount("w2")
	testx.WaitUntil(t, wait, func() bool { return credentialEntry(t, app).Target == "w1" })

	// Closing the connection mid-prompt withdraws the popup.
	s.Close()
	noCredentialEntry(t, app)
	testx.WaitUntil(t, wait, func() bool { return len(app.W.GitCredential.Pending()) == 0 })

	// An answered prompt closes its popup. The repository's op slot frees when the first connection
	// drops, so the fetch retries until it gets one.
	s2 := app.OpenGitStreamFor("w1")
	id = openOn(t, s2, local.Dir).RepoID
	res := make(chan gitsession.RemoteOpResult, 1)
	go func() {
		for {
			var got gitsession.RemoteOpResult
			_ = s2.Request("remote.run", gitrpc.RemoteRunParams{RepoID: id, RemoteOpParams: fetchOp}, &got)
			if got.Error == nil || got.Error.Kind != "OperationInProgress" {
				res <- got
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	e = credentialEntry(t, app)
	if e.Origin != "w1" {
		t.Fatalf("second stream entry = %+v", e)
	}
	answerLogin(t, app, "ana", "s3cret")
	select {
	case got := <-res:
		if got.Error != nil {
			t.Fatalf("answered fetch = %+v", got.Error)
		}
	case <-time.After(wait):
		t.Fatal("the answered fetch never finished")
	}
	noCredentialEntry(t, app)
}
