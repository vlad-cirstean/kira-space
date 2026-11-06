package apifiles

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// EnvironmentRef is an environment as its file last read: the last good value, so an edit that
// is mid-way invalid never widens or drops what agents may use.
type EnvironmentRef struct {
	ID          string
	Name        string
	Description string
	Color       string
	File        EnvironmentFile
}

// EnvironmentList reads every environment file that has a valid value, in file order.
func (s *Store) EnvironmentList() ([]EnvironmentRef, error) {
	s.mu.Lock()
	s.pend = &Changes{}
	entries := s.environments.List()
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)

	out := make([]EnvironmentRef, 0, len(entries))
	for _, e := range entries {
		if e.Value == nil {
			continue
		}
		stem, ok := StemOf(e.File, EnvironmentExt)
		if !ok {
			continue
		}
		var id string
		c := kiraschema.APIEnvironments
		sb := s.o.Dialect.Select(c.ID).From(c.Table)
		sb.Where(sb.Equal(c.Stem, stem))
		err := sqlb.QueryRow(s.o.DB, sb).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("apifiles: read environment %s: %w", stem, err)
		}
		out = append(out, EnvironmentRef{ID: id, Name: e.Value.Name, Description: e.Value.Description, Color: orDefault(e.Value.Color, "none"), File: *e.Value})
	}
	return out, nil
}

// EnvironmentByID is one environment's last good file; an unknown id is a not-found error.
func (s *Store) EnvironmentByID(id string) (EnvironmentRef, error) {
	list, err := s.EnvironmentList()
	if err != nil {
		return EnvironmentRef{}, err
	}
	for _, e := range list {
		if e.ID == id {
			return e, nil
		}
	}
	return EnvironmentRef{}, ipcerr.NotFoundNamed("environment")
}
