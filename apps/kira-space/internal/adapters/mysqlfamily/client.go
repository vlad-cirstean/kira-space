package mysqlfamily

import (
	"context"
	"crypto/tls"
	"database/sql"
	"database/sql/driver"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

const (
	connectTimeout = 10 * time.Second
	maxConns       = 8
	// closeGrace bounds how long a Close waits for an op still holding the connection before it
	// closes the socket under that op.
	closeGrace = 10 * time.Second
	// cancelTimeout bounds one server-side cancel (side dial plus KILL QUERY): the op's connection
	// waits on it before release.
	cancelTimeout = 10 * time.Second
	// primaryKey is a key no real database name can collide with.
	primaryKey = "\x00primary"
)

// applyFixedDefaults sets BuildConfig's fixed field block — every option that never varies by
// connection config.
func applyFixedDefaults(mc *mysql.Config) {
	mc.Net = "tcp"
	// B2: the text protocol, matching pgx.QueryExecModeSimpleProtocol's own reasoning — the driver
	// hands back the server's own bytes, not a re-typed Go value.
	mc.InterpolateParams = true
	mc.ParseTime = false
	mc.MultiStatements = false
	mc.AllowNativePasswords = true
	// Without CLIENT_FOUND_ROWS, MySQL/MariaDB report "rows changed" from an UPDATE, not
	// "rows matched" — so editing a cell back to the value it already had returns affectedRows=0,
	// which mutate()'s AssertAffectedExactlyOne(kind, 0) then rejects as a failed update, rolling
	// back the whole batch even though the row was found and the statement succeeded. Postgres and
	// SQLite both report rows matched natively; this flag is what makes MySQL/MariaDB consistent
	// with them (the old TS client got this for free — the mariadb connector's own foundRows option
	// defaults to true).
	mc.ClientFoundRows = true
	// B23: the Go analogue of connectAttributes: { program_name: 'kira-studio' }.
	mc.ConnectionAttributes = "program_name:kira-studio"
	mc.Timeout = connectTimeout
}

// applyURIOptions fills mc's address/credentials from the connection URI and layers its own known
// query keys into options — never handed to the driver as literal session-variable text (see
// below).
func applyURIOptions(mc *mysql.Config, options map[string]any, uri string, log LogFunc) error {
	parsed, err := url.Parse(uri)
	if err != nil {
		return adapters.T(adapters.CodeConnect, "backend.studio.adapter.uriParseFailed.body", nil, err)
	}
	mc.Addr = parsed.Host
	if mc.Addr == "" {
		mc.Addr = "127.0.0.1:3306"
	} else if parsed.Port() == "" {
		mc.Addr += ":3306"
	}
	if u := parsed.User; u != nil {
		mc.User = u.Username()
		if pw, ok := u.Password(); ok {
			mc.Passwd = pw
		}
	}
	mc.DBName = strings.TrimPrefix(parsed.Path, "/")
	// The driver's own Params field is not a bag of DSN options: go-sql-driver concatenates every
	// entry into a literal `SET <k> = <v>, ...` and executes it verbatim at connect time
	// (handleParams). Splicing an arbitrary URI query string into that would both break the only
	// documented TLS path for these two engines (?sslmode=... would try `SET sslmode = ...` and
	// fail with "Unknown system variable") and run unescaped user text as SQL. So only translate
	// known keys into real config; everything else is dropped with a warn log rather than
	// forwarded to the driver.
	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		switch key {
		case "sslmode":
			options["sslmode"] = values[0]
		default:
			if log != nil {
				log("warn", "mysql-family: ignoring unrecognized connection URI option \""+key+"\"")
			}
		}
	}
	return nil
}

// applyFieldsOptions fills mc's address/credentials from cfg's discrete fields form.
func applyFieldsOptions(mc *mysql.Config, cfg model.ResolvedConnectionConfig) {
	host := ""
	if cfg.Host != nil {
		host = *cfg.Host
	}
	port := 3306
	if cfg.Port != nil {
		port = *cfg.Port
	}
	mc.Addr = host + ":" + strconv.Itoa(port)
	if cfg.Username != nil {
		mc.User = *cfg.Username
	}
	if cfg.Password != nil {
		mc.Passwd = *cfg.Password
	}
	if cfg.Database != nil {
		mc.DBName = *cfg.Database
	}
}

