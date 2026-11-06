package servers

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/reflection"
	v1reflectiongrpc "google.golang.org/grpc/reflection/grpc_reflection_v1"
	v1alphareflectiongrpc "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

//go:embed testdata
var testdata embed.FS

// ServiceName is the fully qualified name of the served service.
const ServiceName = "kira.flow.v1.Flow"

const extraMarker = "// @extra"

// GRPCOptions tunes StartGRPC.
type GRPCOptions struct {
	// TLS serves TLS with a leaf from CA for SANs (default 127.0.0.1 and localhost).
	CA   *CA
	SANs []string
	// RequireMetadata makes every call, reflection included, fail UNAUTHENTICATED unless each key
	// carries the value.
	RequireMetadata map[string]string
	// ExtraMethod serves one more unary method, Extra, beyond the proto file clients load.
	ExtraMethod bool
	// Addr is a fixed listen address, to restart a server where a client expects it; default free port.
	Addr string
}

// Call is one RPC the server received.
type Call struct {
	Method   string
	Metadata metadata.MD
	Peer     string
	TLS      bool
}

// GRPCServer is a running gRPC server over kira.flow.v1.Flow with reflection (v1 and v1alpha).
type GRPCServer struct {
	Addr string

	srv *grpc.Server
	mu  sync.Mutex
	rec []Call
}

// WriteProtoDir writes flow.proto and common/types.proto into dir and returns dir, so a client can
// load the schema from a .proto file with dir as an import path.
func WriteProtoDir(dir string) (string, error) {
	err := fs.WalkDir(testdata, "testdata", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := testdata.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, strings.TrimPrefix(p, "testdata/"))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	return dir, err
}

// compile builds the served schema, with the Extra method when asked.
func compile(extra bool) (linker.Files, error) {
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: func(path string) (io.ReadCloser, error) {
				data, err := testdata.ReadFile("testdata/" + path)
				if err != nil {
					return nil, err
				}
				if extra && path == "flow.proto" {
					data = []byte(strings.Replace(string(data), extraMarker, "rpc Extra(Msg) returns (Msg);", 1))
				}
				return readCloser{strings.NewReader(string(data))}, nil
			},
		}),
	}
	return c.Compile(context.Background(), "flow.proto")
}

// noExtensions resolves no extension: the served schema declares none.
type noExtensions struct{}

func (noExtensions) FindExtensionByName(protoreflect.FullName) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

func (noExtensions) FindExtensionByNumber(protoreflect.FullName, protoreflect.FieldNumber) (protoreflect.ExtensionType, error) {
	return nil, protoregistry.NotFound
}

func (noExtensions) RangeExtensionsByMessage(protoreflect.FullName, func(protoreflect.ExtensionType) bool) {
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }

// StartGRPC serves the flow service. The server drops open streams on Close.
func StartGRPC(opts GRPCOptions) (*GRPCServer, error) {
	files, err := compile(opts.ExtraMethod)
	if err != nil {
		return nil, fmt.Errorf("compile flow.proto: %w", err)
	}
	svc := files[0].Services().ByName("Flow")
	registry := new(protoregistry.Files)
	if err := register(registry, files[0]); err != nil {
		return nil, err
	}

	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &GRPCServer{Addr: ln.Addr().String()}
	sopts := []grpc.ServerOption{grpc.UnaryInterceptor(s.unary(opts)), grpc.StreamInterceptor(s.stream(opts))}
	if opts.CA != nil {
		sans := opts.SANs
		if len(sans) == 0 {
			sans = []string{"127.0.0.1", "localhost"}
		}
		cert, err := opts.CA.Issue(sans...)
		if err != nil {
			_ = ln.Close()
			return nil, err
		}
		sopts = append(sopts, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}})))
	}
	// 64 MB: the 4 MB unary case and long streams need headroom over grpc's 4 MB default.
	sopts = append(sopts, grpc.MaxSendMsgSize(64<<20), grpc.MaxRecvMsgSize(64<<20))
	s.srv = grpc.NewServer(sopts...)
	s.srv.RegisterService(serviceDesc(svc), nil)
	ropts := reflection.ServerOptions{Services: s.srv, DescriptorResolver: registry, ExtensionResolver: noExtensions{}}
	// Not reflection.Register: that resolves from protoregistry.GlobalFiles, where the dynamic
	// descriptors must not be registered.
	v1reflectiongrpc.RegisterServerReflectionServer(s.srv, reflection.NewServerV1(ropts))
	v1alphareflectiongrpc.RegisterServerReflectionServer(s.srv, reflection.NewServer(ropts))
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

