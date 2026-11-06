package grpcclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// maxRecvMsgSize is D15's own bound — 16 MiB, this app's limit and not the server's, reported
// (never hidden) as codes.ResourceExhausted when exceeded (F9).
const maxRecvMsgSize = 16 * 1024 * 1024

// maxStoredMessages bounds ServerStream's own in-memory CallResult.Messages accumulator — kept
// numerically equal to repos/grpc_history.go's own maxGrpcStoredMessages (that package cannot be
// imported here, storage sitting above this client in the dependency order, and one bound int is
// not worth a shared-constants package). Without this, a long-running stream would grow
// CallResult.Messages without limit for the full life of the call, long before Record ever gets a
// chance to cap what it persists — MessageCount/MessageBytes (below) keep counting the true totals
// regardless, which is what recordGrpcHistory's own elision check (repos/grpc_history.go) compares
// against, not the length of this already-capped slice.
const maxStoredMessages = 100

// CallRequest carries everything Unary/ServerStream need: the target and its TLS decision,
// the Source the method's descriptor resolves from (reusing Describe's own cache), the fully
// qualified "/pkg.Service/Method" path, the request message JSON and the metadata — every field
// here is already resolved (this package is called only from inside the bridge's own
// resolved-at-point-of-use closure).
type CallRequest struct {
	Target     string
	TLS        TLSConfig
	Source     Source
	FullMethod string // "/pkg.Service/Method"

	MessageJSON string
	Metadata    []MetaPair

	// Timeout bounds a unary call end to end, E_TIMEOUT on expiry; zero means no deadline
	// (the dial bound still applies), like api.requestTimeoutMs.
	// A server stream is unbounded by design: the user ends it with Stop.
	Timeout time.Duration
}

func grpcPath(m protoreflect.MethodDescriptor) string {
	svc := m.Parent().(protoreflect.ServiceDescriptor)
	return "/" + string(svc.FullName()) + "/" + string(m.Name())
}

func unmarshalRequestJSON(js string, msg *dynamicpb.Message) error {
	if strings.TrimSpace(js) == "" {
		js = "{}"
	}
	// F4: unknown-field rejection stays ON for the request — a typo'd field name in a request the
	// user is authoring must be an error, not a silently dropped field, and the error carries its
	// own line:col.
	if err := (protojson.UnmarshalOptions{}).Unmarshal([]byte(js), msg); err != nil {
		return BadRequest(sanitizeUnmarshalError(err))
	}
	return nil
}

// protojsonSyntaxErrorPos pulls the "(line N:M)" position out of a protojson syntax error's own
// message text — the one part of it that is always safe to keep (P108 F13).
var protojsonSyntaxErrorPos = regexp.MustCompile(`\(line \d+:\d+\)`)

// sanitizeUnmarshalError: protojson's own JSON *syntax* error — as opposed to a
// semantic one like "unknown field", which only ever quotes a field name the user typed — quotes
// the offending token straight from the input (e.g. "proto: syntax error (line 1:15): invalid
// value abc"). js is built by stage 2's own {{secret}} substitution (resolve.go); a secret whose
// value carries a raw quote or backslash breaks the JSON string literal it landed in, and the
// quoted token in that case is a FRAGMENT of the secret — too short for the full-value replacer
// bridge/grpc.go's maskGrpcError builds to match, so it would otherwise reach the UI and error
// text unmasked. Keep only the line:col position, drop the token entirely, whenever the error
// names itself a syntax error; a semantic error never quotes a value and stays verbatim.
func sanitizeUnmarshalError(err error) string {
	msg := err.Error()
	if !strings.Contains(msg, "syntax error") {
		return msg
	}
	if pos := protojsonSyntaxErrorPos.FindString(msg); pos != "" {
		return "request JSON is invalid after variable substitution " + pos
	}
	return "request JSON is invalid after variable substitution"
}

func marshalResponseJSON(msg *dynamicpb.Message) (string, error) {
	// DiscardUnknown is ON for the response (a server running a newer schema than the .proto
	// the user supplied must not make its own answer unreadable) and EmitUnpopulated is ON
	// (a proto3 scalar at its zero value is otherwise absent, and "the field is missing" vs. "the
	// field is 0" is precisely the question this pane exists to answer).
	b, err := (protojson.MarshalOptions{EmitUnpopulated: true}).Marshal(msg)
	if err != nil {
		return "", err
	}
	return compactJSON(b)
}

// compactJSON strips the deliberate, per-build random whitespace protojson emits, so the same
// message always renders the same text.
func compactJSON(b []byte) (string, error) {
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		return "", err
	}
	return out.String(), nil
}

