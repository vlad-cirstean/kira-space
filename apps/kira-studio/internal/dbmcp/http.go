package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mcpauth"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// closeHTTPGraceTimeout bounds closeHTTP's own graceful Shutdown wait:
// http.Server.Shutdown given context.Background() blocks until every in-flight handler returns,
// with no deadline of its own — a run_query handler parked in ApprovalBroker.Request would never
// return on its own, so an unbounded Shutdown here would hang the whole app-quit path on it.
// bridge/dbmcp.go's stopLocked unblocks that handler first (AbandonAll before Close), so
// this deadline is normally never reached — it exists as the hard backstop for whatever ordering
// mistake or slow handler reaches it anyway, matching mcpinstall/exec.go's own
// grace-then-force shape.
const closeHTTPGraceTimeout = 5 * time.Second

// mcpPath is every `claude mcp add --transport http` example's own convention — ends in "/mcp".
const mcpPath = "/mcp"

// httpState is server.go's own Server struct split out for this file's cohesion.
type httpState struct {
	listener net.Listener
	http     *http.Server
}

type requestContextKeyType struct{}

var requestContextKey requestContextKeyType

// withRequestContext stores the HTTP request's own context where tool handlers can read it back.
// go-sdk v1.8.0 detaches the handler context from the request unless PropagateRequestCancellation
// applies, and a client's cancel arrives on a separate stateless POST, so a disconnect or
// client-side timeout otherwise never reaches the handler. Context values survive
// the detach.
func withRequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), requestContextKey, r.Context())
		ctx = context.WithValue(ctx, originContextKey, ApprovalOrigin{Client: clientFromUserAgent(r.UserAgent())})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type originContextKeyType struct{}

var originContextKey originContextKeyType

// originFrom is the caller identity withRequestContext stored; zero outside an HTTP request.
func originFrom(ctx context.Context) ApprovalOrigin {
	o, _ := ctx.Value(originContextKey).(ApprovalOrigin)
	return o
}

const maxClientBytes = 64

// clientFromUserAgent turns "claude-code/2.1.318 (cli)" into "Claude Code 2.1.318": the first product
// token only, printable ASCII, at most 64 bytes. The header is client-controlled text and not proof of
// identity.
func clientFromUserAgent(ua string) string {
	product, _, _ := strings.Cut(strings.TrimSpace(ua), " ")
	name, version, _ := strings.Cut(product, "/")
	if name == "claude-code" {
		name = "Claude Code"
	}
	out := strings.TrimSpace(name + " " + version)
	var b strings.Builder
	for _, r := range out {
		if r < 0x20 || r > 0x7e {
			continue
		}
		b.WriteRune(r)
	}
	out = b.String()
	if len(out) > maxClientBytes {
		out = out[:maxClientBytes]
	}
	return out
}

// maxRequestBytes caps one MCP request body: a proof step carries up to 4 MiB of data plus the
// JSON-RPC envelope; every other tool sends kilobytes.
const maxRequestBytes = 8 << 20

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
		next.ServeHTTP(w, r)
	})
}

// bindRequestCancellation returns ctx cancelled when the HTTP request stored by
// withRequestContext ends (client gone). Without a stored request context it is a plain child.
func bindRequestCancellation(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	if reqCtx, ok := ctx.Value(requestContextKey).(context.Context); ok {
		stop := context.AfterFunc(reqCtx, cancel)
		return ctx, func() { stop(); cancel() }
	}
	return ctx, cancel
}

// bindHTTP binds DefaultPort — never 0.0.0.0, always loopback only — mounts the auth-wrapped
// Streamable HTTP handler, and records the bound listener. Does not start serving; call Serve for
// that.
//
// DefaultPort conflict is refused outright, never silently falls back to an OS-assigned
// ephemeral port. Every existing registration (Command's own text, or whatever the user already
// ran) names DefaultPort — with no re-registration signal of its own, a fallback would leave that
// registration pointing at a port another local uid could hold instead, silently receiving
// Claude Code's own live bearer token on its next connection attempt (the loopback listener is
// reachable by every local uid). Refusing surfaces through the exact same path an ordinary bind
// failure already does — statusFn's own Error field, startIfEnabled's boot-time warn log,
// SetEnabled's st.Error — so this needs no new status surface, only a real error where a silent
// fallback used to be.
func (s *Server) bindHTTP() error {
	port := DefaultPort
	switch p := s.cfg.Port; {
	case p == EphemeralPort:
		port = 0
	case p > 0:
		port = p
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("dbmcp: bind 127.0.0.1:%d: %w (another process is using this port — free it and try again)", port, err)
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.server() }, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// Only takes effect for protocol >= 2026-07-28; older clients rely on requestContextKey.
		PropagateRequestCancellation: true,
	})

	verifier := s.tokenVerifier()
	protected := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		// mcpauth.Check does its own expiry test and reports OutcomeExpired with an actionable
		// message; the SDK's own Expiration-based check would only ever see a zero
		// TokenInfo.Expiration (deliberately left empty, see tokenVerifier below) and produce its
		// own flat "token missing expiration" body instead, so that check stays opted out here.
		AllowMissingExpiration: true,
	})(withRequestContext(limitBody(handler)))

	// The go-sdk applies DNS-rebinding protection by default but explicitly does not apply
	// cross-origin protection unless the caller wraps the handler itself: a browser tab on an
	// unrelated origin must not be able to reach it just because it happens to be running on
	// loopback.
	mux := http.NewServeMux()
	mux.Handle(mcpPath, http.NewCrossOriginProtection().Handler(protected))

	s.listener = ln
	s.http = &http.Server{
		Handler: mux,
		// ReadHeaderTimeout bounds a slowloris-shaped client; IdleTimeout reclaims a connection
		// that never issues a second request.
		// WriteTimeout/ReadTimeout stay unset: the Streamable HTTP transport holds long-lived
		// server-to-client streams, and a slow client body on a long POST is not a threat on
		// loopback the way a slow header is.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return nil
}

// tokenVerifier is this instance's own auth.TokenVerifier: mcpauth.TokenVerifier reads the current
// record under s.tokenMu on every request, so a Regenerate mid-flight is picked up by the very
// next one.
func (s *Server) tokenVerifier() auth.TokenVerifier {
	return mcpauth.TokenVerifier("kira-db", func() mcpauth.Record {
		s.tokenMu.RLock()
		defer s.tokenMu.RUnlock()
		return s.token
	})
}

// Port returns the actually-bound port — DefaultPort unless Config.Port overrides it (F11: bindHTTP
// never falls back on a conflict).
func (s *Server) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// URL returns this instance's own MCP endpoint, e.g. "http://127.0.0.1:8766/mcp".
func (s *Server) URL() string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", s.Port(), mcpPath)
}

// Serve blocks, accepting connections on the already-bound listener, until Close is called (or the
// listener otherwise fails). bridge/dbmcp.go runs this in its own goroutine (`go srv.Serve()`).
func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if err != nil && errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) closeHTTP() error {
	if s.http == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), closeHTTPGraceTimeout)
	defer cancel()
	if err := s.http.Shutdown(ctx); err != nil {
		// Shutdown's own deadline lapsed (or another error) — Close never blocks on an in-flight
		// handler, dropping any connection still open rather than hanging the app-quit path on it.
		_ = s.http.Close()
		return err
	}
	return nil
}
