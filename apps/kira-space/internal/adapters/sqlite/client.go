package sqlite

import (
	"net/url"
	"os"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// busyTimeoutMS is client.ts's BUSY_TIMEOUT_MS — node:sqlite's own default is 0 (fail immediately
// on a lock); 5s matches storage/db.ts's own `PRAGMA busy_timeout` for Kira's app database.
const busyTimeoutMS = "5000"

// resolveFilePath is client.ts's resolveFilePath. Fields mode repurposes `database` for
// the absolute path; URI mode's `sqlite:////abs/path` already round-trips through
// net/url.Parse the same way parseConnectionUri's own new URL(uri) does — u.Path comes back
// "//abs/path", one leading slash short of the real path, exactly like JS's url.pathname does.
// One divergence from the TS port, not a bug: Go's net/url.Parse already percent-decodes Path,
// unlike JS's URL.pathname (which never decodes it — client.ts calls decodeURIComponent itself for
// exactly that reason). Decoding here too would double-decode a literal "%" in a real path, so this
// does not call url.PathUnescape a second time.
func resolveFilePath(cfg model.ResolvedConnectionConfig) (string, error) {
	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		u, err := url.Parse(*cfg.URI)
		if err != nil {
			return "", adapters.T(adapters.CodeConnect, "backend.studio.adapter.uriParseFailed.body", nil, err)
		}
		path := strings.TrimPrefix(u.Path, "/")
		if path == "" {
			return "", adapters.T(adapters.CodeConnect, "backend.studio.adapter.uriParseFailed.body", nil, nil)
		}
		if err := rejectDSNMetacharacters(path); err != nil {
			return "", err
		}
		return path, nil
	}
	path := ""
	if cfg.Database != nil {
		path = strings.TrimSpace(*cfg.Database)
	}
	if path == "" {
		return "", adapters.T(adapters.CodeConnect, "backend.studio.adapter.databaseFileMissing.body", nil, nil)
	}
	if err := rejectDSNMetacharacters(path); err != nil {
		return "", err
	}
	return path, nil
}

// rejectDSNMetacharacters: buildDSN below builds
// the SQLite URI filename by plain string concatenation ("file:" + path + "?" + query), and
// SQLite's own URI parser splits on the *first* `?` (and treats `#` as starting a fragment) — so a
// path containing either character silently truncates the filename and lets the remainder be read
// as query parameters, which is exactly where `mode=ro`/`_query_only=1` (this adapter's entire
// read-only enforcement) live. A `database` value of `/data/x.db?mode=rwc&_query_only=0` would
// defeat the read-only toggle quietly, with the UI still showing the connection as read-only, and
// assertFileExists (which stats the real, un-truncated path) would not catch it either. Rejecting
// outright — rather than percent-encoding the path — keeps the read-only guarantee an explicit
// refusal rather than a best-effort escape.
//
// `%` is rejected too: SQLite's own URI filename parsing percent-decodes the path
// component (e.g. a literal `%2F`/`%20`), so a path assertFileExists already confirmed exists could
// still open a *different* file once SQLite decodes it — defeating that safeguard the same way an
// unrejected `?`/`#` would defeat the read-only toggle above. buildDSN never percent-encodes path
// itself, so the safe fix is refusing a raw `%` outright rather than assuming the caller meant one
// literal byte.
func rejectDSNMetacharacters(path string) error {
	if strings.ContainsAny(path, "?#%") {
		return adapters.T(adapters.CodeConnect, "backend.studio.adapter.databasePathCharacters.body", nil, nil)
	}
	return nil
}

// assertFileExists is client.ts's assertFileExists — Kira never creates a database. A plain
// DSN open would also silently create an empty file at path when nothing is there (verified: see
// buildDSN's own mode=ro/mode=rw comment) — the worst failure mode for a tool whose own promise is
// that DDL is read-only. Checked before the driver ever touches the path, so a typo fails as
// E_NOT_FOUND rather than "connecting" to a database Kira itself just created.
func assertFileExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return adapters.New(adapters.CodeNotFound, "no database file at \""+path+"\"", nil)
	}
	if !info.Mode().IsRegular() {
		return adapters.New(adapters.CodeConnect, "\""+path+"\" is not a regular file", nil)
	}
	return nil
}

// buildDSN is client.ts's own DSN construction, minus the options node:sqlite took as a struct —
// modernc.org/sqlite takes everything through the DSN's query string instead. `mode=ro`/
// `mode=rw`, never `rwc`: SQLite's own URI filename parsing honours `mode` directly (verified: a
// missing file under `mode=ro` or `mode=rw` fails immediately as SQLITE_CANTOPEN and creates
// nothing; the default no-mode DSN silently creates the file, which is exactly why
// assertFileExists runs first as the primary defence and this is only the second line of it).
func buildDSN(path string, readOnly bool) string {
	q := url.Values{}
	q.Set("_busy_timeout", busyTimeoutMS)
	q.Set("_foreign_keys", "1")
	q.Set("_txlock", "immediate")
	if readOnly {
		q.Set("_query_only", "1")
		q.Set("mode", "ro")
	} else {
		q.Set("mode", "rw")
	}
	return "file:" + path + "?" + q.Encode()
}
