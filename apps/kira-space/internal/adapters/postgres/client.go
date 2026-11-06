package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const (
	connectTimeout = 10 * time.Second
	maxConns       = 8
	// closeGrace bounds how long a Close waits for an op still holding the connection before it
	// closes the socket under that op.
	closeGrace = 10 * time.Second
	// cancelTimeout bounds one server-side cancel (side dial plus the cancel call): the op's
	// connection waits on it before release.
	cancelTimeout = 10 * time.Second
	// primaryKey is a key no real database name can collide with — NUL can't appear in a
	// Postgres identifier.
	primaryKey = "\x00primary"
)

// buildConfig is client.ts's buildClientConfig.
func buildConfig(cfg model.ResolvedConnectionConfig, database string, log func(level, message string)) (*pgx.ConnConfig, error) {
	var connConfig *pgx.ConnConfig
	// overrode tracks whether this function itself set Host/Port/TLSConfig on connConfig:
	// pgx.ParseConfig's own default sslmode=prefer builds a TLS-primary + plaintext-fallback
	// pair, each carrying its own Host/Port pulled from PG* environment variables when parsing an
	// empty connection string — an override below only ever touches the primary, never a Fallbacks
	// entry, so without also clearing Fallbacks, pgconn.ConnectConfig's own retry-on-any-non-auth-
	// error (including "server refused TLS") could silently fall through to a fallback carrying an
	// environment-derived host/port or a plaintext TLSConfig this override was never meant to allow
	// — exactly the "must fail loudly" this file's own sslmode default case already commits to.
	overrode := false
	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		parsed, err := pgx.ParseConfig(*cfg.URI)
		if err != nil {
			return nil, err
		}
		connConfig = parsed
	} else {
		parsed, err := pgx.ParseConfig("")
		if err != nil {
			return nil, err
		}
		connConfig = parsed
		if cfg.Host != nil {
			connConfig.Host = *cfg.Host
			overrode = true
		}
		if cfg.Port != nil {
			connConfig.Port = uint16(*cfg.Port)
			overrode = true
		}
		if cfg.Username != nil {
			connConfig.User = *cfg.Username
		}
		if cfg.Password != nil {
			connConfig.Password = *cfg.Password
		}
	}
	switch {
	case database != "":
		connConfig.Database = database
	case cfg.Database != nil && *cfg.Database != "":
		connConfig.Database = *cfg.Database
	case connConfig.Database == "":
		// No explicit database anywhere (cfg.Database blank, and neither the URI's own path
		// nor pgx.ParseConfig("")'s PGDATABASE fallback supplied one). Left alone, the Postgres
		// wire protocol defaults an omitted "database" startup parameter to the connecting
		// *user* name (pgconn's own SendStartupMessage — client.go's non-URI branch above never
		// sets one either) — which fails outright with "database \"<user>\" does not exist" for
		// any real least-privilege role whose name doesn't happen to match an existing database.
		// "postgres" is the maintenance database every real server ships with, and the sane
		// bootstrap target anyway: this connection's whole point is to enumerate every database
		// on the server (docs/ARCHITECTURE.md's PostgreSQL / MariaDB / MySQL section, database ->
		// schema -> table), not
		// to land in one particular one.
		connConfig.Database = "postgres"
	}

	connConfig.ConnectTimeout = connectTimeout
	if connConfig.RuntimeParams == nil {
		connConfig.RuntimeParams = map[string]string{}
	}
	connConfig.RuntimeParams["application_name"] = "kira-studio"
	// The app cancels explicitly via pg_cancel_backend; a silent server-side statement_timeout
	// would make the stop button's contract a lie.
	connConfig.RuntimeParams["statement_timeout"] = "0"

	sslmode, sslEnabled, err := adapters.ParseSSLMode(cfg.Options, "postgres", "require", "prefer", "verify-full", "verify-ca")
	if err != nil {
		return nil, err
	}
	if sslEnabled {
		switch sslmode {
		case "require", "prefer":
			connConfig.TLSConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // matches client.ts's own rejectUnauthorized:false for these two modes
			overrode = true
		case "verify-full":
			connConfig.TLSConfig = &tls.Config{ServerName: connConfig.Host}
			overrode = true
		// pgx.ParseConfig already resolves
		// verify-ca correctly from a URI's own ?sslmode= (libpq's "verify the CA chain but not
		// the hostname" — the standard case for an internal CA or an IP-addressed host), but this
		// Options-driven override then re-read the same value and hit the default branch,
		// overriding a config pgx had already gotten right with a hard "unknown sslmode" refusal.
		// InsecureSkipVerify plus a VerifyPeerCertificate that chains against the system roots
		// without checking the hostname is the standard Go recipe for "verify-CA-not-host".
		case "verify-ca":
			connConfig.TLSConfig = &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // hostname check is intentionally skipped; chain verification still runs below
				VerifyPeerCertificate: func(certs [][]byte, _ [][]*x509.Certificate) error {
					return verifyChainSkipHostname(certs, nil) // nil roots = the system trust store
				},
			}
			overrode = true
		}
	}

	if overrode {
		// F6: no environment-derived or protocol-downgraded fallback survives an override above —
		// pgconn.ConnectConfig retries a Fallbacks entry on any non-auth error from the primary,
		// which would otherwise silently defeat exactly the override this function just made.
		connConfig.Fallbacks = nil
	}

	return connConfig, nil
}

