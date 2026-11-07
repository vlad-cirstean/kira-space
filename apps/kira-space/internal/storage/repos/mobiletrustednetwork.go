package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

// MobileTrustedNetworkRepo holds the single trusted network (row id 1).
type MobileTrustedNetworkRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// Get reads the trusted network; found is false when none is set.
func (r *MobileTrustedNetworkRepo) Get() (model.TrustedNetwork, bool, error) {
	var n model.TrustedNetwork
	c := kiraschema.MobileTrustedNetwork
	sb := r.Dialect.Select(c.Subnet, c.RouterIP, c.RouterMac, c.Interface, c.TrustedAt).From(c.Table)
	sb.Where(sb.Equal(c.ID, 1))
	err := sqlb.QueryRow(r.DB, sb).
		Scan(&n.Subnet, &n.RouterIP, &n.RouterMAC, &n.Interface, &n.TrustedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return model.TrustedNetwork{}, false, nil
	}
	if err != nil {
		return model.TrustedNetwork{}, false, fmt.Errorf("repos: get mobile trusted network: %w", err)
	}
	return n, true, nil
}

// Set replaces the trusted network.
func (r *MobileTrustedNetworkRepo) Set(n model.TrustedNetwork) error {
	c := kiraschema.MobileTrustedNetwork
	ib := r.Dialect.InsertInto(c.Table).Cols(c.ID, c.Subnet, c.RouterIP, c.RouterMac, c.Interface, c.TrustedAt).
		Values(1, n.Subnet, n.RouterIP, n.RouterMAC, n.Interface, n.TrustedAt)
	sqlb.Upsert(ib, []string{c.ID}, []string{c.Subnet, c.RouterIP, c.RouterMac, c.Interface, c.TrustedAt}, "")
	_, err := sqlb.Exec(r.DB, ib)
	if err != nil {
		return fmt.Errorf("repos: set mobile trusted network: %w", err)
	}
	return nil
}

// Clear forgets the trusted network.
func (r *MobileTrustedNetworkRepo) Clear() error {
	c := kiraschema.MobileTrustedNetwork
	db := r.Dialect.DeleteFrom(c.Table)
	db.Where(db.Equal(c.ID, 1))
	if _, err := sqlb.Exec(r.DB, db); err != nil {
		return fmt.Errorf("repos: clear mobile trusted network: %w", err)
	}
	return nil
}
