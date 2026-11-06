package configsyncflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/configsync"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/internal/configfs"
)

const (
	scenario = "config-sync"
	manifest = "version: 1\nmodules: [connections]\n"
	wait     = 10 * time.Second
)

var ctx = context.Background()

func configPath(app *flowharness.App, rel string) string {
	return filepath.Join(app.ConfigDir("connections"), rel)
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func row(t *testing.T, p configsync.Preview, id string) configsync.Row {
	t.Helper()
	for _, r := range p.Rows {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("row %s not in preview %+v", id, p.Rows)
	return configsync.Row{}
}

func setup(t *testing.T, app *flowharness.App, files map[string]string) (*flowharness.Repo, configsync.Status) {
	t.Helper()
	repo := app.NewRepo("shared")
	files["kira-config.yaml"] = manifest
	repo.Commit("init", files)
	svc := app.W.ConfigSyncSvc
	st, err := svc.SetRepo(bridge.ConfigSyncSetRepoArgs{URL: "file://" + repo.Dir, Branch: "main", WindowKey: "w1"})
	if err != nil {
		t.Fatalf("SetRepo: %v", err)
	}
	if _, err := svc.SetMode(bridge.ConfigSyncModeArgs{Module: "connections", Mode: "shared"}); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	return repo, st
}

func TestConfigSync(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.ConfigSyncSvc
	repo, st := setup(t, app, map[string]string{
		"connections/team.yaml":  "name: team\nhost: db.example.com\n",
		"connections/extra.yaml": "name: extra\n",
	})
	head := repo.Git("rev-parse", "HEAD")
	if !st.Configured || st.Fetched != head || st.Changes != 2 || !st.Available || !st.ShowToast {
		t.Fatalf("status after SetRepo = %+v", st)
	}
	app.Contract(t, scenario, "ConfigSyncService.Status", st,
		flowharness.Omit("url", "fetched", "fetchedAt", "appliedAt"))
	if app.Events.Since(0, bridge.ChannelConfigSyncChanged) == nil {
		t.Fatal("no configSync event")
	}

	if got, err := svc.SetFetchMinutes(bridge.ConfigSyncMinutesArgs{Minutes: 15}); err != nil || got.FetchMinutes != 15 {
		t.Fatalf("SetFetchMinutes(15) = %+v, %v", got, err)
	}
	if _, err := svc.SetFetchMinutes(bridge.ConfigSyncMinutesArgs{Minutes: 3}); err == nil {
		t.Fatal("SetFetchMinutes(3) accepted, want rejection below the 5 minute floor")
	}

	// Review window: one window, the second call focuses it.
	if err := svc.OpenReview(); err != nil {
		t.Fatal(err)
	}
	if err := svc.OpenReview(); err != nil {
		t.Fatal(err)
	}
	if len(app.WindowMgr.OpenedKeyed) != 1 || !bridge.IsConfigSyncKey(app.WindowMgr.OpenedKeyed[0]) {
		t.Fatalf("keyed windows = %v", app.WindowMgr.OpenedKeyed)
	}
	key := app.WindowMgr.OpenedKeyed[0]
	if app.WindowMgr.Titles[key] != "Shared config changes" || len(app.WindowMgr.Focused) != 1 {
		t.Fatalf("title %q, focused %v", app.WindowMgr.Titles[key], app.WindowMgr.Focused)
	}
	if ok, _ := svc.IsReviewWindow(bridge.ConfigSyncWindowArgs{WindowKey: key}); !ok {
		t.Fatal("review key not recognised")
	}

	// Preview: two adds, diffs carry the text after the pull.
	pv, err := svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	app.Contract(t, scenario, "ConfigSyncService.Preview", pv, flowharness.Omit("token", "commit"))
	if row(t, pv, "connections/team.yaml").Kind != configsync.KindAdd || len(pv.Files) != 2 {
		t.Fatalf("preview = %+v", pv)
	}

	// Deactivate one unit; the next preview drops its pull.
	if _, err := svc.SetDeactivated(bridge.ConfigSyncDeactivateArgs{Module: "connections", Unit: "extra.yaml", Off: true}); err != nil {
		t.Fatal(err)
	}
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if r := row(t, pv, "connections/extra.yaml"); !r.Deactivated {
		t.Fatalf("extra row = %+v", r)
	}
	res, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	app.Contract(t, scenario, "ConfigSyncService.Apply", res, flowharness.Omit("backup"))
	if read(t, configPath(app, "team.yaml")) != "name: team\nhost: db.example.com\n" {
		t.Fatal("team.yaml not applied")
	}
	if _, err := os.Stat(configPath(app, "extra.yaml")); !os.IsNotExist(err) {
		t.Fatalf("deactivated unit applied: %v", err)
	}
	if st := mustStatus(t, svc); st.Applied != head || st.Changes != 0 {
		t.Fatalf("status after apply = %+v", st)
	}
	if _, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err == nil {
		t.Fatal("preview token reused")
	}

	// Conflict: both sides change team.yaml. Default keeps mine and files stay untouched.
	write(t, configPath(app, "team.yaml"), "name: team\nhost: mine.example.com\n")
	repo.Commit("shared host", map[string]string{"connections/team.yaml": "name: team\nhost: shared.example.com\n"})
	if _, err := svc.FetchNow(bridge.ConfigSyncFetchArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if row(t, pv, "connections/team.yaml").Kind != configsync.KindConflict {
		t.Fatalf("rows = %+v", pv.Rows)
	}
	res, err = svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token, Choices: map[string]string{"connections/team.yaml": configsync.ChoiceMine}})
	if err != nil || res.Kept != 1 {
		t.Fatalf("keep mine = %+v, %v", res, err)
	}
	if read(t, configPath(app, "team.yaml")) != "name: team\nhost: mine.example.com\n" {
		t.Fatal("keep mine changed the file")
	}
	// Keeping mine settles that shared change; the next shared change conflicts again.
	repo.Commit("shared host again", map[string]string{"connections/team.yaml": "name: team\nhost: shared2.example.com\n"})
	if _, err := svc.FetchNow(bridge.ConfigSyncFetchArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	if row(t, pv, "connections/team.yaml").Kind != configsync.KindConflict {
		t.Fatalf("rows = %+v", pv.Rows)
	}
	res, err = svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token, Choices: map[string]string{"connections/team.yaml": configsync.ChoiceShared}})
	if err != nil || res.Backup == "" {
		t.Fatalf("take shared = %+v, %v", res, err)
	}
	if read(t, configPath(app, "team.yaml")) != "name: team\nhost: shared2.example.com\n" {
		t.Fatal("take shared kept the local text")
	}
	if got := read(t, filepath.Join(res.Backup, "connections", "team.yaml")); !strings.Contains(got, "mine.example.com") {
		t.Fatalf("backup holds %q", got)
	}

	// Gate: a literal credential is blocked and never offered.
	repo.Commit("leak", map[string]string{"connections/leaky.yaml": "name: leaky\ntoken: sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789\n"})
	if _, err := svc.FetchNow(bridge.ConfigSyncFetchArgs{WindowKey: "w1"}); err != nil {
		t.Fatal(err)
	}
	pv, err = svc.Preview()
	if err != nil {
		t.Fatal(err)
	}
	blocked := row(t, pv, "connections/leaky.yaml")
	if blocked.Kind != configsync.KindBlocked || strings.Contains(blocked.Problem, "sk-ant") {
		t.Fatalf("blocked row = %+v", blocked)
	}
	app.Contract(t, scenario, "ConfigSyncService.Preview#blocked", blocked)
	if _, err := svc.Apply(bridge.ConfigSyncApplyArgs{Token: pv.Token}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath(app, "leaky.yaml")); !os.IsNotExist(err) {
		t.Fatalf("blocked unit applied: %v", err)
	}

	// Dismiss hides the toast for this commit.
	st = mustStatus(t, svc)
	if st, err = svc.Dismiss(bridge.ConfigSyncDismissArgs{Sha: st.Fetched}); err != nil || st.ShowToast {
		t.Fatalf("dismiss = %+v, %v", st, err)
	}
	if err := svc.CloseReview(); err != nil || len(app.WindowMgr.Closed) != 1 {
		t.Fatalf("close review: %v, closed %v", err, app.WindowMgr.Closed)
	}

	// Remove drops the sync data; applied files stay.
	if st, err = svc.RemoveRepo(); err != nil || st.Configured {
		t.Fatalf("remove = %+v, %v", st, err)
	}
	if _, err := os.Stat(configPath(app, "team.yaml")); err != nil {
		t.Fatalf("applied file removed: %v", err)
	}
}