// connEntry pairs one database's *pgx.Conn with the mutex that serializes every use of it (P2 R2):
// pgx.Conn is not safe for concurrent use by multiple goroutines, but nothing above this package
// serializes ops against the same Adapter — adapterhost dispatches each inbound frame on its own
// goroutine (bounded only by the session's own in-flight cap), so two Reads on two tabs, or a Read
// racing a Mutate, can and do reach the same *pgx.Conn concurrently without this lock.
//
// inFlight (finding F2, MEDIUM-HIGH/data integrity) is this entry's own count of still-running
// adapters.RunWithAbortRace background goroutines — distinct from adapter.go's tracker.inFlight,
// which is adapter-wide and only ever Waited on by Disconnect. RunWithAbortRace can return to its
// own caller (on the op's ctx.Done(), a Stop/cancel) well before the goroutine it spawned actually
// stops calling conn.Query/conn.Exec (by design: a local abort must not itself try to kill the
// query — see RunWithAbortRace's own doc comment). Without this, Acquire's release unlocked mu (and
// a detached ROLLBACK/COMMIT cleanup ran conn.Exec) the instant the aborted op's own call returned —
// racing that still-running background goroutine on the same non-concurrency-safe *pgx.Conn,
// silently swallowed by cleanup's own `_, _ =` and leaving the connection wedged in an aborted or
// unexpectedly-open transaction for whatever op ran next.
type connEntry struct {
	conn  *pgx.Conn
	guard adapters.ConnGuard
}

// trackedConn is Acquire's own handle over one connEntry: embedding *pgx.Conn keeps every existing
// conn.Query/conn.Exec/conn.PgConn() call site unchanged, while track/waitInFlight expose the
// entry's own inFlight WaitGroup (F2, above).
type trackedConn struct {
	*pgx.Conn
	entry *connEntry
}

// track registers one RunWithAbortRace background goroutine against this connection's own entry.
// Call synchronously, at the same point a TrackQuery closure is called — before
// adapters.RunWithAbortRace's own goroutine starts — and compose the returned done into whatever
// release RunWithAbortRace already calls once that goroutine finishes (never only on the
// caller's own ctx.Done()).
func (c *trackedConn) track() (done func()) {
	return c.entry.guard.Track()
}

// running is the RunningQuery to track for a statement run on this connection.
func (c *trackedConn) running() RunningQuery {
	return RunningQuery{BackendPID: c.PgConn().PID(), Guard: &c.entry.guard}
}

// waitInFlight blocks until every RunWithAbortRace goroutine track() has registered against this
// connection has finished touching it. A detached cleanup statement (mutate's own
// ROLLBACK, console's read-only-wrap COMMIT) calls this immediately before its own conn.Exec, so it
// never races a just-aborted op's background goroutine still using the same *pgx.Conn.
func (c *trackedConn) waitInFlight() {
	c.entry.guard.WaitInFlight()
}

// verifyChainSkipHostname is sslmode=verify-ca's certificate check, split out of buildConfig so
// it can be tested against a controlled root pool rather than only the live system trust store
// (which a test can't add a throwaway CA to). roots == nil means "the system trust store", the
// same as x509.VerifyOptions' own zero value and what production always passes.
func verifyChainSkipHostname(rawCerts [][]byte, roots *x509.CertPool) error {
	chain := make([]*x509.Certificate, 0, len(rawCerts))
	for _, raw := range rawCerts {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return err
		}
		chain = append(chain, cert)
	}
	if len(chain) == 0 {
		return adapters.T(adapters.CodeConnect, "backend.studio.adapter.serverCertificateMissing.body", nil, nil)
	}
	opts := x509.VerifyOptions{Roots: roots, Intermediates: x509.NewCertPool()}
	for _, cert := range chain[1:] {
		opts.Intermediates.AddCert(cert)
	}
	_, err := chain[0].Verify(opts)
	return err
}

// ConnSet is client.ts's ClientSet — misleadingly-named "Pool" avoided on purpose: one
// *pgx.Conn per (connection, database), never a pool, because pg_cancel_backend needs a known
// backend pid and a pool does not reliably tell you which backend ran your query. The LRU pool
// with single-flight dial is adapters.ConnSet — only Dial and
// the eviction Close stay dialect-specific here.
type ConnSet struct {
	cfg model.ResolvedConnectionConfig
	log func(level, message string)

	inner *adapters.ConnSet[string, *connEntry]
}

