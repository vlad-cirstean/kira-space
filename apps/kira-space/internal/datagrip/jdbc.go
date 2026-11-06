package datagrip

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// defaultPort mirrors packages/shared/domain/connection.ts's DEFAULT_PORT — kept as its own copy
// here rather than a cross-language import, the same way every other Go package in this repo
// re-states a handful of adapter constants (see CLAUDE.md's per-adapter port literals).
var defaultPort = map[string]int{
	"postgres":   5432,
	"mariadb":    3306,
	"mysql":      3306,
	"clickhouse": 8123,
	"mongodb":    27017,
	"redis":      6379,
}

// dbmsToKind implements D6 signal 1 — DataGrip's own detected DBMS, the strongest signal when
// present.
var dbmsToKind = map[string]string{
	"POSTGRES":   "postgres",
	"MYSQL":      "mysql",
	"MARIADB":    "mariadb",
	"SQLITE":     "sqlite",
	"CLICKHOUSE": "clickhouse",
	"MONGO":      "mongodb",
	"REDIS":      "redis",
}

// driverFamilyToKind implements D6 signal 2 — everything in <driver-ref> before the first '.'.
var driverFamilyToKind = map[string]string{
	"postgresql": "postgres",
	"mysql":      "mysql",
	"mariadb":    "mariadb",
	"sqlite":     "sqlite",
	"clickhouse": "clickhouse",
	"mongo":      "mongodb",
	"redis":      "redis",
}

// urlSchemeToKind implements D6 signal 3 — the <jdbc-url> scheme, checked as a set of literal
// prefixes rather than a generic scheme parse: "jdbc:ch:" has no dot-separated "family" the way
// driver-ref does, and mongo/redis URLs are not always jdbc:-prefixed.
var urlSchemePrefixes = []struct {
	prefix string
	kind   string
}{
	{"jdbc:postgresql:", "postgres"},
	{"jdbc:mysql:", "mysql"},
	{"jdbc:mariadb:", "mariadb"},
	{"jdbc:sqlite:", "sqlite"},
	{"jdbc:clickhouse:", "clickhouse"},
	{"jdbc:ch:", "clickhouse"},
	{"jdbc:mongodb:", "mongodb"},
	{"mongodb://", "mongodb"},
	{"mongodb+srv://", "mongodb"},
	{"jdbc:redis:", "redis"},
	{"redis://", "redis"},
	{"rediss://", "redis"},
}

// driverFamily is everything in a <driver-ref> before the first '.' (F1: "postgresql",
// "mysql.8", "sqlite.xerial", "mongo.4" — the convention is "<family>[.<variant>]").
func driverFamily(driverRef string) string {
	family, _, _ := strings.Cut(driverRef, ".")
	return family
}

// mapEngine is D6's three-signal cascade, first hit wins. ok is false when nothing matches
// (Oracle, SQL Server, H2, Snowflake, …) — label is what the caller quotes back in the
// unsupported-engine reason (the driver-ref verbatim when there is one, else the dbms, else the
// literal jdbc-url).
func mapEngine(ds DataSource) (kind string, ok bool, label string) {
	if ds.DBMS != "" {
		if k, hit := dbmsToKind[ds.DBMS]; hit {
			return k, true, ""
		}
	}
	if ds.DriverRef != "" {
		if k, hit := driverFamilyToKind[driverFamily(ds.DriverRef)]; hit {
			return k, true, ""
		}
	}
	for _, s := range urlSchemePrefixes {
		if strings.HasPrefix(ds.JDBCURL, s.prefix) {
			return s.kind, true, ""
		}
	}
	switch {
	case ds.DriverRef != "":
		return "", false, ds.DriverRef
	case ds.DBMS != "":
		return "", false, ds.DBMS
	default:
		// ds.JDBCURL is the one label case that can itself carry a "user:password@host" userinfo
		// credential (the configured-by-url exception embeds the password directly in the URL) —
		// this label crosses the bridge verbatim as PreviewRow.SkipDetail/
		// ReportRow.Error, which must never carry a password (the guarantee).
		return "", false, redactURLCredentials(ds.JDBCURL)
	}
}

