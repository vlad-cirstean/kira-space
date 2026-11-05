package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// SortTerm is one entry of a structured SortSpec's terms.
type SortTerm struct {
	Column    string `json:"column"`
	Direction string `json:"direction"` // "asc" | "desc"
}

// SortSpec is packages/shared/domain/queries.ts's discriminated union (sortSpecSchema). The custom
// codec is what keeps the two arms honest: marshalling never emits the other arm's key, and
// unmarshalling rejects a value that carries neither (or a malformed one).
type SortSpec struct {
	Kind  string     // "structured" | "text"
	Terms []SortTerm // structured only
	Text  string     // text only, <= 4096 UTF-16 units (queries.ts's own cap)
}

const maxSortSpecText = 4096

func (s SortSpec) MarshalJSON() ([]byte, error) {
	switch s.Kind {
	case "structured":
		return json.Marshal(struct {
			Kind  string     `json:"kind"`
			Terms []SortTerm `json:"terms"`
		}{Kind: "structured", Terms: s.Terms})
	case "text":
		return json.Marshal(struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		}{Kind: "text", Text: s.Text})
	default:
		return nil, fmt.Errorf("model: SortSpec: invalid kind %q", s.Kind)
	}
}

func (s *SortSpec) UnmarshalJSON(data []byte) error {
	var probe struct {
		Kind  string          `json:"kind"`
		Terms json.RawMessage `json:"terms"`
		Text  json.RawMessage `json:"text"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Kind {
	case "structured":
		if probe.Terms == nil {
			return fmt.Errorf("model: SortSpec: structured kind missing terms")
		}
		var terms []SortTerm
		if err := json.Unmarshal(probe.Terms, &terms); err != nil {
			return fmt.Errorf("model: SortSpec: invalid terms: %w", err)
		}
		s.Kind, s.Terms, s.Text = "structured", terms, ""
		return nil
	case "text":
		if probe.Text == nil {
			return fmt.Errorf("model: SortSpec: text kind missing text")
		}
		var text string
		if err := json.Unmarshal(probe.Text, &text); err != nil {
			return fmt.Errorf("model: SortSpec: invalid text: %w", err)
		}
		if UTF16Len(text) > maxSortSpecText {
			return fmt.Errorf("model: SortSpec: text exceeds %d characters", maxSortSpecText)
		}
		s.Kind, s.Terms, s.Text = "text", nil, text
		return nil
	default:
		return fmt.Errorf("model: SortSpec: unknown kind %q", probe.Kind)
	}
}

// FilterBody is queries.ts's filterBodySchema — a saved_queries row of kind 'filter'.
type FilterBody struct {
	Where   *string   `json:"where"`
	OrderBy *SortSpec `json:"orderBy"`
}

// ConsoleBody is queries.ts's consoleBodySchema — a saved_queries row of kind 'console'.
type ConsoleBody struct {
	Text string `json:"text"`
}

// SavedQuery mirrors queries.ts's savedQuerySchema. Body stays json.RawMessage rather than a Go
// sum type: the bytes are the wire value, and the repo validates them against Kind by
// unmarshalling into FilterBody/ConsoleBody and discarding the result — one struct on the wire,
// no Go-side union the renderer would have to be taught about.
type SavedQuery struct {
	ID           string          `json:"id"`
	ConnectionID string          `json:"connectionId"`
	Path         string          `json:"path"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Body         json.RawMessage `json:"body"`
	Pinned       bool            `json:"pinned"`
	CreatedAt    string          `json:"createdAt"`
	UsedAt       *string         `json:"usedAt"`
}

// SavedQueryPatch is queries.ts's updateSavedQuery patch shape.
type SavedQueryPatch struct {
	Name   *string
	Pinned *bool
}

// FilterHistoryEntry mirrors queries.ts's filterHistoryEntrySchema.
type FilterHistoryEntry struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connectionId"`
	Path         string    `json:"path"`
	Where        *string   `json:"where"`
	OrderBy      *SortSpec `json:"orderBy"`
	UsedAt       string    `json:"usedAt"`
}

// ValidSavedQueryName mirrors queries.ts's savedQueryBase name field: trimmed, 1..120 chars.
func ValidSavedQueryName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("model: saved query name must not be empty")
	}
	if UTF16Len(trimmed) > 120 {
		return fmt.Errorf("model: saved query name exceeds 120 characters")
	}
	return nil
}

// ErrInvalidDiscoverBody is a DiscoverBody that fails Validate.
var ErrInvalidDiscoverBody = errors.New("model: invalid discover body")

// maxDiscoverBodyBytes caps one encoded body.
const maxDiscoverBodyBytes = 256 << 10

