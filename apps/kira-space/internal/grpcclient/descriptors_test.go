package grpcclient

import (
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// TestDescribe_Proto_WellKnownImport is §6.2 descriptors_test.go case 1: a .proto with a
// well-known import compiles and projects the right method set, streaming flags included (F1).
func TestDescribe_Proto_WellKnownImport(t *testing.T) {
	protoPath := writeProto(t, "echo.proto", echoProtoSource)
	schema, err := Describe(context.Background(), Source{Mode: SourceProto, ProtoPath: protoPath})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if schema.Mode != string(SourceProto) {
		t.Errorf("Mode = %q, want %q", schema.Mode, SourceProto)
	}
	if len(schema.Services) != 1 || schema.Services[0].Name != "Echo" {
		t.Fatalf("Services = %+v, want exactly one named Echo", schema.Services)
	}
	methods := schema.Services[0].Methods
	if len(methods) != 2 {
		t.Fatalf("Methods = %+v, want 2", methods)
	}
	byName := map[string]Method{}
	for _, m := range methods {
		byName[m.Name] = m
	}
	unary, ok := byName["Unary"]
	if !ok || unary.ClientStreaming || unary.ServerStreaming {
		t.Errorf("Unary = %+v, want a non-streaming method", unary)
	}
	if unary.FullName != "kira.probe.v1.Echo.Unary" {
		t.Errorf("Unary.FullName = %q", unary.FullName)
	}
	if unary.InputType != "kira.probe.v1.EchoRequest" || unary.OutputType != "kira.probe.v1.EchoResponse" {
		t.Errorf("Unary in/out = %s/%s", unary.InputType, unary.OutputType)
	}
	if unary.RequestTemplate == "" {
		t.Error("Unary.RequestTemplate is empty, want the seeded-empty-instance JSON")
	}
	stream, ok := byName["ServerStream"]
	if !ok || stream.ClientStreaming || !stream.ServerStreaming {
		t.Errorf("ServerStream = %+v, want server-streaming only", stream)
	}
}

// TestDescribe_Reflection_SameSchemaAsProto is §6.2 case 2: reflection against a real server
// projects the SAME Schema the .proto source does for the same file — the check that D4's
// one-abstraction claim holds.
func TestDescribe_Reflection_SameSchemaAsProto(t *testing.T) {
	protoPath := writeProto(t, "echo.proto", echoProtoSource)
	protoSchema, err := Describe(context.Background(), Source{Mode: SourceProto, ProtoPath: protoPath})
	if err != nil {
		t.Fatalf("Describe(proto): %v", err)
	}

	server := startEchoServer(t, echoProtoSource, &echoImpl{}, true)
	reflSchema, err := Describe(context.Background(), Source{Mode: SourceReflection, Target: server.addr})
	if err != nil {
		t.Fatalf("Describe(reflection): %v", err)
	}

	if len(reflSchema.Services) != len(protoSchema.Services) {
		t.Fatalf("reflection Services = %+v, proto Services = %+v", reflSchema.Services, protoSchema.Services)
	}
	rs, ps := reflSchema.Services[0], protoSchema.Services[0]
	if rs.Name != ps.Name || len(rs.Methods) != len(ps.Methods) {
		t.Fatalf("reflection service = %+v, proto service = %+v", rs, ps)
	}
	for i := range ps.Methods {
		pm, rm := ps.Methods[i], rs.Methods[i]
		if pm.FullName != rm.FullName || pm.ClientStreaming != rm.ClientStreaming ||
			pm.ServerStreaming != rm.ServerStreaming || pm.InputType != rm.InputType || pm.OutputType != rm.OutputType {
			t.Errorf("method %d: reflection=%+v proto=%+v", i, rm, pm)
		}
	}
}

// TestDescribe_Reflection_NoReflection_YieldsSchemaError is §6.2 case 3: a server without
// reflection yields E_BAD_REQUEST carrying F11's Unimplemented cause.
func TestDescribe_Reflection_NoReflection_YieldsSchemaError(t *testing.T) {
	server := startEchoServer(t, echoProtoSource, &echoImpl{}, false)
	_, err := Describe(context.Background(), Source{Mode: SourceReflection, Target: server.addr})
	if err == nil {
		t.Fatal("Describe against a server with no reflection: want an error, got nil")
	}
	gerr, ok := err.(*Error)
	if !ok || gerr.Code != CodeSchema {
		t.Fatalf("error = %v, want a *Error with code %s", err, CodeSchema)
	}
	if !strings.Contains(gerr.Message, "reflection") {
		t.Errorf("message = %q, want it to name reflection (D17)", gerr.Message)
	}
}

// TestDescribe_Reflection_RecursiveFallback is §6.2 case 4: a dependency the server does not
// volunteer is fetched by the recursive FileByFilename fallback (F2) — driven by a stub reflection
// server that answers FileContainingSymbol with the leaf file only.
func TestDescribe_Reflection_RecursiveFallback(t *testing.T) {
	protoPath := writeProto(t, "echo.proto", echoProtoSource)
	compiled, err := resolveProto(context.Background(), Source{Mode: SourceProto, ProtoPath: protoPath})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	var leaf, dep *descriptorpb.FileDescriptorProto
	compiledRangeFiles(compiled, func(path string, raw *descriptorpb.FileDescriptorProto) {
		if path == "echo.proto" {
			leaf = raw
		}
		if path == "google/protobuf/timestamp.proto" {
			dep = raw
		}
	})
	if leaf == nil || dep == nil {
		t.Fatalf("could not find both echo.proto and its timestamp.proto dependency in the compiled set")
	}

	stub := &stubReflectionServer{
		services:      []string{"kira.probe.v1.Echo"},
		fileForSymbol: map[string]*descriptorpb.FileDescriptorProto{"kira.probe.v1.Echo": leaf},
		fileByName:    map[string]*descriptorpb.FileDescriptorProto{"google/protobuf/timestamp.proto": dep},
	}
	addr := startStubReflectionServer(t, stub)

	schema, err := Describe(context.Background(), Source{Mode: SourceReflection, Target: addr})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(schema.Services) != 1 || schema.Services[0].Name != "Echo" {
		t.Fatalf("Services = %+v, want the Echo service resolved via the fallback fetch", schema.Services)
	}
}

// TestDescribe_TwoSourcesSamePackage_NeitherPanics is §6.2 case 5 and F14's own regression test:
// two sources whose files declare the same package both resolve, and neither panics — the crash
// protoregistry.GlobalFiles would otherwise cause (F14).
func TestDescribe_TwoSourcesSamePackage_NeitherPanics(t *testing.T) {
	const protoA = `syntax = "proto3";
package kira.dup.v1;
message A { string name = 1; }
service SvcA { rpc M(A) returns (A); }
`
	const protoB = `syntax = "proto3";
package kira.dup.v1;
message B { string name = 1; }
service SvcB { rpc M(B) returns (B); }
`
	pathA := writeProto(t, "a.proto", protoA)
	pathB := writeProto(t, "b.proto", protoB)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Describe panicked on a duplicate-package source: %v", r)
		}
	}()

	schemaA, err := Describe(context.Background(), Source{Mode: SourceProto, ProtoPath: pathA})
	if err != nil {
		t.Fatalf("Describe(a.proto): %v", err)
	}
	schemaB, err := Describe(context.Background(), Source{Mode: SourceProto, ProtoPath: pathB})
	if err != nil {
		t.Fatalf("Describe(b.proto): %v", err)
	}
	if len(schemaA.Services) != 1 || schemaA.Services[0].Name != "SvcA" {
		t.Errorf("schemaA.Services = %+v", schemaA.Services)
	}
	if len(schemaB.Services) != 1 || schemaB.Services[0].Name != "SvcB" {
		t.Errorf("schemaB.Services = %+v", schemaB.Services)
	}
}

