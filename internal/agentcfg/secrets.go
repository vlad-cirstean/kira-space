// Package agentcfg is the app's one agent settings definition: a YAML file in the agent folder that
// every launched agent receives, rendered natively per provider. Credentials the file references live
// sealed in kira.db.
package agentcfg

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// secretNamespace is the resolver namespace of agent settings, fixed here and never written in a file.
const secretNamespace = "agent"

// SecretOwner is the owner of every agent credential: one shared definition.
const SecretOwner = "agent"

// SecretStore is the agent credential store: sealed values in kira.db's secret_values, scope
// agent-credential.
type SecretStore struct {
	Store *secrets.Store
	DB    *sql.DB
}

// NewSecrets wraps db, the shared kira.db pool, which the caller owns and closes.
func NewSecrets(db *sql.DB, cipher *secrets.Cipher) *SecretStore {
	return &SecretStore{Store: secrets.NewStore(db, cipher), DB: db}
}

// Put seals value as name.
func (s *SecretStore) Put(name, value string) error {
	return s.Store.Put(s.DB, secrets.ScopeAgentCredential, SecretOwner, name, value)
}

// Delete removes name; an absent name is not an error.
func (s *SecretStore) Delete(name string) error {
	_, err := s.DB.Exec(`DELETE FROM secret_values WHERE scope = ? AND owner = ? AND name = ?`,
		string(secrets.ScopeAgentCredential), SecretOwner, name)
	return err
}

// Names lists the saved secret names, never their values.
func (s *SecretStore) Names() ([]string, error) {
	m, err := s.Store.Names(secrets.ScopeAgentCredential)
	if err != nil {
		return nil, err
	}
	return m[SecretOwner], nil
}

// Snapshot resolves ${secret:name} from the store for one launch: the first reference opens and decrypts
// every agent secret once and later references read that copy, so n references cost one query, not n.
// A value set in the other app is seen by the next launch's Snapshot.
func (s *SecretStore) Snapshot() configfs.SecretSource {
	var (
		once       sync.Once
		vals       map[string]string
		unreadable map[string]error
		err        error
	)
	return configfs.SecretSourceFunc(func(_ context.Context, _, name string) (string, error) {
		once.Do(func() { vals, unreadable, err = s.Store.Open(secrets.ScopeAgentCredential, SecretOwner) })
		if err != nil {
			return "", err
		}
		if bad, ok := unreadable[name]; ok {
			return "", fmt.Errorf("the saved value cannot be read, set it again: %w", bad)
		}
		v, ok := vals[name]
		if !ok {
			return "", configfs.ErrSecretMissing
		}
		return v, nil
	})
}