// terminalOutcome is the dispatch: nil for OK, a *Error for the two cases that mean "the
// call could not be made" (a context-caused Canceled/DeadlineExceeded or codes.Unavailable,
// which covers a refused connection, a TLS failure and a plaintext/TLS mismatch alike, so it
// is never split further here; bridge/grpc.go's own mapGrpcError is what gives it the three
// separate sentences, keyed on the error text). Every other code — including a real
// codes.ResourceExhausted from the message-size cap — is a result, not an error.
func terminalOutcome(ctx context.Context, err error) (code codes.Code, message string, asError *Error) {
	if err == nil {
		return codes.OK, "", nil
	}
	st := status.Convert(err)
	code, message = st.Code(), st.Message()
	if ctx.Err() != nil && (code == codes.Canceled || code == codes.DeadlineExceeded) {
		return code, message, Cancelled(message)
	}
	if code == codes.Unavailable {
		return code, message, Transport(message)
	}
	return code, message, nil
}

// Unary runs one request/response call (D7) — conn.Invoke over dynamicpb messages, no generated
// stubs anywhere.
func Unary(ctx context.Context, req CallRequest) (CallResult, error) {
	method, err := resolveMethod(ctx, req.Source, req.FullMethod)
	if err != nil {
		return CallResult{}, err
	}
	if method.IsStreamingClient() || method.IsStreamingServer() {
		return CallResult{}, BadRequest(req.FullMethod + " is a streaming method, not unary")
	}

	conn, err := dialConn(ctx, req.Target, req.TLS)
	if err != nil {
		return CallResult{}, err
	}
	defer conn.Close()

	callCtx, err := withMetadata(ctx, req.Metadata)
	if err != nil {
		return CallResult{}, err
	}
	timeout := req.Timeout
	cancelCall := context.CancelFunc(func() {})
	if timeout > 0 {
		callCtx, cancelCall = context.WithTimeout(callCtx, timeout)
	}
	defer cancelCall()

	in := dynamicpb.NewMessage(method.Input())
	if err := unmarshalRequestJSON(req.MessageJSON, in); err != nil {
		return CallResult{}, err
	}
	out := dynamicpb.NewMessage(method.Output())

	var header, trailer metadata.MD
	start := time.Now()
	callErr := conn.Invoke(callCtx, grpcPath(method), in, out,
		grpc.Header(&header), grpc.Trailer(&trailer), grpc.MaxCallRecvMsgSize(maxRecvMsgSize))
	elapsed := time.Since(start)

	if callErr != nil && ctx.Err() == nil && callDeadlineExpired(callCtx) {
		return CallResult{}, Timeout(fmt.Sprintf("no response from %s within %s", req.Target, timeout))
	}
	code, message, asError := terminalOutcome(callCtx, callErr)
	if asError != nil {
		return CallResult{}, asError
	}

	result := CallResult{
		Code: int32(code), CodeName: code.String(), StatusMessage: message,
		ElapsedMs: elapsed.Milliseconds(),
		Header:    mdToPairs(header), Trailer: mdToPairs(trailer),
	}
	if code == codes.OK {
		js, err := marshalResponseJSON(out)
		if err != nil {
			return CallResult{}, SchemaError("could not render the response: " + err.Error())
		}
		wire := proto.Size(out)
		result.Messages = []Message{{Seq: 0, JSON: js, WireBytes: wire, OffsetMs: elapsed.Milliseconds()}}
		result.MessageCount = 1
		result.MessageBytes = wire
	}
	return result, nil
}

// callDeadlineExpired reports whether ctx's deadline has passed, by the clock. The context's own
// error lags the clock under load, and the server enforces the propagated deadline too, so its
// DeadlineExceeded status can arrive first; it is still this call's deadline, not a server answer.
func callDeadlineExpired(ctx context.Context) bool {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	d, ok := ctx.Deadline()
	return ok && !time.Now().Before(d)
}

// ServerStream runs one server-streaming call, delivering each message through onMessage as
// it arrives (the arrival offset measured is real, and is the single most informative thing
// about a stream) and returning the terminal result once the stream ends. Cancelling ctx mid-
// stream (the Stop button, via Host.RunOp's derived context) ends the stream at the next RecvMsg
// with codes.Canceled — the messages already delivered stay delivered: they were already
// handed to onMessage, and the returned *Error's Partial field carries the true counts so a caller
// can still record a completed history entry.
func ServerStream(ctx context.Context, req CallRequest, onMessage func(Message)) (CallResult, error) {
	method, err := resolveMethod(ctx, req.Source, req.FullMethod)
	if err != nil {
		return CallResult{}, err
	}
	if !method.IsStreamingServer() || method.IsStreamingClient() {
		return CallResult{}, BadRequest(req.FullMethod + " is not a server-streaming method")
	}

	conn, stream, callCtx, err := openStream(ctx, req, method)
	if conn != nil {
		defer conn.Close()
	}
	if err != nil {
		return CallResult{}, err
	}

	return recvLoop(callCtx, stream, method, onMessage)
}

