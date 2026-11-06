// parse.go turns a Postman Collection v2.1 file into a Tree. It is lenient on read
// and strict on write (Postel): real Postman files fail their own published schema
// routinely — `info` requires `schema`, which hand-written and SDK-generated collections omit;
// `header` requires `value`, which a valueless header row violates — so this never validates
// against the schema, it decodes the members it models and keeps every other byte.
package postman

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// builderMethods is the seven-member vocabulary this app's request builder can show
// (HTTP_METHODS, packages/shared/domain/http.ts). Postman's own list is fifteen plus any custom
// string; a method outside this set is stored verbatim and merely *counted* here — the
// coercion to GET happens at exactly one renderer boundary, and export restores the original from
// origin.
var builderMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	"HEAD": true, "OPTIONS": true,
}

// maxCollectionBytes bounds Parse's own io.ReadAll: The memory side — a user-chosen file read
// whole into memory, then decoded into a map[string]json.RawMessage plus a cloneOrigin copy per
// node — was never bounded. 64 MiB is generously above any real Postman export and matches the
// control plane's own "if it's bigger than this, something is wrong" ceiling used elsewhere in
// this app.
const maxCollectionBytes = 64 << 20

// Parse reads one collection. The only three refusals are "the file is implausibly large",
// "not a JSON object with an info block", and D10's version gate — everything else degrades and
// is reported.
func Parse(r io.Reader) (*Tree, error) {
	doc, err := readDocument(r)
	if err != nil {
		return nil, err
	}
	return parseCollection(unwrapAPIDocument(doc))
}

// readDocument reads one bounded JSON object, the shared first step of Parse and ParseFile.
func readDocument(r io.Reader) (map[string]json.RawMessage, error) {
	limited := io.LimitReader(r, maxCollectionBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("postman: read: %w", err)
	}
	if len(data) > maxCollectionBytes {
		return nil, fmt.Errorf("postman: collection file is larger than %d bytes", maxCollectionBytes)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("postman: not a JSON object: %w", err)
	}
	return doc, nil
}

func parseCollection(doc map[string]json.RawMessage) (*Tree, error) {
	info := decodeObject(doc["info"])
	if info == nil {
		return nil, fmt.Errorf("postman: not a Postman collection: no info block")
	}
	if err := checkSchemaVersion(info); err != nil {
		return nil, err
	}

	name, _ := decodeString(info["name"])
	if strings.TrimSpace(name) == "" {
		name = "Imported collection"
	}

	// D5's origin capture for the collection itself: everything minus `item` (those become rows)
	// and minus `info.name` (that becomes a column).
	origin := cloneOrigin(doc)
	delete(origin, "item")
	infoOrigin := cloneOrigin(info)
	delete(infoOrigin, "name")
	origin["info"] = mustRaw(infoOrigin)
	// A collection-level `auth` block (bearer token, API key, password) is inert — kept but
	// never applied (countScriptsAndAuth below) — but "inert" must not mean "kept unencrypted in
	// kira.sqlite's origin_json forever". Stripped here, not merely blanked on export, so the
	// plaintext never reaches storage at all.
	delete(origin, "auth")

	tree := &Tree{Name: name, Origin: origin, Report: Report{Warnings: map[string]int{}}}
	// D15: the collection level is promoted, not inert — scripts and auth still count as inert
	// here, but `variable` does not go through countInertMembers at this one level.
	countScriptsAndAuth(doc, &tree.Report)
	var counts variableCounts
	tree.Variables, counts = decodeVariables(doc["variable"])
	delete(tree.Origin, "variable")
	if len(tree.Variables) > 0 {
		tree.Report.warnN(WarnVariablesImported, len(tree.Variables))
	}
	if counts.Disabled > 0 {
		tree.Report.warnN(WarnDisabledVariables, counts.Disabled)
	}
	if counts.Empty > 0 {
		tree.Report.warnN(WarnEmptyValues, counts.Empty)
	}
	if counts.Unnamed > 0 {
		tree.Report.warnN(WarnUnnamedVariables, counts.Unnamed)
	}
	if raw := doc["item"]; len(raw) > 0 && raw[0] == '[' {
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.Token(); err != nil { // opening '['
			return nil, fmt.Errorf("postman: decode items: %w", err)
		}
		nodes, err := decodeItems(dec, 1)
		if err != nil {
			return nil, err
		}
		walkItems(nodes, RootParent, tree)
	}
	return tree, nil
}

