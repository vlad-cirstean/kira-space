package clickhouse

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// fixedSettings travel as URL query parameters on every request (no persistent client-level
// session). `readonly` is not here: query.go sends it per request, never on Cancel's KILL QUERY.
// output_format_json_quote_64bit_integers and output_format_json_validate_utf8 are gone: a
// readonly=1 profile refuses to change either (measured), catalog SQL casts 64-bit columns with
// toString and Go's JSON decode already replaces invalid UTF-8. A readonly=1 profile gets
// no settings at all (query.go buildURL).
var fixedSettings = map[string]string{
	"show_table_uuid_in_table_create_query_if_not_nil": "0",
	"date_time_output_format":                          "simple",
}

// defaultFormat is an HTTP parameter, not a setting: allowed under every profile.
const defaultFormat = "JSONCompactStringsEachRowWithNamesAndTypes"

// Handle is client.ts's ClickHouseHandle — B11: a plain *http.Client, no ClickHouse driver at all.
type Handle struct {
	Client          *http.Client
	URL             string
	Username        string
	Password        string
	DefaultDatabase string
	ReadOnly        bool
	// noEngineTimeout is set once the server refused the timeout settings (a readonly=1 profile):
	// later requests skip them and the adapter's app timer enforces the limit.
	noEngineTimeout atomic.Bool
	// profileReadonly is the profile's `readonly` setting (0, 1 or 2) read at Connect.
	profileReadonly atomic.Int32
	// probed is set once profileReadonly is known; until then no settings are sent.
	probed atomic.Bool
}

// settingsAllowed reports whether per-request settings may be sent: probed, and not a readonly=1
// profile (which refuses any settings change).
func (h *Handle) settingsAllowed() bool { return h.probed.Load() && h.profileReadonly.Load() != 1 }

// sendReadonly reports whether the per-request readonly=2 may be sent: only a readonly=0 profile
// can raise it (readonly=2 is sticky, readonly=1 refuses the change).
func (h *Handle) sendReadonly() bool { return h.probed.Load() && h.profileReadonly.Load() == 0 }

type resolvedTarget struct {
	scheme   string
	host     string
	port     int
	database string
	username string
	password string
}

// targetFromURI is resolveTarget's own URI-parse branch (cfg.Mode == "uri"): host/port/database/
// username/password out of a parsed connection URI.
func targetFromURI(rawURI string) (host, database, username, password *string, port *int, err error) {
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, nil, nil, nil, nil, adapters.T(adapters.CodeConnect, "backend.studio.adapter.uriParseFailed.body", nil, err)
	}
	h := u.Hostname()
	host = &h
	if p := u.Port(); p != "" {
		if n, perr := parsePort(p); perr == nil {
			port = &n
		}
	}
	d := u.Path
	if len(d) > 0 && d[0] == '/' {
		d = d[1:]
	}
	if d != "" {
		database = &d
	}
	if u.User != nil {
		un := u.User.Username()
		username = &un
		if pw, ok := u.User.Password(); ok {
			password = &pw
		}
	}
	return host, database, username, password, port, nil
}

// targetFromFields is resolveTarget's own fields-mode branch: cfg's Host/Port/Database/Username/
// Password fields, passed straight through.
func targetFromFields(cfg model.ResolvedConnectionConfig) (host, database, username, password *string, port *int) {
	return cfg.Host, cfg.Database, cfg.Username, cfg.Password, cfg.Port
}

// resolveScheme is resolveTarget's own sslmode switch — D12: sslmode's only real distinction here
// is http vs https; require/verify-full/prefer all just mean "speak https", the same collapsed
// distinction client.ts's own comment already committed to for this driver.
func resolveScheme(options map[string]any) (string, error) {
	// `prefer` is a valid, documented value for
	// every other engine (postgres/mysqlfamily/redis/mongo/kafka) — ClickHouse rejecting it
	// was the one adapter out of step, and since Options only ever comes from a connection
	// string's own query parameters (there is no fields-mode sslmode UI), a user carrying one
	// connection-string style across engines hit this immediately. ClickHouse has no
	// plaintext-with-opportunistic-upgrade transport, so `prefer` means the same as `require`
	// here, same as it already does for postgres/mysqlfamily.
	_, sslEnabled, err := adapters.ParseSSLMode(options, "clickhouse", "require", "prefer", "verify-full")
	if err != nil {
		return "", err
	}
	if sslEnabled {
		return "https", nil
	}
	return "http", nil
}

// resolveTarget is client.ts's own.
func resolveTarget(cfg model.ResolvedConnectionConfig, log func(level, message string)) (resolvedTarget, error) {
	var host, database, username, password *string
	var port *int
	var err error

	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		host, database, username, password, port, err = targetFromURI(*cfg.URI)
		if err != nil {
			return resolvedTarget{}, err
		}
	} else {
		host, database, username, password, port = targetFromFields(cfg)
	}
	if host == nil || *host == "" {
		return resolvedTarget{}, adapters.T(adapters.CodeConnect, "backend.studio.adapter.hostMissing.body", nil, nil)
	}

	scheme, err := resolveScheme(cfg.Options)
	if err != nil {
		return resolvedTarget{}, err
	}

	target := resolvedTarget{scheme: scheme, host: *host, port: 8123, database: "default"}
	if port != nil {
		target.port = *port
	}
	if database != nil && *database != "" {
		target.database = *database
	}
	if username != nil {
		target.username = *username
	}
	if password != nil {
		target.password = *password
	}
	return target, nil
}

func parsePort(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, adapters.T(adapters.CodeConnect, "backend.studio.adapter.portInvalid.body", nil, nil)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// dialTimeout/tlsHandshakeTimeout bound connection *setup* only — never a query's own
// running time. http.Client's own Timeout field (removed here) covers the whole request including
// reading the response body, so any query genuinely running longer than it failed client-side with
// no KILL QUERY ever sent, breaking the Stop-button contract Postgres already honours via
// statement_timeout=0 (per ClickHouse's own defaults the server keeps running a query after the
// client gives up on it). Cancellation is ctx-driven instead (query.go's own
// http.NewRequestWithContext, honoured natively), matching every other adapter's own contract; a
// local abort sends KILL QUERY over a side request exactly like Cancel already does.
const (
	dialTimeout         = 10 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
)

// OpenClient is client.ts's openClient — B11/B7: MaxIdleConnsPerHost is at least 2 so Cancel's own
// KILL QUERY request always has a free connection in the pool (caps.ts's own F7/F9 note: "a second
// HTTP request the client's own connection pool already has free").
func OpenClient(cfg model.ResolvedConnectionConfig, log func(level, message string)) (*Handle, error) {
	target, err := resolveTarget(cfg, log)
	if err != nil {
		return nil, err
	}
	transport := &http.Transport{
		MaxIdleConnsPerHost: 4,
		DialContext:         (&net.Dialer{Timeout: dialTimeout}).DialContext,
		TLSHandshakeTimeout: tlsHandshakeTimeout,
	}
	client := &http.Client{Transport: transport}
	u := target.scheme + "://" + target.host + ":" + strconv.Itoa(target.port)
	return &Handle{
		Client: client, URL: u, Username: target.username, Password: target.password,
		DefaultDatabase: target.database, ReadOnly: cfg.ReadOnly,
	}, nil
}