func mustStatus(t *testing.T, svc *bridge.ConfigSyncService) configsync.Status {
	t.Helper()
	st, err := svc.Status()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// standalone builds a second service over its own home, with a Write seam to inject faults.
func standalone(t *testing.T, write func(path string, data []byte) error) (*configsync.Service, string, *flowharness.Repo) {
	t.Helper()
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	home := t.TempDir()
	repo := flowharness.NewRepo(t, filepath.Join(t.TempDir(), "shared"))
	repo.Commit("init", map[string]string{
		"kira-config.yaml":       manifest,
		"connections/one.yaml":   "name: one\n",
		"connections/two.yaml":   "name: two\n",
		"connections/three.yaml": "name: three\n",
	})
	svc := configsync.New(configsync.Options{
		Home:       home,
		Runner:     gitclient.NewExecRunner(),
		GitPath:    func(context.Context) (string, error) { return gitBin, nil },
		Write:      write,
		StartDelay: time.Hour,
	})
	t.Cleanup(svc.Stop)
	if _, err := svc.SetRepo(ctx, "file://"+repo.Dir, "main", ""); err != nil {
		t.Fatalf("SetRepo: %v", err)
	}
	if _, err := svc.SetMode("connections", "shared"); err != nil {
		t.Fatal(err)
	}
	return svc, home, repo
}

func journalPath(home string) string {
	return filepath.Join(home, "config", ".sync", "apply.json")
}

// A failed write rolls every file back and removes the journal.
func TestApplyRollsBackOnWriteFailure(t *testing.T) {
	writes := 0
	svc, home, _ := standalone(t, func(path string, data []byte) error {
		writes++
		if writes == 2 {
			return os.ErrPermission
		}
		return configfs.WriteAtomic(path, data, 0o644)
	})
	pv, err := svc.Preview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, pv.Token, nil); err == nil {
		t.Fatal("apply succeeded despite a failed write")
	}
	ents, _ := os.ReadDir(filepath.Join(home, "config", "connections"))
	if len(ents) != 0 {
		t.Fatalf("files left after rollback: %v", ents)
	}
	if _, err := os.Stat(journalPath(home)); !os.IsNotExist(err) {
		t.Fatalf("journal left: %v", err)
	}
	if st := svc.Status(); st.Applied != "" {
		t.Fatalf("applied = %q after rollback", st.Applied)
	}
}

// A crash between two writes leaves an applying journal; Recover rolls the files back.
func TestRecoverRollsBackInterruptedApply(t *testing.T) {
	writes := 0
	svc, home, _ := standalone(t, func(path string, data []byte) error {
		writes++
		if writes == 2 {
			runtime.Goexit()
		}
		return configfs.WriteAtomic(path, data, 0o644)
	})
	pv, err := svc.Preview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.Apply(ctx, pv.Token, nil)
	}()
	select {
	case <-done:
	case <-time.After(wait):
		t.Fatal("apply goroutine did not stop")
	}
	if _, err := os.Stat(journalPath(home)); err != nil {
		t.Fatalf("no journal after the interrupted apply: %v", err)
	}
	if err := configsync.Recover(home); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	ents, _ := os.ReadDir(filepath.Join(home, "config", "connections"))
	if len(ents) != 0 {
		t.Fatalf("files left after recovery: %v", ents)
	}
	if _, err := os.Stat(journalPath(home)); !os.IsNotExist(err) {
		t.Fatalf("journal left: %v", err)
	}
	if err := configsync.Recover(home); err != nil {
		t.Fatalf("second Recover: %v", err)
	}
}