// Value sets mirror kibana.ts: kibanaTimePresetSchema, KIBANA_FILTER_OPERATORS,
// kibanaLanguageSchema, kibanaSampleSizeSchema, kibanaIntervalSchema.
var (
	discoverTimePresets = []string{
		"last15m", "last30m", "last1h", "last4h", "last12h", "last24h", "last7d", "last30d",
		"last90d", "last1y", "today", "thisWeek", "thisMonth", "thisYear",
	}
	discoverFilterOperators = []string{"is", "is_not", "is_one_of", "is_not_one_of", "exists", "does_not_exist", "between"}
	discoverLanguages       = []string{"kuery", "lucene"}
	discoverSampleSizes     = []int{100, 500, 1000, 5000, 10000}
	discoverIntervals       = []string{"auto", "m", "h", "d", "w", "M", "y"}
)

// KibanaTimeSelection mirrors kibana.ts's kibanaTimeSelectionSchema.
type KibanaTimeSelection struct {
	Mode   string `json:"mode"`             // preset | absolute
	Preset string `json:"preset,omitempty"` // mode preset
	From   string `json:"from,omitempty"`   // mode absolute, ISO
	To     string `json:"to,omitempty"`
}

// DiscoverBody is queries.ts's discoverBodySchema: a saved_queries row of kind 'discover'.
type DiscoverBody struct {
	Query        string              `json:"query"`
	DataViewID   string              `json:"dataViewId"`
	DataViewName *string             `json:"dataViewName"`
	Language     string              `json:"language"`
	Time         KibanaTimeSelection `json:"time"`
	Filters      []KibanaFilter      `json:"filters"`
	Columns      []string            `json:"columns"`
	Sort         []KibanaSort        `json:"sort"`
	SampleSize   int                 `json:"sampleSize"`
	Interval     string              `json:"interval"`
}

// Normalized turns nil slices into empty ones, so the encoded body never holds null.
func (b DiscoverBody) Normalized() DiscoverBody {
	if b.Filters == nil {
		b.Filters = []KibanaFilter{}
	}
	if b.Columns == nil {
		b.Columns = []string{}
	}
	if b.Sort == nil {
		b.Sort = []KibanaSort{}
	}
	b.Filters = slices.Clone(b.Filters)
	for i, f := range b.Filters {
		if f.Values == nil {
			b.Filters[i].Values = []string{}
		}
	}
	return b
}

// Validate mirrors the zod schema, so a body Go accepts always parses in TypeScript.
func (b DiscoverBody) Validate() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidDiscoverBody, fmt.Sprintf(format, args...))
	}
	if b.DataViewID == "" {
		return bad("dataViewId is empty")
	}
	if !slices.Contains(discoverLanguages, b.Language) {
		return bad("language %q", b.Language)
	}
	switch b.Time.Mode {
	case "preset":
		if !slices.Contains(discoverTimePresets, b.Time.Preset) || b.Time.From != "" || b.Time.To != "" {
			return bad("time preset %q", b.Time.Preset)
		}
	case "absolute":
		if b.Time.From == "" || b.Time.To == "" || b.Time.Preset != "" {
			return bad("absolute time needs from and to")
		}
	default:
		return bad("time mode %q", b.Time.Mode)
	}
	for _, f := range b.Filters {
		if f.Field == "" || !slices.Contains(discoverFilterOperators, f.Operator) {
			return bad("filter %q %q", f.Field, f.Operator)
		}
	}
	seen := map[string]bool{}
	for _, c := range b.Columns {
		if c == "" || seen[c] {
			return bad("column %q", c)
		}
		seen[c] = true
	}
	for _, s := range b.Sort {
		if s.Field == "" || (s.Order != "asc" && s.Order != "desc") {
			return bad("sort %q %q", s.Field, s.Order)
		}
	}
	if !slices.Contains(discoverSampleSizes, b.SampleSize) {
		return bad("sampleSize %d", b.SampleSize)
	}
	if !slices.Contains(discoverIntervals, b.Interval) {
		return bad("interval %q", b.Interval)
	}
	if enc, err := json.Marshal(b); err != nil || len(enc) > maxDiscoverBodyBytes {
		return bad("body exceeds %d bytes", maxDiscoverBodyBytes)
	}
	return nil
}

// Condition ops (P535); the structured filter of an MCP browse. Mirrors packages/shared/domain/dbmcp.ts.
const (
	CondEq      = "eq"
	CondNe      = "ne"
	CondLt      = "lt"
	CondLte     = "lte"
	CondGt      = "gt"
	CondGte     = "gte"
	CondIn      = "in"
	CondLike    = "like"
	CondIsNull  = "isNull"
	CondNotNull = "notNull"
)

// Condition is one structured comparison on a column. Values hold string, bool or json.Number entries:
// none for isNull and notNull, several for in, one otherwise. Adapters bind them as parameters.
type Condition struct {
	Column string
	Op     string
	Values []any
}
