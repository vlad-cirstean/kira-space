package grpcclient

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// F1: a reflection server that returns a.proto importing b.proto and b.proto importing a.proto
// must not send linker.link into unbounded recursion (a fatal, unrecoverable Go stack overflow).
// Bounded with a timeout so a regression here fails the test instead of hanging the suite.
func TestLinkerLink_CyclicDependency_ReturnsError(t *testing.T) {
	a := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("a.proto"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"b.proto"},
	}
	b := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("b.proto"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"a.proto"},
	}

	l := &linker{
		reg: new(protoregistry.Files),
		known: map[string]*descriptorpb.FileDescriptorProto{
			"a.proto": a,
			"b.proto": b,
		},
		linked:     map[string]bool{},
		inProgress: map[string]bool{},
	}

	done := make(chan error, 1)
	go func() {
		done <- l.link("a.proto")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error for a cyclic proto dependency, got nil")
		}
		if !strings.Contains(err.Error(), "cyclic") {
			t.Fatalf("expected a cycle-naming error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "a.proto") || !strings.Contains(err.Error(), "b.proto") {
			t.Fatalf("expected the error to name the cycle, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("linker.link did not return — cyclic dependency was not detected")
	}
}

// blockingReflectionServer accepts a ServerReflectionInfo stream and never sends a response, the
// same shape as an unresponsive or overloaded reflection endpoint (P108 F4).
type blockingReflectionServer struct {
	grpc_reflection_v1.UnimplementedServerReflectionServer
	unblock chan struct{}
	// started, when non-nil, receives a signal per accepted reflection stream.
	started chan struct{}
}

func (s *blockingReflectionServer) ServerReflectionInfo(stream grpc.BidiStreamingServer[grpc_reflection_v1.ServerReflectionRequest, grpc_reflection_v1.ServerReflectionResponse]) error {
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	select {
	case <-s.unblock:
	case <-stream.Context().Done():
	}
	return stream.Context().Err()
}

// TestResolveReflection_UnresponsiveServer_TimesOutBounded: a server that accepts the
// reflection stream and never answers must not block Describe forever — resolveReflection has to
// bound the whole resolution itself, since Describe (bridge/grpc.go) calls it directly rather than
// through the RunOp/Stop cancel path. defaultReflectionTimeout is lowered for the test so this
// does not need a real 30s wait.
func TestResolveReflection_UnresponsiveServer_TimesOutBounded(t *testing.T) {
	old := defaultReflectionTimeout
	defaultReflectionTimeout = 200 * time.Millisecond
	t.Cleanup(func() { defaultReflectionTimeout = old })

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := grpc.NewServer()
	blocker := &blockingReflectionServer{unblock: make(chan struct{})}
	defer close(blocker.unblock)
	grpc_reflection_v1.RegisterServerReflectionServer(srv, blocker)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	src := Source{Mode: SourceReflection, Target: lis.Addr().String()}
	start := time.Now()
	rdone := make(chan error, 1)
	go func() {
		_, err := resolveReflection(context.Background(), src)
		rdone <- err
	}()
	select {
	case err := <-rdone:
		if err == nil {
			t.Fatal("resolveReflection returned nil for an unresponsive server, want a timeout error")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("resolveReflection took %v to time out, want close to defaultReflectionTimeout (%v)", elapsed, defaultReflectionTimeout)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resolveReflection did not return — it is not bounded by defaultReflectionTimeout")
	}
}

// TestResolveSource_CancelOnlyAffectsOwnCaller is P168 Part 7 F10: one caller pressing Stop during
// a shared reflection resolution returns Cancelled for that caller only; the other waiter still
// gets the resolution's own outcome.
func TestResolveSource_CancelOnlyAffectsOwnCaller(t *testing.T) {
	old := defaultReflectionTimeout
	defaultReflectionTimeout = 2 * time.Second
	t.Cleanup(func() { defaultReflectionTimeout = old })

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := grpc.NewServer()
	blocker := &blockingReflectionServer{unblock: make(chan struct{}), started: make(chan struct{}, 1)}
	defer close(blocker.unblock)
	grpc_reflection_v1.RegisterServerReflectionServer(srv, blocker)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	src := Source{Mode: SourceReflection, Target: lis.Addr().String()}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	other := make(chan error, 1)
	go func() { _, err := resolveSource(cancelCtx, src); cancelled <- err }()
	go func() { _, err := resolveSource(context.Background(), src); other <- err }()
	// Shared resolution is in flight once the server sees its one reflection stream.
	select {
	case <-blocker.started:
	case <-time.After(10 * time.Second):
		t.Fatal("reflection stream never reached the server")
	}
	cancel()

	var ge *Error
	if err := <-cancelled; !errors.As(err, &ge) || ge.Code != CodeCancelled {
		t.Fatalf("cancelled caller err = %v, want %s", err, CodeCancelled)
	}
	if err := <-other; !errors.As(err, &ge) || ge.Code != CodeTimeout {
		t.Fatalf("other caller err = %v, want %s (the shared timeout), not a cancellation", err, CodeTimeout)
	}
}
