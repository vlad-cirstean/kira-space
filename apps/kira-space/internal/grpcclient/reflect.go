package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// reflectKind is which of ServerReflectionRequest's two file-locating variants a fetch uses.
type reflectKind int

const (
	byFileContainingSymbol reflectKind = iota
	byFilename
)

// fetchWith runs one reflection request/response round trip — send, receive, then either the
// error the server returned or the file descriptor bytes — the shape v1Transport.fetch and
// v1AlphaTransport.fetch shared exactly except for their own generated Req/Resp package.
// getError reports whether resp carries an ErrorResponse; getFiles extracts the
// FileDescriptorProto bytes, nil when the response carried neither.
func fetchWith[Req, Resp any](
	send func(Req) error, recv func() (Resp, error), req Req,
	getError func(Resp) (code int32, msg string, ok bool),
	getFiles func(Resp) [][]byte,
) ([][]byte, error) {
	if err := send(req); err != nil {
		return nil, err
	}
	resp, err := recv()
	if err != nil {
		return nil, err
	}
	if code, msg, ok := getError(resp); ok {
		return nil, status.Error(codes.Code(code), msg)
	}
	files := getFiles(resp)
	if files == nil {
		return nil, fmt.Errorf("grpcclient: reflection: unexpected response")
	}
	return files, nil
}

// listServicesWith runs one reflection ListServices request/response round trip — the shape
// v1Transport.listServices and v1AlphaTransport.listServices shared exactly except for their own
// generated Req/Resp package and (v1alpha) an extra type conversion. getError reports
// an ErrorResponse; getServices converts resp's own service list into v1's ServiceResponse shape,
// nil when the response carried neither.
func listServicesWith[Req, Resp any](
	send func(Req) error, recv func() (Resp, error), req Req,
	getError func(Resp) (code int32, msg string, ok bool),
	getServices func(Resp) []*grpc_reflection_v1.ServiceResponse,
) ([]string, error) {
	if err := send(req); err != nil {
		return nil, err
	}
	resp, err := recv()
	if err != nil {
		return nil, err
	}
	if code, msg, ok := getError(resp); ok {
		return nil, status.Error(codes.Code(code), msg)
	}
	services := getServices(resp)
	if services == nil {
		return nil, fmt.Errorf("grpcclient: reflection: unexpected response to ListServices")
	}
	return filterServiceNames(services), nil
}

// reflectionTransport is the one seam between the version-negotiation this file does (v1, falling
// back to v1alpha on codes.Unimplemented) and the recursive dependency-linking algorithm
// resolveViaTransport runs — v1 and v1alpha are structurally identical wire protocols with two
// distinct generated Go packages, so this interface is what lets that algorithm be written once.
type reflectionTransport interface {
	listServices() ([]string, error)
	fetch(kind reflectKind, value string) ([][]byte, error)
}

// ---- v1 ----

type v1Transport struct {
	stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoClient
}

func newV1Transport(ctx context.Context, conn *grpc.ClientConn) (*v1Transport, error) {
	stream, err := grpc_reflection_v1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &v1Transport{stream: stream}, nil
}

func (t *v1Transport) listServices() ([]string, error) {
	req := &grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{ListServices: "*"},
	}
	return listServicesWith(t.stream.Send, t.stream.Recv, req,
		func(resp *grpc_reflection_v1.ServerReflectionResponse) (int32, string, bool) {
			if e := resp.GetErrorResponse(); e != nil {
				return e.GetErrorCode(), e.GetErrorMessage(), true
			}
			return 0, "", false
		},
		func(resp *grpc_reflection_v1.ServerReflectionResponse) []*grpc_reflection_v1.ServiceResponse {
			list := resp.GetListServicesResponse()
			if list == nil {
				return nil
			}
			return list.GetService()
		},
	)
}

func (t *v1Transport) fetch(kind reflectKind, value string) ([][]byte, error) {
	req := &grpc_reflection_v1.ServerReflectionRequest{}
	switch kind {
	case byFileContainingSymbol:
		req.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: value}
	case byFilename:
		req.MessageRequest = &grpc_reflection_v1.ServerReflectionRequest_FileByFilename{FileByFilename: value}
	}
	return fetchWith(t.stream.Send, t.stream.Recv, req,
		func(resp *grpc_reflection_v1.ServerReflectionResponse) (int32, string, bool) {
			if e := resp.GetErrorResponse(); e != nil {
				return e.GetErrorCode(), e.GetErrorMessage(), true
			}
			return 0, "", false
		},
		func(resp *grpc_reflection_v1.ServerReflectionResponse) [][]byte {
			fd := resp.GetFileDescriptorResponse()
			if fd == nil {
				return nil
			}
			return fd.GetFileDescriptorProto()
		},
	)
}

