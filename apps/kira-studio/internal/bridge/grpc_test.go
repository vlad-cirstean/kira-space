package bridge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apivars"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/grpcclient"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// TestMaskGrpcError_MasksMessageAndPartial is the unit-level assertion, mirroring
// bridge/http_test.go's TestMaskSendErrTimeline_MasksFailedSendHopURL: a *grpcclient.Error's own
// Message — and, for a partial stream result, its StatusMessage/Header/Trailer values — must be
// masked back to {{name}} form before mapGrpcError ever turns it into ipcerr.Error.Details, a
// copyable surface. The full persistence-round-trip case (through a real migrated SQLite
// database, mirroring TestMaskSecrets_RedirectURLsFinalURLAndTimelineHopsBeforePersisting) is
// grpc_history_test.go, once grpc_call_history exists to persist into.
func TestMaskGrpcError_MasksMessageAndPartial(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	const masked = "{{apiToken}}"
	used := []apivars.UsedSecret{{Name: "apiToken", Rendered: secret, Placeholder: "{{apiToken}}"}}

	err := grpcclient.Transport("dial tcp: bearer " + secret + " rejected")
	err.Partial = &grpcclient.CallResult{
		StatusMessage: "denied for token " + secret,
		Header:        []grpcclient.MetaPair{{Name: "x-upstream", Value: "seen " + secret}},
		Trailer:       []grpcclient.MetaPair{{Name: "x-detail", Value: "retry with " + secret}},
	}

	maskGrpcError(err, used)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, secret) {
			t.Errorf("%s = %q still contains the raw secret", label, s)
		}
		if !strings.Contains(s, masked) {
			t.Errorf("%s = %q, want it to contain %q", label, s, masked)
		}
	}
	assertMasked(t, "Message", err.Message)
	assertMasked(t, "Partial.StatusMessage", err.Partial.StatusMessage)
	assertMasked(t, "Partial.Header[0].Value", err.Partial.Header[0].Value)
	assertMasked(t, "Partial.Trailer[0].Value", err.Partial.Trailer[0].Value)
}

// TestMaskGrpcError_NoOpWithNothingUsed confirms maskGrpcError never touches an error when no
// secret was actually substituted — the same "a no-op when there is no rendered exchange or no
// secret was actually substituted" contract maskSecrets (bridge/http.go) documents.
func TestMaskGrpcError_NoOpWithNothingUsed(t *testing.T) {
	err := grpcclient.BadRequest("plain message, nothing resolved")
	maskGrpcError(err, nil)
	if err.Message != "plain message, nothing resolved" {
		t.Errorf("Message = %q, want unchanged", err.Message)
	}
}

// TestMaskGrpcResult_MasksStatusMessageAndMetadata is maskGrpcError's own success-path sibling:
// a *successful* call's
// StatusMessage/Header/Trailer must be masked exactly as an error's own fields are — this test
// fails against the pre-fix code, which called no masker at all on the success path, so a
// substituted secret echoed back by the server (a gateway reflecting a header, a status message
// quoting the credential it rejected on an otherwise-OK transport) reached both the renderer and
// grpc_call_history.snapshot_json unmasked.
func TestMaskGrpcResult_MasksStatusMessageAndMetadata(t *testing.T) {
	const secret = "sk_live_super_secret_token"
	const masked = "{{apiToken}}"
	used := []apivars.UsedSecret{{Name: "apiToken", Rendered: secret, Placeholder: "{{apiToken}}"}}

	result := grpcclient.CallResult{
		StatusMessage: "accepted token " + secret,
		Header:        []grpcclient.MetaPair{{Name: "x-echo", Value: "saw " + secret}},
		Trailer:       []grpcclient.MetaPair{{Name: "x-detail", Value: "used " + secret}},
	}

	maskGrpcResult(&result, used)

	assertMasked := func(t *testing.T, label, s string) {
		t.Helper()
		if strings.Contains(s, secret) {
			t.Errorf("%s = %q still contains the raw secret", label, s)
		}
		if !strings.Contains(s, masked) {
			t.Errorf("%s = %q, want it to contain %q", label, s, masked)
		}
	}
	assertMasked(t, "StatusMessage", result.StatusMessage)
	assertMasked(t, "Header[0].Value", result.Header[0].Value)
	assertMasked(t, "Trailer[0].Value", result.Trailer[0].Value)
}

