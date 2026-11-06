package dbmcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/tokenauth"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	// SessionServerName is the MCP server name an agent session sees. It differs from the external
	// "kira-db" so a user-scope registration of that name never shadows it.
	SessionServerName = "kira-db-session"
	// SessionAllowTool allows every tool of the session server (server-level allow rule).
	SessionAllowTool = "mcp__" + SessionServerName
)

// SessionIdentity names the agent session behind a call on the session server.
type SessionIdentity struct {
	SessionID, TaskID, StepID, RunID, Cwd string
}

type sessionToken struct {
	id         uint64
	hash, salt []byte
	identity   SessionIdentity
	// ctx ends when the token is revoked, ending calls in flight with it.
	ctx    context.Context
	cancel context.CancelFunc
}

// SessionTokens is the table of live per-launch tokens. Hashes live in memory only.
type SessionTokens struct {
	mu      sync.RWMutex
	seq     uint64
	entries []sessionToken
}

func (t *SessionTokens) add(hash, salt []byte, id SessionIdentity) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	ctx, cancel := context.WithCancel(context.Background())
	t.entries = append(t.entries, sessionToken{id: t.seq, hash: hash, salt: salt, identity: id, ctx: ctx, cancel: cancel})
	return t.seq
}

func (t *SessionTokens) remove(id uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := t.entries[:0]
	for _, e := range t.entries {
		if e.id != id {
			kept = append(kept, e)
		} else {
			e.cancel()
		}
	}
	t.entries = kept
}

func (t *SessionTokens) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, e := range t.entries {
		e.cancel()
	}
	t.entries = nil
}

// Len is the number of live tokens.
func (t *SessionTokens) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

func (t *SessionTokens) lookup(presented string) (sessionToken, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, e := range t.entries {
		if tokenauth.Verify(presented, e.hash, e.salt) {
			return e, true
		}
	}
	return sessionToken{}, false
}

const sessionUserPrefix = "session:"

// TokenInfo.Extra keys of a verified session token.
const (
	sessionExtraKey = "identity"
	sessionCtxKey   = "ctx"
)

// verifier authenticates only live session tokens; the external token never matches.
func (t *SessionTokens) verifier() auth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		e, ok := t.lookup(token)
		if !ok {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{
			UserID:     sessionUserPrefix + e.identity.SessionID,
			Expiration: time.Now().Add(24 * time.Hour),
			Extra:      map[string]any{sessionExtraKey: e.identity, sessionCtxKey: e.ctx},
		}, nil
	}
}

// SessionFrom returns the agent session behind the call in ctx; false for an external client.
func SessionFrom(ctx context.Context) (SessionIdentity, bool) {
	return sessionOf(auth.TokenInfoFromContext(ctx))
}

func sessionOf(info *auth.TokenInfo) (SessionIdentity, bool) {
	if info == nil {
		return SessionIdentity{}, false
	}
	id, ok := info.Extra[sessionExtraKey].(SessionIdentity)
	return id, ok
}

// SessionPool owns the session-only server: loopback, ephemeral port, session tokens only. It starts
// with the first grant and closes with the last release, whatever the Settings switch says.
type SessionPool struct {
	build     func(*SessionTokens) Config
	approvals *ApprovalBroker
	log       *slog.Logger

	tokens SessionTokens

	mu     sync.Mutex
	srv    *Server
	grants int
	closed bool
}

// NewSessionPool returns a pool. build supplies the server config; the pool sets Sessions, Port and clears Token.
func NewSessionPool(approvals *ApprovalBroker, build func(*SessionTokens) Config) *SessionPool {
	return &SessionPool{build: build, approvals: approvals, log: slog.Default()}
}

// Grant mints a token for one agent process and returns the MCP server entry that carries it. release is
// idempotent: it revokes the token, abandons the session's parked approvals and closes the server when no
// grant is left.
func (p *SessionPool) Grant(id SessionIdentity) (harness.MCPServer, func(), error) {
	if id.SessionID == "" {
		return harness.MCPServer{}, nil, errors.New("dbmcp: session grant needs a session id")
	}
	plain, hash, salt, err := tokenauth.Mint()
	if err != nil {
		return harness.MCPServer{}, nil, fmt.Errorf("dbmcp: mint session token: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return harness.MCPServer{}, nil, errors.New("dbmcp: session server is shut down")
	}
	if p.srv == nil {
		cfg := p.build(&p.tokens)
		cfg.Token, cfg.Sessions, cfg.Port = nil, &p.tokens, EphemeralPort
		srv, err := New(cfg)
		if err != nil {
			return harness.MCPServer{}, nil, err
		}
		go func() {
			if err := srv.Serve(); err != nil {
				p.log.Warn("db mcp session server", "scope", "dbmcp", "err", err)
			}
		}()
		p.srv = srv
	}
	tokenID := p.tokens.add(hash, salt, id)
	p.grants++
	entry := harness.MCPServer{
		Name:    SessionServerName,
		URL:     p.srv.URL(),
		Headers: map[string]string{"Authorization": "Bearer " + plain},
		Timeout: harness.MCPCallTimeout,
	}
	var once sync.Once
	return entry, func() { once.Do(func() { p.release(tokenID, id.SessionID) }) }, nil
}

func (p *SessionPool) release(tokenID uint64, sessionID string) {
	p.tokens.remove(tokenID)
	p.approvals.AbandonSession(sessionID)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.grants > 0 {
		p.grants--
	}
	if p.grants == 0 && p.srv != nil {
		_ = p.srv.Close()
		p.srv = nil
	}
}

// Close revokes every token, abandons every session approval and stops the server. Later grants fail.
func (p *SessionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.tokens.clear()
	p.grants = 0
	p.approvals.AbandonSessions()
	if p.srv != nil {
		_ = p.srv.Close()
		p.srv = nil
	}
}

// Running reports whether the session server is listening.
func (p *SessionPool) Running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.srv != nil
}

// URL is the session server's endpoint, "" when not running.
func (p *SessionPool) URL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srv == nil {
		return ""
	}
	return p.srv.URL()
}
