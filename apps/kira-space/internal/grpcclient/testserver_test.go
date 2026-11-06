package grpcclient

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// echoProtoSource is the probe's own service (P11 F1/F2/F5): one message with a well-known-type
// import (F1's own case) and two methods — Unary and a genuine server-streaming method — compiled
// with no protoc binary and served with no generated code of any kind (F1, F5).
const echoProtoSource = `syntax = "proto3";
package kira.probe.v1;

import "google/protobuf/timestamp.proto";

message EchoRequest {
  string text = 1;
  int32 index = 2;
  int32 count = 3;
}

message EchoResponse {
  string text = 1;
  int32 index = 2;
  google.protobuf.Timestamp at = 3;
}

service Echo {
  rpc Unary(EchoRequest) returns (EchoResponse);
  rpc ServerStream(EchoRequest) returns (stream EchoResponse);
}
`

// writeProto writes src to name inside a fresh t.TempDir(), returning its path.
func writeProto(t *testing.T, name, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// compileService compiles protoPath the same way resolveProto does, returning the linked
// kira.probe.v1.Echo service descriptor a real grpc.Server registers against below — F1/F5's own
// technique: a real server built from a compiled descriptor with no generated code at all.
func compileService(t *testing.T, protoPath string) protoreflect.ServiceDescriptor {
	t.Helper()
	r, err := resolveProto(context.Background(), Source{Mode: SourceProto, ProtoPath: protoPath})
	if err != nil {
		t.Fatalf("compile %s: %v", protoPath, err)
	}
	var svc protoreflect.ServiceDescriptor
	r.files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if fd.Services().Len() > 0 {
			svc = fd.Services().Get(0)
			return false
		}
		return true
	})
	if svc == nil {
		t.Fatalf("no service found compiling %s", protoPath)
	}
	return svc
}

// echoImpl is the dynamic (no generated stubs) service implementation registered onto a real
// grpc.Server below. streamMessages/streamSleep make F5's arrival-offset claim testable with real,
// non-zero spacing; unaryStatus lets a test make Unary answer with an arbitrary status.
type echoImpl struct {
	unaryStatus    *status.Status // nil = echo the request back as the response
	unaryHugeReply int            // > 0: Unary ignores the request and replies with this many bytes of text — D15's cap test
	streamMessages int
	streamSleep    time.Duration
	streamStatus   *status.Status // nil = end with codes.OK after streamMessages
}

// registerEcho registers svc onto s with generic dynamicpb-based handlers (grpc.MethodHandler's
// `dec func(any) error` and grpc.StreamHandler's ServerStream both work uniformly over a
// *dynamicpb.Message, since it implements proto.Message and the default "proto" codec just calls
// proto.Marshal/Unmarshal) — no generated code anywhere, mirroring the probe's own registration.
// `ss: nil` on RegisterService skips its type-check against HandlerType entirely, which
// is what makes a fully dynamic registration possible with no interface to satisfy.
func registerEcho(s *grpc.Server, svc protoreflect.ServiceDescriptor, impl *echoImpl) {
	unary := svc.Methods().ByName("Unary")
	stream := svc.Methods().ByName("ServerStream")

	desc := &grpc.ServiceDesc{
		ServiceName: string(svc.FullName()),
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: string(unary.Name()),
				Handler: func(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
					in := dynamicpb.NewMessage(unary.Input())
					if err := dec(in); err != nil {
						return nil, err
					}
					if impl.unaryStatus != nil {
						return nil, impl.unaryStatus.Err()
					}
					out := dynamicpb.NewMessage(unary.Output())
					if impl.unaryHugeReply > 0 {
						out.Set(unary.Output().Fields().ByName("text"), protoreflect.ValueOfString(strings.Repeat("x", impl.unaryHugeReply)))
						return out, nil
					}
					out.Set(unary.Output().Fields().ByName("text"), in.Get(unary.Input().Fields().ByName("text")))
					out.Set(unary.Output().Fields().ByName("index"), in.Get(unary.Input().Fields().ByName("index")))
					return out, nil
				},
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    string(stream.Name()),
				ServerStreams: true,
				Handler: func(_ any, ss grpc.ServerStream) error {
					in := dynamicpb.NewMessage(stream.Input())
					if err := ss.RecvMsg(in); err != nil {
						return err
					}
					textField := stream.Input().Fields().ByName("text")
					outTextField := stream.Output().Fields().ByName("text")
					outIndexField := stream.Output().Fields().ByName("index")
					for i := 0; i < impl.streamMessages; i++ {
						if impl.streamSleep > 0 && i > 0 {
							time.Sleep(impl.streamSleep)
						}
						out := dynamicpb.NewMessage(stream.Output())
						out.Set(outTextField, in.Get(textField))
						out.Set(outIndexField, protoreflect.ValueOfInt32(int32(i)))
						if err := ss.SendMsg(out); err != nil {
							return err
						}
					}
					if impl.streamStatus != nil {
						return impl.streamStatus.Err()
					}
					return nil
				},
			},
		},
	}
	s.RegisterService(desc, nil)
}