// VariableValue renders a Postman variable `value` as text. The schema leaves it untyped: a
// string, number or boolean renders as its literal; an object or array as compact JSON; null or
// absent as "".
func VariableValue(raw json.RawMessage) string {
	if s, ok := decodeScalarString(raw); ok {
		return s
	}
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return ""
	}
	return buf.String()
}

// decodeVariableEntry is the one rule set for a collection `variable[]` entry and an environment
// `values[]` entry. `type == "secret"` marks the entry secret; the original `type` string is kept so
// an unrecognised one survives export. disabled is `enabled: false` or `disabled: true`; the caller
// skips it. A key that is not a non-empty scalar becomes `unnamed_<n>` (numbered per array, skipping
// names the array already uses); a disabled entry never consumes a number.
func decodeVariableEntry(obj map[string]json.RawMessage, unnamed *unnamedKeys) (v Variable, disabled bool) {
	if enabled, has := decodeBool(obj["enabled"]); has && !enabled {
		disabled = true
	}
	if off, has := decodeBool(obj["disabled"]); has && off {
		disabled = true
	}
	if disabled {
		return Variable{}, true
	}
	name, isScalar := decodeScalarString(obj["key"])
	if !isScalar || name == "" {
		name = unnamed.next()
	}
	typ, _ := decodeString(obj["type"])
	return Variable{
		Name: name, Value: VariableValue(obj["value"]), Secret: typ == "secret", Type: typ,
		Description: decodeDescription(obj["description"]),
	}, false
}

// unnamedKeys hands out the generated keys of one array. taken holds the array's real keys, so a
// generated key never collides with one.
type unnamedKeys struct {
	taken map[string]bool
	n     int // last number tried
	count int // keys handed out
}

func newUnnamedKeys(entries []json.RawMessage) *unnamedKeys {
	u := &unnamedKeys{taken: map[string]bool{}}
	for _, entry := range entries {
		obj := decodeObject(entry)
		if obj == nil {
			continue
		}
		if name, ok := decodeScalarString(obj["key"]); ok && name != "" {
			u.taken[name] = true
		}
	}
	return u
}

func (u *unnamedKeys) next() string {
	u.count++
	for {
		u.n++
		if name := fmt.Sprintf("unnamed_%d", u.n); !u.taken[name] {
			return name
		}
	}
}

// DecodeVariables is decodeVariables for storage's legacy promotion, so promoted entries follow
// the fresh-import rules. ok is false when raw is not a JSON array.
func DecodeVariables(raw json.RawMessage) (vars []Variable, ok bool) {
	if t := bytes.TrimSpace(raw); len(t) == 0 || t[0] != '[' {
		return nil, false
	}
	vars, _ = decodeVariables(raw)
	return vars, true
}

// variableCounts is what decodeVariables counted besides the imported entries.
type variableCounts struct {
	Disabled int // skipped: Postman marks them disabled
	Empty    int // imported with value ""
	Unnamed  int // imported under a generated unnamed_<n> key
}

// decodeVariables decodes a collection-level `variable[]` array. Disabled entries are skipped and
// counted; every other object entry imports. Non-object entries carry no field to default and are
// skipped.
func decodeVariables(raw json.RawMessage) (vars []Variable, counts variableCounts) {
	vars = []Variable{}
	entries := decodeArray(raw)
	unnamed := newUnnamedKeys(entries)
	for _, entry := range entries {
		obj := decodeObject(entry)
		if obj == nil {
			continue
		}
		v, off := decodeVariableEntry(obj, unnamed)
		if off {
			counts.Disabled++
			continue
		}
		if v.Value == "" {
			counts.Empty++
		}
		vars = append(vars, v)
	}
	counts.Unnamed = unnamed.count
	return vars, counts
}

// checkSchemaVersion: v2.1 parses, an absent or unrecognised `info.schema` parses anyway
// (refusing a file for a missing advisory URL is a validator mistake), and a file
// that explicitly names v2.0.0 or v1 is refused with the version in the message. v2.0's `url` is
// string-only and it has no `options.raw.language` at all, so "supporting" it would mean a second
// translation table that is half-tested by construction.
func checkSchemaVersion(info map[string]json.RawMessage) error {
	schema, _ := decodeString(info["schema"])
	if schema == "" || strings.Contains(schema, "v2.1") {
		return nil
	}
	switch {
	case strings.Contains(schema, "v2.0"):
		return fmt.Errorf("postman: this is a Collection v2.0.0 file; only v2.1 is supported — re-export it from Postman as Collection v2.1")
	case strings.Contains(schema, "v1.0"), strings.Contains(schema, "/v1/"):
		return fmt.Errorf("postman: this is a Collection v1 file; only v2.1 is supported — convert it in Postman first, then export it as Collection v2.1")
	}
	return nil
}

