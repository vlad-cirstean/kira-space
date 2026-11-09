package configsyncflow_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

// A repo imported by the watch-folder rescan must reach the repos module without a manual fetch (P585).
func TestConfigSyncReposWatchFolderRescan(t *testing.T) {
	app := flowharness.New(t)
	svc := app.W.ConfigSyncSvc

	shared := app.NewRepo("shared")
	shared.Commit("init", map[string]string{
		"kira-config.yaml":    "version: 1\nmodules: [repos]\n",
		"repos/acme-app.yaml": acmeShared,
	})
	if _, err := svc.SetRepo(bridge.ConfigSyncSetRepoArgs{URL: "file://" + shared.Dir, Branch: "main", WindowKey: "w1"}); err != nil {
		t.Fatalf("SetRepo: %v", err)
	}
	if st, err := svc.FetchNow(bridge.ConfigSyncFetchArgs{WindowKey: "w1"}); err != nil || st.Changes != 0 {
		t.Fatalf("before import: changes=%d err=%v, want 0 (no matching repo)", st.Changes, err)
	}

	folder := filepath.Join(app.Work, "folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.W.AdeTask.AddFolder(ctx, adewire.FolderArgs{Path: folder, Watch: true}); err != nil {
		t.Fatal(err)
	}
	r := flowharness.NewRepo(t, filepath.Join(folder, "app"))
	r.Commit("init", map[string]string{"a.txt": "a\n"})
	r.AddRemote("git@example.com:acme/app.git")

	deadline := time.Now().Add(15 * time.Second)
	for {
		if st := svc.Sync.Status(); st.Changes > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("changes still 0 after the watched folder imported a repo matching the shared origin")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