// expandMacros implements D7's IDE path-macro expansion — $PROJECT_DIR$ and $ProjectFileDir$ both
// resolve to the folder the picker returned (F1: both names appear in real files for the same
// thing), and $USER_HOME$ to the real home directory.
func expandMacros(raw, projectDir string) string {
	out := strings.ReplaceAll(raw, "$PROJECT_DIR$", projectDir)
	out = strings.ReplaceAll(out, "$ProjectFileDir$", projectDir)
	if strings.Contains(out, "$USER_HOME$") {
		home, err := os.UserHomeDir()
		if err == nil {
			out = strings.ReplaceAll(out, "$USER_HOME$", home)
		}
	}
	return out
}

// ResolvedFields is D7's field mapping — everything Apply needs to build a
// model.ConnectionFields-shaped Input for one importable row, plus a from-URL password when D7's
// configured-by-url exception applies.
type ResolvedFields struct {
	Kind     string
	Host     *string
	Port     *int
	Database *string
	Username *string
	// Password/FromURL: D7's configured-by-url exception (F3) — a password that arrived inside
	// the JDBC URL's own userinfo, used directly with no credential-store lookup.
	Password *string
	FromURL  bool
}

// resolveFields is D7. skipCode is one of D11's four "no row at all" reasons
// (ReasonUnsupportedEngine, ReasonUnrepresentableURL, ReasonSQLitePathNotAbsolute,
// ReasonNoJDBCURL) when ok is false — every other D11 code is about the *password*, not the row.
func resolveFields(ds DataSource, projectDir string) (fields ResolvedFields, ok bool, skipCode string, skipDetail string) {
	kind, mapped, label := mapEngine(ds)
	if !mapped {
		return ResolvedFields{}, false, ReasonUnsupportedEngine, label
	}
	fields.Kind = kind

	if strings.TrimSpace(ds.JDBCURL) == "" {
		return ResolvedFields{}, false, ReasonNoJDBCURL, ""
	}
	expanded := expandMacros(ds.JDBCURL, projectDir)

	if kind == "sqlite" {
		path := stripJDBCPrefix(expanded, "jdbc:sqlite:")
		path = strings.TrimSpace(path)
		if path == "" {
			return ResolvedFields{}, false, ReasonNoJDBCURL, ""
		}
		if !strings.HasPrefix(path, "/") {
			return ResolvedFields{}, false, ReasonSQLitePathNotAbsolute, path
		}
		fields.Database = &path
		fields.Username = optionalString(ds.Username)
		applyConfiguredByURLPassword(&fields, ds, expanded)
		return fields, true, "", ""
	}

	rest := expanded
	rest = stripAnyJDBCPrefix(rest)
	if strings.Contains(rest, "mongodb+srv://") {
		// expanded can itself carry a "user:password@host" userinfo credential (the
		// configured-by-url exception, or any URL DataGrip otherwise embedded one into) —
		// this crosses the bridge verbatim as PreviewRow.SkipDetail/ReportRow.Error, which
		// must never carry a password (the guarantee).
		return ResolvedFields{}, false, ReasonUnrepresentableURL, redactURLCredentials(expanded)
	}

	u, err := url.Parse(rest)
	if err != nil || u.Host == "" {
		// expanded can itself carry a "user:password@host" userinfo credential (the
		// configured-by-url exception, or any URL DataGrip otherwise embedded one into) — this
		// crosses the bridge verbatim as PreviewRow.SkipDetail/ReportRow.Error, which must never
		// carry a password.
		return ResolvedFields{}, false, ReasonUnrepresentableURL, redactURLCredentials(expanded)
	}
	if strings.Contains(u.Host, ",") {
		// A comma-separated multi-host URL (Postgres/MongoDB replica-set style):
		// canRoundTripToFields only ever represents a single host.
		// expanded can itself carry a "user:password@host" userinfo credential (the
		// configured-by-url exception, or any URL DataGrip otherwise embedded one into):
		// this crosses the bridge verbatim as PreviewRow.SkipDetail/ReportRow.Error, which must
		// never carry a password (the guarantee).
		return ResolvedFields{}, false, ReasonUnrepresentableURL, redactURLCredentials(expanded)
	}

	host := u.Hostname()
	if host == "" {
		// expanded can itself carry a "user:password@host" userinfo credential
		// (the configured-by-url exception, or any URL DataGrip otherwise embedded one into) — this
		// crosses the bridge verbatim as PreviewRow.SkipDetail/ReportRow.Error, which must never
		// carry a password (the guarantee).
		return ResolvedFields{}, false, ReasonUnrepresentableURL, redactURLCredentials(expanded)
	}
	fields.Host = &host

	if portStr := u.Port(); portStr != "" {
		if p, perr := strconv.Atoi(portStr); perr == nil {
			fields.Port = &p
		}
	}
	if fields.Port == nil {
		if p, hasDefault := defaultPort[kind]; hasDefault {
			fields.Port = &p
		} else {
			// "a kind with no default and no port in the URL is a skip" — none of the kinds can
			// currently map to lacks a default (see defaultPort above), so this is unreached today;
			// kept as a named guard rather than a silent nil port reaching
			// connections.Input.Validate's own "Port is required" rejection.
			return ResolvedFields{}, false, ReasonNoJDBCURL, expanded
		}
	}

	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		fields.Database = &db
	}
	fields.Username = optionalString(ds.Username)
	applyConfiguredByURLPassword(&fields, ds, expanded)
	// A username embedded in the URL's own userinfo (configured-by-url) takes over from the
	// local file's <user-name> only when the local file had none — <user-name> is written by
	// DataGrip itself and is the more authoritative of the two when both exist.
	if u.User != nil && fields.Username == nil {
		if name := u.User.Username(); name != "" {
			fields.Username = &name
		}
	}
	return fields, true, "", ""
}