// NewConnSet constructs a ConnSet for cfg.
func NewConnSet(cfg model.ResolvedConnectionConfig, log func(level, message string)) *ConnSet {
	s := &ConnSet{cfg: cfg, log: log}
	s.inner = adapters.NewConnSet(adapters.ConnSetOptions[string, *connEntry]{
		// key normalizes "" (the primary) to primaryKey for the pool's own map/lru; dial itself
		// always wants the original, possibly-empty database argument (buildConfig's own "" means
		// no explicit override).
		Dial: func(ctx context.Context, key string) (*connEntry, error) {
			database := key
			if database == primaryKey {
				database = ""
			}
			return s.dial(ctx, database)
		},
		// The victim's own per-connection lock (held for its entire in-flight op) is taken
		// here, not inside adapters.ConnSet — the pool's own mu is already released by the time
		// Close runs (see adapters.ConnSet.Get's own doc comment). The wait is bounded; a
		// holder stuck on a dead socket gets the socket closed under it instead.
		Close: func(ctx context.Context, e *connEntry) {
			e.guard.CloseWithin(ctx, closeGrace, func() { _ = e.conn.Close(ctx) }, func() { _ = e.conn.PgConn().Conn().Close() })
		},
		Max:     maxConns,
		Primary: primaryKey,
	})
	return s
}

// pgxConnect — pgx.ConnectConfig, the exact function dial calls — is a package-level var (the
// same seam-by-var shape httpclient/wire.go's own wireProxyFunc and bridge/grpc.go's own
// serverStreamFn already establish) so a test can substitute a fake with a controlled delay and
// outcome, with no real network dial needed, to observe get()'s own single-flight behavior under
// genuine goroutine concurrency.
var pgxConnect = pgx.ConnectConfig

// dial is get()'s own single-attempt dial, split out so get() can call it with no lock held —
// pgx.ConnectConfig is a real network round trip, and the whole point of the dialing placeholder
// above is that nothing here needs s.mu at all.
func (s *ConnSet) dial(ctx context.Context, database string) (*connEntry, error) {
	connConfig, err := buildConfig(s.cfg, database, s.log)
	if err != nil {
		return nil, mapError(err)
	}
	conn, err := pgxConnect(ctx, connConfig)
	if err != nil {
		return nil, mapError(err)
	}

	// A read-only connection is enforced by the server itself, not just Mutate's own app-level
	// AssertWritable check — matching ClickHouse's readonly=2 and SQLite's mode=ro precedent, and
	// closing the gap the console's Execute() would otherwise leave (it has no per-statement
	// write/read classifier of its own, unlike Redis/Mongo's console).
	if s.cfg.ReadOnly {
		if _, err := conn.Exec(ctx, "SET default_transaction_read_only = on"); err != nil {
			_ = conn.Close(ctx)
			return nil, mapError(err)
		}
	}

	return &connEntry{conn: conn}, nil
}

// Acquire returns database's connection (empty string means the primary) together with a release
// func that must be called exactly once, however the caller's own use of conn ends — it holds the
// per-connection lock get's own doc comment describes, for the caller's own entire op, not just one
// statement: a mutate's BEGIN…COMMIT or a console "run all" must keep any concurrent op off this
// same conn for its whole duration, not just between individual statements, or a racing Read could
// execute inside the open transaction.
func (s *ConnSet) Acquire(ctx context.Context, database string) (*trackedConn, func(), error) {
	key := database
	if key == "" {
		key = primaryKey
	}
	for {
		entry, err := s.inner.Get(ctx, key)
		if err != nil {
			return nil, nil, err
		}
		if err := entry.guard.Lock(ctx); err != nil {
			return nil, nil, err
		}
		// An LRU eviction's own Close (adapters.ConnSet.Get's own doc comment) contends for this
		// same entry.mu, so it may already have closed entry.conn by the time this Lock succeeds.
		// Re-check that entry is still the set's own live entry for key before trusting it — retry
		// from the top (a fresh Get, dialing again if nothing else raced in first) rather than hand
		// back a connection that was just closed out from under it.
		if current, ok := s.inner.Current(key); !ok || current != entry {
			entry.guard.Unlock()
			continue
		}
		// F5: a dead connection (a cancelled or broken query closed it) stays this key's entry
		// forever otherwise; drop it and dial a fresh one.
		if entry.conn.IsClosed() {
			entry.guard.Unlock()
			s.inner.Drop(ctx, key, entry)
			continue
		}
		return &trackedConn{Conn: entry.conn, entry: entry}, func() {
			// F2: hold this connection's lock (and so keep the next Acquire waiting) until every
			// RunWithAbortRace goroutine started under this acquisition has actually finished
			// touching entry.conn, and a Cancel aimed at it has been delivered (F7).
			entry.guard.Release()
		}, nil
	}
}

// Primary acquires the primary (no explicit database override) connection.
func (s *ConnSet) Primary(ctx context.Context) (*trackedConn, func(), error) {
	return s.Acquire(ctx, "")
}

// CloseAll closes every open connection, taking each one's own lock first (P2 R2 — see the Close
// closure NewConnSet builds above).
func (s *ConnSet) CloseAll(ctx context.Context) {
	s.inner.CloseAll(ctx)
}