// TestMaskGrpcResult_NoOpWithNothingUsed mirrors TestMaskGrpcError_NoOpWithNothingUsed for the
// success-path masker.
func TestMaskGrpcResult_NoOpWithNothingUsed(t *testing.T) {
	result := grpcclient.CallResult{StatusMessage: "plain, nothing resolved"}
	maskGrpcResult(&result, nil)
	if result.StatusMessage != "plain, nothing resolved" {
		t.Errorf("StatusMessage = %q, want unchanged", result.StatusMessage)
	}
}

// TestGrpcHasAnyReference is the short-circuit's own small table — the same "is there anything to
// resolve at all" gate apivars' own referencedFields walk exists for.
func TestGrpcHasAnyReference(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		metadata []grpcclient.MetaPair
		message  string
		want     bool
	}{
		{name: "nothing", target: "api.example.com:443", message: `{"a":1}`, want: false},
		{name: "target reference", target: "{{host}}:443", want: true},
		{name: "metadata name reference", metadata: []grpcclient.MetaPair{{Name: "{{headerName}}", Value: "v"}}, want: true},
		{name: "metadata value reference", metadata: []grpcclient.MetaPair{{Name: "authorization", Value: "Bearer {{token}}"}}, want: true},
		{name: "message reference", message: `{"token":"{{token}}"}`, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := grpcHasAnyReference(c.target, c.metadata, c.message)
			if got != c.want {
				t.Errorf("grpcHasAnyReference(%q, %+v, %q) = %v, want %v", c.target, c.metadata, c.message, got, c.want)
			}
		})
	}
}

// strPtr is repos_test's own helper (storage/repos/ops_test.go), restated here — F2's three-state
// VariablesRepo.Upsert (P108 Part 3) needs it for every secret seeded below.
func strPtr(s string) *string { return &s }

// newGrpcServiceForTest opens a real (tmpfile-backed) SQLite database through the real migrations
// and wires a *GrpcService against it — mirrors apivars/resolve_test.go's own newResolveService
// and bridge/http_test.go's own newRepos-backed setup.
func newGrpcServiceForTest(t *testing.T) (*GrpcService, *storage.DB, *repos.VariablesRepo, *repos.CollectionsRepo) {
	t.Helper()
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	t.Setenv("KIRA_HOME", t.TempDir())
	db, err := storage.Open()
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repositories, err := repos.New(db.DB)
	if err != nil {
		t.Fatalf("repos.New: %v", err)
	}
	cipher := secrets.New()
	variablesRepo := repos.NewVariables(db.DB, cipher)
	repositories.Variables = variablesRepo
	apiVars := apivars.New(variablesRepo, nil)

	svc := &GrpcService{Deps: appcore.Deps{DB: db.DB, Repos: repositories, ApiVars: apiVars}}
	return svc, db, variablesRepo, repositories.Collections
}

