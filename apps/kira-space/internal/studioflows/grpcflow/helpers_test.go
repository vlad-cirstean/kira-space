package grpcflow

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
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/grpcclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

const (
	flowSvc = "kira.flow.v1.Flow"
	window  = "w1"
)

var (
	ctx = context.Background()
)

func newOp() string { return ids.NewID() }

func describeArgs(addr string) bridge.GrpcDescribeArgs {
	return bridge.GrpcDescribeArgs{DescriptorMode: "reflection", Target: addr}
}

func callArgs(addr, method, msg string) bridge.GrpcCallArgs {
	return bridge.GrpcCallArgs{
		OpID: newOp(), TabID: "tab-1", WindowKey: window, DescriptorMode: "reflection",
		Target: addr, Service: flowSvc, Method: method, MessageJSON: msg,
	}
}

func call(t *testing.T, app *studioflowharness.App, args bridge.GrpcCallArgs) grpcclient.CallResult {
	t.Helper()
	res, err := app.W.Grpc.Call(ctx, args)
	if err != nil {
		t.Fatalf("Call %s: %v", args.Method, err)
	}
	return res
}

func ipcErr(t *testing.T, err error) *ipcerr.Error {
	t.Helper()
	if err == nil {
		t.Fatal("want an error, got none")
	}
	var ie *ipcerr.Error
	if !errors.As(err, &ie) {
		t.Fatalf("error %T is not an ipcerr.Error: %v", err, err)
	}
	return ie
}

func describe(t *testing.T, app *studioflowharness.App, args bridge.GrpcDescribeArgs) grpcclient.Schema {
	t.Helper()
	s, err := app.W.Grpc.Describe(ctx, args)
	if err != nil {
		t.Fatalf("Describe %s: %v", args.Target, err)
	}
	return s
}

func findMethod(t *testing.T, s grpcclient.Schema, name string) (grpcclient.Method, bool) {
	t.Helper()
	for _, svc := range s.Services {
		if svc.Name != "Flow" {
			continue
		}
		if svc.FullName != flowSvc {
			t.Fatalf("service FullName = %q, want %q (the UI calls by it)", svc.FullName, flowSvc)
		}
		for _, m := range svc.Methods {
			if m.Name == name {
				return m, true
			}
		}
	}
	return grpcclient.Method{}, false
}

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
			t.Fatalf("op %s did not finish", opID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// callEvents decodes every kira:grpc:call event recorded after mark.
func callEvents(t *testing.T, app *studioflowharness.App, mark int) []bridge.GrpcCallEvent {
	t.Helper()
	var out []bridge.GrpcCallEvent
	for _, ev := range app.Events.Since(mark, bridge.ChannelGrpcCall) {
		if ev.Window != window {
			t.Errorf("grpc event aimed at window %q, want %q", ev.Window, window)
		}
		var d bridge.GrpcCallEvent
		ev.Decode(t, &d)
		out = append(out, d)
	}
	return out
}

func history(t *testing.T, app *studioflowharness.App, tab string) []model.GrpcCallHistoryEntry {
	t.Helper()
	rows, err := app.W.GrpcHistory.List(bridge.GrpcHistoryScopeArgs{TabID: tab})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func pairValue(pairs []grpcclient.MetaPair, name string) string {
	for _, p := range pairs {
		if p.Name == name {
			return p.Value
		}
	}
	return ""
}

func dbBytes(t *testing.T, app *studioflowharness.App) []byte {
	t.Helper()
	var all []byte
	err := filepath.WalkDir(app.KiraHome, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if bytes.HasPrefix([]byte(filepath.Base(p)), []byte("kira.db")) {
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

func secretVars(t *testing.T, app *studioflowharness.App, vars map[string]string) (collectionID, envID string) {
	t.Helper()
	col := app.SeedCollection(t, "grpc")
	seeded := make([]studioflowharness.APIVar, 0, len(vars))
	for name, value := range vars {
		seeded = append(seeded, studioflowharness.APIVar{Name: name, Value: value, Secret: true})
	}
	env := app.SeedEnvironment(t, "dev", seeded...)
	return col.ID, env.ID
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
