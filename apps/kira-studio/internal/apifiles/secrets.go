package apifiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// SecretOwner owns every Api secret: one module-wide namespace of names a file's `${secret:name}` refers to.
const SecretOwner = "api"

// ErrSecretUnreadable marks a saved value that cannot be decrypted.
var ErrSecretUnreadable = errors.New("apifiles: the saved secret cannot be read")

// Secrets is the module's sealed values: secret_values rows of scope variable, owner api.
type Secrets struct {
	Store *secrets.Store
	DB    *sql.DB
}

// Status reports whether values can be sealed here.
func (s *Secrets) Status() secrets.Status { return s.Store.Status() }

// Put seals value as name.
func (s *Secrets) Put(name, value string) error {
	return s.Store.Put(s.DB, secrets.ScopeVariable, SecretOwner, name, value)
}

// Delete removes name; an absent name is not an error.
func (s *Secrets) Delete(name string) error {
	_, err := s.DB.Exec(`DELETE FROM secret_values WHERE scope = ? AND owner = ? AND name = ?`, string(secrets.ScopeVariable), SecretOwner, name)
	return err
}

// Names lists the saved names, never the values.
func (s *Secrets) Names() ([]string, error) {
	m, err := s.Store.Names(secrets.ScopeVariable)
	if err != nil {
		return nil, err
	}
	return m[SecretOwner], nil
}

// Reveal opens one value. A name that is not set wraps configfs.ErrSecretMissing.
func (s *Secrets) Reveal(name string) (string, error) {
	return s.Snapshot().Secret(context.Background(), "", name)
}

// Opened is every Api secret decrypted once: the values, and the names that cannot be read.
type Opened struct {
	vals       map[string]string
	unreadable map[string]error
}

// OpenAll decrypts every Api secret once.
func (s *Secrets) OpenAll() (*Opened, error) {
	vals, unreadable, err := s.Store.Open(secrets.ScopeVariable, SecretOwner)
	if err != nil {
		return nil, err
	}
	return &Opened{vals: vals, unreadable: unreadable}, nil
}

// Values lists every readable value, for masking.
func (o *Opened) Values() []string {
	out := make([]string, 0, len(o.vals))
	for _, v := range o.vals {
		out = append(out, v)
	}
	return out
}

// Source resolves ${secret:name} from the opened copy.
func (o *Opened) Source() configfs.SecretSource {
	return configfs.SecretSourceFunc(func(_ context.Context, _, name string) (string, error) {
		if bad, ok := o.unreadable[name]; ok {
			return "", fmt.Errorf("%w: %w", ErrSecretUnreadable, bad)
		}
		v, ok := o.vals[name]
		if !ok {
			return "", configfs.ErrSecretMissing
		}
		return v, nil
	})
}

// Snapshot resolves ${secret:name} for one send: the first reference decrypts every Api secret once and
// later references read that copy.
func (s *Secrets) Snapshot() configfs.SecretSource {
	var (
		once   sync.Once
		opened *Opened
		err    error
	)
	return configfs.SecretSourceFunc(func(ctx context.Context, ns, name string) (string, error) {
		once.Do(func() { opened, err = s.OpenAll() })
		if err != nil {
			return "", err
		}
		return opened.Source().Secret(ctx, ns, name)
	})
}