// TestRecordGrpcHistory_NeverPersistsResolvedSecrets is the masking checklist's own
// persistence-round-trip case, mirroring bridge/http_test.go's
// TestMaskSecrets_RedirectURLsFinalURLAndTimelineHopsBeforePersisting: a call's target, metadata
// and message JSON can each carry a resolved secret at the point grpcclient.Unary/ServerStream
// use them (the "the reflection call... and the descriptor cache key" checklist rows apply the
// identical discipline one call earlier, in-memory only, with nothing to persist to round-trip
// in the first place) — but recordGrpcHistory is handed the UNRESOLVED args, so what reaches
// grpc_call_history, read back as a raw column straight out of a real migrated SQLite database,
// must still be spelled {{name}}, never the live value.
func TestRecordGrpcHistory_NeverPersistsResolvedSecrets(t *testing.T) {
	svc, db, variablesRepo, collections := newGrpcServiceForTest(t)
	c, err := collections.CreateCollection("Probes")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	const secretToken = "sk_live_super_secret_token"
	if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "token", strPtr(secretToken), true, ""); err != nil {
		t.Fatalf("Upsert secret: %v", err)
	}

	// The call args as the bridge's own RunOp closure builds them before ever resolving anything —
	// still spelled {{token}}.
	args := GrpcCallArgs{
		OpID: "op1", TabID: "tab1", Streaming: false,
		DescriptorMode: "reflection", Target: "api.example.com:443",
		Service: "kira.probe.v1.Echo", Method: "Unary",
		MessageJSON:  `{"token":"{{token}}"}`,
		Metadata:     []grpcclient.MetaPair{{Name: "authorization", Value: "Bearer {{token}}"}},
		CollectionID: c.ID,
	}
	// The CallResult as if the live call actually resolved and sent the secret — recordGrpcHistory
	// must never be handed this and must never derive persisted fields from it; passed here as the
	// call's own terminal outcome to record, exactly the shape Call's own RunOp closure produces.
	result := grpcclient.CallResult{
		Code: 0, CodeName: "OK", ElapsedMs: 5,
		Header: []grpcclient.MetaPair{{Name: "x-ok", Value: "1"}},
	}

	svc.recordGrpcHistory(args, result)

	entries, err := svc.Deps.Repos.GrpcHistory.List("tab:tab1")
	if err != nil || len(entries) != 1 {
		t.Fatalf("List(tab:tab1) = %d entries, err %v, want 1", len(entries), err)
	}

	var rawSnapshot string
	if err := db.DB.QueryRow(
		`SELECT snapshot_json FROM grpc_call_history WHERE id = ?`, entries[0].ID,
	).Scan(&rawSnapshot); err != nil {
		t.Fatalf("query snapshot_json: %v", err)
	}
	if strings.Contains(rawSnapshot, secretToken) {
		t.Fatalf("stored snapshot_json contains the raw secret — it reached kira.sqlite in plaintext:\n%s", rawSnapshot)
	}
	if !strings.Contains(rawSnapshot, "{{token}}") {
		t.Fatalf("stored snapshot_json does not contain the masked placeholder {{token}} at all:\n%s", rawSnapshot)
	}

	// The round trip survives Get too — what a viewer of this history entry actually sees later,
	// not just what was written (mirrors P10 D14's own check via response_history's Get).
	snap, err := svc.Deps.Repos.GrpcHistory.Get(entries[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if strings.Contains(snap.Message, secretToken) {
		t.Fatalf("decoded Message = %q still contains the raw secret", snap.Message)
	}
	for _, m := range snap.Metadata {
		if strings.Contains(m.Value, secretToken) {
			t.Fatalf("decoded Metadata %+v still contains the raw secret", m)
		}
	}
}

// TestResolveGrpcCallSource_ResolvesTargetAndMetadataInProtoDescriptorMode: the old
// resolveGrpcSource (Describe's own stage 2) short-circuited on any non-reflection descriptor mode
// — correct for Describe (a .proto file needs no target), wrong for Call, which reuses the exact
// same function and always needs a real network target and its metadata resolved, .proto schema
// source or not. resolveGrpcCallSource is Call's own replacement, with no such short-circuit.
func TestResolveGrpcCallSource_ResolvesTargetAndMetadataInProtoDescriptorMode(t *testing.T) {
	svc, _, variablesRepo, collections := newGrpcServiceForTest(t)
	c, err := collections.CreateCollection("Probes")
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	const secretHost = "internal-secret-host.example.com"
	const secretToken = "sk_live_super_secret_token"
	if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "host", strPtr(secretHost), true, ""); err != nil {
		t.Fatalf("Upsert host secret: %v", err)
	}
	if _, err := variablesRepo.Upsert(model.VariableScopeCollection, c.ID, "", "token", strPtr(secretToken), true, ""); err != nil {
		t.Fatalf("Upsert token secret: %v", err)
	}

	args := GrpcCallArgs{
		OpID: "op1", TabID: "tab1", DescriptorMode: "proto",
		Target:    "{{host}}:443",
		ProtoPath: "/tmp/probe.proto",
		Metadata:  []grpcclient.MetaPair{{Name: "authorization", Value: "Bearer {{token}}"}},
		Service:   "kira.probe.v1.Echo", Method: "Unary",
		MessageJSON:  `{"ok":true}`,
		CollectionID: c.ID,
	}

	src, _, used, err := svc.resolveGrpcCallSource(args)
	if err != nil {
		t.Fatalf("resolveGrpcCallSource: %v", err)
	}
	if src.Target != secretHost+":443" {
		t.Errorf("Target = %q, want it resolved to %q:443", src.Target, secretHost)
	}
	if len(src.Metadata) != 1 || src.Metadata[0].Value != "Bearer "+secretToken {
		t.Errorf("Metadata = %+v, want authorization resolved to Bearer %s", src.Metadata, secretToken)
	}
	if usedRendered(used, "host") != secretHost || usedRendered(used, "token") != secretToken {
		t.Errorf("used = %+v, want both host and token recorded", used)
	}
}

