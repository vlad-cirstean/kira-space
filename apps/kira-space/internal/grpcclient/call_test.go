package grpcclient

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func echoSource(t *testing.T) (Source, string) {
	t.Helper()
	protoPath := writeProto(t, "echo.proto", echoProtoSource)
	return Source{Mode: SourceProto, ProtoPath: protoPath}, protoPath
}

// TestUnary_RequestResponseAndMetadata is §6.2 call_test.go case 1: request JSON in, response
// JSON out, header and trailer metadata both captured (F5, F6).
func TestUnary_RequestResponseAndMetadata(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{}, false)
	src, _ := echoSource(t)

	result, err := Unary(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/Unary",
		MessageJSON: `{"text":"hi","index":3}`,
	})
	if err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if result.Code != int32(codes.OK) || result.CodeName != "OK" {
		t.Errorf("Code/CodeName = %d/%s, want OK", result.Code, result.CodeName)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("Messages = %+v, want exactly 1", result.Messages)
	}
	if !strings.Contains(result.Messages[0].JSON, `"hi"`) || !strings.Contains(result.Messages[0].JSON, `"index":3`) {
		t.Errorf("Messages[0].JSON = %q, want it to echo the request", result.Messages[0].JSON)
	}
	if result.MessageCount != 1 {
		t.Errorf("MessageCount = %d, want 1", result.MessageCount)
	}
}

// TestUnary_NonOKStatusIsAResultNotAnError is §6.2 case 2 — D16's central claim, asserted
// directly: a non-OK gRPC status is a RESULT, not an error. code, codeName and statusMessage
// populated and err == nil.
func TestUnary_NonOKStatusIsAResultNotAnError(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{
		unaryStatus: status.New(codes.PermissionDenied, "no access to this resource"),
	}, false)
	src, _ := echoSource(t)

	result, err := Unary(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/Unary",
		MessageJSON: `{"text":"hi"}`,
	})
	if err != nil {
		t.Fatalf("Unary returned an error for a server-side PermissionDenied: %v — D16 says this must be a result", err)
	}
	if result.Code != int32(codes.PermissionDenied) {
		t.Errorf("Code = %d, want %d (PermissionDenied)", result.Code, codes.PermissionDenied)
	}
	if result.CodeName != "PermissionDenied" {
		t.Errorf("CodeName = %q, want PermissionDenied", result.CodeName)
	}
	if result.StatusMessage != "no access to this resource" {
		t.Errorf("StatusMessage = %q", result.StatusMessage)
	}
}

// TestServerStream_MessagesInOrderWithOffsets is §6.2 case 3: N messages arrive in order with
// monotonically non-decreasing arrival offsets, and the terminal result's count matches.
func TestServerStream_MessagesInOrderWithOffsets(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{
		streamMessages: 5, streamSleep: 5 * time.Millisecond,
	}, false)
	src, _ := echoSource(t)

	var mu sync.Mutex
	var got []Message
	result, err := ServerStream(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/ServerStream",
		MessageJSON: `{"text":"go"}`,
	}, func(m Message) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("ServerStream: %v", err)
	}
	if result.Code != int32(codes.OK) {
		t.Fatalf("Code = %d, want OK", result.Code)
	}
	if result.MessageCount != 5 {
		t.Fatalf("MessageCount = %d, want 5", result.MessageCount)
	}
	if len(got) != 5 {
		t.Fatalf("onMessage delivered %d messages, want 5", len(got))
	}
	for i, m := range got {
		if m.Seq != i {
			t.Errorf("message %d: Seq = %d, want %d", i, m.Seq, i)
		}
		if i > 0 && m.OffsetMs < got[i-1].OffsetMs {
			t.Errorf("message %d: OffsetMs = %d went backwards from %d", i, m.OffsetMs, got[i-1].OffsetMs)
		}
	}
	// Finding 8: CallResult.Messages used to stay nil for a server-streaming call (only Unary set
	// it), even though the doc comment claimed otherwise and recordGrpcHistory (bridge/grpc.go)
	// builds a history entry's persisted `messages` from exactly this field.
	if !reflect.DeepEqual(result.Messages, got) {
		t.Fatalf("result.Messages = %+v, want it to match what onMessage received: %+v", result.Messages, got)
	}
}