// resolveOptions fills mc's address/credentials from either the URI or the fields form and
// returns the options map the sslmode switch below reads from — cfg.Options plus, in URI mode,
// whatever known keys the URI's own query string names.
func resolveOptions(mc *mysql.Config, cfg model.ResolvedConnectionConfig, database string, log LogFunc) (map[string]any, error) {
	// options is what applyTLS reads from. In fields mode it's exactly cfg.Options (populated by
	// the renderer). In URI mode we start from cfg.Options too (for parity, though the renderer
	// only populates it on a fields<->URI flip) and layer in the URI's own query string.
	options := map[string]any{}
	for k, v := range cfg.Options {
		options[k] = v
	}

	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		if err := applyURIOptions(mc, options, *cfg.URI, log); err != nil {
			return nil, err
		}
	} else {
		applyFieldsOptions(mc, cfg)
	}
	if database != "" {
		mc.DBName = database
	}
	return options, nil
}

// applyTLS sets mc.TLSConfig from options' sslmode, registering a shared, idempotent TLS config
// under a fixed name per mode (P21 round 2 architecture/security finding 8: never a fresh registry
// entry per connection id, which would never be deregistered).
func applyTLS(mc *mysql.Config, options map[string]any) error {
	sslmode, sslEnabled, err := adapters.ParseSSLMode(options, "mysql-family", "require", "prefer", "verify-full")
	if err != nil {
		return err
	}
	if !sslEnabled {
		return nil
	}
	switch sslmode {
	case "require", "prefer":
		// The tls.Config for "require"/"prefer" is identical for every connection that uses it
		// (InsecureSkipVerify, nothing host-specific), so registering it once under one fixed,
		// shared name makes every such BuildConfig call idempotent instead of leaking a new entry
		// per connection.
		const tlsName = "kira-mysql-insecure-skip-verify"
		if err := mysql.RegisterTLSConfig(tlsName, &tls.Config{InsecureSkipVerify: true}); err != nil { //nolint:gosec // matches client.ts's own rejectUnauthorized:false for these two modes
			return err
		}
		mc.TLSConfig = tlsName
	case "verify-full":
		// Same reasoning, keyed on the effective ServerName instead of the connection id: every
		// connection to the same host reuses one registration, so the registry's size is bounded
		// by the number of distinct hosts this process has ever connected to (typically small and
		// stable), not by how many connection records the user has created and deleted over the
		// app's lifetime.
		serverName := parseHost(mc.Addr)
		tlsName := "kira-mysql-verify-full:" + serverName
		if err := mysql.RegisterTLSConfig(tlsName, &tls.Config{ServerName: serverName}); err != nil {
			return err
		}
		mc.TLSConfig = tlsName
	}
	return nil
}

// BuildConfig is client.ts's buildConnectionOptions. database overrides cfg's own database when
// non-empty (a side database Get() call).
func BuildConfig(cfg model.ResolvedConnectionConfig, database string, profile Profile, log LogFunc) (*mysql.Config, error) {
	mc := mysql.NewConfig()
	applyFixedDefaults(mc)

	options, err := resolveOptions(mc, cfg, database, log)
	if err != nil {
		return nil, err
	}

	if err := applyTLS(mc, options); err != nil {
		return nil, err
	}

	profile.ApplyEngineOptions(mc, cfg, log)

	return mc, nil
}

// parseHost extracts the bare host from a "host:port" address for use as a TLS ServerName.
// A hand-rolled strings.LastIndex split used to be
// used here instead of the standard library's net.SplitHostPort — for a URI-mode IPv6 address
// (net/url.Parse's own Host field, e.g. "[::1]:3306"), that returned ServerName = "[::1]",
// brackets included, which cannot match any certificate (a real server's certificate names the
// bare address, "::1", never the bracketed literal). net.SplitHostPort strips the brackets
// correctly, matching how every other Go TLS ServerName in this codebase is derived.
func parseHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// A port-less address (net.SplitHostPort's own "missing port in address") is passed
		// through unchanged, matching this function's original tolerant behaviour.
		return addr
	}
	return host
}

