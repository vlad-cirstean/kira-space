package secrets

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Execer is the write half of *sql.DB and *sql.Tx, so a caller seals values inside its own tx.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Store is the only reader and writer of secret_values: sealed values keyed by scope, owner and
// name. A scope's owner ids are the caller's own (custom_scripts.id for ScopeScriptParam).
type Store struct {
	db *sql.DB
	c  *Cipher
}

func NewStore(db *sql.DB, c *Cipher) *Store { return &Store{db: db, c: c} }

func (s *Store) Status() Status { return s.c.Status() }

// Put seals plain under scope and upserts it through q.
func (s *Store) Put(q Execer, scope Scope, owner, name, plain string) error {
	sealed, err := s.c.Encrypt(scope, plain)
	if err != nil {
		return err
	}
	_, err = q.Exec(`INSERT INTO secret_values (scope, owner, name, value, updated_ms) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (scope, owner, name) DO UPDATE SET value = excluded.value, updated_ms = excluded.updated_ms`,
		string(scope), owner, name, sealed, time.Now().UnixMilli())
	return err
}

// Prune deletes the owner's names not in keep; keep nil deletes them all.
func (s *Store) Prune(q Execer, scope Scope, owner string, keep []string) error {
	query := `DELETE FROM secret_values WHERE scope = ? AND owner = ?`
	args := []any{string(scope), owner}
	if len(keep) > 0 {
		query += ` AND name NOT IN (?` + strings.Repeat(`, ?`, len(keep)-1) + `)`
		for _, n := range keep {
			args = append(args, n)
		}
	}
	_, err := q.Exec(query, args...)
	return err
}

// Names lists saved names per owner, for snapshots. No decrypt.
func (s *Store) Names(scope Scope) (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT owner, name FROM secret_values WHERE scope = ? ORDER BY owner, name`, string(scope))
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
	rows, err := s.db.Query(`SELECT name, value FROM secret_values WHERE scope = ? AND owner = ?`, string(scope), owner)
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