// usedRendered is this file's own small lookup over []apivars.UsedSecret (P17 D9) — a test still
// wants "what did this name render to", without caring about entry order.
func usedRendered(used []apivars.UsedSecret, name string) string {
	for _, u := range used {
		if u.Name == name {
			return u.Rendered
		}
	}
	return ""
}

// fakeEmitter is appcore.Emitter's own test double: it records every EmitTo call so a test can
// inspect exactly what runServerStream pushed to the renderer, including the terminal error event
// finding 4 is about.
type fakeEmitter struct {
	mu   sync.Mutex
	sent []GrpcCallEvent
}

func (f *fakeEmitter) Emit(name string, data any) {}
func (f *fakeEmitter) EmitTo(windowKey string, name string, data any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if evt, ok := data.(GrpcCallEvent); ok {
		f.sent = append(f.sent, evt)
	}
}
func (f *fakeEmitter) EmitFocused(name string, data any) {}

func (f *fakeEmitter) terminal() *GrpcCallEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.sent {
		if f.sent[i].Done {
			return &f.sent[i]
		}
	}
	return nil
}

// echoStreamProto is a minimal server-streaming schema — just enough for resolveMethod (a .proto
// descriptor source, so schema resolution needs no network round trip of its own) to validate the
// call before grpcclient.ServerStream ever tries to dial.
const echoStreamProto = `syntax = "proto3";
package kira.probe.v1;

message EchoRequest {
  string text = 1;
}

message EchoResponse {
  string text = 1;
}

service Echo {
  rpc ServerStream(EchoRequest) returns (stream EchoResponse);
}
`

// TestRunServerStream_MasksDialTargetSecretBeforeEmittingErrorEvent: a failed
// streaming call used to emit its terminal error event (the push channel, the renderer's own
// preferred error source over Call's return value — views/grpcrequest/state.ts) before Call's own
// maskGrpcError ever ran, so a secret substituted into the dial target leaked straight to the
// renderer unmasked. runServerStream now masks before emitting; this dials a real (unreachable)
// TCP port with the secret spelled directly into the target, so grpcclient.ServerStream's own
// dial-failure message — which embeds the literal address it tried — is exactly the vector this
// proves closed, with no server or reflection round trip required.
func TestRunServerStream_MasksDialTargetSecretBeforeEmittingErrorEvent(t *testing.T) {
	const secretHost = "127.0.0.1"
	protoPath := filepath.Join(t.TempDir(), "echo.proto")
	if err := os.WriteFile(protoPath, []byte(echoStreamProto), 0o644); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	emitter := &fakeEmitter{}
	svc := &GrpcService{Deps: appcore.Deps{Events: emitter}}

	used := []apivars.UsedSecret{{Name: "host", Rendered: secretHost, Placeholder: "{{host}}"}}
	req := grpcclient.CallRequest{
		// Port 1: nothing ever listens there, so the dial fails almost instantly with
		// "connection refused" rather than timing out.
		Target:      secretHost + ":1",
		Source:      grpcclient.Source{Mode: grpcclient.SourceProto, ProtoPath: protoPath},
		FullMethod:  "/kira.probe.v1.Echo/ServerStream",
		MessageJSON: `{"text":"hi"}`,
	}
	args := GrpcCallArgs{OpID: "call-1", WindowKey: "win-1"}

	_, err := svc.runServerStream(context.Background(), args, req, used)
	if err == nil {
		t.Fatal("expected a dial failure")
	}
	if strings.Contains(err.Error(), secretHost) {
		t.Fatalf("returned error %q still contains the raw secret", err.Error())
	}

	terminal := emitter.terminal()
	if terminal == nil {
		t.Fatal("no terminal (done) event was emitted")
	}
	if terminal.Error == nil {
		t.Fatal("terminal event carries no Error")
	}
	if strings.Contains(terminal.Error.Message, secretHost) {
		t.Fatalf("emitted terminal event Error.Message = %q still contains the raw secret", terminal.Error.Message)
	}
	if !strings.Contains(terminal.Error.Message, "{{host}}") {
		t.Fatalf("emitted terminal event Error.Message = %q, want it masked to {{host}}", terminal.Error.Message)
	}
}

