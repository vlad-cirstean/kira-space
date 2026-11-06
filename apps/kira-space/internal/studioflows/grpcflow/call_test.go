package grpcflow

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/grpcclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

func TestUnaryMetadataAndStatus(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	args := callArgs(srv.Addr, "Unary", `{"text":"hello","index":7}`)
	args.Metadata = []grpcclient.MetaPair{{Name: "x-trace", Value: "abc-123"}}
	res := call(t, app, args)
	app.Contract(t, "api-grpc", "GrpcService.Call", res, studioflowharness.Mask("elapsedMs", "offsetMs", "wireBytes", "messageBytes"), studioflowharness.Replace(srv.Addr, "<grpc>"))
	if res.Code != 0 || res.CodeName != "OK" || res.MessageCount != 1 {
		t.Fatalf("result = %+v, want OK with one message", res)
	}
	var msg struct {
		Text  string `json:"text"`
		Index int    `json:"index"`
	}
	if err := json.Unmarshal([]byte(res.Messages[0].JSON), &msg); err != nil || msg.Text != "hello" || msg.Index != 7 {
		t.Errorf("response = %s (%v), want text hello index 7", res.Messages[0].JSON, err)
	}
	if pairValue(res.Header, "echo-x-trace") != "abc-123" {
		t.Errorf("header = %+v, want echo-x-trace=abc-123", res.Header)
	}
	if pairValue(res.Trailer, "trailer-x-trace") != "abc-123" {
		t.Errorf("trailer = %+v, want trailer-x-trace=abc-123", res.Trailer)
	}

	fail := callArgs(srv.Addr, "Fail", `{"code":5,"message":"no such thing"}`)
	failRes := call(t, app, fail)
	if failRes.Code != 5 || failRes.CodeName != "NotFound" || failRes.StatusMessage != "no such thing" {
		t.Errorf("Fail result = %d %s %q, want 5 NotFound \"no such thing\"", failRes.Code, failRes.CodeName, failRes.StatusMessage)
	}
	if failRes.MessageCount != 0 {
		t.Errorf("Fail result carried %d messages", failRes.MessageCount)
	}

	rows := history(t, app, "tab-1")
	if len(rows) != 2 {
		t.Fatalf("history rows = %d, want one per completed call (2)", len(rows))
	}
	byCode := map[string]string{}
	for _, r := range rows {
		byCode[r.CodeName] = r.ID
	}
	snap, err := app.W.GrpcHistory.Get(bridge.GrpcHistoryIDArgs{ID: byCode["OK"]})
	if err != nil {
		t.Fatal(err)
	}
	if snap.Message != args.MessageJSON || snap.Target != srv.Addr || snap.Method != flowSvc+"/Unary" {
		t.Errorf("snapshot = %q %q %q, want the request as authored", snap.Message, snap.Target, snap.Method)
	}
	if snap.Entry.Streaming != "unary" || len(snap.Messages) != 1 {
		t.Errorf("snapshot streaming %q messages %d", snap.Entry.Streaming, len(snap.Messages))
	}
	if byCode["NotFound"] == "" {
		t.Error("the NOT_FOUND call has no history row")
	}

	for _, c := range []struct {
		args bridge.GrpcCallArgs
		code string
	}{{args, "OK"}, {fail, "NotFound"}} {
		op := waitOp(t, app, c.args.OpID)
		if op.Status != "ok" || op.Command == nil || !strings.HasSuffix(*op.Command, "→ "+c.code) {
			t.Errorf("op %s = %s %v, want ok ending in %s", c.args.OpID, op.Status, op.Command, c.code)
		}
	}
}