func register(r *protoregistry.Files, fd protoreflect.FileDescriptor) error {
	if _, err := r.FindFileByPath(fd.Path()); err == nil {
		return nil
	}
	imports := fd.Imports()
	for i := range imports.Len() {
		if err := register(r, imports.Get(i).FileDescriptor); err != nil {
			return err
		}
	}
	return r.RegisterFile(fd)
}

// Close stops the server and drops open streams.
func (s *GRPCServer) Close() { s.srv.Stop() }

// Calls returns a snapshot of the RPCs received, reflection excluded.
func (s *GRPCServer) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.rec...)
}

func (s *GRPCServer) note(ctx context.Context, method string) {
	if strings.HasPrefix(method, "/grpc.reflection.") {
		return
	}
	md, _ := metadata.FromIncomingContext(ctx)
	c := Call{Method: method, Metadata: md}
	if p, ok := peer.FromContext(ctx); ok {
		c.Peer = p.Addr.String()
		c.TLS = p.AuthInfo != nil
	}
	s.mu.Lock()
	s.rec = append(s.rec, c)
	s.mu.Unlock()
}

func authorize(ctx context.Context, required map[string]string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	for k, v := range required {
		vals := md.Get(k)
		if len(vals) == 0 || vals[0] != v {
			return status.Errorf(codes.Unauthenticated, "missing or wrong %s metadata", k)
		}
	}
	return nil
}

func (s *GRPCServer) unary(o GRPCOptions) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, o.RequireMetadata); err != nil {
			return nil, err
		}
		s.note(ctx, info.FullMethod)
		return h(ctx, req)
	}
}

func (s *GRPCServer) stream(o GRPCOptions) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if err := authorize(ss.Context(), o.RequireMetadata); err != nil {
			return err
		}
		s.note(ss.Context(), info.FullMethod)
		return h(srv, ss)
	}
}

// echoMetadata copies request metadata into the response header (echo-) and trailer (trailer-).
func echoMetadata(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	hdr, trl := metadata.MD{}, metadata.MD{}
	for k, v := range md {
		if strings.HasPrefix(k, ":") || strings.HasPrefix(k, "grpc-") || k == "content-type" || k == "user-agent" {
			continue
		}
		hdr["echo-"+k] = v
		trl["trailer-"+k] = v
	}
	_ = grpc.SetHeader(ctx, hdr)
	_ = grpc.SetTrailer(ctx, trl)
}

// flowSvc implements kira.flow.v1.Flow over dynamic messages: no generated code, the same
// technique as grpcclient's own test server.
type flowSvc struct {
	svc protoreflect.ServiceDescriptor
}

func serviceDesc(svc protoreflect.ServiceDescriptor) *grpc.ServiceDesc {
	f := flowSvc{svc}
	methods := []grpc.MethodDesc{f.unary("Unary"), f.unary("Fail"), f.unary("Slow")}
	if svc.Methods().ByName("Extra") != nil {
		methods = append(methods, f.unary("Extra"))
	}
	return &grpc.ServiceDesc{
		ServiceName: string(svc.FullName()),
		HandlerType: (*any)(nil),
		Methods:     methods,
		Streams: []grpc.StreamDesc{
			{StreamName: "ServerStream", ServerStreams: true, Handler: f.serverStream},
			{StreamName: "ClientStream", ClientStreams: true, Handler: f.clientStream},
			{StreamName: "Bidi", ClientStreams: true, ServerStreams: true, Handler: f.bidi},
		},
	}
}

func (f flowSvc) method(name string) protoreflect.MethodDescriptor {
	return f.svc.Methods().ByName(protoreflect.Name(name))
}

func field(m protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	return m.Fields().ByName(protoreflect.Name(name))
}