// ---- v1alpha (D4's fallback, on codes.Unimplemented from v1 — the same negotiation
// grpcreflect.NewClientAuto performs, F2) ----

type v1AlphaTransport struct {
	stream grpc_reflection_v1alpha.ServerReflection_ServerReflectionInfoClient
}

func newV1AlphaTransport(ctx context.Context, conn *grpc.ClientConn) (*v1AlphaTransport, error) {
	stream, err := grpc_reflection_v1alpha.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &v1AlphaTransport{stream: stream}, nil
}

func (t *v1AlphaTransport) listServices() ([]string, error) {
	req := &grpc_reflection_v1alpha.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1alpha.ServerReflectionRequest_ListServices{ListServices: "*"},
	}
	return listServicesWith(t.stream.Send, t.stream.Recv, req,
		func(resp *grpc_reflection_v1alpha.ServerReflectionResponse) (int32, string, bool) {
			if e := resp.GetErrorResponse(); e != nil {
				return e.GetErrorCode(), e.GetErrorMessage(), true
			}
			return 0, "", false
		},
		func(resp *grpc_reflection_v1alpha.ServerReflectionResponse) []*grpc_reflection_v1.ServiceResponse {
			list := resp.GetListServicesResponse()
			if list == nil {
				return nil
			}
			names := make([]*grpc_reflection_v1.ServiceResponse, 0, len(list.GetService()))
			for _, s := range list.GetService() {
				names = append(names, &grpc_reflection_v1.ServiceResponse{Name: s.GetName()})
			}
			return names
		},
	)
}

func (t *v1AlphaTransport) fetch(kind reflectKind, value string) ([][]byte, error) {
	req := &grpc_reflection_v1alpha.ServerReflectionRequest{}
	switch kind {
	case byFileContainingSymbol:
		req.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: value}
	case byFilename:
		req.MessageRequest = &grpc_reflection_v1alpha.ServerReflectionRequest_FileByFilename{FileByFilename: value}
	}
	return fetchWith(t.stream.Send, t.stream.Recv, req,
		func(resp *grpc_reflection_v1alpha.ServerReflectionResponse) (int32, string, bool) {
			if e := resp.GetErrorResponse(); e != nil {
				return e.GetErrorCode(), e.GetErrorMessage(), true
			}
			return 0, "", false
		},
		func(resp *grpc_reflection_v1alpha.ServerReflectionResponse) [][]byte {
			fd := resp.GetFileDescriptorResponse()
			if fd == nil {
				return nil
			}
			return fd.GetFileDescriptorProto()
		},
	)
}

// filterServiceNames drops the reflection service's own entries (grpc.reflection.v1.*,
// grpc.reflection.v1alpha.*) — showing gRPC's own bookkeeping service in a schema browser serves
// no purpose.
func filterServiceNames(services []*grpc_reflection_v1.ServiceResponse) []string {
	out := make([]string, 0, len(services))
	for _, s := range services {
		if strings.HasPrefix(s.GetName(), "grpc.reflection.") {
			continue
		}
		out = append(out, s.GetName())
	}
	return out
}

// defaultReflectionTimeout bounds a whole reflection resolution (negotiate, ListServices, every
// FileContainingSymbol/FileByFilename fetch and link) — Describe calls grpcclient.Describe
// directly, not through the RunOp/Stop path, so nothing else in this package ever cancels a
// reflection stream a server accepts and then never answers. Without this, that leaves the
// goroutine and connection blocked forever and the UI spinner never ends. The connect and
// handshake share this deadline but are bounded sooner by defaultDialTimeout.
// A var, not a const, so reflect_test.go can lower it for a bounded regression test without a
// real 30s wait.
var defaultReflectionTimeout = 15 * time.Second

