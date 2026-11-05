package apifiles

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// ErrInvalidOrder marks a reorder with no ids, an unknown id or a repeated id.
var ErrInvalidOrder = errors.New("apifiles: the order is not valid")

// ReorderCollections puts the listed collections first, in that order; ids left out keep their relative
// order after them. Order is app state in sort_order, not in the files.
func (s *Store) ReorderCollections(ids []string) error {
	return s.reorderTable("api_collections", ids, func(ch *Changes) { ch.Tree = true })
}

// ReorderEnvironments is ReorderCollections for environments.
func (s *Store) ReorderEnvironments(ids []string) error {
	return s.reorderTable("api_environments", ids, func(ch *Changes) { ch.Environments = true })
}

func (s *Store) reorderTable(table string, ids []string, mark func(*Changes)) error {
	if len(ids) == 0 {
		return fmt.Errorf("%w: no ids", ErrInvalidOrder)
	}
	s.mu.Lock()
	s.pend = &Changes{}
	err := s.tx(func(tx *sql.Tx, ch *Changes) error {
		if err := sqlitex.ReorderSortOrder(tx, `SELECT id FROM `+table+` ORDER BY sort_order, name, stem`,
			`UPDATE `+table+` SET sort_order = ? WHERE id = ?`, ids); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidOrder, err)
		}
		mark(ch)
		return nil
	})
	ch := *s.pend
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.emit(ch)
	return nil
}