// applyConfiguredByURLPassword is D7's exception: a <configured-by-url>true</…> source can carry
// user:password@ in its JDBC URL (F3, the one documented case where a password needs no
// credential-store access at all).
func applyConfiguredByURLPassword(fields *ResolvedFields, ds DataSource, expanded string) {
	if !ds.ConfiguredByURL {
		return
	}
	rest := stripAnyJDBCPrefix(expanded)
	u, err := url.Parse(rest)
	if err != nil || u.User == nil {
		return
	}
	if pw, hasPw := u.User.Password(); hasPw && pw != "" {
		fields.Password = &pw
		fields.FromURL = true
	}
}

// redactURLCredentials returns raw with any embedded "user:password@" (or bare "user@") userinfo,
// and any query parameter that looks like a credential, replaced by a fixed placeholder — the one
// thing standing between a configured-by-url source's JDBC URL and the "a password never crosses
// the bridge" guarantee once that URL is quoted back in a user-facing skip/error string
// (PreviewRow.SkipDetail, ReportRow.Error). A real net/url round-trip is deliberately not used
// here: every caller of this function is on a path where the URL is, by definition, one net/url
// may not parse cleanly (mongodb+srv, a comma-separated multi-host authority, an
// unmapped/unsupported scheme) — a plain scan-and-mask over the raw text works uniformly across
// all of them, parseable or not.
//
// Once a real "://" scheme boundary is found, the last '@' after it used to be
// treated as a userinfo delimiter only when nothing between the scheme and it looked like a raw
// '/', '?' or '#' — a password containing one of those (e.g.
// "jdbc:postgresql://u:pa/ss@h1,h2/db") fell through that guard and returned raw *unredacted*,
// password included. Now redacted unconditionally in that case: once "://" is found, everything up
// to the next '@' IS the authority, and a raw '/', '?' or '#' inside it can only mean an unencoded
// password, never a coincidence — over-redacting a small amount of non-credential error text is an
// acceptable tradeoff against ever leaking a real one. Without a "://" at all (e.g. sqlite's own
// "jdbc:sqlite:/path" — no authority to anchor on), the old conservative check still applies: an
// '@' that turns out to sit in a bare path/filename ("jdbc:sqlite:/db/user@host.sqlite") is left
// alone rather than mangling text that was never a credential. redactQueryCredentials below is
// this function's own second half — a query-string password (?user=u&password=secret) was never
// masked at all before this fix, regardless of userinfo.
func redactURLCredentials(raw string) string {
	return redactQueryCredentials(redactUserinfo(raw))
}

