package grpcflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/grpcclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

func TestDescribeReflection(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	s := describe(t, app, describeArgs(srv.Addr))
	app.Contract(t, "api-grpc", "GrpcService.Describe", s, studioflowharness.Replace(srv.Addr, "<grpc>"))
	if s.Mode != "reflection-v1" {
		t.Errorf("mode = %q, want reflection-v1", s.Mode)
	}
	for _, svc := range s.Services {
		if strings.HasPrefix(svc.Name, "grpc.reflection") {
			t.Errorf("reflection service %s is listed", svc.Name)
		}
	}
	want := map[string][2]bool{ // name -> client, server streaming
		"Unary": {false, false}, "ServerStream": {false, true}, "ClientStream": {true, false},
		"Bidi": {true, true}, "Fail": {false, false}, "Slow": {false, false},
	}
	for name, flags := range want {
		m, ok := findMethod(t, s, name)
		if !ok {
			t.Errorf("method %s missing", name)
			continue
		}
		if m.ClientStreaming != flags[0] || m.ServerStreaming != flags[1] {
			t.Errorf("%s streaming = client %v server %v, want %v", name, m.ClientStreaming, m.ServerStreaming, flags)
		}
		if m.FullName != flowSvc+"."+name {
			t.Errorf("%s full name = %q", name, m.FullName)
		}
		field := "text"
		if name == "Fail" {
			field = "code"
		}
		if !strings.Contains(m.RequestTemplate, field) {
			t.Errorf("%s request template = %q, want the %q field", name, m.RequestTemplate, field)
		}
	}
	if len(srv.Calls()) != 0 {
		t.Errorf("describe reached the service: %+v", srv.Calls())
	}
	t.Run("server needs metadata", func(t *testing.T) { describeNeedsMetadata(t, app) })
}

func describeNeedsMetadata(t *testing.T, app *studioflowharness.App) {
	srv := studioflowharness.GRPC(t, studioflowharness.WithRequiredMetadata("authorization", "Bearer sek-reflect"))

	_, err := app.W.Grpc.Describe(ctx, describeArgs(srv.Addr))
	e := ipcErr(t, err)
	if !strings.Contains(strings.ToLower(e.Message), "unauthenticated") {
		t.Errorf("error = %s %q, want the status named", e.Code, e.Message)
	}

	colID, envID := secretVars(t, app, map[string]string{"authz": "Bearer sek-reflect"})
	args := describeArgs(srv.Addr)
	args.CollectionID, args.EnvironmentID = colID, envID
	args.Metadata = []grpcclient.MetaPair{{Name: "authorization", Value: "{{authz}}"}}
	s := describe(t, app, args)
	if _, ok := findMethod(t, s, "Unary"); !ok {
		t.Error("describe with the secret metadata lacks Unary")
	}

	unary := callArgs(srv.Addr, "Unary", `{}`)
	unary.CollectionID, unary.EnvironmentID, unary.Metadata = colID, envID, args.Metadata
	if res := call(t, app, unary); res.CodeName != "OK" || len(srv.Calls()) != 1 {
		t.Errorf("unary with the metadata = %s, %d server calls, want OK and the call recorded", res.CodeName, len(srv.Calls()))
	}
}

func TestDescribeProtoAndCall(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	// flow.proto in one dir, its import under a second dir reachable only through importPaths.
	mainDir, libDir := t.TempDir(), t.TempDir()
	flowSrc, err := os.ReadFile(filepath.Join(srv.ProtoDir, "flow.proto"))
	if err != nil {
		t.Fatal(err)
	}
	typesSrc, err := os.ReadFile(filepath.Join(srv.ProtoDir, "common", "types.proto"))
	if err != nil {
		t.Fatal(err)
	}
	protoPath := filepath.Join(mainDir, "flow.proto")
	if err := os.WriteFile(protoPath, flowSrc, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libDir, "common"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "common", "types.proto"), typesSrc, 0o600); err != nil {
		t.Fatal(err)
	}

	missing := describeArgsProto(protoPath, nil)
	_, err = app.W.Grpc.Describe(ctx, missing)
	e := ipcErr(t, err)
	if e.Code != grpcclient.CodeSchema || !strings.Contains(e.Message, "common/types.proto") {
		t.Errorf("missing import error = %s %q, want E_BAD_REQUEST naming common/types.proto", e.Code, e.Message)
	}

	byProto := describe(t, app, describeArgsProto(protoPath, []string{libDir}))
	byReflection := describe(t, app, describeArgs(srv.Addr))
	if byProto.Mode != "proto" {
		t.Errorf("mode = %q, want proto", byProto.Mode)
	}
	for _, name := range []string{"Unary", "ServerStream", "ClientStream", "Bidi", "Fail", "Slow"} {
		a, aok := findMethod(t, byProto, name)
		b, bok := findMethod(t, byReflection, name)
		if !aok || !bok || a != b {
			t.Errorf("%s differs between proto %+v and reflection %+v", name, a, b)
		}
	}

	args := callArgs(srv.Addr, "Unary", `{"text":"via proto","meta":{"origin":"x"}}`)
	args.DescriptorMode, args.ProtoPath, args.ImportPaths = "proto", protoPath, []string{libDir}
	res := call(t, app, args)
	if res.CodeName != "OK" || !strings.Contains(res.Messages[0].JSON, "via proto") {
		t.Errorf("proto-mode call = %s %+v", res.CodeName, res.Messages)
	}
	if got := srv.Calls(); len(got) != 1 || got[0].Method != "/"+flowSvc+"/Unary" {
		t.Errorf("server calls = %+v, want one Unary", got)
	}
}