// echoServer is one running in-process server plus its address and a Stop cleanup, registered for
// t.Cleanup already.
type echoServer struct {
	addr string
}

// startEchoServer compiles protoSource, serves it over a real loopback TCP listener (grpc-go's own
// HTTP/2 client needs a real net.Conn — F7 — so bufconn's in-memory pipe is not a substitute for
// what this phase actually dials), optionally with reflection registered, and returns its address.
func startEchoServer(t *testing.T, protoSource string, impl *echoImpl, withReflection bool) echoServer {
	t.Helper()
	protoPath := writeProto(t, "echo.proto", protoSource)
	svc := compileService(t, protoPath)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	registerEcho(s, svc, impl)
	if withReflection {
		// grpc-go's own reflection.Register(s) answers FileContainingSymbol/FileByFilename from
		// protoregistry.GlobalFiles, exactly as a real server generated by protoc would (its
		// generated code registers its own descriptors globally via an init() function this test
		// fixture stands in for). This is the test SERVER's own registration, playing the role of
		// an arbitrary real gRPC server — a different actor from internal/grpcclient's own client
		// code under test, which is what the "never touch GlobalFiles" rule is about.
		registerGlobalForReflection(t, svc.ParentFile())
		reflection.Register(s)
	}
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	addr := lis.Addr().String()
	waitEchoServerReady(t, addr)
	return echoServer{addr: addr}
}

// waitEchoServerReady blocks until the server behind addr completes a real request,
// instead of returning the instant net.Listen succeeds. net.Listen binds the socket immediately,
// so a client's TCP connect can succeed into the kernel backlog well before the s.Serve goroutine
// above — let alone its per-connection HTTP/2 dispatch — has run: under heavy load the
// gap is wide enough that a real request lands on a connection nobody is properly serving yet and
// sees a raw io.EOF instead of a response (the flake this fixes: descriptors_test.go's own
// NoReflection case, "error = EOF" instead of a proper E_BAD_REQUEST).
//
// This issues the exact RPC descriptors_test.go's flake raced on
// (grpc_reflection_v1.ServerReflectionInfo — a real Send+Recv, a stronger signal than only
// checking connectivity.Ready, which proves the HTTP/2 handshake completed but not that the
// server's own per-connection dispatch has reached the point of answering) and retries until it
// gets back any properly-coded gRPC status — a real ListServices reply when reflection is
// registered, or the well-formed codes.Unimplemented a plain server correctly answers with when it
// is not — rather than a transport-level failure (io.EOF/codes.Unknown, codes.Unavailable).
//
// Verified this closes the race it targets: 50 consecutive `go test -count=50` runs of this
// package alone, clean. It does not fully suppress the flake under this sandbox's own worse-than-
// typical contention (confirmed reproducible with zero Docker involvement, under a synthetic
// all-cores CPU stress) — that residual is a different, broader hazard (any fresh connection's
// first RPC, not only one made right after a listener opens) whose full fix is a client-side retry
// inside internal/grpcclient's own production dial path.
func waitEchoServerReady(t *testing.T, addr string) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer conn.Close()

	deadline := time.Now().Add(5 * time.Second)
	for {
		probeCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		probeErr := probeReflection(probeCtx, conn)
		cancel()
		if probeReady(probeErr) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("echo test server at %s never became ready: %v", addr, probeErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// probeReflection issues one ServerReflectionInfo round trip (send + recv), the same RPC
// reflect.go's own resolveReflection makes first against a real target.
func probeReflection(ctx context.Context, conn *grpc.ClientConn) error {
	stream, err := grpc_reflection_v1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{ListServices: "*"},
	}); err != nil {
		return err
	}
	_, err = stream.Recv()
	return err
}

// probeReady reports whether err is a genuine gRPC-level answer (nil, a real ListServices
// response, or a well-formed codes.Unimplemented) rather than a transport-level failure that means
// the server is not actually serving yet.
func probeReady(err error) bool {
	if err == nil {
		return true
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.Unknown, codes.DeadlineExceeded:
		return false
	default:
		return true
	}
}

func registerGlobalForReflection(t *testing.T, fd protoreflect.FileDescriptor) {
	t.Helper()
	if _, err := protoregistry.GlobalFiles.FindFileByPath(fd.Path()); err == nil {
		return // already present (a well-known type, or a previous test's identical fixture)
	}
	imports := fd.Imports()
	for i := 0; i < imports.Len(); i++ {
		registerGlobalForReflection(t, imports.Get(i).FileDescriptor)
	}
	if err := protoregistry.GlobalFiles.RegisterFile(fd); err != nil {
		t.Fatalf("registering %s into GlobalFiles for the test server: %v", fd.Path(), err)
	}
}
