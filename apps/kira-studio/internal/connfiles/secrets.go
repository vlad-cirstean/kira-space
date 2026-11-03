package connfiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// SecretOwner owns every connection secret: one module-wide namespace of names a file's `${secret:name}`
// refers to, per kind.
const SecretOwner = "connections"

// Secret kinds. The field a reference sits in decides the kind, and the kind decides the sealing scope,
// so a value sealed before the files existed keeps its authenticated scope.
const (
	KindPassword = "password"
	KindURI      = "uri"
)

// Kinds lists the secret kinds.
var Kinds = []string{KindPassword, KindURI}

// ErrSecretUnreadable marks a saved value that cannot be decrypted.
var ErrSecretUnreadable = errors.New("connfiles: the saved secret cannot be read")

func scopeOf(kind string) (secrets.Scope, error) {
	switch kind {
	case KindPassword:
		return secrets.ScopeConnection, nil
	case KindURI:
		return secrets.ScopeConnectionURI, nil
	}
	return "", fmt.Errorf("connfiles: unknown secret kind %q", kind)
}

// Secrets is the module's sealed values: secret_values rows of scope connection and connection-uri,
// owner connections.
type Secrets struct {
	Store *secrets.Store
	DB    *sql.DB
}

// Status reports whether values can be sealed here.
func (s *Secrets) Status() secrets.Status { return s.Store.Status() }

// Put seals value as name under kind.
func (s *Secrets) Put(kind, name, value string) error {
	scope, err := scopeOf(kind)
	if err != nil {
		return err
	}
	return s.Store.Put(s.DB, scope, SecretOwner, name, value)
}

// Delete removes name; an absent name is not an error.
func (s *Secrets) Delete(kind, name string) error {
	scope, err := scopeOf(kind)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`DELETE FROM secret_values WHERE scope = ? AND owner = ? AND name = ?`, string(scope), SecretOwner, name)
	return err
}

// Names lists the saved names of kind, never the values.
func (s *Secrets) Names(kind string) ([]string, error) {
	scope, err := scopeOf(kind)
	if err != nil {
		return nil, err
	}
	m, err := s.Store.Names(scope)
	if err != nil {
		return nil, err
	}
	return m[SecretOwner], nil
}

// Reveal opens one value. A name that is not set wraps configfs.ErrSecretMissing.
func (s *Secrets) Reveal(kind, name string) (string, error) {
	return s.Snapshot().Secret(context.Background(), kind, name)
}

// Snapshot resolves ${secret:name} for one connect: the first reference of a kind decrypts every
// secret of that kind once and later references read that copy. The namespace is the kind.
func (s *Secrets) Snapshot() configfs.SecretSource {
	type opened struct {
		vals       map[string]string
		unreadable map[string]error
		err        error
	}
	var (
		mu    sync.Mutex
		cache = map[string]*opened{}
	)
	return configfs.SecretSourceFunc(func(_ context.Context, kind, name string) (string, error) {
		scope, err := scopeOf(kind)
		if err != nil {
			return "", err
		}
		mu.Lock()
		o, ok := cache[kind]
		if !ok {
			o = &opened{}
			o.vals, o.unreadable, o.err = s.Store.Open(scope, SecretOwner)
			cache[kind] = o
		}
		mu.Unlock()
		if o.err != nil {
			return "", o.err
		}
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