func describeArgsProto(path string, importPaths []string) bridge.GrpcDescribeArgs {
	return bridge.GrpcDescribeArgs{DescriptorMode: "proto", ProtoPath: path, ImportPaths: importPaths}
}

func TestReloadAfterSchemaChange(t *testing.T) {
	app := studioflowharness.New(t)
	first := studioflowharness.GRPC(t)
	addr := first.Addr

	if _, ok := findMethod(t, describe(t, app, describeArgs(addr)), "Extra"); ok {
		t.Fatal("Extra present before the schema change")
	}
	first.Close()
	studioflowharness.GRPC(t, studioflowharness.WithExtraMethod(), studioflowharness.WithGRPCAddr(addr))

	if _, ok := findMethod(t, describe(t, app, describeArgs(addr)), "Extra"); ok {
		t.Error("plain describe saw the new method; want the cached schema")
	}
	reload := describeArgs(addr)
	reload.Reload = true
	if _, ok := findMethod(t, describe(t, app, reload), "Extra"); !ok {
		t.Error("reload did not return the new method")
	}
	if _, ok := findMethod(t, describe(t, app, describeArgs(addr)), "Extra"); !ok {
		t.Error("after reload, the cache still holds the old schema")
	}
}

// A server that accepts TCP and never answers fails within the dial bound, naming target and wait.
func TestDescribeSilentServer(t *testing.T) {
	app := studioflowharness.New(t)
	addr := studioflowharness.Silent(t)

	start := time.Now()
	_, err := app.W.Grpc.Describe(ctx, describeArgs(addr))
	took := time.Since(start)
	e := ipcErr(t, err)
	if e.Code != grpcclient.CodeTimeout || !strings.Contains(e.Message, addr) || !strings.Contains(e.Message, "5s") {
		t.Errorf("error = %s %q, want %s naming %s and the 5s wait", e.Code, e.Message, grpcclient.CodeTimeout, addr)
	}
	if took > 8*time.Second {
		t.Errorf("Describe took %s, want an error within 8s", took.Round(time.Second))
	}
}

func TestDescribeCancel(t *testing.T) {
	app := studioflowharness.New(t)
	addr, accepted := studioflowharness.SilentAccepting(t)
	args := describeArgs(addr)
	args.OpID = newOp()

	done := make(chan error, 1)
	go func() {
		_, err := app.W.Grpc.Describe(ctx, args)
		done <- err
	}()
	select {
	case <-accepted:
	case <-time.After(10 * time.Second):
		t.Fatal("Describe never connected to the silent server")
	}
	if err := app.W.StudioOps.Cancel(bridge.StudioOpsCancelArgs{OpID: args.OpID}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if e := ipcErr(t, err); e.Code != grpcclient.CodeCancelled {
			t.Errorf("error = %s %q, want %s", e.Code, e.Message, grpcclient.CodeCancelled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Describe did not return after Ops.Cancel")
	}
}

func TestCallSilentServer(t *testing.T) {
	app := studioflowharness.New(t)
	addr := studioflowharness.Silent(t)

	for _, m := range []string{"Unary", "ServerStream"} {
		args := callArgs(addr, m, `{"text":"x"}`)
		args.Streaming = m == "ServerStream"
		start := time.Now()
		_, err := app.W.Grpc.Call(ctx, args)
		e := ipcErr(t, err)
		if e.Code != grpcclient.CodeTimeout || !strings.Contains(e.Message, addr) {
			t.Errorf("%s error = %s %q, want %s naming %s", m, e.Code, e.Message, grpcclient.CodeTimeout, addr)
		}
		if took := time.Since(start); took > 8*time.Second {
			t.Errorf("%s took %s, want an error within 8s", m, took.Round(time.Second))
		}
	}
}

// A connected server that never replies ends a unary call at its own deadline, not at Stop.
func TestUnaryDeadline(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)
	args := callArgs(srv.Addr, "Slow", `{}`)
	args.TimeoutMs = 400

	start := time.Now()
	_, err := app.W.Grpc.Call(ctx, args)
	e := ipcErr(t, err)
	if e.Code != grpcclient.CodeTimeout || !strings.Contains(e.Message, srv.Addr) || !strings.Contains(e.Message, "400ms") {
		t.Errorf("error = %s %q, want %s naming %s and 400ms", e.Code, e.Message, grpcclient.CodeTimeout, srv.Addr)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("deadline call took %s, want about 400ms", took.Round(time.Millisecond))
	}
}