// resolveReflection is the reflection source: one ServerReflectionInfo bidi stream, ListServices,
// then FileContainingSymbol per service, linking every returned FileDescriptorProto into this
// Source's own private registry — recursing through FileByFilename for any dependency the
// server did not volunteer (grpc-go volunteers them, the protocol does not require it).
func resolveReflection(ctx context.Context, src Source) (*resolved, error) {
	if src.Target == "" {
		return nil, BadRequest("a target is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultReflectionTimeout)
	defer cancel()

	conn, err := dialConn(ctx, src.Target, src.TLS)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	callCtx, err := withMetadata(ctx, src.Metadata)
	if err != nil {
		return nil, err
	}

	transport, mode, services, err := negotiateAndListServices(callCtx, conn)
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			// F11: distinguishable from every other failure by code alone — "This server does not
			// expose gRPC reflection" (D17) rather than an opaque "call failed".
			return nil, SchemaError("This server does not expose gRPC reflection. Supply a .proto file instead.")
		}
		if isTransportGlitch(err) {
			// D17: retryOnTransportGlitch already retried this exact failure firstRPCGlitchRetries
			// times — surfacing the bare "EOF" negotiateAndListServices returns here is not
			// legible on its own, so name what actually happened instead.
			return nil, Transport("the reflection request ended the connection unexpectedly (" + err.Error() + ") after retrying")
		}
		return nil, reflectionFailure(ctx, src.Target, err)
	}

	l := &linker{
		transport:  transport,
		reg:        new(protoregistry.Files),
		known:      map[string]*descriptorpb.FileDescriptorProto{},
		linked:     map[string]bool{},
		inProgress: map[string]bool{},
	}

	for _, svc := range services {
		protos, err := transport.fetch(byFileContainingSymbol, svc)
		if err != nil {
			return nil, reflectionFailure(ctx, src.Target, err)
		}
		if err := absorb(l.known, protos); err != nil {
			return nil, SchemaError(err.Error())
		}
	}
	for path := range l.known {
		if err := l.link(path); err != nil {
			return nil, SchemaError(err.Error())
		}
	}

	return &resolved{files: l.reg, mode: mode}, nil
}

// reflectionFailure names a deadline expiry as E_TIMEOUT, with the target and the wait; any other
// failure stays a transport error. gRPC's own deadline timer can fire before ctx.Err() reports the
// expiry, so the stream's DeadlineExceeded status counts too.
func reflectionFailure(ctx context.Context, target string, err error) *Error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return Timeout(fmt.Sprintf("no reflection answer from %s within %s", target, defaultReflectionTimeout))
	}
	return Transport(err.Error())
}

// linker resolves one file's own dependency closure into reg — resolveReflection's own recursive
// link step, a method rather than an inline closure so the recursion has a receiver to call
// through.
type linker struct {
	transport  reflectionTransport
	reg        *protoregistry.Files
	known      map[string]*descriptorpb.FileDescriptorProto
	linked     map[string]bool
	inProgress map[string]bool
	stack      []string
}

func (l *linker) link(path string) error {
	if l.linked[path] {
		return nil
	}
	if l.inProgress[path] {
		// A reflection server controls the descriptors it returns, so a cyclic import
		// (a.proto -> b.proto -> a.proto) is untrusted input, not a programming error. Reject
		// it explicitly instead of recursing forever into a Go stack overflow, which is fatal
		// and unrecoverable — it would kill the whole process, not just this request.
		cycle := append(append([]string{}, l.stack...), path)
		return fmt.Errorf("cyclic proto dependency: %s", strings.Join(cycle, " -> "))
	}
	l.inProgress[path] = true
	l.stack = append(l.stack, path)
	defer func() {
		delete(l.inProgress, path)
		l.stack = l.stack[:len(l.stack)-1]
	}()

	raw, ok := l.known[path]
	if !ok {
		protos, err := l.transport.fetch(byFilename, path)
		if err != nil {
			return err
		}
		if err := absorb(l.known, protos); err != nil {
			return err
		}
		raw, ok = l.known[path]
		if !ok {
			return fmt.Errorf("grpcclient: reflection: server did not return %s", path)
		}
	}
	for _, dep := range raw.GetDependency() {
		if err := l.link(dep); err != nil {
			return err
		}
	}
	fd, err := protodesc.NewFile(raw, l.reg)
	if err != nil {
		return fmt.Errorf("linking %s: %w", path, err)
	}
	if err := l.reg.RegisterFile(fd); err != nil {
		// F14: this is a private registry (never GlobalFiles), so a duplicate returns an
		// error rather than panicking.
		return fmt.Errorf("registering %s: %w", path, err)
	}
	l.linked[path] = true
	return nil
}

