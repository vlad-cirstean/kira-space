// Package proof is the validation-proof store's service (P420): an MCP agent pushes ordered steps
// of generic data into a session; the user reviews closed sessions in a window.
package proof

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Payload and field limits, each enforced before any write.
const (
	maxNameRunes     = 80
	maxDescRunes     = 200
	maxSummaryRunes  = 500
	maxSourceRunes   = 64
	maxQueryBytes    = 64 << 10
	maxColumns       = 500
	maxColumnName    = 256
	maxRows          = 10_000
	maxCellBytes     = 64 << 10
	maxDocuments     = 10_000
	maxDocumentBytes = 1 << 20
	maxDocIDRunes    = 256
	maxEntries       = 10_000
	maxFieldBytes    = 1 << 10
	maxValueBytes    = 64 << 10
	maxPayloadBytes  = 4 << 20
)

// Views a step can show.
const (
	ViewTable    = "table"
	ViewDocument = "document"
	ViewKeyValue = "keyvalue"
	ViewChart    = "chart"
)

// InvalidError is a caller-correctable validation failure; the message names the field and limit.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &InvalidError{Msg: fmt.Sprintf(format, args...)}
}

// NormalizeName trims name, checks it and returns the name with its uniqueness key.
func NormalizeName(name string) (clean, key string, err error) {
	clean = strings.TrimSpace(name)
	n := utf8.RuneCountInString(clean)
	if n < 1 || n > maxNameRunes {
		return "", "", invalid("name must be 1-%d characters", maxNameRunes)
	}
	if !utf8.ValidString(clean) || strings.IndexFunc(clean, unicode.IsControl) >= 0 {
		return "", "", invalid("name must not contain control characters")
	}
	return clean, strings.ToLower(clean), nil
}

// CheckText validates an optional free-text field: valid UTF-8 within max runes.
func CheckText(field, s string, maxRunes int) error {
	if !utf8.ValidString(s) {
		return invalid("%s must be valid UTF-8", field)
	}
	if utf8.RuneCountInString(s) > maxRunes {
		return invalid("%s must be at most %d characters", field, maxRunes)
	}
	return nil
}

// StepInput is one proof_add_step call. Data is the raw JSON of the data argument, taken from the
// wire so number precision and key order survive.
type StepInput struct {
	DraftID, Description, Query, Language, Source, View string
	Data                                                json.RawMessage
	KeepOrdinal, ReplacesOrdinal                        *int
}

var languages = map[string]bool{"sql": true, "json": true, "shell": true, "text": true}

type column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

var columnTypes = map[string]bool{"number": true, "text": true, "boolean": true, "temporal": true, "json": true, "other": true}

// checked is a validated step ready to store.
type checked struct {
	Description, Query, Language, Source, View string
	Payload                                    []byte
}

func (in StepInput) validate() (checked, bool, error) {
	if in.KeepOrdinal != nil && in.ReplacesOrdinal != nil {
		return checked{}, false, invalid("keepOrdinal and replacesOrdinal are mutually exclusive")
	}
	if in.KeepOrdinal != nil {
		if in.Description != "" || in.Query != "" || in.Language != "" || in.Source != "" || in.View != "" || len(in.Data) != 0 {
			return checked{}, false, invalid("keepOrdinal copies the old step as is; pass no description, query, language, source, view or data")
		}
		return checked{}, true, nil
	}
	desc := strings.TrimSpace(in.Description)
	if n := utf8.RuneCountInString(desc); n < 1 || n > maxDescRunes || !utf8.ValidString(desc) || strings.ContainsAny(desc, "\r\n") {
		return checked{}, false, invalid("description must be one line of 1-%d characters", maxDescRunes)
	}
	if err := CheckText("source", in.Source, maxSourceRunes); err != nil {
		return checked{}, false, err
	}
	if len(in.Query) > maxQueryBytes || !utf8.ValidString(in.Query) {
		return checked{}, false, invalid("query must be valid UTF-8 and at most %d KiB", maxQueryBytes>>10)
	}
	lang := in.Language
	if lang == "" {
		lang = "text"
	}
	if !languages[lang] {
		return checked{}, false, invalid("language must be one of sql, json, shell, text")
	}
	if len(in.Data) == 0 {
		return checked{}, false, invalid("data is required")
	}
	if !utf8.Valid(in.Data) {
		return checked{}, false, invalid("data must be valid UTF-8")
	}
	var payload any
	var err error
	switch in.View {
	case ViewTable:
		payload, err = checkTable(in.Data)
	case ViewDocument:
		payload, err = checkDocuments(in.Data)
	case ViewKeyValue:
		payload, err = checkEntries(in.Data)
	case ViewChart:
		payload, err = checkChart(in.Data)
	default:
		return checked{}, false, invalid("view must be one of table, document, keyvalue, chart")
	}
	if err != nil {
		return checked{}, false, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return checked{}, false, fmt.Errorf("proof: encode payload: %w", err)
	}
	if len(raw) > maxPayloadBytes {
		return checked{}, false, invalid("data is %d KiB serialized; the limit is %d MiB per step", len(raw)>>10, maxPayloadBytes>>20)
	}
	return checked{Description: desc, Query: in.Query, Language: lang, Source: in.Source, View: in.View, Payload: raw}, false, nil
}

func decodeStrict(data json.RawMessage, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return invalid("data is not valid for this view: %v", err)
	}
	if dec.More() {
		return invalid("data has trailing content")
	}
	return nil
}

type tablePayload struct {
	Columns []column            `json:"columns"`
	Rows    [][]json.RawMessage `json:"rows"`
}