// connEntry pairs one database's pinned *sql.Conn with the mutex that serializes every use of it
// (mirroring postgres/client.go's own connEntry): a *sql.Conn
// is not safe for concurrent use by multiple goroutines any more than a *pgx.Conn is, but nothing
// above this package serializes ops against the same Adapter — adapterhost dispatches each inbound
// frame on its own goroutine (bounded only by the session's own in-flight cap), so two Reads on two
// tabs, or a Read racing a console Execute, can and do reach the same pinned connection
// concurrently without this lock, which go-sql-driver on a single-conn *sql.DB (SetMaxOpenConns(1))
// surfaces as a busy-buffer/bad-connection error rather than silently corrupting anything — still a
// real, user-visible failure with no cause the error message names.
//
// inFlight is this entry's own count of still-running
// adapters.RunWithAbortRace background goroutines — see postgres/client.go's own connEntry doc
// comment for the full reasoning (identical here): RunWithAbortRace can return to its own caller
// (on a Stop/cancel) well before the goroutine it spawned stops calling
// conn.QueryContext/conn.ExecContext, so Acquire's release and a detached ROLLBACK/COMMIT cleanup
// must both wait for every such goroutine this acquisition spawned before touching the connection
// again. database/sql serializes concurrent use of one *sql.Conn rather than racing it the way
// *pgx.Conn would, so this is a correctness/hang fix here (a cleanup statement can otherwise queue
// behind a still-running query and blow its own 5s cleanupCtx deadline) rather than a data race.
type connEntry struct {
	db       *sql.DB
	conn     *sql.Conn
	threadID uint32
	// netConn is the socket under conn, captured at dial so Close can unblock a stuck reader.
	netConn net.Conn
	guard   adapters.ConnGuard
}

// alive reports whether the driver still considers the pinned connection usable. A test entry with
// no connection counts as alive.
func (e *connEntry) alive() bool {
	if e.conn == nil {
		return true
	}
	err := e.conn.Raw(func(dc any) error {
		if v, ok := dc.(driver.Validator); ok && !v.IsValid() {
			return driver.ErrBadConn
		}
		return nil
	})
	return err == nil
}

// ConnSet is client.ts's ConnectionSet (mirrors postgres/client.go's ConnSet): one *sql.DB per
// (connection, database), each bounded to a single open connection (SetMaxOpenConns(1)) so a
// pinned *sql.Conn — never a pool — is what every query in this package runs against.
// KILL QUERY needs a known thread id, which a pool does not reliably give you. The LRU pool with
// single-flight dial is adapters.ConnSet — only Dial and the
// eviction Close stay dialect-specific here.
type ConnSet struct {
	cfg     model.ResolvedConnectionConfig
	profile Profile
	log     LogFunc

	inner *adapters.ConnSet[string, *connEntry]
}

// NewConnSet constructs a ConnSet for cfg.
func NewConnSet(cfg model.ResolvedConnectionConfig, profile Profile, log LogFunc) *ConnSet {
	s := &ConnSet{cfg: cfg, profile: profile, log: log}
	s.inner = adapters.NewConnSet(adapters.ConnSetOptions[string, *connEntry]{
		// key normalizes "" (the primary) to primaryKey for the pool's own map/lru; dial itself
		// always wants the original, possibly-empty database argument.
		Dial: func(ctx context.Context, key string) (*connEntry, error) {
			database := key
			if database == primaryKey {
				database = ""
			}
			return s.dial(ctx, database)
		},
		// F6: the wait for an op still holding the connection is bounded; a holder stuck on a
		// dead socket gets the socket closed under it instead.
		Close: func(ctx context.Context, e *connEntry) {
			e.guard.CloseWithin(ctx, closeGrace, func() {
				_ = e.conn.Close()
				_ = e.db.Close()
			}, func() {
				if e.netConn != nil {
					_ = e.netConn.Close()
				}
			})
		},
		Max:     maxConns,
		Primary: primaryKey,
	})
	return s
}

// Entry is one pinned connection plus its own server-assigned thread id, cached at Acquire time
// (the Go-only addition query.ts's own RunningQuery gets for free from the driver's own
// conn.threadId). Conn is embedded so every existing conn.QueryContext/conn.ExecContext call site
// stays unchanged; track/waitInFlight expose the entry's own inFlight WaitGroup.
type Entry struct {
	*sql.Conn
	ThreadID uint32
	entry    *connEntry
}

// track registers one RunWithAbortRace background goroutine against this connection's own entry.
// Call synchronously, at the same point a TrackQuery closure is called — before
// adapters.RunWithAbortRace's own goroutine starts — and compose the returned done into whatever
// release RunWithAbortRace already calls once that goroutine finishes.
func (e Entry) track() (done func()) {
	return e.entry.guard.Track()
}

// running is the RunningQuery to track for a statement run on this connection.
func (e Entry) running() RunningQuery {
	return RunningQuery{ThreadID: e.ThreadID, Guard: &e.entry.guard}
}

// waitInFlight blocks until every RunWithAbortRace goroutine track() has registered against this
// connection has finished touching it. A detached cleanup statement (mutate's own
// ROLLBACK, console's read-only-wrap COMMIT) calls this immediately before its own
// conn.ExecContext, so it never races — or queues, on this dialect, possibly past its own 5s
// deadline — a just-aborted op's background goroutine still using the same *sql.Conn.
func (e Entry) waitInFlight() {
	e.entry.guard.WaitInFlight()
}