// maxFolderDepth bounds folder nesting. Real collections nest a few folders; a deeper file is
// refused before it costs anything, and the cap also bounds the recursion below.
const maxFolderDepth = 64

// itemNode is one decoded item[] entry. members holds every member except `item`; the nested
// `item` array is decoded into children by the same single pass, never copied as raw bytes per
// level (that re-copied the whole subtree at every depth: O(depth x size) allocation).
type itemNode struct {
	members   map[string]json.RawMessage
	hasItem   bool
	children  []itemNode
	malformed bool // entry is not a JSON object
}

// decodeItems streams one item[] array (its '[' already consumed) in a single pass. depth is the
// folder nesting of this array, 1 for the document's own.
func decodeItems(dec *json.Decoder, depth int) ([]itemNode, error) {
	if depth > maxFolderDepth {
		return nil, fmt.Errorf("postman: folders are nested deeper than %d levels", maxFolderDepth)
	}
	nodes := []itemNode{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("postman: decode items: %w", err)
		}
		d, isDelim := tok.(json.Delim)
		if isDelim && d == '{' {
			node, err := decodeItemObject(dec, depth)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
			continue
		}
		if isDelim {
			if err := skipNested(dec); err != nil {
				return nil, err
			}
		}
		nodes = append(nodes, itemNode{malformed: true})
	}
	if _, err := dec.Token(); err != nil { // closing ']'
		return nil, fmt.Errorf("postman: decode items: %w", err)
	}
	return nodes, nil
}

// decodeItemObject reads one item's members (its '{' already consumed).
func decodeItemObject(dec *json.Decoder, depth int) (itemNode, error) {
	node := itemNode{members: map[string]json.RawMessage{}}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return node, fmt.Errorf("postman: decode items: %w", err)
		}
		key, _ := keyTok.(string)
		if key != "item" {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return node, fmt.Errorf("postman: decode items: %w", err)
			}
			node.members[key] = raw
			continue
		}
		node.hasItem, node.children = true, nil
		if node.children, err = decodeItemMember(dec, depth); err != nil {
			return node, err
		}
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return node, fmt.Errorf("postman: decode items: %w", err)
	}
	return node, nil
}

// decodeItemMember reads an `item` member's value: an array becomes children, anything else is
// consumed and dropped (the folder keeps no children).
func decodeItemMember(dec *json.Decoder, depth int) ([]itemNode, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("postman: decode items: %w", err)
	}
	d, isDelim := tok.(json.Delim)
	switch {
	case isDelim && d == '[':
		return decodeItems(dec, depth+1)
	case isDelim:
		return nil, skipNested(dec)
	}
	return nil, nil
}

// skipNested consumes tokens up to the close of the container whose opener was just read.
func skipNested(dec *json.Decoder) error {
	for level := 1; level > 0; {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("postman: decode items: %w", err)
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '[' || d == '{' {
				level++
			} else {
				level--
			}
		}
	}
	return nil
}

