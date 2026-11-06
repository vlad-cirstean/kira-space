package httpflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

var (
	ctx = context.Background()
)

func ptr[T any](v T) *T { return &v }

func newOp() string { return ids.NewID() }

// get builds a GET send on the scratch tab.
func get(url string) bridge.HttpSendArgs {
	return bridge.HttpSendArgs{OpID: newOp(), TabID: "tab-1", Method: "GET", URL: url}
}

func send(t *testing.T, app *studioflowharness.App, args bridge.HttpSendArgs) httpclient.Response {
	t.Helper()
	res, err := app.W.Http.Send(ctx, args)
	if err != nil {
		t.Fatalf("Send %s %s: %v", args.Method, args.URL, err)
	}
	return res
}

func sendErr(t *testing.T, app *studioflowharness.App, args bridge.HttpSendArgs) *ipcerr.Error {
	t.Helper()
	_, err := app.W.Http.Send(ctx, args)
	if err == nil {
		t.Fatalf("Send %s %s: want an error, got none", args.Method, args.URL)
	}
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("Send error %T is not an ipcerr.Error: %v", err, err)
	}
	return ie
}

func setApi(t *testing.T, app *studioflowharness.App, patch model.ApiPatch) {
	t.Helper()
	if _, err := app.W.StudioSettings.Set(bridge.StudioSettingsSetArgs{Patch: model.SettingsPatch{Api: &patch}}); err != nil {
		t.Fatalf("Settings.Set: %v", err)
	}
}

// waitOp returns the op-log row of opID once it left "running".
func waitOp(t *testing.T, app *studioflowharness.App, opID string) model.OpRecord {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		recs, err := app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 200})
		if err != nil {
			t.Fatalf("Ops.Recent: %v", err)
		}
		for _, r := range recs {
			if r.ID == opID && r.Status != "running" {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("op %s did not finish; rows: %+v", opID, recs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// dbBytes is every byte of the app database, WAL included.
func dbBytes(t *testing.T, app *studioflowharness.App) []byte {
	t.Helper()
	var all []byte
	err := filepath.WalkDir(app.KiraHome, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if base := filepath.Base(p); len(base) >= 7 && base[:7] == "kira.db" {
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			all = append(all, b...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatalf("no kira.db files under %s", app.KiraHome)
	}
	return all
}

func mustNotContain(t *testing.T, what, haystack, needle string) {
	t.Helper()
	if bytes.Contains([]byte(haystack), []byte(needle)) {
		t.Errorf("%s contains %q", what, needle)
	}
}

// errTimeline decodes the failed-send timeline an ipcerr carries in Details.
func errTimeline(t *testing.T, e *ipcerr.Error) httpclient.Timeline {
	t.Helper()
	var tl httpclient.Timeline
	if err := json.Unmarshal(e.Details, &tl); err != nil {
		t.Fatalf("error details %q: %v", e.Details, err)
	}
	return tl
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

type flowharnessRequest = studioflowharness.Request

type errBox struct{ err error }

func ipcCode(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Code
	}
	return ""
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