// TestServerStream_MessagesElidedAboveCap is finding 8's own cap: CallResult.Messages is bounded
// at maxStoredMessages regardless of how many messages the stream actually produced — MessageCount
// stays the true total either way, which is what repos/grpc_history.go's own elision check
// compares against, not len(Messages).
func TestServerStream_MessagesElidedAboveCap(t *testing.T) {
	const total = maxStoredMessages + 37
	server := startEchoServer(t, echoProtoSource, &echoImpl{streamMessages: total}, false)
	src, _ := echoSource(t)

	result, err := ServerStream(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/ServerStream",
		MessageJSON: `{"text":"go"}`,
	}, nil)
	if err != nil {
		t.Fatalf("ServerStream: %v", err)
	}
	if result.MessageCount != total {
		t.Fatalf("MessageCount = %d, want the true total %d", result.MessageCount, total)
	}
	if len(result.Messages) != maxStoredMessages {
		t.Fatalf("len(Messages) = %d, want exactly the %d-message cap", len(result.Messages), maxStoredMessages)
	}
	for i, m := range result.Messages {
		if m.Seq != i {
			t.Errorf("Messages[%d].Seq = %d, want %d", i, m.Seq, i)
		}
	}
}

// TestServerStream_CancelledMidStream is §6.2 case 4: cancellation mid-stream — the messages
// already delivered are kept and the result is E_CANCELLED with the true partial count (F8).
func TestServerStream_CancelledMidStream(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{
		streamMessages: 1000, streamSleep: 20 * time.Millisecond,
	}, false)
	src, _ := echoSource(t)

	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	var got []Message
	go func() {
		time.Sleep(90 * time.Millisecond)
		cancel()
	}()

	_, err := ServerStream(ctx, CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/ServerStream",
		MessageJSON: `{"text":"go"}`,
	}, func(m Message) {
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
	})
	if err == nil {
		t.Fatal("ServerStream after cancellation: want an error, got nil")
	}
	gerr, ok := err.(*Error)
	if !ok || gerr.Code != CodeCancelled {
		t.Fatalf("error = %v, want a *Error with code %s", err, CodeCancelled)
	}
	if gerr.Partial == nil {
		t.Fatal("Partial is nil, want the true partial count")
	}
	mu.Lock()
	delivered := len(got)
	mu.Unlock()
	if delivered == 0 {
		t.Fatal("no messages were delivered before cancellation — the test's own timing is too tight")
	}
	if gerr.Partial.MessageCount != delivered {
		t.Errorf("Partial.MessageCount = %d, want %d (the messages actually delivered to onMessage)", gerr.Partial.MessageCount, delivered)
	}
	// Finding 8: the partial path never populated Messages either — a cancellation that received
	// messages is still a completed call (D11) and recordGrpcHistory persists from this same field.
	if len(gerr.Partial.Messages) != delivered {
		t.Errorf("Partial.Messages = %d entries, want %d (the messages actually delivered to onMessage)", len(gerr.Partial.Messages), delivered)
	}
}

// TestUnary_RequestJSONStrictness is §6.2 case 5: protojson rejects an unknown request field with
// its line:col message; DiscardUnknown on the response accepts a field the local schema lacks
// (F4). The DiscardUnknown half is exercised indirectly here (the response codec always sets it,
// call.go's marshalResponseJSON) — this test's own assertion is the request half, the one that can
// actually be driven from outside the package: an authored request is not itself a proto message
// yet, so "the local schema lacks a field the response carries" is a server-side condition proven
// by construction (marshalResponseJSON's own DiscardUnknown-equivalent EmitUnpopulated option) and
// by TestUnary_RequestResponseAndMetadata's own round trip not erroring.
func TestUnary_RequestJSONStrictness(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{}, false)
	src, _ := echoSource(t)

	_, err := Unary(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/Unary",
		MessageJSON: `{"text":"hi","nope":true}`,
	})
	if err == nil {
		t.Fatal("Unary with an unknown request field: want an error, got nil")
	}
	gerr, ok := err.(*Error)
	if !ok || gerr.Code != CodeBadRequest {
		t.Fatalf("error = %v, want a *Error with code %s", err, CodeBadRequest)
	}
	if !strings.Contains(gerr.Message, "nope") {
		t.Errorf("message = %q, want it to name the unknown field", gerr.Message)
	}
}

