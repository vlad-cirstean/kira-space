package flowharness

import (
	"sync"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness/servers"
)

type (
	// Request is one request an HTTP server received.
	Request = servers.Request
	// Call is one RPC the gRPC server received.
	Call = servers.Call
)

var (
	caOnce sync.Once
	caVal  *servers.CA
	caErr  error
)

// sharedCA is minted once per test binary; every TLS server in it chains to this CA.
func sharedCA(t testing.TB) *servers.CA {
	t.Helper()
	caOnce.Do(func() { caVal, caErr = servers.NewCA() })
	if caErr != nil {
		t.Fatalf("mint test CA: %v", caErr)
	}
	return caVal
}

// CAFile writes the test CA certificate into a temp file and returns its path, for caFile/CA-bundle
// settings of a client.
func CAFile(t testing.TB) string {
	t.Helper()
	path, err := sharedCA(t).WriteFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// HTTPServer is a real plain-HTTP server on a free loopback port. URL uses 127.0.0.1 and AltURL
// reaches the same listener as localhost (a second host to a client); Requests lists what it saw.
type HTTPServer = servers.HTTPServer

// HTTP starts a plain HTTP/1.1 server, closed at cleanup. Routes: /echo, /redirect?n=&code=&to=,
// /cookie/set, /cookie/echo, /basic (kira:secret), /bearer, /bytes?n=, /gzip, /slow?ms=,
// /status?code=.
func HTTP(t testing.TB) *HTTPServer {
	t.Helper()
	s, err := servers.StartHTTP()
	if err != nil {
		t.Fatalf("start http server: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

// HTTPSServer is a TLS server (HTTP/2 and HTTP/1.1) whose certificate chains to CAFile only.
type HTTPSServer struct {
	*HTTPServer
	CAFile string
}

// HTTPS starts a TLS server with the same routes as HTTP. The default certificate covers
// 127.0.0.1 and localhost; pass sans to replace them.
func HTTPS(t testing.TB, sans ...string) *HTTPSServer {
	t.Helper()
	s, err := servers.StartHTTPS(sharedCA(t), sans...)
	if err != nil {
		t.Fatalf("start https server: %v", err)
	}
	t.Cleanup(s.Close)
	return &HTTPSServer{HTTPServer: s, CAFile: CAFile(t)}
}

// GRPCServer is a real gRPC server over kira.flow.v1.Flow (see servers/testdata/flow.proto) with
// reflection. ProtoDir holds flow.proto and its import, to load the schema from a file.
type GRPCServer struct {
	*servers.GRPCServer
	ProtoDir string
	// CAFile is set when the server uses TLS.
	CAFile string
}

// GRPCOpt tweaks GRPC.
type GRPCOpt func(*servers.GRPCOptions)

// WithGRPCTLS serves TLS from the test CA; sans default to 127.0.0.1 and localhost.
func WithGRPCTLS(t testing.TB, sans ...string) GRPCOpt {
	return func(o *servers.GRPCOptions) { o.CA, o.SANs = sharedCA(t), sans }
}

// WithRequiredMetadata makes every call, reflection included, need key=value metadata.
func WithRequiredMetadata(key, value string) GRPCOpt {
	return func(o *servers.GRPCOptions) {
		if o.RequireMetadata == nil {
			o.RequireMetadata = map[string]string{}
		}
		o.RequireMetadata[key] = value
	}
}

// WithExtraMethod serves one more unary method, Extra, beyond flow.proto, for schema-reload tests.
func WithExtraMethod() GRPCOpt { return func(o *servers.GRPCOptions) { o.ExtraMethod = true } }

// WithGRPCAddr listens on a fixed address, to restart a server where a client expects it.
func WithGRPCAddr(addr string) GRPCOpt { return func(o *servers.GRPCOptions) { o.Addr = addr } }

// GRPC starts the flow service, stopped at cleanup.
func GRPC(t testing.TB, opts ...GRPCOpt) *GRPCServer {
	t.Helper()
	var o servers.GRPCOptions
	for _, opt := range opts {
		opt(&o)
	}
	s, err := servers.StartGRPC(o)
	if err != nil {
		t.Fatalf("start grpc server: %v", err)
	}
	t.Cleanup(s.Close)
	dir, err := servers.WriteProtoDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g := &GRPCServer{GRPCServer: s, ProtoDir: dir}
	if o.CA != nil {
		g.CAFile = CAFile(t)
	}
	return g
}

// Silent starts a TCP server that accepts connections and never writes; returns its address.
func Silent(t testing.TB) string {
	t.Helper()
	addr, _ := SilentAccepting(t)
	return addr
}

// SilentAccepting is Silent plus a channel that receives once per accepted connection.
func SilentAccepting(t testing.TB) (string, <-chan struct{}) {
	t.Helper()
	s, err := servers.StartSilent()
	if err != nil {
		t.Fatalf("start silent server: %v", err)
	}
	t.Cleanup(s.Close)
	return s.Addr, s.Accepted
}
