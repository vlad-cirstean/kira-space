package apifiles

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// ErrInvalidOrder marks a reorder with no ids, an unknown id or a repeated id.
var ErrInvalidOrder = errors.New("apifiles: the order is not valid")

// ReorderCollections puts the listed collections first, in that order; ids left out keep their relative
// order after them. Order is app state in sort_order, not in the files.
func (s *Store) ReorderCollections(ids []string) error {
	c := kiraschema.APICollections
	return s.reorderTable(sqlb.SortSet{Table: c.Table, Key: c.ID, Order: c.SortOrder}, []string{c.SortOrder, c.Name, c.Stem}, ids, func(ch *Changes) { ch.Tree = true })
}

// ReorderEnvironments is ReorderCollections for environments.
func (s *Store) ReorderEnvironments(ids []string) error {
	c := kiraschema.APIEnvironments
	return s.reorderTable(sqlb.SortSet{Table: c.Table, Key: c.ID, Order: c.SortOrder}, []string{c.SortOrder, c.Name, c.Stem}, ids, func(ch *Changes) { ch.Environments = true })
}

func (s *Store) reorderTable(set sqlb.SortSet, orderBy []string, ids []string, mark func(*Changes)) error {
	if len(ids) == 0 {
		return fmt.Errorf("%w: no ids", ErrInvalidOrder)
	}
	s.mu.Lock()
	s.pend = &Changes{}
	err := s.tx(func(tx *sql.Tx, ch *Changes) error {
		set.Select = s.o.Dialect.Select(set.Key).From(set.Table).OrderBy(orderBy...)
		if err := sqlb.ReorderSortOrder(tx, s.o.Dialect, set, ids); err != nil {
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
