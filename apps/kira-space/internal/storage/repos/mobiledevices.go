package repos

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
)

// MobileDeviceRow is the full row, including the salted hash. Used by internal/mobileweb's token
// verification; model.MobileDevice is the projection that crosses to the renderer.
type MobileDeviceRow struct {
	ID         string
	Label      string
	UserAgent  string
	TokenHash  []byte
	TokenSalt  []byte
	CreatedAt  int64
	LastSeenAt int64
	LastIP     string
	RevokedAt  *int64
	// ExpiresAt is epoch ms; a device past it must pair again.
	ExpiresAt int64
	// CanWrite lets the phone add and shelve tasks and change steps; CanAgentInput lets it reply to agents and
	// control their terminals. Both are set from the desktop pane only.
	CanWrite      bool
	CanAgentInput bool
}

// MobileDevicesRepo is the trust store for phones paired with the mobile agents web.
type MobileDevicesRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

var mobileDeviceColumns = []string{
	kiraschema.MobileDevices.ID, kiraschema.MobileDevices.Label, kiraschema.MobileDevices.UserAgent,
	kiraschema.MobileDevices.TokenHash, kiraschema.MobileDevices.TokenSalt, kiraschema.MobileDevices.CreatedAt,
	kiraschema.MobileDevices.LastSeenAt, kiraschema.MobileDevices.LastIP, kiraschema.MobileDevices.RevokedAt,
	kiraschema.MobileDevices.CanWrite, kiraschema.MobileDevices.CanAgentInput, kiraschema.MobileDevices.ExpiresAt,
}

// ByID reads one row. found is false (with a nil error) when no such row exists; the timing
// discipline around a miss is the caller's job.
func (r *MobileDevicesRepo) ByID(id string) (MobileDeviceRow, bool, error) {
	var rec MobileDeviceRow
	c := kiraschema.MobileDevices
	sb := r.Dialect.Select(mobileDeviceColumns...).From(c.Table)
	sb.Where(sb.Equal(c.ID, id))
	err := sqlb.QueryRow(r.DB, sb).Scan(
		&rec.ID, &rec.Label, &rec.UserAgent, &rec.TokenHash, &rec.TokenSalt,
		&rec.CreatedAt, &rec.LastSeenAt, &rec.LastIP, &rec.RevokedAt, &rec.CanWrite, &rec.CanAgentInput, &rec.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MobileDeviceRow{}, false, nil
	}
	if err != nil {
		return MobileDeviceRow{}, false, fmt.Errorf("repos: mobile device by id %s: %w", id, err)
	}
	return rec, true, nil
}

// Insert records a newly approved device. Device ids are minted server-side per approval, so a
// conflict is a bug, never a re-pair.
func (r *MobileDevicesRepo) Insert(row MobileDeviceRow) error {
	ib := r.Dialect.InsertInto(kiraschema.MobileDevices.Table).Cols(mobileDeviceColumns...).Values(
		row.ID, row.Label, row.UserAgent, row.TokenHash, row.TokenSalt,
		row.CreatedAt, row.LastSeenAt, row.LastIP, row.RevokedAt, row.CanWrite, row.CanAgentInput, row.ExpiresAt,
	)
	_, err := sqlb.Exec(r.DB, ib)
	if err != nil {
		return fmt.Errorf("repos: insert mobile device %s: %w", row.ID, err)
	}
	return nil
}

// TouchLastSeen records an authenticated request.
func (r *MobileDevicesRepo) TouchLastSeen(id string, now int64, ip string) error {
	c := kiraschema.MobileDevices
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.LastSeenAt, now), ub.Assign(c.LastIP, ip)).Where(ub.Equal(c.ID, id))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: touch mobile device %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "mobile device "+id)
}

// Revoke sets revoked_at; the row stays so the pane keeps history.
func (r *MobileDevicesRepo) Revoke(id string, now int64) error {
	c := kiraschema.MobileDevices
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.RevokedAt, now)).Where(ub.Equal(c.ID, id), ub.IsNull(c.RevokedAt))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: revoke mobile device %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "mobile device "+id)
}

// SetPermissions replaces both flags on an unrevoked device.
func (r *MobileDevicesRepo) SetPermissions(id string, canWrite, canAgentInput bool) error {
	c := kiraschema.MobileDevices
	ub := r.Dialect.Update(c.Table)
	ub.Set(ub.Assign(c.CanWrite, canWrite), ub.Assign(c.CanAgentInput, canAgentInput)).Where(ub.Equal(c.ID, id), ub.IsNull(c.RevokedAt))
	res, err := sqlb.Exec(r.DB, ub)
	if err != nil {
		return fmt.Errorf("repos: set mobile device permissions %s: %w", id, err)
	}
	return sqlitex.RequireOneRow(res, "mobile device "+id)
}

// List returns every device, revoked included, most recently seen first, projected to
// model.MobileDevice (never the hash or the salt).
func (r *MobileDevicesRepo) List() ([]model.MobileDevice, error) {
	c := kiraschema.MobileDevices
	sb := r.Dialect.Select(c.ID, c.Label, c.UserAgent, c.CreatedAt, c.LastSeenAt, c.LastIP, c.RevokedAt, c.CanWrite, c.CanAgentInput, c.ExpiresAt).
		From(c.Table).OrderBy(c.LastSeenAt + " DESC")
	rows, err := sqlb.Query(r.DB, sb)
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.MobileDevice, bool, error) {
		var d model.MobileDevice
		err := rows.Scan(&d.ID, &d.Label, &d.UserAgent, &d.CreatedAt, &d.LastSeenAt, &d.LastIP, &d.RevokedAt, &d.CanWrite, &d.CanAgentInput, &d.ExpiresAt)
		return d, true, err
	})
}