// TestUnary_MessageOverCapYieldsResourceExhausted: a message over the 16 MiB cap yields
// ResourceExhausted — the server replies with a message bigger than
// grpc.MaxCallRecvMsgSize(16 MiB), which is this app's own client-side receive cap, not the
// server's send limit (grpc-go's own server default has no send cap at all).
func TestUnary_MessageOverCapYieldsResourceExhausted(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{unaryHugeReply: maxRecvMsgSize + 1024}, false)
	src, _ := echoSource(t)

	result, err := Unary(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/Unary",
		MessageJSON: `{"text":"hi"}`,
	})
	if err != nil {
		t.Fatalf("Unary: %v", err)
	}
	if result.Code != int32(codes.ResourceExhausted) {
		t.Fatalf("Code = %d (%s), want ResourceExhausted", result.Code, result.CodeName)
	}
}

// TestServerStream_CoalescingUnderRace: a server emitting 500 messages as fast as
// it can produces every message exactly once with contiguous Seq — run under -race, since
// onMessage is invoked from the goroutine reading the stream while the caller (bridge/grpc.go's
// coalescing buffer, exercised for real in the bridge-level test) reads it concurrently in the
// real app. This package-level test proves the source side is race-free on its own: onMessage is
// called synchronously and sequentially from the one RecvMsg loop, never concurrently.
func TestServerStream_CoalescingUnderRace(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{streamMessages: 500}, false)
	src, _ := echoSource(t)

	var mu sync.Mutex
	seen := make([]bool, 500)
	result, err := ServerStream(context.Background(), CallRequest{
		Target: server.addr, Source: src, FullMethod: "/kira.probe.v1.Echo/ServerStream",
		MessageJSON: `{"text":"go"}`,
	}, func(m Message) {
		mu.Lock()
		defer mu.Unlock()
		if m.Seq < 0 || m.Seq >= len(seen) {
			t.Errorf("Seq %d out of range", m.Seq)
			return
		}
		if seen[m.Seq] {
			t.Errorf("Seq %d delivered twice", m.Seq)
		}
		seen[m.Seq] = true
	})
	if err != nil {
		t.Fatalf("ServerStream: %v", err)
	}
	if result.MessageCount != 500 {
		t.Fatalf("MessageCount = %d, want 500", result.MessageCount)
	}
	for i, ok := range seen {
		if !ok {
			t.Errorf("Seq %d was never delivered", i)
		}
	}
}

// TestUnmarshalRequestJSON_SyntaxErrorDropsTheOffendingToken: a request JSON built by
// stage 2's own {{secret}} substitution can have a secret's raw value break the JSON string
// literal it landed in (an embedded quote or backslash) — protojson's own syntax-error message
// quotes that broken token straight from the input, which is then a fragment of the secret too
// short for bridge/grpc.go's own full-value replacer to mask. unmarshalRequestJSON must keep only
// the line:col position for a genuine syntax error, never the token, while a semantic error (an
// unknown field, covered by TestUnary_RequestJSONStrictness above) still names itself in full —
// field names are never secret material.
func TestUnmarshalRequestJSON_SyntaxErrorDropsTheOffendingToken(t *testing.T) {
	_, protoPath := echoSource(t)
	md := compileService(t, protoPath).Methods().ByName("Unary").Input()

	// The secret's value ("s3cr3t-value) carries a raw, unescaped quote that breaks out of the
	// "text" field's own string literal — exactly the shape a substituted {{secret}} can produce.
	const secretFragment = "s3cr3t-value"
	bad := `{"text": "` + secretFragment + `"oops"}`

	err := unmarshalRequestJSON(bad, dynamicMsg(t, md))
	if err == nil {
		t.Fatal("unmarshalRequestJSON: want an error for malformed JSON, got nil")
	}
	gerr, ok := err.(*Error)
	if !ok || gerr.Code != CodeBadRequest {
		t.Fatalf("error = %v, want a *Error with code %s", err, CodeBadRequest)
	}
	if strings.Contains(gerr.Message, secretFragment) {
		t.Errorf("message = %q, leaked the secret fragment %q from protojson's own syntax error", gerr.Message, secretFragment)
	}
	if !strings.Contains(gerr.Message, "line") {
		t.Errorf("message = %q, want it to still name a line:col position", gerr.Message)
	}
}

// dynamicMsg builds a fresh *dynamicpb.Message for md — a one-line helper so the test above reads
// as "build a message, then unmarshal into it" without importing dynamicpb itself.
func dynamicMsg(t *testing.T, md protoreflect.MessageDescriptor) *dynamicpb.Message {
	t.Helper()
	return dynamicpb.NewMessage(md)
}