func TestServerStreamAndCancel(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	t.Run("messages reach the calling window in order", func(t *testing.T) {
		mark := app.Events.Mark()
		args := callArgs(srv.Addr, "ServerStream", `{"text":"s","count":5,"intervalMs":50}`)
		args.Streaming, args.TabID = true, "tab-stream"

		type outcome struct {
			res grpcclient.CallResult
			err error
		}
		done := make(chan outcome, 1)
		go func() {
			r, err := app.W.Grpc.Call(ctx, args)
			done <- outcome{r, err}
		}()
		// The history row is written before the terminal event, so it exists when that event is seen.
		app.Events.WaitAfter(t, mark, bridge.ChannelGrpcCall, func(e studioflowharness.Event) bool {
			var d bridge.GrpcCallEvent
			e.Decode(t, &d)
			return d.Done
		}, 10*time.Second)
		if rows := history(t, app, "tab-stream"); len(rows) != 1 {
			t.Errorf("history rows at the terminal event = %d, want 1", len(rows))
		}
		o := <-done
		if o.err != nil {
			t.Fatal(o.err)
		}
		if o.res.CodeName != "OK" || o.res.MessageCount != 5 {
			t.Errorf("result = %s with %d messages, want OK with 5", o.res.CodeName, o.res.MessageCount)
		}

		var seqs []int
		var last int64 = -1
		sawDone := false
		for _, ev := range callEvents(t, app, mark) {
			if ev.CallID != args.OpID {
				t.Errorf("event call id = %q, want %q", ev.CallID, args.OpID)
			}
			if ev.Seq != len(seqs) {
				t.Errorf("event seq = %d, want %d (first message index of the batch)", ev.Seq, len(seqs))
			}
			for _, m := range ev.Messages {
				seqs = append(seqs, m.Seq)
				if m.OffsetMs < last {
					t.Errorf("offsets not rising: %d after %d", m.OffsetMs, last)
				}
				last = m.OffsetMs
			}
			if ev.Done {
				sawDone = true
				if ev.Status == nil || ev.Status.CodeName != "OK" {
					t.Errorf("terminal status = %+v", ev.Status)
				}
			}
		}
		if !sawDone {
			t.Error("no terminal event")
		}
		// Batching decides how many messages one event carries; the contract is one message.
		first := callEvents(t, app, mark)[0]
		first.Messages = first.Messages[:1]
		app.Contract(t, "api-grpc", "event:"+bridge.ChannelGrpcCall+"#message", first, studioflowharness.Mask("callId", "offsetMs", "wireBytes"))
		for i, s := range seqs {
			if s != i {
				t.Errorf("message seqs = %v, want 0..4 in order", seqs)
				break
			}
		}
		if len(seqs) != 5 {
			t.Errorf("events carried %d messages, want 5", len(seqs))
		}
		if last < 150 {
			t.Errorf("last offset %d ms, want >= 150 for 5 messages 50 ms apart", last)
		}
	})

	t.Run("cancel mid stream", func(t *testing.T) {
		mark := app.Events.Mark()
		args := callArgs(srv.Addr, "ServerStream", `{"count":100,"intervalMs":300}`)
		args.Streaming, args.TabID = true, "tab-cancel"
		done := make(chan error, 1)
		go func() {
			_, err := app.W.Grpc.Call(ctx, args)
			done <- err
		}()
		received := func() int {
			n := 0
			for _, ev := range callEvents(t, app, mark) {
				n += len(ev.Messages)
			}
			return n
		}
		waitUntil(t, "3 messages", func() bool { return received() >= 3 })
		if err := app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: args.OpID}); err != nil {
			t.Fatal(err)
		}
		var err error
		select {
		case err = <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Call did not return after Ops.Cancel")
		}
		e := ipcErr(t, err)
		if e.Code != grpcclient.CodeCancelled {
			t.Fatalf("error code = %s, want %s", e.Code, grpcclient.CodeCancelled)
		}
		var partial grpcclient.CallResult
		if jerr := json.Unmarshal(e.Details, &partial); jerr != nil {
			t.Fatalf("details %s: %v", e.Details, jerr)
		}
		if partial.CodeName != "Canceled" || partial.MessageCount < 3 || partial.MessageCount > 4 {
			t.Errorf("partial = %s with %d messages, want Canceled with 3 (4 at most)", partial.CodeName, partial.MessageCount)
		}
		rows := history(t, app, "tab-cancel")
		if len(rows) != 1 || rows[0].CodeName != "Canceled" || rows[0].MessageCount != partial.MessageCount {
			t.Errorf("history = %+v, want one Canceled row with %d messages", rows, partial.MessageCount)
		}
		if op := waitOp(t, app, args.OpID); op.Status != "cancelled" {
			t.Errorf("op status = %s, want cancelled", op.Status)
		}
		events := callEvents(t, app, mark)
		final := events[len(events)-1]
		if !final.Done || final.Error == nil || final.Error.Code != grpcclient.CodeCancelled {
			t.Errorf("terminal event = %+v, want done with %s", final, grpcclient.CodeCancelled)
		}
		// How many messages arrived before the cancel varies run to run.
		partialStatus := *final.Status
		partialStatus.Messages = nil
		final.Status = &partialStatus
		app.Contract(t, "api-grpc", "event:"+bridge.ChannelGrpcCall+"#cancelled", final,
			studioflowharness.Mask("callId", "seq", "elapsedMs", "messageCount", "messageBytes", "wireBytes"))
	})
}

