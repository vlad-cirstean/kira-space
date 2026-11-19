// Package keptvars holds values Kira keeps in memory for later requests: a script keeps a result
// under a name, a later request refers to it by name, the value never returns to the agent. The store
// is MCP-agnostic (entries carry their source MCP and request) and imports stdlib only.
package keptvars

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"
)

// SourceKind says who produced an entry.
type SourceKind string

const (
	// SourceScript is a value a script kept.
	SourceScript SourceKind = "script"
	// SourceUser is a value the user supplied.
	SourceUser SourceKind = "user"
)

// Limits. An entry is refused at a cap, never evicted: dropping a value the user saw and approved would
// break a chain silently.
const (
	MaxValueBytes   = 1 << 20
	MaxScopeEntries = 64
	MaxStoreEntries = 512
	MaxStoreBytes   = 32 << 20
	MaxTTL          = 240 * time.Minute
	DefaultTTL      = time.Hour
	maxDepth        = 64
	// PreviewBytes caps the preview that travels in list and approval payloads.
	PreviewBytes = 2 << 10
)

// Source says where an entry came from. Request is display text only: secrets stay {{name}}.
type Source struct {
	Kind      SourceKind `json:"kind"`
	MCP       string     `json:"mcp"`
	Tool      string     `json:"tool"`
	Request   string     `json:"request"`
	Client    string     `json:"client"`
	TaskID    string     `json:"taskId,omitempty"`
	SessionID string     `json:"sessionId,omitempty"`
}

// Meta is an entry without its value.
type Meta struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Bytes     int       `json:"bytes"`
	Items     int       `json:"items"`
	Secret    bool      `json:"secret"`
	Source    Source    `json:"source"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Entry is a Meta with its compact JSON value. Store reads return a copy.
type Entry struct {
	Meta
	Value json.RawMessage
}

// Errors carry fixed text and never a value.
var (
	ErrName      = errors.New("a kept variable name starts with a letter and has at most 40 letters, digits or underscores")
	ErrTooLarge  = errors.New("a kept value is larger than 1 MiB or nested deeper than 64 levels")
	ErrScopeFull = errors.New("too many kept variables (64 per session); ask the user to clear some in the Kept variables view")
	ErrStoreFull = errors.New("Kira holds too many kept variables; ask the user to clear some in the Kept variables view")
	ErrNameTaken = errors.New("a kept variable of that name was supplied by the user and cannot be replaced")
	ErrClosed    = errors.New("the kept variable store is closed")
	errBadValue  = errors.New("a kept value must be valid JSON")
	errBadScope  = errors.New("a kept variable needs a scope")
	nameRE       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,39}$`)
)

// ValidName checks a variable name: usable as a JS identifier and inside a {{kept.<name>}} span.
func ValidName(name string) error {
	if !nameRE.MatchString(name) {
		return ErrName
	}
	return nil
}

// analyze compacts raw and reports its JSON type and item count. It refuses invalid JSON, a value
// over MaxValueBytes after compaction and one nested deeper than 64 levels.
func analyze(raw json.RawMessage) (compact []byte, typ string, items int, err error) {
	if !json.Valid(raw) {
		return nil, "", 0, errBadValue
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, "", 0, errBadValue
	}
	if buf.Len() > MaxValueBytes || depthOf(buf.Bytes()) > maxDepth {
		return nil, "", 0, ErrTooLarge
	}
	compact = buf.Bytes()
	switch compact[0] {
	case '{':
		var m map[string]json.RawMessage
		_ = json.Unmarshal(compact, &m)
		return compact, "object", len(m), nil
	case '[':
		var a []json.RawMessage
		_ = json.Unmarshal(compact, &a)
		return compact, "array", len(a), nil
	case '"':
		return compact, "string", 0, nil
	case 't', 'f':
		return compact, "boolean", 0, nil
	case 'n':
		return compact, "null", 0, nil
	}
	return compact, "number", 0, nil
}

// depthOf is the nesting depth of valid compact JSON.
func depthOf(b []byte) int {
	depth, deepest := 0, 0
	inString := false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inString:
			switch c {
			case '\\':
				i++
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '[' || c == '{':
			depth++
			deepest = max(deepest, depth)
		case c == ']' || c == '}':
			depth--
		}
	}
	return deepest
}

// Preview cuts a compact value to at most n bytes on a rune boundary.
func Preview(value []byte, n int) (text string, truncated bool) {
	if len(value) <= n {
		return string(value), false
	}
	for n > 0 && !utf8.RuneStart(value[n]) {
		n--
	}
	return string(value[:n]), true
}
