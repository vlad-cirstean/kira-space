package model

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kirathecat/kira-studio/internal/jsonx"
)

// RowValues is domain/mutations.ts's rowValuesSchema (Record<string, string | null>), kept as an
// order-preserving slice rather than a Go map: sql-mutate.ts's renderRowOp emits columns
// in the wire's own key order, and a preview/mutate statement's column order is asserted verbatim
// by packages/db-fixtures/postgres.spec.ts test 21. A map would randomise that order on every run.
type RowValues []RowValue

// RowValue is one entry of a RowValues, preserving its position in the original JSON object.
type RowValue struct {
	Name  string
	Value *string
}

// Get returns the value for name and whether it was present, mirroring a map lookup for callers
// that only need to read one column rather than iterate in order.
func (r RowValues) Get(name string) (*string, bool) {
	for _, v := range r {
		if v.Name == name {
			return v.Value, true
		}
	}
	return nil, false
}

// Names returns the column names in their original wire order.
func (r RowValues) Names() []string {
	names := make([]string, len(r))
	for i, v := range r {
		names[i] = v.Name
	}
	return names
}

func (r RowValues) MarshalJSON() ([]byte, error) {
	pairs := make([]jsonx.Pair, len(r))
	for i, v := range r {
		pairs[i] = jsonx.Pair{Name: v.Name, Value: v.Value}
	}
	return jsonx.MarshalOrderedObject(pairs)
}

// UnmarshalJSON reads the object through json.Decoder's own token stream to preserve the wire's
// key order — the whole reason this type exists rather than a map (A4).
func (r *RowValues) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("model: RowValues: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("model: RowValues: expected a JSON object")
	}
	var out RowValues
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("model: RowValues: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("model: RowValues: expected a string key")
		}
		var value *string
		if err := dec.Decode(&value); err != nil {
			return fmt.Errorf("model: RowValues: key %q: %w", key, err)
		}
		out = append(out, RowValue{Name: key, Value: value})
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("model: RowValues: %w", err)
	}
	*r = out
	return nil
}

// MutationRowOp is domain/mutations.ts's mutationRowOpSchema discriminated union. Exactly one of
// Key/Changes (update), Key (delete), or Values (insert) is populated per Kind.
type MutationRowOp struct {
	Kind    string // "update" | "insert" | "delete"
	Key     RowValues
	Changes RowValues
	Values  RowValues
}

func (op MutationRowOp) MarshalJSON() ([]byte, error) {
	switch op.Kind {
	case "update":
		return json.Marshal(struct {
			Kind    string    `json:"kind"`
			Key     RowValues `json:"key"`
			Changes RowValues `json:"changes"`
		}{Kind: "update", Key: op.Key, Changes: op.Changes})
	case "insert":
		return json.Marshal(struct {
			Kind   string    `json:"kind"`
			Values RowValues `json:"values"`
		}{Kind: "insert", Values: op.Values})
	case "delete":
		return json.Marshal(struct {
			Kind string    `json:"kind"`
			Key  RowValues `json:"key"`
		}{Kind: "delete", Key: op.Key})
	default:
		return nil, fmt.Errorf("model: MutationRowOp: invalid kind %q", op.Kind)
	}
}

func (op *MutationRowOp) UnmarshalJSON(data []byte) error {
	var probe struct {
		Kind    string          `json:"kind"`
		Key     json.RawMessage `json:"key"`
		Changes json.RawMessage `json:"changes"`
		Values  json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Kind {
	case "update":
		var key, changes RowValues
		if err := json.Unmarshal(probe.Key, &key); err != nil {
			return fmt.Errorf("model: MutationRowOp: update: key: %w", err)
		}
		if err := json.Unmarshal(probe.Changes, &changes); err != nil {
			return fmt.Errorf("model: MutationRowOp: update: changes: %w", err)
		}
		*op = MutationRowOp{Kind: "update", Key: key, Changes: changes}
	case "insert":
		var values RowValues
		if err := json.Unmarshal(probe.Values, &values); err != nil {
			return fmt.Errorf("model: MutationRowOp: insert: values: %w", err)
		}
		*op = MutationRowOp{Kind: "insert", Values: values}
	case "delete":
		var key RowValues
		if err := json.Unmarshal(probe.Key, &key); err != nil {
			return fmt.Errorf("model: MutationRowOp: delete: key: %w", err)
		}
		*op = MutationRowOp{Kind: "delete", Key: key}
	default:
		return fmt.Errorf("model: MutationRowOp: unknown kind %q", probe.Kind)
	}
	return nil
}

// MutationPlan is domain/mutations.ts's MutationPlan — adapter-side only, always one table.
type MutationPlan struct {
	Path NodePath
	Ops  []MutationRowOp
}

// MutationResult is domain/mutations.ts's MutationResult.
type MutationResult struct {
	AffectedRows int `json:"affectedRows"`
}
