// Package mcpauth mints, persists and verifies the DB MCP server's static bearer token.
// 32 crypto/rand bytes, base64url on the wire,
// sha256(salt‖token) at rest, one token per server instance, persisted as one small JSON file
// rather than a database table: the caller supplies the file's own identifying name (the DB
// server's fixed name), so this package stays storage-shape-agnostic and importable from both
// internal/dbmcp (verify) and internal/bridge (mint) with no shared database dependency.
//
// Every token now carries a 7-day expiry (TTL), rotated by remint — never mid-flight, only at a
// moment a human can read the fresh plaintext (server start, or an explicit Regenerate). See
// MintTTL/LoadOrMintTTL/Check.
package mcpauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio/v2"
	"github.com/kirathecat/kira-studio/internal/tokenauth"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

// TTL is every token's fixed lifetime, applied uniformly to both servers (M1 §2.1).
const TTL = 7 * 24 * time.Hour

// Record is the at-rest shape: hash+salt only, the plaintext is never written to disk as THIS
// record — HelperTokenPathNamed/SaveHelperToken below are a separate, deliberately scoped
// exception, a different file for a different purpose, never this one.
// ExpiresAt's zero value means "no expiry recorded yet", so a stamp-on-load (LoadOrMintTTL) can
// apply the property retroactively with no migration and no file-format version.
type Record struct {
	Hash      []byte    `json:"hash"`
	Salt      []byte    `json:"salt"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// MintTTL returns a fresh token in both forms D8 needs: plain (rendered into the registration
// command exactly once) and the Record (what gets persisted), with ExpiresAt set to now+ttl. A
// short read or any crypto/rand error is a hard failure — never a weaker token.
func MintTTL(ttl time.Duration) (plain string, rec Record, err error) {
	plain, hash, salt, err := tokenauth.Mint()
	if err != nil {
		return "", Record{}, fmt.Errorf("mcpauth: mint: %w", err)
	}
	return plain, Record{
		Hash:      hash,
		Salt:      salt,
		ExpiresAt: time.Now().Add(ttl),
	}, nil
}

// Verify recomputes sha256(salt‖presented) and compares against rec.Hash in constant time. Does
// not check expiry — Check wraps this plus the expiry test for the two-outcome distinction a
// lapsed-token response needs.
func Verify(presented string, rec Record) bool {
	return tokenauth.Verify(presented, rec.Hash, rec.Salt)
}

// Expired reports whether rec has lapsed as of now. A zero ExpiresAt (a pre-M1 file not yet
// stamped) never expires.
func Expired(rec Record, now time.Time) bool {
	return !rec.ExpiresAt.IsZero() && !now.Before(rec.ExpiresAt)
}

// Outcome distinguishes the three cases a bearer check can land in — a lapsed token must not be
// reported as a wrong one, since the remedies differ (M1 §2.5).
type Outcome int

const (
	OutcomeValid Outcome = iota
	OutcomeExpired
	OutcomeInvalid
)

// Check compares hashes first and only then tests expiry, so a wrong token never learns the real
// one's expiry instant.
func Check(presented string, rec Record, now time.Time) Outcome {
	if !Verify(presented, rec) {
		return OutcomeInvalid
	}
	if Expired(rec, now) {
		return OutcomeExpired
	}
	return OutcomeValid
}

// TokenVerifier is the auth.TokenVerifier the DB MCP server mounts. load reads the server's own
// current record under its own lock, so a Regenerate mid-flight is picked up by the very next
// request. label names the server in the lapsed-token error text ("kira-db").
func TokenVerifier(label string, load func() Record) auth.TokenVerifier {
	return func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		rec := load()
		switch Check(token, rec, time.Now()) {
		case OutcomeValid:
			return &auth.TokenInfo{}, nil
		case OutcomeExpired:
			// F8: post-F2, the registered command never carries the token — the live plaintext
			// lives only in the helper token mirror file, which Regenerate rewrites in place. No
			// re-registration is needed on expiry, unlike before F2.
			return nil, fmt.Errorf("%w: the %s MCP token expired at %s. Press Regenerate in Kira Studio's Database MCP settings to mint a fresh one — no need to re-register this server.",
				auth.ErrInvalidToken, label, rec.ExpiresAt.Format(time.RFC3339))
		default:
			return nil, auth.ErrInvalidToken
		}
	}
}

// PathNamed returns name's token file location under home — home is normally KIRA_HOME.
func PathNamed(home, name string) string {
	return filepath.Join(home, name+"-token.json")
}

// HelperTokenPathNamed returns name's plaintext headersHelper mirror file location under home — a
// deliberate, scoped exception to this package's own hash-only-at-rest rule above: Claude Code's
// own `headersHelper` mechanism (verified against the installed CLI, 2.1.280 — undocumented in
// `--help` but real: it runs a local command and parses its stdout as a JSON object of header
// values) lets `mcpinstall` register this server without ever putting the bearer token on that
// process's own argv, but the helper script it writes has to read the LIVE plaintext from somewhere
// at connection time, including well after a restart mid-token-life, when the hash-only Record
// above no longer carries a recoverable plaintext at all. This file exists for that helper script
// to read — 0600 (SaveHelperToken) — and is also read back by this app itself (LoadHelperToken) to
// confirm the mirror still verifies against the live Record before showing Command/Install, never
// for any other purpose — narrowing the exposure from the original threat (nothing on disk,
// anywhere) to "readable by this OS user account, same as every other local process already running
// as it", which is the same posture internal/localsock's own 0700 socket directory already takes,
// and strictly better than the argv exposure it replaces (readable by every local uid via /proc,
// and by exec-event EDR logging).
func HelperTokenPathNamed(home, name string) string {
	return filepath.Join(home, name+"-header.token")
}

// Load reads path's Record. A missing file returns ok=false with no error — there is nothing wrong
// with a server instance that has never been enabled or started before.
func Load(path string) (Record, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, false, nil
		}
		return Record{}, false, fmt.Errorf("mcpauth: read %s: %w", path, err)
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return Record{}, false, fmt.Errorf("mcpauth: parse %s: %w", path, err)
	}
	return rec, true, nil
}

// Save writes rec to path, mode 0600 — hash+salt only, never the plaintext. Creates path's
// parent directory first (mode 0700) — a machine where KIRA_HOME does not exist yet otherwise
// fails outright on the very first mint.
//
// Writes via a temp file + rename rather than truncating path in place: a crash or a
// full disk mid-write used to leave a truncated token file that Load then fails to parse, taking
// the whole server down at next start with no path forward but deleting a file the user has no
// reason to know about. Rename is atomic on both target platforms (POSIX same-filesystem rename;
// this repo's own KIRA_HOME layout keeps the temp file alongside the real one, so it is), so a
// reader only ever sees the old complete file or the new complete file, never a partial one.
func Save(path string, rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("mcpauth: encode %s: %w", path, err)
	}
	return writeFile(path, data, 0o600)
}

// SaveHelperToken persists plain to path (HelperTokenPathNamed's own path), mode 0600, atomically
// — the same temp-file-plus-rename discipline Save uses, so a reader (the headersHelper script)
// never observes a truncated file mid-write. Call this every time a fresh plaintext is minted
// (dbMcpTokenProviderFor's mint branch, Regenerate) — never on a load-only path, where this file
// already holds the correct value from the last mint and needs no update.
func SaveHelperToken(path, plain string) error {
	return writeFile(path, []byte(plain), 0o600)
}

// LoadHelperToken reads path's plaintext back: the embedded server's own "can I show
// Command/Install" gate needs to confirm the on-disk helper mirror still verifies against the
// currently-held Record, not just that this process minted a token at some point this run. A
// missing file returns ok=false with no error, same posture as Load.
func LoadHelperToken(path string) (plain string, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("mcpauth: read %s: %w", path, err)
	}
	return string(data), true, nil
}

// writeFile writes atomically (temp file plus rename, via renameio) after ensuring the parent
// directory exists at mode 0700.
func writeFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mcpauth: mkdir %s: %w", dir, err)
	}
	// WriteFile keeps an existing target's mode; static permissions re-tighten a file left looser.
	f, err := renameio.NewPendingFile(path, renameio.WithStaticPermissions(perm))
	if err != nil {
		return fmt.Errorf("mcpauth: write %s: %w", path, err)
	}
	defer func() { _ = f.Cleanup() }()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("mcpauth: write %s: %w", path, err)
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		return fmt.Errorf("mcpauth: write %s: %w", path, err)
	}
	return nil
}

// LoadOrMintTTL loads path's existing Record, or mints and persists a fresh one with the given ttl
// if none exists yet or the stored one has lapsed — the headless instance's own startup path and
// the embedded instance's app-boot-with-the-leaf-already-true path: neither is an explicit "toggle
// turned on" event, so neither forces a fresh token over one that might already be authenticating
// a previously-registered command, except when that token can no longer work at all (remint
// only at a moment a human can read the new plaintext — server start is one of the two). minted
// reports whether a fresh mint happened (plain is "" when it did not — a hash cannot be reversed).
//
// A loaded record whose ExpiresAt is zero (a file minted before rotation existed) is
// stamped instead of reminted: ExpiresAt is set to now+ttl and re-saved with the same Hash/Salt,
// reported as a load (minted=false, plain=""), so every existing registration gets exactly one
// ordinary TTL window before rotation starts applying to it. Stamping happens once — the
// next load sees a non-zero ExpiresAt and leaves it alone, so the clock is not reset by restarts.
func LoadOrMintTTL(path string, ttl time.Duration) (plain string, rec Record, minted bool, err error) {
	rec, ok, err := Load(path)
	if err != nil {
		return "", Record{}, false, err
	}
	if ok && rec.ExpiresAt.IsZero() {
		rec.ExpiresAt = time.Now().Add(ttl)
		if err := Save(path, rec); err != nil {
			return "", Record{}, false, err
		}
		return "", rec, false, nil
	}
	if ok && !Expired(rec, time.Now()) {
		return "", rec, false, nil
	}
	plain, rec, err = MintTTL(ttl)
	if err != nil {
		return "", Record{}, false, err
	}
	if err := Save(path, rec); err != nil {
		return "", Record{}, false, err
	}
	return plain, rec, true, nil
}