func checkTable(data json.RawMessage) (tablePayload, error) {
	var t tablePayload
	if err := decodeStrict(data, &t); err != nil {
		return t, err
	}
	if len(t.Columns) < 1 || len(t.Columns) > maxColumns {
		return t, invalid("data.columns must hold 1-%d columns", maxColumns)
	}
	if len(t.Rows) > maxRows {
		return t, invalid("data.rows holds %d rows; the limit is %d", len(t.Rows), maxRows)
	}
	seen := map[string]bool{}
	for i := range t.Columns {
		c := &t.Columns[i]
		if c.Name == "" || len(c.Name) > maxColumnName {
			return t, invalid("data.columns[%d].name must be 1-%d bytes", i, maxColumnName)
		}
		if seen[c.Name] {
			return t, invalid("data.columns[%d].name %q is a duplicate", i, c.Name)
		}
		seen[c.Name] = true
		if c.Type != "" && !columnTypes[c.Type] {
			return t, invalid("data.columns[%d].type must be one of number, text, boolean, temporal, json, other", i)
		}
	}
	for r, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return t, invalid("data.rows[%d] has %d values; the table has %d columns", r, len(row), len(t.Columns))
		}
		for c, cell := range row {
			if len(cell) > maxCellBytes {
				return t, invalid("data.rows[%d][%d] is %d KiB serialized; the limit is %d KiB per cell", r, c, len(cell)>>10, maxCellBytes>>10)
			}
			if t.Columns[c].Type == "" {
				t.Columns[c].Type = inferType(cell)
			}
		}
	}
	for i := range t.Columns {
		if t.Columns[i].Type == "" {
			t.Columns[i].Type = "other"
		}
	}
	if t.Rows == nil {
		t.Rows = [][]json.RawMessage{}
	}
	return t, nil
}

// inferType classifies a cell; "" for null, so the first non-null cell decides.
func inferType(cell json.RawMessage) string {
	switch c := bytes.TrimSpace(cell); {
	case len(c) == 0 || string(c) == "null":
		return ""
	case c[0] == '"':
		return "text"
	case c[0] == '{' || c[0] == '[':
		return "json"
	case string(c) == "true" || string(c) == "false":
		return "boolean"
	default:
		return "number"
	}
}

type documentPayload struct {
	Documents []document `json:"documents"`
}

type document struct {
	ID   json.RawMessage `json:"id,omitempty"`
	Body json.RawMessage `json:"body"`
}

func checkDocuments(data json.RawMessage) (any, error) {
	var p documentPayload
	if err := decodeStrict(data, &p); err != nil {
		return nil, err
	}
	if len(p.Documents) > maxDocuments {
		return nil, invalid("data.documents holds %d documents; the limit is %d", len(p.Documents), maxDocuments)
	}
	out := struct {
		Documents []struct {
			ID   string          `json:"id"`
			Body json.RawMessage `json:"body"`
		} `json:"documents"`
	}{}
	seen := map[string]bool{}
	for i, d := range p.Documents {
		body := bytes.TrimSpace(d.Body)
		if len(body) == 0 || body[0] != '{' {
			return nil, invalid("data.documents[%d].body must be a JSON object", i)
		}
		if len(body) > maxDocumentBytes {
			return nil, invalid("data.documents[%d].body is %d KiB serialized; the limit is %d MiB", i, len(body)>>10, maxDocumentBytes>>20)
		}
		id := strconv.Itoa(i + 1)
		if raw := bytes.TrimSpace(d.ID); len(raw) > 0 && string(raw) != "null" {
			switch {
			case raw[0] == '"':
				if err := json.Unmarshal(raw, &id); err != nil {
					return nil, invalid("data.documents[%d].id is not a valid string", i)
				}
			case raw[0] >= '0' && raw[0] <= '9' || raw[0] == '-':
				id = string(raw)
			default:
				return nil, invalid("data.documents[%d].id must be a string or number", i)
			}
		}
		if id == "" || utf8.RuneCountInString(id) > maxDocIDRunes {
			return nil, invalid("data.documents[%d].id must be 1-%d characters", i, maxDocIDRunes)
		}
		if seen[id] {
			return nil, invalid("data.documents[%d].id %q is a duplicate", i, id)
		}
		seen[id] = true
		out.Documents = append(out.Documents, struct {
			ID   string          `json:"id"`
			Body json.RawMessage `json:"body"`
		}{ID: id, Body: body})
	}
	if out.Documents == nil {
		out.Documents = []struct {
			ID   string          `json:"id"`
			Body json.RawMessage `json:"body"`
		}{}
	}
	return out, nil
}

type entriesPayload struct {
	Entries []entry `json:"entries"`
}

type entry struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

func checkEntries(data json.RawMessage) (any, error) {
	var p entriesPayload
	if err := decodeStrict(data, &p); err != nil {
		return nil, err
	}
	if len(p.Entries) > maxEntries {
		return nil, invalid("data.entries holds %d entries; the limit is %d", len(p.Entries), maxEntries)
	}
	for i, e := range p.Entries {
		if e.Field == "" || len(e.Field) > maxFieldBytes {
			return nil, invalid("data.entries[%d].field must be 1-%d bytes", i, maxFieldBytes)
		}
		if len(e.Value) == 0 {
			return nil, invalid("data.entries[%d].value is required (use null for none)", i)
		}
		if len(e.Value) > maxValueBytes {
			return nil, invalid("data.entries[%d].value is %d KiB serialized; the limit is %d KiB", i, len(e.Value)>>10, maxValueBytes>>10)
		}
	}
	if p.Entries == nil {
		p.Entries = []entry{}
	}
	return p, nil
}