// openStream is ServerStream's own connect/send phase: dial, attach metadata, unmarshal the
// request, open the stream, send the one request message, and half-close. conn is returned
// whenever dial itself succeeded, even on a later error — the caller defers its Close once non-nil
// regardless, the same single defer conn.Close() this replaces used to cover every later failure.
func openStream(ctx context.Context, req CallRequest, method protoreflect.MethodDescriptor) (conn *grpc.ClientConn, stream grpc.ClientStream, callCtx context.Context, err error) {
	conn, err = dialConn(ctx, req.Target, req.TLS)
	if err != nil {
		return nil, nil, nil, err
	}

	callCtx, err = withMetadata(ctx, req.Metadata)
	if err != nil {
		return conn, nil, nil, err
	}

	in := dynamicpb.NewMessage(method.Input())
	if err = unmarshalRequestJSON(req.MessageJSON, in); err != nil {
		return conn, nil, nil, err
	}

	desc := &grpc.StreamDesc{StreamName: string(method.Name()), ServerStreams: true}
	stream, err = conn.NewStream(callCtx, desc, grpcPath(method), grpc.MaxCallRecvMsgSize(maxRecvMsgSize))
	if err != nil {
		_, _, asError := terminalOutcome(callCtx, err)
		if asError != nil {
			return conn, nil, callCtx, asError
		}
		return conn, nil, callCtx, Transport(err.Error())
	}
	if err = stream.SendMsg(in); err != nil {
		_, _, asError := terminalOutcome(callCtx, err)
		if asError != nil {
			return conn, nil, callCtx, asError
		}
		return conn, nil, callCtx, Transport(err.Error())
	}
	if err = stream.CloseSend(); err != nil {
		_, _, asError := terminalOutcome(callCtx, err)
		if asError != nil {
			return conn, nil, callCtx, asError
		}
		return conn, nil, callCtx, Transport(err.Error())
	}
	return conn, stream, callCtx, nil
}

// recvLoop is ServerStream's own receive loop: read one message at a time, deliver it via
// onMessage, and accumulate Messages/count/bytes for the terminal result — on every return path,
// including an error, so a caller can still see what arrived (the messages already
// delivered to onMessage stay delivered, and either the returned *Error's Partial field or the
// returned CallResult itself carries the true counts).
func recvLoop(callCtx context.Context, stream grpc.ClientStream, method protoreflect.MethodDescriptor, onMessage func(Message)) (CallResult, error) {
	start := time.Now()
	var count, totalBytes int
	var header metadata.MD
	messages := make([]Message, 0, maxStoredMessages)
	for {
		out := dynamicpb.NewMessage(method.Output())
		recvErr := stream.RecvMsg(out)
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			elapsed := time.Since(start)
			if header == nil {
				header, _ = stream.Header()
			}
			partial := CallResult{
				ElapsedMs: elapsed.Milliseconds(), Header: mdToPairs(header),
				Trailer: mdToPairs(stream.Trailer()), MessageCount: count, MessageBytes: totalBytes,
				Messages: messages,
			}
			code, message, asError := terminalOutcome(callCtx, recvErr)
			partial.Code, partial.CodeName, partial.StatusMessage = int32(code), code.String(), message
			if asError != nil {
				asError.Partial = &partial
				return CallResult{}, asError
			}
			return partial, nil
		}

		if header == nil {
			header, _ = stream.Header()
		}
		js, err := marshalResponseJSON(out)
		if err != nil {
			return CallResult{}, SchemaError("could not render a stream message: " + err.Error())
		}
		wire := proto.Size(out)
		msg := Message{Seq: count, JSON: js, WireBytes: wire, OffsetMs: time.Since(start).Milliseconds()}
		count++
		totalBytes += wire
		if len(messages) < maxStoredMessages {
			messages = append(messages, msg)
		}
		if onMessage != nil {
			onMessage(msg)
		}
	}

	elapsed := time.Since(start)
	if header == nil {
		header, _ = stream.Header()
	}
	return CallResult{
		Code: int32(codes.OK), CodeName: codes.OK.String(),
		ElapsedMs: elapsed.Milliseconds(), Header: mdToPairs(header), Trailer: mdToPairs(stream.Trailer()),
		MessageCount: count, MessageBytes: totalBytes, Messages: messages,
	}, nil
}
