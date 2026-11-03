package model

import "fmt"

// VariableScope discriminates which owner an api_variables row belongs to. It is computed on
// scan from which foreign key is non-null (P5 D4) — never a stored column, so there is no second
// source of the same fact to drift from the CHECK the migration already enforces.
type VariableScope string

const (
	VariableScopeCollection  VariableScope = "collection"
	VariableScopeEnvironment VariableScope = "environment"
)

// Environment is one api_environments row. IsActive is the app-global selection — the repo
// guarantees at most one row carries it. Description is app-local free text, no Postman round-trip
// question at all (unlike a variable's). Color is a user-assigned value from the same palette
// Studio's connections use (ValidPaletteColor), never derived; 'none' is the default.
type Environment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SortOrder   int    `json:"sortOrder"`
	IsActive    bool   `json:"isActive"`
	Description string `json:"description"`
	Color       string `json:"color"`
	// File is the environment file's name in the api folder, "" before the file is projected.
	File string `json:"file"`
}

// Variable is one api_variables row's **list projection**. Value is the empty string whenever
// IsSecret, and the row's own encrypted-secret column is not a field of this struct at all, so
// "the list never returns a secret's plaintext or ciphertext" is a property of this type, not of a
// per-row branch somewhere that could later be forgotten. Description is not a secret,
// so it does not weaken that property.
type Variable struct {
	ID          string        `json:"id"`
	Scope       VariableScope `json:"scope"`
	OwnerID     string        `json:"ownerId"`
	Name        string        `json:"name"`
	Value       string        `json:"value"`
	IsSecret    bool          `json:"isSecret"`
	SortOrder   int           `json:"sortOrder"`
	Description string        `json:"description"`
}

// VariableHistoryEntry is one api_variable_history row's list projection — the same "no secret
// column here" discipline as Variable above. apivars.Service.RevealHistory is the only path to a
// secret history entry's plaintext.
type VariableHistoryEntry struct {
	ID         string `json:"id"`
	VariableID string `json:"variableId"`
	Value      string `json:"value"`
	IsSecret   bool   `json:"isSecret"`
	RecordedAt string `json:"recordedAt"`
}

// Validate checks what SQL cannot: a non-empty name. D12: a duplicate name within one scope is
// allowed (resolved first-wins by sort_order in the renderer) — this is the only invariant the
// repo does not otherwise enforce through a constraint.
func (v Variable) Validate() error {
	if v.Name == "" {
		return fmt.Errorf("model: variable: name is required")
	}
	return nil
}

// VariableBulkEntry is one line of a parsed `.env` bulk edit (P17 D21/D22/D23) — the renderer's
// own dotenv.ts `EnvEntry`, carried across the bridge. HasValue is false for a bare `KEY=` line
// (D22 rule 3: a secret row whose line carries no value keeps its stored secret untouched).
type VariableBulkEntry struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	HasValue    bool   `json:"hasValue"`
	Description string `json:"description"`
}

// VariableBulkResult is ApplyBulk's own answer — the same counts the renderer's own
// dotenv.ts#reconcileEnv already computed locally for its live summary, so returning them here is
// also the guard that the two reconciles agree (§4 of the plan).
type VariableBulkResult struct {
	Added     int  `json:"added"`
	Updated   int  `json:"updated"`
	Removed   int  `json:"removed"`
	Reordered bool `json:"reordered"`
}
