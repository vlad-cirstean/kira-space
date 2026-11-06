package connfiles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/sqlb"
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

// ErrSecretExists marks a new-name write that found the name in use; the existing value is untouched.
var ErrSecretExists = errors.New("connfiles: the secret name is in use")

// Secrets is the module's sealed values: secret_values rows of scope connection and connection-uri,
// owner connections.
type Secrets struct {
	Store   *secrets.Store
	DB      *sql.DB
	Dialect sqlb.Dialect
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

// PutNew seals value as name only when the name is free, else ErrSecretExists. It never overwrites, so a
// caller's failure path may delete the name: only this call's insert can be there.
func (s *Secrets) PutNew(kind, name, value string) error {
	scope, err := scopeOf(kind)
	if err != nil {
		return err
	}
	ok, err := s.Store.PutNew(s.DB, scope, SecretOwner, name, value)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrSecretExists, name)
	}
	return nil
}

// Taken reports whether either kind holds a secret named name.
func (s *Secrets) Taken(name string) bool {
	var one int
	c := kiraschema.SecretValues
	sb := s.Dialect.Select("1").From(c.Table)
	sb.Where(sb.Equal(c.Owner, SecretOwner), sb.Equal(c.Name, name),
		sb.In(c.Scope, string(secrets.ScopeConnection), string(secrets.ScopeConnectionURI)))
	return sqlb.QueryRow(s.DB, sb).Scan(&one) == nil
}

// Delete removes name; an absent name is not an error.
func (s *Secrets) Delete(kind, name string) error {
	scope, err := scopeOf(kind)
	if err != nil {
		return err
	}
	c := kiraschema.SecretValues
	del := s.Dialect.DeleteFrom(c.Table)
	del.Where(del.Equal(c.Scope, string(scope)), del.Equal(c.Owner, SecretOwner), del.Equal(c.Name, name))
	_, err = sqlb.Exec(s.DB, del)
	return err
}

// Copy duplicates the sealed value of from as to, verbatim: the scope is the authenticated data, so no
// decrypt is needed. An absent from copies nothing. A taken to is never overwritten: ErrSecretExists.
// It reports whether it inserted a row.
func (s *Secrets) Copy(kind, from, to string) (bool, error) {
	scope, err := scopeOf(kind)
	if err != nil {
		return false, err
	}
	c := kiraschema.SecretValues
	ib := s.Dialect.InsertInto(c.Table).Cols(c.Scope, c.Owner, c.Name, c.Value, c.UpdatedMs)
	ib.SQL("SELECT " + c.Scope + ", " + c.Owner + ", " + ib.Var(to) + ", " + c.Value + ", " + ib.Var(time.Now().UnixMilli()) +
		" FROM " + c.Table + " WHERE " + c.Scope + " = " + ib.Var(string(scope)) + " AND " + c.Owner + " = " + ib.Var(SecretOwner) +
		" AND " + c.Name + " = " + ib.Var(from))
	sqlb.UpsertNothing(ib, []string{c.Scope, c.Owner, c.Name})
	res, err := sqlb.Exec(s.DB, ib)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return true, nil
	}
	var one int
	chk := s.Dialect.Select("1").From(c.Table)
	chk.Where(chk.Equal(c.Scope, string(scope)), chk.Equal(c.Owner, SecretOwner), chk.Equal(c.Name, to))
	if sqlb.QueryRow(s.DB, chk).Scan(&one) == nil {
		return false, fmt.Errorf("%w: %s", ErrSecretExists, to)
	}
	return false, nil
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
