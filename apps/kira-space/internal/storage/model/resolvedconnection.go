package model

// ResolvedConnectionConfig is engine-ops.ts's ResolvedConnectionConfig.
// internal/connections' own resolve() still builds it (and still holds the secret-injection
// logic); this is the shape.
//
// This is the one shape that carries a secret (Password) — it must never cross the control-plane
// wire to the renderer. Only an Adapter's Connect and, historically, the engine child ever saw it.
type ResolvedConnectionConfig struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Color     string         `json:"color"`
	Mode      string         `json:"mode"`
	ReadOnly  bool           `json:"readOnly"`
	Host      *string        `json:"host"`
	Port      *int           `json:"port"`
	Database  *string        `json:"database"`
	Username  *string        `json:"username"`
	URI       *string        `json:"uri"`
	Options   map[string]any `json:"options"`
	SortOrder int            `json:"sortOrder"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
	Password  *string        `json:"password"`
}
