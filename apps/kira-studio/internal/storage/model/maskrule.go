package model

// MaskKind mirrors internal/mask's own Kind — kept as a separate string type here rather than
// imported directly, the same layering internal/mask's own header comment requires (a leaf
// package, no dependency on storage/model): this package converts between the two by string value
// at the one seam that needs both (internal/maskrules).
type MaskKind string

const (
	MaskKindName   MaskKind = "name"
	MaskKindEmail  MaskKind = "email"
	MaskKindText   MaskKind = "text"
	MaskKindNumber MaskKind = "number"
	MaskKindDate   MaskKind = "date"
	MaskKindRedact MaskKind = "redact"
)

// Six kinds, not seven: an `id` kind (identifier columns, tag-only) was in the original plan draft
// and dropped before implementation — internal ids are not PII, so no rule should apply to them at
// all; a column meant to stay untouched carries no rule, rather than one whose kind is a no-op.
var maskKinds = map[MaskKind]bool{
	MaskKindName: true, MaskKindEmail: true, MaskKindText: true, MaskKindNumber: true,
	MaskKindDate: true, MaskKindRedact: true,
}

// MaskKinds lists every mask kind, sorted.
func MaskKinds() []string { return sortedKeys(maskKinds) }

// ValidMaskKind mirrors packages/shared/domain/mask.ts's maskKindSchema, the same
// "ValidMcpPermissionMode mirrors mcpPermissionModeSchema" pattern connection.go:105 already uses.
func ValidMaskKind(v string) bool { return maskKinds[MaskKind(v)] }

// MaskRule mirrors packages/shared/domain/mask.ts's maskRuleSchema — one connection_mask_rules row
// (M5 §3.1/§3.2).
type MaskRule struct {
	ID           string   `json:"id"`
	ConnectionID string   `json:"connectionId"`
	TableName    string   `json:"tableName"`
	ColumnName   string   `json:"columnName"`
	Kind         MaskKind `json:"kind"`
	KeepHint     bool     `json:"keepHint"`
	Correlate    bool     `json:"correlate"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// MaskRuleFields is what MaskRulesRepo.Upsert accepts — mirrors maskRuleFieldsSchema, the same
// "Fields struct omits id/timestamps" shape ConnectionFields uses relative to ConnectionSummary.
type MaskRuleFields struct {
	TableName  string   `json:"tableName"`
	ColumnName string   `json:"columnName"`
	Kind       MaskKind `json:"kind"`
	KeepHint   bool     `json:"keepHint"`
	Correlate  bool     `json:"correlate"`
}
