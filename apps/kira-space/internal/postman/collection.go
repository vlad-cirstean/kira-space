// Package postman reads and writes the Postman Collection Format v2.1. It is
// self-contained by design — no storage, no adapters, no Wails, one Parse and one Write, drivable
// from a plain `go test` with a testdata/ corpus — the shape internal/httpclient already has for a
// second self-contained concern. It imports internal/storage/model only for SavedRequest, which
// imports nothing but the standard library, so there is no cycle.
//
// The decision the whole package turns on: every object it reads is kept **verbatim** as a
// map[string]json.RawMessage minus only its recursive `item` array, so an export re-emits every
// member this app does not model — auth, event[] scripts, variable[], saved response[],
// protocolProfileBehavior, per-row descriptions, and every member a future revision adds —
// unchanged. Fidelity here is a retention problem, not a parsing problem: encoding/json drops
// every member a struct does not declare, so no typed model, library or hand-written, can do it.
package postman

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// SchemaURL is what export always writes for info.schema, regardless of what the imported file
// said — the file being written *is* v2.1 by construction, and this is the exact string Postman's
// own exporter writes and its importer checks (D10).
const SchemaURL = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"

// ItemKind discriminates the two row kinds. Postman carries no `type` field: a folder is
// "has an `item` member", a request is "has a `request` member" (F1).
type ItemKind string

const (
	KindFolder  ItemKind = "folder"
	KindRequest ItemKind = "request"
)

// RootParent is Item.Parent for a direct child of the collection root.
const RootParent = -1

// Item is one row of the flattened tree. The slice it lives in is depth-first in document order,
// so Parent is always an index strictly below this item's own (D3).
type Item struct {
	// Parent indexes Tree.Items, or is RootParent.
	Parent int
	Kind   ItemKind
	Name   string
	// Order is the dense index within this item's own parent's `item[]` array.
	Order int
	// Protocol is P11 D12/F22's own addition — 'http' or 'grpc', empty for anything Parse produces
	// (an import can never create a gRPC row: the Postman format has no representation for one).
	// Write skips every 'grpc' item before it ever reads Request, which is meaningless for one.
	Protocol string
	// Request is meaningful only for KindRequest, and only when Protocol != 'grpc'.
	Request model.SavedRequest
	// Origin is the original Postman item object verbatim, minus its own `item` array (D5).
	Origin map[string]json.RawMessage
}

// Tree is one collection: its name, its own origin object, the flat ordered item list, and its
// own collection-level variables. Both directions use it — Parse produces one, Write consumes
// one, and repos/collections.go maps it onto rows in about twenty lines.
type Tree struct {
	Name string
	// Origin is the whole original collection object minus its `item` array, minus `info.name`
	// (both of which are columns) and minus `variable` (D15 — promoted into Variables below, and
	// re-emitted from there, never from here). '{}' for a collection created in this app.
	Origin map[string]json.RawMessage
	Items  []Item
	// Variables is the collection's own top-level `variable[]` (D15) — only this level is
	// promoted; a folder's or an item's own `variable` member stays inert in its own Origin.
	Variables []Variable
	// Report is what Parse observed; Write leaves it zero.
	Report Report
}

// Variable is one collection-level `variable[]` entry (D15).
type Variable struct {
	Name  string
	Value string
	// Secret is Type == "secret" (F2/OQ-1: unverified against a real Postman export from this
	// sandbox, so any other Type value is treated as non-secret rather than refused).
	Secret bool
	// Type preserves the original `type` string verbatim, so an unrecognised one survives export
	// unchanged; "" for a variable created in this app that was never secret.
	Type string
	// Description is P17 D14/F10 — Postman carries one on a variable[] entry, decoded leniently
	// (a plain string, or an object's own "content" member — decodeDescription) and re-emitted as
	// a plain string on export, "" when absent.
	Description string
}

