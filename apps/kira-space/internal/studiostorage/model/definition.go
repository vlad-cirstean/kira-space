package model

// constraintTypes mirrors domain/definition.ts's constraintMetaSchema.type enum.
var constraintTypes = map[string]bool{
	"primaryKey": true, "unique": true, "foreignKey": true, "check": true, "exclusion": true,
}

// ValidConstraintType mirrors domain/definition.ts's constraintMetaSchema.type enum.
func ValidConstraintType(v string) bool { return constraintTypes[v] }

// ConstraintMeta mirrors domain/definition.ts's constraintMetaSchema.
type ConstraintMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Definition is the engine's own text (pg_get_constraintdef(), MariaDB's CHECK_CLAUSE / key
	// column list) — rendered verbatim, never re-composed here (P19 D11).
	Definition string `json:"definition"`
}

// DocumentSchemaMeta mirrors domain/definition.ts's documentSchemaMetaSchema. Null for every SQL
// engine.
type DocumentSchemaMeta struct {
	Validator        *string `json:"validator"`
	IsJSONSchema     bool    `json:"isJsonSchema"`
	ValidationLevel  *string `json:"validationLevel"`
	ValidationAction *string `json:"validationAction"`
}

// DefinitionSectionRow mirrors one entry of domain/definition.ts's definitionSectionSchema.rows.
type DefinitionSectionRow struct {
	Name   string  `json:"name"`
	Value  string  `json:"value"`
	Detail *string `json:"detail"`
}

// DefinitionSection mirrors domain/definition.ts's definitionSectionSchema.
type DefinitionSection struct {
	Title string                 `json:"title"`
	Rows  []DefinitionSectionRow `json:"rows"`
}

// ObjectDefinition mirrors domain/definition.ts's objectDefinitionSchema.
type ObjectDefinition struct {
	Path          string           `json:"path"` // encoded NodePath — the L1 cache key's second component
	Kind          string           `json:"kind"`
	QualifiedName string           `json:"qualifiedName"`
	Language      string           `json:"language"` // "sql" | "json"
	Statements    []string         `json:"statements"`
	Origin        string           `json:"origin"` // "server" | "composed"
	Notes         []string         `json:"notes"`
	Constraints   []ConstraintMeta `json:"constraints"`
	// DocumentSchema is nil for every SQL engine (P19 D12).
	DocumentSchema *DocumentSchemaMeta `json:"documentSchema"`
	// Sections is [] for postgres/mariadb/mongo (P23 D6).
	Sections    []DefinitionSection `json:"sections"`
	GeneratedAt string              `json:"generatedAt"` // ISO-8601, stamped by the adapter
}

// definitionLanguages/definitionOrigins mirror domain/definition.ts's enums.
var (
	definitionLanguages = map[string]bool{"sql": true, "json": true}
	definitionOrigins   = map[string]bool{"server": true, "composed": true}
)

// ValidateObjectDefinition is the explicit check that replaces zod's safeParse,
// including the .default([]) that keeps a cached definition (with no `sections` key at
// all) parsing: Sections/Notes/Constraints normalize from nil to [] first, exactly as
// ValidateObjectMeta does for its own list fields.
func ValidateObjectDefinition(def *ObjectDefinition) bool {
	if def.Notes == nil {
		def.Notes = []string{}
	}
	if def.Constraints == nil {
		def.Constraints = []ConstraintMeta{}
	}
	if def.Sections == nil {
		def.Sections = []DefinitionSection{}
	}

	if !ValidNodeKind(def.Kind) {
		return false
	}
	if !definitionLanguages[def.Language] {
		return false
	}
	if !definitionOrigins[def.Origin] {
		return false
	}
	if len(def.Statements) < 1 { // zod's .min(1) — statements is never empty
		return false
	}
	for _, c := range def.Constraints {
		if !ValidConstraintType(c.Type) {
			return false
		}
	}
	return true
}