// redactUserinfo is redactURLCredentials' own userinfo half.
func redactUserinfo(raw string) string {
	at := strings.LastIndex(raw, "@")
	if at == -1 {
		return raw
	}
	schemeEnd := strings.LastIndex(raw[:at], "://")
	if schemeEnd == -1 {
		userinfo := raw[:at]
		if userinfo == "" || strings.ContainsAny(userinfo, "/?#") {
			return raw
		}
		return "REDACTED" + raw[at:]
	}
	start := schemeEnd + len("://")
	if raw[start:at] == "" {
		return raw
	}
	return raw[:start] + "REDACTED" + raw[at:]
}

// credentialQueryKeys names the substrings (case-insensitive) redactQueryCredentials treats a
// query parameter's key as a credential for — covers the common spellings a JDBC URL's own query
// string uses (password, pwd, secret, token, apiToken, access_token, …) without needing an exact
// key match.
var credentialQueryKeys = []string{"pass", "pwd", "secret", "token"}

// redactQueryCredentials masks the value of every query parameter (raw's first '?' to its next
// '#', or to its end) whose key contains one of credentialQueryKeys, case-insensitively. A key
// with no matching substring, or a value-less key (bare "?flag"), is left untouched.
func redactQueryCredentials(raw string) string {
	q := strings.IndexByte(raw, '?')
	if q < 0 {
		return raw
	}
	query := raw[q+1:]
	fragment := ""
	if h := strings.IndexByte(query, '#'); h >= 0 {
		query, fragment = query[:h], query[h:]
	}
	if query == "" {
		return raw
	}

	pairs := strings.Split(query, "&")
	for i, pair := range pairs {
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue
		}
		key, value := pair[:eq], pair[eq+1:]
		if value == "" {
			continue
		}
		lower := strings.ToLower(key)
		for _, marker := range credentialQueryKeys {
			if strings.Contains(lower, marker) {
				pairs[i] = key + "=REDACTED"
				break
			}
		}
	}
	return raw[:q+1] + strings.Join(pairs, "&") + fragment
}

func stripJDBCPrefix(raw, prefix string) string {
	return strings.TrimPrefix(raw, prefix)
}

// stripAnyJDBCPrefix removes a leading "jdbc:" so the remainder parses as a normal
// scheme://host[:port]/path URL — DataGrip's own jdbc-url values are either "jdbc:<scheme>://…"
// or (for mongo/redis in some captures) a bare "<scheme>://…" already.
func stripAnyJDBCPrefix(raw string) string {
	return strings.TrimPrefix(raw, "jdbc:")
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// kindAccent mirrors ConnectionDialog.vue's own KIND_ACCENT (D7): an imported connection gets the
// same rail color a hand-made one of the same kind would, rather than standing out as imported.
// Only the kinds D6 can ever map onto need an entry — kafka/sqs/s3 have no DataGrip counterpart.
var kindAccent = map[string]string{
	"postgres":   "cyan",
	"mariadb":    "blue",
	"mysql":      "teal",
	"sqlite":     "violet",
	"clickhouse": "orange",
	"mongodb":    "green",
	"redis":      "red",
}

// truncateName is D7's 120-character name cap, applied before the connection is ever built —
// connections.Input.Validate *rejects* a name over 120 UTF-16 units rather than truncating it, so the
// importer must already have shortened it, and separately report the truncation as a warning.
// Cuts on a rune boundary so the result is never invalid UTF-8.
func truncateName(name string) (out string, truncated bool) {
	return model.TruncateUTF16(name, 120)
}
