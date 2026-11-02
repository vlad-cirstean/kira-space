// Package agentcfg is the app's one agent settings definition: a YAML file in the shared agent folder
// that every launched agent receives, rendered natively per provider. Kira Space hosts the folder and
// its editor; Kira Studio reads it for its own launches. Credentials the file references live sealed
// in one store both apps open.
package agentcfg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kirathecat/kira-studio/internal/agentcfg/migrations"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// SecretOwner is the owner of every agent credential: one shared definition.
const SecretOwner = "agent"

// SecretStore is the open shared credential store: sealed values in agent-secrets.db.
type SecretStore struct {
	Store *secrets.Store
	DB    *sql.DB
}

// OpenSecrets opens (creating it) the shared credential store at path. Both apps open the file, so
// it is written through immediate transactions. Close closes the database.
func OpenSecrets(path string, cipher *secrets.Cipher) (*SecretStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("agentcfg: create %s: %w", filepath.Dir(path), err)
	}
	db, err := sqlitex.OpenImmediate(path)
	if err != nil {
		return nil, err
	}
	steps, err := migrations.All()
	if err == nil {
		err = sqlitex.Migrate(db, steps)
	}
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SecretStore{Store: secrets.NewStore(db, cipher), DB: db}, nil
}

// Close closes the database.
func (s *SecretStore) Close() error { return s.DB.Close() }

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

// Source resolves ${secret:name} from the store. Reading is per call: a value set in the other app is
// seen at the next launch.
func (s *SecretStore) Source() configfs.SecretSource {
	return configfs.SecretSourceFunc(func(_ context.Context, _, name string) (string, error) {
		vals, unreadable, err := s.Store.Open(secrets.ScopeAgentCredential, SecretOwner)
		if err != nil {
			return "", err
		}
		if _, bad := unreadable[name]; bad {
			return "", errors.New("the saved value cannot be read; set it again")
		}
		v, ok := vals[name]
		if !ok {
			return "", configfs.ErrSecretMissing
		}
		return v, nil
	})
}
