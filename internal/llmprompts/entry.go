// Package llmprompts is the one place every LLM prompt and LLM-facing instruction both apps send
// is declared (registry.go). Callers read the effective text through a typed handle. Not to be
// confused with internal/prompts, the popup router.
package llmprompts

import (
	"fmt"
	"reflect"
)

// ID names one registry entry.
type ID string

// App is one of the two apps; an entry lists the apps whose Settings show it.
type App string

// The apps.
const (
	Studio App = "studio"
	Space  App = "space"
)

// Kind says what an entry is.
type Kind string

// The entry kinds.
const (
	KindSystem       Kind = "system"
	KindMessage      Kind = "message"
	KindSuffix       Kind = "suffix"
	KindInstructions Kind = "instructions"
	KindTool         Kind = "tool"
	KindArgument     Kind = "argument"
	KindToolResult   Kind = "toolResult"
	KindFragment     Kind = "fragment"
)

// Applies says when an edit reaches a consumer.
type Applies string

// The freshness classes.
const (
	// NextCall: the consumer resolves the text on every call.
	NextCall Applies = "nextCall"
	// NewSession: the consumer builds the text once per Claude session.
	NewSession Applies = "newSession"
)

// NoVars is the vars type of an entry without variables.
type NoVars struct{}

// Entry is the declaration of one prompt. Vars type and example come from define.
type Entry struct {
	ID   ID
	Kind Kind
	Apps []App
	// Parent and Field tie an argument entry to its tool and JSON property.
	Parent ID
	Field  string
	// Name is the tool or MCP prompt name shown for tool entries.
	Name string
	// Catalog keys of the Settings copy; empty for tool and argument entries.
	TitleKey, PurposeKey, UsedByKey string
	Applies                         Applies
	Default                         string
	// RequiredFragments are entries the text must include with {{template}}.
	RequiredFragments []ID
	// Schema is the fixed JSON output schema of a schema-bound prompt, shown read-only.
	Schema string
}

// Prompt is the typed handle of one entry; V is its variables type.
type Prompt[V any] struct{ id ID }

// ID is the entry id.
func (p Prompt[V]) ID() ID { return p.id }

// Schema is the fixed JSON output schema of a schema-bound prompt.
func (p Prompt[V]) Schema() []byte { return []byte(mustEntry(p.id).Entry.Schema) }

// Text is the effective text for v: the valid override rendered, else the default.
func (p Prompt[V]) Text(src Source, v V) string { return Render(src, p.id, v) }

type entry struct {
	Entry
	varsType reflect.Type
	example  any
	literal  bool
	required []string
	fields   []field
}

type field struct {
	name     string
	required bool
}

var (
	registry []*entry
	byID     = map[ID]*entry{}
)

func define[V any](e Entry, example V) Prompt[V] {
	t := reflect.TypeFor[V]()
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("llmprompts: %s vars must be a struct", e.ID))
	}
	en := &entry{Entry: e, varsType: t, example: example}
	en.literal = t == reflect.TypeFor[NoVars]() && len(e.RequiredFragments) == 0
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		req := f.Tag.Get("prompt") == "required"
		en.fields = append(en.fields, field{name: f.Name, required: req})
		if req {
			en.required = append(en.required, f.Name)
		}
	}
	registry = append(registry, en)
	byID[e.ID] = en
	return Prompt[V]{id: e.ID}
}

func mustEntry(id ID) *entry {
	e, ok := byID[id]
	if !ok {
		panic("llmprompts: unknown prompt " + string(id))
	}
	return e
}

// hasApp reports whether the entry shows for app a; the empty app is every entry.
func (e *entry) hasApp(a App) bool {
	if a == "" {
		return true
	}
	for _, x := range e.Apps {
		if x == a {
			return true
		}
	}
	return false
}

// Info is the registry data of one entry that other packages read.
type Info struct {
	ID      ID
	Kind    Kind
	Apps    []App
	Parent  ID
	Field   string
	Default string
}

// All lists every entry in registry order.
func All() []Info {
	out := make([]Info, len(registry))
	for i, e := range registry {
		out[i] = Info{ID: e.ID, Kind: e.Kind, Apps: e.Apps, Parent: e.Parent, Field: e.Field, Default: e.Default}
	}
	return out
}

// Resolved is what a call sends for one entry.
type Resolved struct {
	Text       string
	Overridden bool
}

// Source resolves effective prompt text. Revision changes whenever any override changes.
type Source interface {
	Resolve(id ID) Resolved
	Revision() uint64
}

type defaultSource struct{}

func (defaultSource) Resolve(id ID) Resolved { return Resolved{Text: mustEntry(id).Default} }
func (defaultSource) Revision() uint64       { return 0 }

// Defaults is the registry-only Source: no store, no overrides.
func Defaults() Source { return defaultSource{} }