// mysqlNewConnector — the exact function dial calls to produce the driver.Connector db.Conn then
// dials through — is a package-level var (the same seam-by-var shape postgres/client.go's
// own pgxConnect establishes) so a test can substitute a fake with a controlled delay and outcome,
// with no real network dial needed, to observe get()'s own single-flight behavior under genuine
// goroutine concurrency.
var mysqlNewConnector = mysql.NewConnector

// dial is get()'s own single-attempt dial, split out so get() can call it with no lock held — the
// driver's own network round trip (db.Conn, then SELECT CONNECTION_ID()) is the whole point of the
// dialing placeholder above being able to run unlocked.
func (s *ConnSet) dial(ctx context.Context, database string) (*connEntry, error) {
	mc, err := BuildConfig(s.cfg, database, s.profile, s.log)
	if err != nil {
		return nil, err
	}
	// The driver's own dial, wrapped only to keep the socket for Close's force path.
	var captured net.Conn
	var dialer net.Dialer
	mc.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		c, err := dialer.DialContext(ctx, network, addr)
		if err == nil {
			captured = c
		}
		return c, err
	}
	connector, err := mysqlNewConnector(mc)
	if err != nil {
		return nil, mapError(err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		_ = db.Close()
		return nil, mapError(err)
	}
	var threadID uint32
	if err := conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&threadID); err != nil {
		_ = conn.Close()
		_ = db.Close()
		return nil, mapError(err)
	}

	// A read-only connection is enforced by the server itself, not just Mutate's own app-level
	// AssertWritable check — matching ClickHouse's readonly=2 and SQLite's mode=ro precedent, and
	// closing the gap the console's Execute() would otherwise leave (it has no per-statement
	// write/read classifier of its own, unlike Redis/Mongo's console).
	if s.cfg.ReadOnly {
		if _, err := conn.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
			_ = conn.Close()
			_ = db.Close()
			return nil, mapError(err)
		}
	}

	return &connEntry{db: db, conn: conn, threadID: threadID, netConn: captured}, nil
}

// Acquire returns database's connection (empty string means the primary) together with a release
// func that must be called exactly once, however the caller's own use of it ends — it holds the
// per-connection lock connEntry's own doc comment describes, for the caller's own entire op, not
// just one statement: a Mutate's several sequential statements or a console Execute's "run all"
// must keep any concurrent op off this same conn for its whole duration, not just between
// individual statements, or a racing Read could execute between two of them (mirroring
// postgres/client.go's own Acquire and its identical reasoning).
func (s *ConnSet) Acquire(ctx context.Context, database string) (Entry, func(), error) {
	key := database
	if key == "" {
		key = primaryKey
	}
	for {
		entry, err := s.inner.Get(ctx, key)
		if err != nil {
			return Entry{}, nil, err
		}
		if err := entry.guard.Lock(ctx); err != nil {
			return Entry{}, nil, err
		}
		// An LRU eviction's own Close (adapters.ConnSet.Get's own doc comment) contends for this
		// same entry.mu, so it may already have closed entry.conn by the time this Lock succeeds.
		// Re-check that entry is still the set's own live entry for key before trusting it — retry
		// from the top rather than hand back a connection that was just closed out from under it.
		if current, ok := s.inner.Current(key); !ok || current != entry {
			entry.guard.Unlock()
			continue
		}
		// F5: a dead connection (the driver closed it after a cancelled or broken query) stays this
		// key's entry forever otherwise; drop it and dial a fresh one.
		if !entry.alive() {
			entry.guard.Unlock()
			s.inner.Drop(ctx, key, entry)
			continue
		}
		return Entry{Conn: entry.conn, ThreadID: entry.threadID, entry: entry}, func() {
			// F2: hold this connection's lock until every RunWithAbortRace goroutine started under this
			// acquisition has actually finished touching entry.conn, and a Cancel aimed at it has been
			// delivered (F7).
			entry.guard.Release()
		}, nil
	}
}

// Primary acquires the primary (no explicit database override) connection.
func (s *ConnSet) Primary(ctx context.Context) (Entry, func(), error) {
	return s.Acquire(ctx, "")
}

// CloseAll closes every open connection, taking each one's own lock first (P21 round 2 performance
// finding 3 — see the Close closure NewConnSet builds above).
func (s *ConnSet) CloseAll(ctx context.Context) {
	s.inner.CloseAll(ctx)
}