// ---- fixtures for TestDescribe_Reflection_RecursiveFallback ----

func compiledRangeFiles(r *resolved, fn func(path string, raw *descriptorpb.FileDescriptorProto)) {
	r.files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		fn(fd.Path(), protodesc.ToFileDescriptorProto(fd))
		return true
	})
}

type stubReflectionServer struct {
	grpc_reflection_v1.UnimplementedServerReflectionServer
	services      []string
	fileForSymbol map[string]*descriptorpb.FileDescriptorProto
	fileByName    map[string]*descriptorpb.FileDescriptorProto
}

func (s *stubReflectionServer) ServerReflectionInfo(stream grpc_reflection_v1.ServerReflection_ServerReflectionInfoServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		resp := &grpc_reflection_v1.ServerReflectionResponse{ValidHost: req.GetHost(), OriginalRequest: req}
		switch mr := req.MessageRequest.(type) {
		case *grpc_reflection_v1.ServerReflectionRequest_ListServices:
			svcs := make([]*grpc_reflection_v1.ServiceResponse, 0, len(s.services))
			for _, n := range s.services {
				svcs = append(svcs, &grpc_reflection_v1.ServiceResponse{Name: n})
			}
			resp.MessageResponse = &grpc_reflection_v1.ServerReflectionResponse_ListServicesResponse{
				ListServicesResponse: &grpc_reflection_v1.ListServiceResponse{Service: svcs},
			}
		case *grpc_reflection_v1.ServerReflectionRequest_FileContainingSymbol:
			fd, ok := s.fileForSymbol[mr.FileContainingSymbol]
			setFileResponse(resp, fd, ok)
		case *grpc_reflection_v1.ServerReflectionRequest_FileByFilename:
			fd, ok := s.fileByName[mr.FileByFilename]
			setFileResponse(resp, fd, ok)
		default:
			resp.MessageResponse = &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
				ErrorResponse: &grpc_reflection_v1.ErrorResponse{ErrorCode: int32(codes.Unimplemented), ErrorMessage: "unsupported request"},
			}
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

// setFileResponse sets resp.MessageResponse directly (its field type is an unexported interface
// of grpc_reflection_v1, so a helper cannot name it as a return type — only assign into it).
func setFileResponse(resp *grpc_reflection_v1.ServerReflectionResponse, fd *descriptorpb.FileDescriptorProto, ok bool) {
	if !ok {
		resp.MessageResponse = &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
			ErrorResponse: &grpc_reflection_v1.ErrorResponse{ErrorCode: int32(codes.NotFound), ErrorMessage: "not found"},
		}
		return
	}
	b, err := proto.Marshal(fd)
	if err != nil {
		resp.MessageResponse = &grpc_reflection_v1.ServerReflectionResponse_ErrorResponse{
			ErrorResponse: &grpc_reflection_v1.ErrorResponse{ErrorCode: int32(codes.Internal), ErrorMessage: err.Error()},
		}
		return
	}
	resp.MessageResponse = &grpc_reflection_v1.ServerReflectionResponse_FileDescriptorResponse{
		FileDescriptorResponse: &grpc_reflection_v1.FileDescriptorResponse{FileDescriptorProto: [][]byte{b}},
	}
}

func startStubReflectionServer(t *testing.T, stub *stubReflectionServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	grpc_reflection_v1.RegisterServerReflectionServer(s, stub)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	addr := lis.Addr().String()
	// P96 §4.4: the same net.Listen-returns-before-Serve-dispatches-a-connection race
	// waitEchoServerReady exists to close — this fixture had no readiness wait at all.
	waitEchoServerReady(t, addr)
	return addr
}
