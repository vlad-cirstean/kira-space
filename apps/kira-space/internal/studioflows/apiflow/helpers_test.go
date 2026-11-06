package apiflow

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func newOp() string { return ids.NewID() }

func get(url string) bridge.HttpSendArgs {
	return bridge.HttpSendArgs{OpID: newOp(), TabID: "tab-1", Method: "GET", URL: url}
}

func send(t *testing.T, app *studioflowharness.App, args bridge.HttpSendArgs) httpclient.Response {
	t.Helper()
	res, err := app.W.Http.Send(ctx, args)
	if err != nil {
		t.Fatalf("Send %s: %v", args.URL, err)
	}
	return res
}

func ipcErr(t *testing.T, err error) *ipcerr.Error {
	t.Helper()
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("want an ipcerr.Error, got %T: %v", err, err)
	}
	return ie
}

func newCollection(t *testing.T, app *studioflowharness.App, name string) model.Collection {
	t.Helper()
	c, err := app.W.Collections.CreateCollection(bridge.CollectionsCreateCollectionArgs{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func savedGet() model.SavedRequest {
	return model.SavedRequest{Method: "GET", BodyMode: "none", CodeLanguage: "json"}
}

func waitOp(t *testing.T, app *studioflowharness.App, opID string) model.OpRecord {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		recs, err := app.W.StudioOps.Recent(bridge.StudioOpsRecentArgs{Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recs {
			if r.ID == opID && r.Status != "running" {
				return r
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("op %s did not finish", opID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitOpUpdate waits for the op-log update event that closes opID; it fires for incognito ops too.
func waitOpUpdate(t *testing.T, app *studioflowharness.App, opID string) {
	t.Helper()
	app.Events.Wait(t, bridge.ChannelStudioOpUpdate, func(e studioflowharness.Event) bool {
		var rec model.OpRecord
		e.Decode(t, &rec)
		return rec.ID == opID && rec.Status != "running"
	}, 10*time.Second)
}

func dbBytes(t *testing.T, app *studioflowharness.App) []byte {
	t.Helper()
	var all []byte
	err := filepath.WalkDir(app.KiraHome, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasPrefix(filepath.Base(p), "kira.db") {
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

func contains(haystack []byte, needle string) bool { return bytes.Contains(haystack, []byte(needle)) }

// apiChanges decodes every kira:api:dataChanged event after mark into its change lists.
func apiChanges(t *testing.T, app *studioflowharness.App, mark int) [][]bridge.ApiDataChange {
	t.Helper()
	var out [][]bridge.ApiDataChange
	for _, ev := range app.Events.Since(mark, bridge.ChannelApiDataChanged) {
		var d bridge.ApiDataChanged
		ev.Decode(t, &d)
		out = append(out, d.Changes)
	}
	return out
}

func hasChange(changes []bridge.ApiDataChange, c bridge.ApiDataChange) bool {
	for _, x := range changes {
		if x == c {
			return true
		}
	}
	return false
}

func lastRequest(srv *studioflowharness.HTTPServer) studioflowharness.Request {
	reqs := srv.Requests()
	return reqs[len(reqs)-1]
}