// The eight warning kinds (D12), each corresponding to a decision the importer makes on the
// user's behalf. Counted rather than listed per-item.
const (
	WarnScriptsInert      = "scripts_inert"
	WarnAuthInert         = "auth_inert"
	WarnVariablesInert    = "variables_inert"
	WarnGraphQLBody       = "graphql_body"
	WarnUnsupportedMethod = "unsupported_method"
	WarnUnresolvedFile    = "unresolved_file"
	WarnInlineFileContent = "inline_file_content"
	WarnMalformedItem     = "malformed_item" // item entry is not a JSON object
	// WarnVariablesImported is D15's promoted-count sibling to WarnVariablesInert: a collection-
	// level variable[] is no longer inert, so it gets its own message rather than being folded
	// into the inert count it no longer belongs to.
	WarnVariablesImported = "variables_imported"
	// WarnDisabledBody covers D7's last row: Postman keeps a disabled body and does not send it;
	// this app has no equivalent and will send it.
	WarnDisabledBody = "disabled_body"
	// WarnDisabledVariables counts environment and collection variables Postman marks disabled: the
	// app has no per-variable enabled flag, so they are skipped rather than applied.
	WarnDisabledVariables = "disabled_variables"
	// WarnEmptyValues counts imported variables with an empty value: Postman exports only shared
	// values, so unshared ones arrive as keys alone; they import with "".
	WarnEmptyValues = "empty_values"
	// WarnUnnamedVariables counts variables with no usable key, imported as unnamed_<n>.
	WarnUnnamedVariables = "unnamed_variables"
	// WarnMissingRequest counts items with neither `item` nor `request`, imported as empty GET requests.
	WarnMissingRequest = "missing_request"
)

// Report is what Parse counted. The per-kind messages are UI text and live at the bridge, not
// here — this package only knows what it did.
type Report struct {
	Folders  int
	Requests int
	Warnings map[string]int
}

func (r *Report) warn(kind string) { r.warnN(kind, 1) }

func (r *Report) warnN(kind string, n int) {
	if r.Warnings == nil {
		r.Warnings = map[string]int{}
	}
	r.Warnings[kind] += n
}

// ---- the small JSON helpers every decoder in this package shares ----

// mustRaw marshals a value this package constructed itself, with HTML escaping off — Go's default
// escapes '<', '>' and '&', and a nested json.RawMessage is copied through verbatim by the outer
// encoder, so escaping here would survive all the way into the written file (D10).
func mustRaw(v any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return json.RawMessage(`null`)
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n"))
}

// decodeString reports whether raw is a JSON string, and its value. It never errors — every
// caller wants "is it a string, or is it something else I should handle" rather than a failure.
func decodeString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// decodeScalarString accepts a JSON string *or* a JSON number/boolean, rendered as its literal
// text — F2's last row: `variable.value` and `url.port` are untyped in the schema, and a numeric
// port or query value is a real shape in real exports.
func decodeScalarString(raw json.RawMessage) (string, bool) {
	if s, ok := decodeString(raw); ok {
		return s, true
	}
	if len(raw) == 0 {
		return "", false
	}
	switch raw[0] {
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return "", false
		}
		return strconv.FormatBool(b), true
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", false
		}
		return n.String(), true
	}
	return "", false
}

// decodeDescription is the leniency rule: Postman writes a `description` member
// either as a plain string or as an object carrying the text under its own "content" member — this
// reads either shape and returns "" for anything else, rather than refusing to decode a variable
// whose description arrived in the richer object form.
func decodeDescription(raw json.RawMessage) string {
	if s, ok := decodeString(raw); ok {
		return s
	}
	if obj := decodeObject(raw); obj != nil {
		if s, ok := decodeString(obj["content"]); ok {
			return s
		}
	}
	return ""
}

func decodeBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false, false
	}
	return b, true
}

// decodeObject returns raw as a member map, or nil when it is anything else.
func decodeObject(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 || raw[0] != '{' {
		return nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	return obj
}

// decodeArray returns raw as an element slice, or nil when it is anything else.
func decodeArray(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	return arr
}

// cloneOrigin copies a member map so a caller's shedding never mutates a shared map.
func cloneOrigin(src map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