func TestClientAndBidiRefused(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	s := describe(t, app, describeArgs(srv.Addr))
	for name, both := range map[string]bool{"ClientStream": false, "Bidi": true} {
		m, ok := findMethod(t, s, name)
		if !ok || !m.ClientStreaming || m.ServerStreaming != both {
			t.Errorf("%s flags = %+v, want clientStreaming and serverStreaming=%v", name, m, both)
		}
	}

	for _, tc := range []struct {
		method    string
		streaming bool
	}{{"ClientStream", false}, {"ClientStream", true}, {"Bidi", false}, {"Bidi", true}, {"Unary", true}} {
		args := callArgs(srv.Addr, tc.method, `{}`)
		args.Streaming = tc.streaming
		_, err := app.W.Grpc.Call(ctx, args)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeBadRequest {
			t.Errorf("%s streaming=%v error = %s %q, want %s", tc.method, tc.streaming, e.Code, e.Message, grpcclient.CodeBadRequest)
		}
		if op := waitOp(t, app, args.OpID); op.Status != "error" {
			t.Errorf("%s op status = %s, want error", tc.method, op.Status)
		}
	}
	if rows := history(t, app, "tab-1"); len(rows) != 0 {
		t.Errorf("refused calls left %d history rows", len(rows))
	}
	if calls := srv.Calls(); len(calls) != 0 {
		t.Errorf("refused calls reached the server: %+v", calls)
	}
}

func TestTLSTargets(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t, studioflowharness.WithGRPCTLS(t))
	tlsCfg := grpcclient.TLSConfig{Enabled: true, CAFile: srv.CAFile}

	d := describeArgs(srv.Addr)
	d.TLS = tlsCfg
	describe(t, app, d)

	unary := callArgs(srv.Addr, "Unary", `{"text":"tls"}`)
	unary.TLS = tlsCfg
	if res := call(t, app, unary); res.CodeName != "OK" {
		t.Errorf("TLS unary = %s", res.CodeName)
	}
	if calls := srv.Calls(); len(calls) != 1 || !calls[0].TLS {
		t.Errorf("server calls = %+v, want one over TLS", calls)
	}

	t.Run("no CA fails the handshake", func(t *testing.T) {
		d := describeArgs(srv.Addr)
		d.TLS = grpcclient.TLSConfig{Enabled: true}
		_, err := app.W.Grpc.Describe(ctx, d)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeTransport {
			t.Errorf("describe error = %s %q, want %s", e.Code, e.Message, grpcclient.CodeTransport)
		}
		u := callArgs(srv.Addr, "Unary", `{}`)
		u.TLS = grpcclient.TLSConfig{Enabled: true}
		_, err = app.W.Grpc.Call(ctx, u)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeTransport {
			t.Errorf("call error = %s %q, want %s", e.Code, e.Message, grpcclient.CodeTransport)
		}
	})

	t.Run("missing CA file", func(t *testing.T) {
		u := callArgs(srv.Addr, "Unary", `{}`)
		u.TLS = grpcclient.TLSConfig{Enabled: true, CAFile: "/nonexistent/ca.pem"}
		_, err := app.W.Grpc.Call(ctx, u)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeBadRequest || !strings.Contains(e.Message, "CA file") {
			t.Errorf("error = %s %q, want %s naming the CA file", e.Code, e.Message, grpcclient.CodeBadRequest)
		}
	})

	t.Run("plaintext against TLS", func(t *testing.T) {
		u := callArgs(srv.Addr, "Unary", `{}`)
		_, err := app.W.Grpc.Call(ctx, u)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeTransport {
			t.Errorf("error = %s %q, want %s", e.Code, e.Message, grpcclient.CodeTransport)
		}
	})

	t.Run("server name override passes a SAN mismatch", func(t *testing.T) {
		other := studioflowharness.GRPC(t, studioflowharness.WithGRPCTLS(t, "flow.example.test"))
		u := callArgs(other.Addr, "Unary", `{"text":"san"}`)
		u.TLS = grpcclient.TLSConfig{Enabled: true, CAFile: other.CAFile}
		_, err := app.W.Grpc.Call(ctx, u)
		if e := ipcErr(t, err); e.Code != grpcclient.CodeTransport {
			t.Fatalf("mismatch error = %s %q, want %s", e.Code, e.Message, grpcclient.CodeTransport)
		}
		u.TLS.ServerName = "flow.example.test"
		if res := call(t, app, u); res.CodeName != "OK" {
			t.Errorf("with serverName = %s, want OK", res.CodeName)
		}
	})
}

