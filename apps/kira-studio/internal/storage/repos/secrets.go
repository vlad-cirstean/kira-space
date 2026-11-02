package repos

import (
	"database/sql"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/secrets"
)

// Cipher is implemented by internal/secrets — the kira:v3: AES-256-GCM envelope. Note
// what is absent from this interface versus the TS SecretCipher: no isEnveloped (its only caller,
// upgradeLegacySecrets, is deleted, not ported), and no status (the repo never reads it;
// the cipher enforces availability inside Encrypt/Decrypt and returns E_SECRET_STORE itself). The
// scope argument is the kind of column the ciphertext lives in — every reader and writer of
// one column uses one scope.
type Cipher interface {
	Encrypt(scope secrets.Scope, plain string) (string, error)
	Decrypt(scope secrets.Scope, stored string) (string, error)
}

// SecretsRepo is the only file in this tree that reads connections.password and connections.uri
// (P1 D8, P181) — the two ciphertext columns, one scope each. ConnectionsRepo never selects uri. It never inspects the stored value's envelope
// itself — that is entirely the Cipher's business.
type SecretsRepo struct {
	db     *sql.DB
	cipher Cipher
}

// NewSecrets is separate from repos.New (D5): SecretsRepo needs a Cipher that does not exist
// until P55, so it is not part of the aggregate every other repo is constructed through.
func NewSecrets(db *sql.DB, cipher Cipher) *SecretsRepo {
	return &SecretsRepo{db: db, cipher: cipher}
}

func (r *SecretsRepo) Get(connectionID string) (*string, error) {
	var stored sql.NullString
	if err := r.db.QueryRow(`SELECT password FROM connections WHERE id = ?`, connectionID).Scan(&stored); err != nil {
		return nil, fmt.Errorf("repos/secrets: get %s: %w", connectionID, err)
	}
	if !stored.Valid {
		return nil, nil
	}
	plain, err := r.cipher.Decrypt(secrets.ScopeConnection, stored.String)
	if err != nil {
		return nil, fmt.Errorf("repos/secrets: decrypt %s: %w", connectionID, err)
	}
	return &plain, nil
}

// GetURI decrypts the stored connection URI (P181). A pre-P181 plaintext value fails here with the
// cipher's envelope error, wrapped with the id.
func (r *SecretsRepo) GetURI(connectionID string) (*string, error) {
	var stored sql.NullString
	if err := r.db.QueryRow(`SELECT uri FROM connections WHERE id = ?`, connectionID).Scan(&stored); err != nil {
		return nil, fmt.Errorf("repos/secrets: get uri %s: %w", connectionID, err)
	}
	if !stored.Valid {
		return nil, nil
	}
	plain, err := r.cipher.Decrypt(secrets.ScopeConnectionURI, stored.String)
	if err != nil {
		return nil, fmt.Errorf("repos/secrets: decrypt uri %s: %w", connectionID, err)
	}
	return &plain, nil
}

func (r *SecretsRepo) Set(connectionID string, secret *string) error {
	var stored *string
	if secret != nil {
		encrypted, err := r.cipher.Encrypt(secrets.ScopeConnection, *secret)
		if err != nil {
			return fmt.Errorf("repos/secrets: encrypt %s: %w", connectionID, err)
		}
		stored = &encrypted
	}
	if _, err := r.db.Exec(`UPDATE connections SET password = ? WHERE id = ?`, stored, connectionID); err != nil {
		return fmt.Errorf("repos/secrets: set %s: %w", connectionID, err)
	}
	return nil
}

func (r *SecretsRepo) Delete(connectionID string) error {
	if _, err := r.db.Exec(`UPDATE connections SET password = NULL WHERE id = ?`, connectionID); err != nil {
		return fmt.Errorf("repos/secrets: delete %s: %w", connectionID, err)
	}
	return nil
}
