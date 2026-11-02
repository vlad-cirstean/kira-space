package repos

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// maskKeyBytes is the correlation key's own size (plan §2.5: "32 random bytes, per connection").
const maskKeyBytes = 32

// MaskKeysRepo is the only file in this tree that reads or writes
// connections.mask_correlation_key — SecretsRepo's own "only file that touches this column"
// discipline (secrets.go:21-23), applied to the correlation key. It is deliberately not part of
// ConnectionFields (model/connection.go), so a key can never reach the renderer, the
// connection URI, or the Copy URI menu item.
type MaskKeysRepo struct {
	db     *sql.DB
	cipher Cipher
}

// NewMaskKeys is separate from repos.New, the same reason NewSecrets is (D5): it needs a Cipher
// that does not exist inside that constructor's aggregate.
func NewMaskKeys(db *sql.DB, cipher Cipher) *MaskKeysRepo {
	return &MaskKeysRepo{db: db, cipher: cipher}
}

// Get returns the connection's own key, or nil when none has been minted yet (empty string stored). Never
// mints — EnsureKey is the only writer.
func (r *MaskKeysRepo) Get(connectionID string) ([]byte, error) {
	stored, err := sqlitex.QueryOne(r.db, func(row *sql.Row) (*string, error) {
		var s string
		if err := row.Scan(&s); err != nil {
			return nil, err
		}
		return &s, nil
	}, `SELECT mask_correlation_key FROM connections WHERE id = ?`, connectionID)
	if err != nil {
		return nil, fmt.Errorf("repos/maskkeys: get %s: %w", connectionID, err)
	}
	if stored == nil || *stored == "" {
		return nil, nil
	}
	plain, err := r.cipher.Decrypt(secrets.ScopeMaskKey, *stored)
	if err != nil {
		return nil, fmt.Errorf("repos/maskkeys: decrypt %s: %w", connectionID, err)
	}
	return []byte(plain), nil
}

// EnsureKey returns the connection's own key, minting and persisting a fresh 32-byte one on first
// call ("lazy creation... the first time a connection renders a mask whose rule has
// correlate: true"). A connection with no correlating rule never calls this and so never gets one.
//
// Two callers can race here on first use (the MCP render path's own MaskSetFor and the
// grid preview's own CorrelationKeyHex, both resolving the same connection's key at once). The
// write below is guarded by
//
//	WHERE mask_correlation_key = ''
//
// rather than an unconditional UPDATE, so only the first writer's key is ever persisted; a losing
// caller's RowsAffected is 0 and it re-reads the winner's key instead of returning the one it
// generated but never stored —
// both callers then build their own mask.Set from the same bytes, with nothing left to invalidate.
func (r *MaskKeysRepo) EnsureKey(connectionID string) ([]byte, error) {
	existing, err := r.Get(connectionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	key := make([]byte, maskKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("repos/maskkeys: generate key: %w", err)
	}
	encrypted, err := r.cipher.Encrypt(secrets.ScopeMaskKey, string(key))
	if err != nil {
		return nil, fmt.Errorf("repos/maskkeys: encrypt %s: %w", connectionID, err)
	}
	res, err := r.db.Exec(
		`UPDATE connections SET mask_correlation_key = ? WHERE id = ? AND mask_correlation_key = ''`,
		encrypted, connectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("repos/maskkeys: set %s: %w", connectionID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repos/maskkeys: set %s: %w", connectionID, err)
	}
	if n == 0 {
		// Lost the race (another EnsureKey minted first), or connectionID has no row — either way
		// the key this call generated was never persisted, so it must never be handed out as if it
		// were. Re-read rather than trust it: nil, nil for a genuinely missing connection, the
		// winner's key otherwise.
		return r.Get(connectionID)
	}
	return key, nil
}

// Regenerate mints and persists a brand-new key unconditionally, discarding whatever key (if any)
// existed before — the Privacy tab's own explicit "Regenerate correlation key" action (§7.4),
// never automatic. Every masked correlation tag for this connection changes as a result.
func (r *MaskKeysRepo) Regenerate(connectionID string) error {
	key := make([]byte, maskKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("repos/maskkeys: generate key: %w", err)
	}
	return r.setKey(connectionID, key)
}

func (r *MaskKeysRepo) setKey(connectionID string, key []byte) error {
	encrypted, err := r.cipher.Encrypt(secrets.ScopeMaskKey, string(key))
	if err != nil {
		return fmt.Errorf("repos/maskkeys: encrypt %s: %w", connectionID, err)
	}
	res, err := r.db.Exec(`UPDATE connections SET mask_correlation_key = ? WHERE id = ?`, encrypted, connectionID)
	if err != nil {
		return fmt.Errorf("repos/maskkeys: set %s: %w", connectionID, err)
	}
	// Regenerate is the only caller — without this check, regenerating against an id with no
	// matching row (e.g. a connection deleted the moment before) reported success while writing
	// nothing, silently leaving the request looking honored (M7 finding).
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("repos/maskkeys: set %s: %w", connectionID, err)
	}
	if n == 0 {
		return fmt.Errorf("repos/maskkeys: set %s: no such connection", connectionID)
	}
	return nil
}