func TestSecretsInTargetAndMetadata(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)
	const authz = "sek-5d2f9a77"
	colID, envID := secretVars(t, app, map[string]string{"host": srv.Addr, "authz": authz})

	stream := callArgs("{{host}}", "ServerStream", `{"text":"m","count":1}`)
	stream.Streaming, stream.TabID = true, "tab-secret"
	stream.CollectionID, stream.EnvironmentID = colID, envID
	stream.Metadata = []grpcclient.MetaPair{{Name: "authorization", Value: "{{authz}}"}}
	call(t, app, stream)

	calls := srv.Calls()
	if len(calls) != 1 || calls[0].Metadata.Get("authorization")[0] != authz {
		t.Fatalf("server calls = %+v, want one carrying the plaintext authorization", calls)
	}

	// The server quotes the credential back in its status message and echoes it in the header.
	fail := callArgs("{{host}}", "Fail", `{"code":3,"message":"rejected {{authz}}"}`)
	fail.TabID = "tab-secret"
	fail.CollectionID, fail.EnvironmentID = colID, envID
	fail.Metadata = stream.Metadata
	res := call(t, app, fail)
	if res.StatusMessage != "rejected {{authz}}" {
		t.Errorf("status message = %q, want the secret masked", res.StatusMessage)
	}
	if got := pairValue(res.Header, "echo-authorization"); got != "{{authz}}" {
		t.Errorf("echoed header = %q, want {{authz}}", got)
	}
	if got := pairValue(res.Trailer, "trailer-authorization"); got != "{{authz}}" {
		t.Errorf("echoed trailer = %q, want {{authz}}", got)
	}

	rows := history(t, app, "tab-secret")
	if len(rows) != 2 {
		t.Fatalf("history rows = %d, want 2", len(rows))
	}
	for _, r := range rows {
		snap, err := app.W.GrpcHistory.Get(bridge.GrpcHistoryIDArgs{ID: r.ID})
		if err != nil {
			t.Fatal(err)
		}
		whole := mustJSON(t, snap)
		for _, secret := range []string{authz, srv.Addr} {
			mustNotContain(t, "history snapshot", whole, secret)
		}
		if snap.Target != "{{host}}" || snap.Metadata[0].Value != "{{authz}}" {
			t.Errorf("snapshot target %q metadata %+v, want placeholders", snap.Target, snap.Metadata)
		}
	}

	for _, id := range []string{stream.OpID, fail.OpID} {
		op := waitOp(t, app, id)
		if op.Command == nil || !strings.Contains(*op.Command, "{{host}}") {
			t.Errorf("op command = %v, want {{host}}", op.Command)
			continue
		}
		mustNotContain(t, "op command", *op.Command, srv.Addr)
	}
	db := string(dbBytes(t, app))
	if !strings.Contains(db, "{{host}}") {
		t.Error("database lacks the stored history target; the scan reads the wrong files")
	}
	mustNotContain(t, "database bytes", db, authz)
	mustNotContain(t, "database bytes", db, srv.Addr)
}

func mustNotContain(t *testing.T, what, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("%s contains %q", what, needle)
	}
}

func TestLargeMessages(t *testing.T) {
	studioflowharness.Complete(t)
	app := studioflowharness.New(t)
	srv := studioflowharness.GRPC(t)

	big := call(t, app, callArgs(srv.Addr, "Unary", `{"payloadBytes":4194304}`))
	if big.CodeName != "OK" || big.Messages[0].WireBytes < 4<<20 {
		t.Fatalf("4 MB unary = %s with %d wire bytes", big.CodeName, big.Messages[0].WireBytes)
	}

	mark := app.Events.Mark()
	args := callArgs(srv.Addr, "ServerStream", `{"count":10000}`)
	args.Streaming = true
	res := call(t, app, args)
	if res.MessageCount != 10000 {
		t.Fatalf("stream result counts %d messages, want 10000", res.MessageCount)
	}
	seen := make([]int, 10000)
	for _, ev := range callEvents(t, app, mark) {
		for _, m := range ev.Messages {
			seen[m.Seq]++
		}
	}
	for seq, n := range seen {
		if n != 1 {
			t.Fatalf("seq %d arrived %d times, want once", seq, n)
		}
	}
}