// reply builds the method's output with text, index and a timestamp, copying meta from in.
func reply(m protoreflect.MethodDescriptor, in *dynamicpb.Message, text string, idx int32) *dynamicpb.Message {
	out := dynamicpb.NewMessage(m.Output())
	out.Set(field(m.Output(), "text"), protoreflect.ValueOfString(text))
	out.Set(field(m.Output(), "index"), protoreflect.ValueOfInt32(idx))
	out.Set(field(m.Output(), "at"), protoreflect.ValueOfMessage(timestamppb.Now().ProtoReflect()))
	if in != nil {
		if meta := field(m.Input(), "meta"); meta != nil && in.Has(meta) {
			out.Set(field(m.Output(), "meta"), in.Get(meta))
		}
	}
	return out
}

func (f flowSvc) unary(name string) grpc.MethodDesc {
	m := f.method(name)
	handle := func(ctx context.Context, in *dynamicpb.Message) (any, error) {
		echoMetadata(ctx)
		switch name {
		case "Fail":
			return nil, status.Error(codes.Code(in.Get(field(m.Input(), "code")).Int()), in.Get(field(m.Input(), "message")).String())
		case "Slow":
			<-ctx.Done()
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		text := in.Get(field(m.Input(), "text")).String()
		if n := int(in.Get(field(m.Input(), "payload_bytes")).Int()); n > 0 {
			text = strings.Repeat("x", n)
		}
		return reply(m, in, text, int32(in.Get(field(m.Input(), "index")).Int())), nil
	}
	return grpc.MethodDesc{MethodName: name, Handler: func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		in := dynamicpb.NewMessage(m.Input())
		if err := dec(in); err != nil {
			return nil, err
		}
		if interceptor == nil {
			return handle(ctx, in)
		}
		info := &grpc.UnaryServerInfo{FullMethod: "/" + string(f.svc.FullName()) + "/" + name}
		return interceptor(ctx, in, info, func(ctx context.Context, req any) (any, error) {
			return handle(ctx, req.(*dynamicpb.Message))
		})
	}}
}

// serverStream sends count (default 3) messages interval_ms apart, until the client leaves.
func (f flowSvc) serverStream(_ any, ss grpc.ServerStream) error {
	m := f.method("ServerStream")
	in := dynamicpb.NewMessage(m.Input())
	if err := ss.RecvMsg(in); err != nil {
		return err
	}
	echoMetadata(ss.Context())
	count := int(in.Get(field(m.Input(), "count")).Int())
	if count == 0 {
		count = 3
	}
	pause := time.Duration(in.Get(field(m.Input(), "interval_ms")).Int()) * time.Millisecond
	text := in.Get(field(m.Input(), "text")).String()
	for i := range count {
		if i > 0 && pause > 0 {
			select {
			case <-time.After(pause):
			case <-ss.Context().Done():
				return status.FromContextError(ss.Context().Err()).Err()
			}
		}
		if err := ss.SendMsg(reply(m, in, text, int32(i))); err != nil {
			return err
		}
	}
	return nil
}

// clientStream counts the messages received and answers once, with the count in index.
func (f flowSvc) clientStream(_ any, ss grpc.ServerStream) error {
	m := f.method("ClientStream")
	n := int32(0)
	for {
		if err := ss.RecvMsg(dynamicpb.NewMessage(m.Input())); err != nil {
			if errors.Is(err, io.EOF) {
				return ss.SendMsg(reply(m, nil, "received", n))
			}
			return err
		}
		n++
	}
}

// bidi echoes each message with a running index.
func (f flowSvc) bidi(_ any, ss grpc.ServerStream) error {
	m := f.method("Bidi")
	for i := int32(0); ; i++ {
		in := dynamicpb.NewMessage(m.Input())
		if err := ss.RecvMsg(in); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := ss.SendMsg(reply(m, in, in.Get(field(m.Input(), "text")).String(), i)); err != nil {
			return err
		}
	}
}

// Silent accepts TCP connections and never writes, to prove a client gives up.
type Silent struct {
	Addr string
	// Accepted receives one value per accepted connection; sends never block.
	Accepted chan struct{}
	ln       net.Listener
	mu       sync.Mutex
	open     []net.Conn
}

// StartSilent listens on a free loopback port.
func StartSilent() (*Silent, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Silent{Addr: ln.Addr().String(), Accepted: make(chan struct{}, 16), ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.open = append(s.open, c)
			s.mu.Unlock()
			select {
			case s.Accepted <- struct{}{}:
			default:
			}
		}
	}()
	return s, nil
}

// Close stops listening and drops accepted connections.
func (s *Silent) Close() {
	_ = s.ln.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.open {
		_ = c.Close()
	}
}
