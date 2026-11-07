package secrets

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// Execer is *sql.DB or *sql.Tx, so a caller seals values inside its own tx.
type Execer = sqlb.Querier

// Store is the only reader and writer of secret_values: sealed values keyed by scope, owner and
// name. A scope's owner ids are the caller's own (scripts.id for ScopeScriptParam).
type Store struct {
	db *sql.DB
	c  *Cipher
	// Dialect builds the SQL; the zero value is SQLite.
	Dialect sqlb.Dialect
}

func NewStore(db *sql.DB, c *Cipher) *Store { return &Store{db: db, c: c} }

func (s *Store) Status() Status { return s.c.Status() }

// Put seals plain under scope and upserts it through q.
func (s *Store) Put(q Execer, scope Scope, owner, name, plain string) error {
	sealed, err := s.c.Encrypt(scope, plain)
	if err != nil {
		return err
	}
	c := kiraschema.SecretValues
	ib := s.Dialect.InsertInto(c.Table).Cols(c.Scope, c.Owner, c.Name, c.Value, c.UpdatedMs).
		Values(string(scope), owner, name, sealed, time.Now().UnixMilli())
	sqlb.Upsert(ib, []string{c.Scope, c.Owner, c.Name}, []string{c.Value, c.UpdatedMs}, "")
	_, err = sqlb.Exec(q, ib)
	return err
}

// PutNew seals plain as name only when no such row exists, and reports whether it inserted.
func (s *Store) PutNew(q Execer, scope Scope, owner, name, plain string) (bool, error) {
	sealed, err := s.c.Encrypt(scope, plain)
	if err != nil {
		return false, err
	}
	c := kiraschema.SecretValues
	ib := s.Dialect.InsertInto(c.Table).Cols(c.Scope, c.Owner, c.Name, c.Value, c.UpdatedMs).
		Values(string(scope), owner, name, sealed, time.Now().UnixMilli())
	sqlb.UpsertNothing(ib, []string{c.Scope, c.Owner, c.Name})
	res, err := sqlb.Exec(q, ib)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Prune deletes the owner's names not in keep; keep nil deletes them all.
func (s *Store) Prune(q Execer, scope Scope, owner string, keep []string) error {
	c := kiraschema.SecretValues
	db := s.Dialect.DeleteFrom(c.Table)
	conds := []string{db.Equal(c.Scope, string(scope)), db.Equal(c.Owner, owner)}
	if len(keep) > 0 {
		conds = append(conds, db.NotIn(c.Name, sqlb.Args(keep)...))
	}
	db.Where(conds...)
	_, err := sqlb.Exec(q, db)
	return err
}

// Names lists saved names per owner, for snapshots. No decrypt.
func (s *Store) Names(scope Scope) (map[string][]string, error) {
	c := kiraschema.SecretValues
	sb := s.Dialect.Select(c.Owner, c.Name).From(c.Table)
	sb.Where(sb.Equal(c.Scope, string(scope))).OrderBy(c.Owner, c.Name)
	rows, err := sqlb.Query(s.db, sb)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var owner, name string
		if err := rows.Scan(&owner, &name); err != nil {
			return nil, err
		}
		out[owner] = append(out[owner], name)
	}
	return out, rows.Err()
}

// Open decrypts one owner's values; unreadable holds names whose value failed to decrypt.
func (s *Store) Open(scope Scope, owner string) (vals map[string]string, unreadable map[string]error, err error) {
	c := kiraschema.SecretValues
	sb := s.Dialect.Select(c.Name, c.Value).From(c.Table)
	sb.Where(sb.Equal(c.Scope, string(scope)), sb.Equal(c.Owner, owner))
	rows, err := sqlb.Query(s.db, sb)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	vals, unreadable = map[string]string{}, map[string]error{}
	for rows.Next() {
		var name, sealed string
		if err := rows.Scan(&name, &sealed); err != nil {
			return nil, nil, err
		}
		plain, derr := s.c.Decrypt(scope, sealed)
		if derr != nil {
			unreadable[name] = derr
			continue
		}
		vals[name] = plain
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("secrets: open %s/%s: %w", scope, owner, err)
	}
	return vals, unreadable, nil
}