// negotiateAndListServices tries v1 first — opening the stream and sending ListServices — and
// falls back to v1alpha on codes.Unimplemented from either step, the same negotiation
// grpcreflect.NewClientAuto performs, here in ~20 lines rather than a third-party dependency.
// The service list comes back alongside the chosen transport so the one ListServices round
// trip this negotiation needs is never repeated. Each round trip (v1's, and v1alpha's own retry on
// v1's Unimplemented) goes through retryOnTransportGlitch — the diagnosed defect is not
// specific to whichever RPC happens to be literally first on the connection, so both get the same
// protection.
func negotiateAndListServices(ctx context.Context, conn *grpc.ClientConn) (reflectionTransport, string, []string, error) {
	v1, services, err := retryOnTransportGlitch(ctx, func() (*v1Transport, []string, error) {
		v1, err := newV1Transport(ctx, conn)
		if err != nil {
			return nil, nil, err
		}
		services, err := v1.listServices()
		return v1, services, err
	})
	if err == nil {
		return v1, "reflection-v1", services, nil
	}
	if status.Code(err) != codes.Unimplemented {
		return nil, "", nil, err
	}

	v1alpha, services, err := retryOnTransportGlitch(ctx, func() (*v1AlphaTransport, []string, error) {
		v1alpha, err := newV1AlphaTransport(ctx, conn)
		if err != nil {
			return nil, nil, err
		}
		services, err := v1alpha.listServices()
		return v1alpha, services, err
	})
	if err != nil {
		return nil, "", nil, err
	}
	return v1alpha, "reflection-v1alpha", services, nil
}

// firstRPCGlitchRetries bounds retryOnTransportGlitch's retries of an early reflection round trip
// on a fresh connection. Measured in this container's own synthetic CPU-stress repro
// (stress-ng --cpu 4 --cpu-load 100, a fresh OS process per iteration): up to ~11% of runs failed
// with a bare io.EOF before this fix. Direct instrumentation (not committed) pinned the failure to
// the v1alpha round trip specifically — v1's own negotiation answered a well-formed Unimplemented
// in under 1ms in every failing run, so the race is in whichever reflection RPC follows shortly
// after connection dial, not literally "the first one"; both v1's and v1alpha's own round trips go
// through the same retry for that reason. 4 retries with firstRPCGlitchDelay measured 0 failures
// across 1000 in-process iterations under the same stressor (baseline: 4/500 with no fix).
const firstRPCGlitchRetries = 4

// firstRPCGlitchDelay is the pause retryOnTransportGlitch takes before a retry — long enough to
// let the connection's own transport goroutines get scheduled once under this container's
// own worst observed contention (a synthetic all-cores CPU stressor); an immediate (no-delay)
// retry on the same connection, measured separately, did not lower the failure rate at all — the
// glitch is tied to scheduler starvation, not a race an instant retry can win.
const firstRPCGlitchDelay = 50 * time.Millisecond

// retryOnTransportGlitch runs attempt up to firstRPCGlitchRetries+1 times, retrying only when its
// error is a bare transport-level failure (isTransportGlitch — io.EOF, which status.Code reports
// as codes.Unknown) rather than a well-formed gRPC status. This is a genuine product defect, not a
// test artifact: grpc-go's HTTP/2 transport occasionally ends an early stream on a fresh
// connection this way under scheduling pressure, even though the connection is otherwise READY and
// fully serving — the residual the waitEchoServerReady probe comment predicted (\"any fresh
// connection's first RPC, not only one made right after a listener opens\"). dialConn builds one
// *grpc.ClientConn per call, so every resolveReflection call is exposed to this.
//
// A real codes.Unimplemented (the server genuinely does not speak this reflection version) is
// never retried — v1's Unimplemented is the normal v1alpha fallback trigger, handled by the
// caller — and no other status is retried either, so a genuine business error (or a
// differently-broken server) still surfaces as-is rather than being masked or retried into a
// false pass.
func retryOnTransportGlitch[T any](ctx context.Context, attempt func() (T, []string, error)) (T, []string, error) {
	var zero T
	for i := 0; ; i++ {
		v, services, err := attempt()
		if err == nil {
			return v, services, nil
		}
		if status.Code(err) == codes.Unimplemented || !isTransportGlitch(err) || i >= firstRPCGlitchRetries {
			return zero, nil, err
		}
		select {
		case <-ctx.Done():
			return zero, nil, err
		case <-time.After(firstRPCGlitchDelay):
		}
	}
}

// isTransportGlitch reports whether err is a bare transport-level failure carrying no real gRPC
// status — codes.Unknown is what status.Code wraps a plain io.EOF (or similar) into — rather than
// a status some server actually chose to answer with.
func isTransportGlitch(err error) bool {
	return status.Code(err) == codes.Unknown
}

// absorb decodes every raw FileDescriptorProto byte blob a response carried and stores it by
// path — a single FileContainingSymbol response commonly bundles the transitive dependencies too
// ("deps fetched=2" from one response), so this is what makes them available to link() without
// a second round trip.
func absorb(known map[string]*descriptorpb.FileDescriptorProto, protos [][]byte) error {
	for _, b := range protos {
		var fd descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(b, &fd); err != nil {
			return fmt.Errorf("grpcclient: reflection: decoding FileDescriptorProto: %w", err)
		}
		known[fd.GetName()] = &fd
	}
	return nil
}