// walkItems is D3: depth-first, in document order, with sort_order the index within this parent's
// own item[] array. The discriminator is structural, not a field (F1) — a folder has an `item`
// member, a request has a `request` member.
func walkItems(nodes []itemNode, parent int, t *Tree) {
	order := 0
	for _, node := range nodes {
		obj := node.members
		if node.malformed {
			t.Report.warn(WarnMalformedItem)
			continue
		}
		isFolder := node.hasItem
		if _, isRequest := obj["request"]; !isFolder && !isRequest {
			// Neither member: a request with defaults (importRequest handles a nil member).
			t.Report.warn(WarnMissingRequest)
		}

		origin := cloneOrigin(obj)
		// F8: folder/item `auth` and a secret-typed folder/item `variable[]` entry's own value are
		// both inert (never applied, D9) but were kept verbatim in origin — plaintext credentials
		// sitting unencrypted in kira.sqlite's origin_json, re-emitted on every export regardless of
		// D16's own blank-on-export rule for a real, promoted (collection-level) secret variable.
		// Stripped at import instead, same as the collection level above.
		stripSensitiveOrigin(origin)
		if raw, ok := origin["request"]; ok {
			origin["request"] = stripRequestAuthOrigin(raw)
		}
		// A saved example repeats its request, auth included, in `originalRequest`.
		if raw, ok := origin["response"]; ok {
			origin["response"] = stripResponseAuthOrigin(raw)
		}
		name, hasName := decodeString(obj["name"])
		idx := len(t.Items)

		if isFolder {
			// Both members present (ill-formed): `item` wins, and the `request` member survives
			// untouched in origin, so the export re-emits exactly what came in.
			if !hasName || strings.TrimSpace(name) == "" {
				name = "New Folder"
			}
			t.Items = append(t.Items, Item{
				Parent: parent, Kind: KindFolder, Name: name, Order: order, Origin: origin,
			})
			t.Report.Folders++
			countInertMembers(obj, &t.Report)
			walkItems(node.children, idx, t)
		} else {
			request := importRequest(obj["request"], &t.Report)
			if !hasName || strings.TrimSpace(name) == "" {
				name = defaultRequestName(request.URL)
			}
			t.Items = append(t.Items, Item{
				Parent: parent, Kind: KindRequest, Name: name, Order: order,
				Request: request, Origin: origin,
			})
			t.Report.Requests++
			countInertMembers(obj, &t.Report)
		}
		order++
	}
}

// countScriptsAndAuth is countInertMembers' shared half, used at every level including the
// document's own — scripts and auth blocks are preserved verbatim and never executed or applied,
// at any level, and the report says so exactly once so nobody believes otherwise.
func countScriptsAndAuth(obj map[string]json.RawMessage, rep *Report) {
	if events := decodeArray(obj["event"]); len(events) > 0 {
		rep.warnN(WarnScriptsInert, len(events))
	}
	if auth := decodeObject(obj["auth"]); auth != nil {
		rep.warn(WarnAuthInert)
	}
}

// countInertMembers is D9's report half for the folder and item levels — variables stay inert at
// these levels (D15 promotes only the collection level, walkItems' own two call sites below).
func countInertMembers(obj map[string]json.RawMessage, rep *Report) {
	countScriptsAndAuth(obj, rep)
	if vars := decodeArray(obj["variable"]); len(vars) > 0 {
		rep.warnN(WarnVariablesInert, len(vars))
	}
}

// stripSensitiveOrigin is the strip applied to a folder or item's own origin clone (never
// the collection's — Parse does that one inline, ahead of the `item`/`info.name` split it is
// already doing). Removes `auth` entirely (its shape differs by type — bearer/apikey/basic/…—
// deleting the whole member is simpler and safer than a per-type partial blank) and blanks the
// `value` of any `variable[]` entry Postman marks `type: "secret"`, keeping its key/type so the
// entry itself still round-trips. A no-op when neither member is present.
func stripSensitiveOrigin(origin map[string]json.RawMessage) {
	delete(origin, "auth")
	if raw, ok := origin["variable"]; ok {
		if blanked, changed := blankSecretVariableValues(raw); changed {
			origin["variable"] = blanked
		}
	}
}

// stripRequestAuthOrigin is stripSensitiveOrigin's twin for a request item's own nested
// `request.auth` — a request object, unlike a folder or item, is never itself the map
// stripSensitiveOrigin walks (it is one raw member of it), so it gets its own pass. The string
// request form (a bare URL) carries no auth and is returned unchanged.
func stripRequestAuthOrigin(raw json.RawMessage) json.RawMessage {
	obj := decodeObject(raw)
	if obj == nil {
		return raw
	}
	if _, ok := obj["auth"]; !ok {
		return raw
	}
	clone := cloneOrigin(obj)
	delete(clone, "auth")
	return mustRaw(clone)
}

// stripResponseAuthOrigin removes `auth` from each saved example's `originalRequest`. raw is
// returned byte for byte when nothing needed stripping.
func stripResponseAuthOrigin(raw json.RawMessage) json.RawMessage {
	arr := decodeArray(raw)
	if arr == nil {
		return raw
	}
	changed := false
	for i, entry := range arr {
		obj := decodeObject(entry)
		orig, ok := obj["originalRequest"]
		if !ok {
			continue
		}
		stripped := stripRequestAuthOrigin(orig)
		if bytes.Equal(stripped, orig) {
			continue
		}
		clone := cloneOrigin(obj)
		clone["originalRequest"] = stripped
		arr[i] = mustRaw(clone)
		changed = true
	}
	if !changed {
		return raw
	}
	return mustRaw(arr)
}