// raceCheckEmitter is fakeEmitter's own sibling: its EmitTo runs synchronously on
// the same goroutine coalescer.finish calls it from (flushLocked's own EmitTo call, under
// c.mu) — exactly mirroring the renderer's own wake-up, which is likewise driven directly off
// this event with no delay of its own. On the terminal (Done) event it snapshots whether
// GrpcHistory already has a row for this tab, the same query a History pane's own immediate
// re-list (noteRecorded -> load) would run the instant it is woken by this same event.
type raceCheckEmitter struct {
	history *repos.GrpcHistoryRepo
	tabID   string

	mu                sync.Mutex
	sawRecordedAtDone bool
	doneSeen          bool
}

func (r *raceCheckEmitter) Emit(name string, data any) {}
func (r *raceCheckEmitter) EmitTo(windowKey string, name string, data any) {
	evt, ok := data.(GrpcCallEvent)
	if !ok || !evt.Done {
		return
	}
	entries, _ := r.history.List("tab:" + r.tabID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doneSeen = true
	r.sawRecordedAtDone = len(entries) > 0
}
func (r *raceCheckEmitter) EmitFocused(name string, data any) {}

// TestRunServerStream_RecordsHistoryBeforeEmittingTerminalEvent: a server-streaming call's
// renderer is woken by runServerStream's own terminal event, not by Call's return value the way a
// unary call's renderer is — so if the row is
// recorded only after that event goes out (the old code: Call's own recordGrpcHistory ran only
// after runServerStream had already returned), a History pane already open for this tab can
// immediately re-list in reaction to the event and be answered before the insert lands, dropping
// the just-finished call from the list until some unrelated later refresh. This swaps in a fake
// serverStreamFn (no real network round trip needed — the ordering bug is entirely on the Go side)
// and an emitter that checks, synchronously and at the moment the terminal event fires, whether
// the row already exists. It fails against the pre-fix ordering and passes once recordGrpcHistory
// runs before coalescer.finish inside runServerStream itself.
func TestRunServerStream_RecordsHistoryBeforeEmittingTerminalEvent(t *testing.T) {
	svc, _, _, _ := newGrpcServiceForTest(t)

	old := serverStreamFn
	t.Cleanup(func() { serverStreamFn = old })
	serverStreamFn = func(ctx context.Context, req grpcclient.CallRequest, onMessage func(grpcclient.Message)) (grpcclient.CallResult, error) {
		onMessage(grpcclient.Message{Seq: 0, JSON: `{"text":"hi"}`})
		return grpcclient.CallResult{Code: 0, CodeName: "OK", MessageCount: 1}, nil
	}

	emitter := &raceCheckEmitter{history: svc.Deps.Repos.GrpcHistory, tabID: "tab1"}
	svc.Deps.Events = emitter

	args := GrpcCallArgs{
		OpID: "op1", TabID: "tab1", WindowKey: "win-1", Streaming: true,
		DescriptorMode: "reflection", Target: "api.example.com:443",
		Service: "kira.probe.v1.Echo", Method: "ServerStream",
	}
	req := grpcclient.CallRequest{FullMethod: "/kira.probe.v1.Echo/ServerStream"}

	if _, err := svc.runServerStream(context.Background(), args, req, nil); err != nil {
		t.Fatalf("runServerStream: %v", err)
	}

	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	if !emitter.doneSeen {
		t.Fatal("no terminal (done) event was emitted")
	}
	if !emitter.sawRecordedAtDone {
		t.Fatal("the terminal event fired before grpc_call_history had a row for this call — a History pane woken by this event can be answered before the insert lands")
	}

	entries, err := svc.Deps.Repos.GrpcHistory.List("tab:tab1")
	if err != nil || len(entries) != 1 {
		t.Fatalf("List(tab:tab1) after the call = %d entries, err %v, want 1", len(entries), err)
	}
}