// blankSecretVariableValues decodes raw as a Postman `variable[]` array and blanks the `value` of
// any entry whose `type` is `"secret"`, matching buildVariables' own export-time blank for a
// promoted (collection-level) secret variable — a folder/item-level one is never promoted, so this
// is the only place its value is ever cleared. changed is false, and raw is returned as-is, when
// nothing needed blanking (no entries, or none secret-typed) — a caller keeps the exact original
// bytes rather than a re-encoded (but semantically identical) copy.
func blankSecretVariableValues(raw json.RawMessage) (blanked json.RawMessage, changed bool) {
	arr := decodeArray(raw)
	if arr == nil {
		return raw, false
	}
	out := make([]json.RawMessage, len(arr))
	for i, entry := range arr {
		obj := decodeObject(entry)
		typ, _ := decodeString(obj["type"])
		if obj == nil || typ != "secret" {
			out[i] = entry
			continue
		}
		changed = true
		clone := cloneOrigin(obj)
		clone["value"] = mustRaw("")
		out[i] = mustRaw(clone)
	}
	if !changed {
		return raw, false
	}
	return mustRaw(out), true
}

// defaultRequestName is D3's one place import *adds* a member the file lacked. A nameless item is
// legal (F1) and a nameless row in the tree is worse than a derived one.
func defaultRequestName(url string) string {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return "Untitled request"
	}
	base := Split(trimmed).Base
	if i := strings.Index(base, "://"); i >= 0 {
		base = base[i+3:]
	}
	if i := strings.Index(base, "/"); i >= 0 && i+1 < len(base) {
		return base[i:]
	}
	return base
}

// importRequest handles F2's first row: `request` is oneOf [object, string], and the schema's own
// note says a string "is assumed to be the request URL and the method is assumed to be 'GET'".
func importRequest(raw json.RawMessage, rep *Report) model.SavedRequest {
	out := model.SavedRequest{Method: "GET", Headers: []model.SavedHeader{}, ParamDescriptions: map[string]string{}}
	defaultBody().applyTo(&out)

	if url, ok := decodeString(raw); ok {
		out.URL = url
		rewriteRequestAliases(&out)
		return out
	}
	obj := decodeObject(raw)
	if obj == nil {
		return out
	}
	if method, ok := decodeString(obj["method"]); ok && method != "" {
		// Stored verbatim, never upper-cased: a custom method is a real shape (F4) and rewriting
		// its spelling would change the bytes an untouched export re-emits.
		out.Method = method
		if !builderMethods[strings.ToUpper(method)] && rep != nil {
			rep.warn(WarnUnsupportedMethod)
		}
	}
	out.URL = ImportURL(obj["url"])
	// P22b D7: a query param's own description, the one place it can arrive from Postman
	// (url.query, not this app's own storage — F10 has no params array).
	out.ParamDescriptions = ImportParamDescriptions(obj["url"])
	out.Headers = importHeaders(obj["header"])
	importBody(obj["body"], rep).applyTo(&out)
	// Last, over the fully assembled request — one pass rather than one per importer.
	// ShedOrigin calls this same function to compare an origin against the saved request, so both
	// sides of that comparison see the identical rewrite and an untouched import still exports
	// byte-for-byte from its Origin.
	rewriteRequestAliases(&out)
	return out
}

// importHeaders handles F2's second row: `header` is oneOf [header-list, string], and the string
// form is a raw "A: 1\nB: 2" block.
func importHeaders(raw json.RawMessage) []model.SavedHeader {
	out := []model.SavedHeader{}
	if block, ok := decodeString(raw); ok {
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			name, value, found := strings.Cut(line, ":")
			if !found {
				continue
			}
			out = append(out, model.SavedHeader{
				Name: strings.TrimSpace(name), Value: strings.TrimSpace(value), Enabled: true,
			})
		}
		return out
	}
	// `value` is required by the schema and routinely absent in real files —
	// importKeyValueRows' decodeScalarString already imports a valueless row as an empty value,
	// not a dropped row. Its decodeDescription() call is the same one Variable import
	// uses — a header's own `description` is oneOf [string, Description-object], never
	// round-tripped verbatim.
	return append(out, importKeyValueRows(raw)...)
}
