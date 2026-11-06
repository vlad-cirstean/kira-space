// url.go is the half of the boundary: this app's model is URL-authoritative (the raw string
// is canonical), Postman's `url` is a broken-down object. A Go sibling of
// frontend/src/views/httprequest/url.ts, deliberately NOT net/url.Parse — like the TypeScript
// half it must handle a half-formed or {{variable}}-bearing URL without throwing, and net/url has
// its own escaping opinions the round trip must not inherit. The shared rule both files state:
// split on the first '?' and the first '#'.
package postman

import (
	"encoding/json"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
)

// SplitURL is the exact counterpart of url.ts's splitUrl.
type SplitURL struct {
	Base string
	// Query is without the leading '?'.
	Query string
	// Hash is without the leading '#'.
	Hash string
}

// Split mirrors url.ts's splitUrl: two IndexOfs, not a parser, so 'api.exa' still splits cleanly.
func Split(text string) SplitURL {
	beforeHash, hash, _ := strings.Cut(text, "#")
	base, query, _ := strings.Cut(beforeHash, "?")
	return SplitURL{Base: base, Query: query, Hash: hash}
}

// queryPair is one `?a=b` pair, split on the first '='.
type queryPair struct{ Key, Value string }

func parseQuery(query string) []queryPair {
	if query == "" {
		return nil
	}
	out := []queryPair{}
	for _, pair := range strings.Split(query, "&") {
		if pair == "" {
			continue
		}
		if i := strings.Index(pair, "="); i >= 0 {
			out = append(out, queryPair{Key: pair[:i], Value: pair[i+1:]})
			continue
		}
		out = append(out, queryPair{Key: pair})
	}
	return out
}

// hasScheme mirrors url.ts's withScheme test without adding one — this side never invents a
// protocol the user did not type.

// containsVariable reports whether s carries a {{name}} reference — P5's own syntax, which must
// survive Build's splitting untouched (D8).
func containsVariable(s string) bool { return strings.Contains(s, "{{") }

// Build turns a raw URL string into Postman's own broken-down `url` object — what Postman itself
// writes in v2.1, and the shape with no ambiguity about how it is consumed. Empty members are
// omitted; `raw` is always present, which is what makes Import(Build(s)) == s by construction.
func Build(raw string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{"raw": mustRaw(raw)}
	parts := Split(raw)

	base := parts.Base
	if httpclient.HasScheme(base) {
		i := strings.Index(base, "://")
		out["protocol"] = mustRaw(base[:i])
		base = base[i+3:]
	}

	hostPort, path, _ := strings.Cut(base, "/")

	if hostPort != "" {
		host := hostPort
		// A trailing ":<digits>" is a port; ':' inside a {{variable}} is not.
		if i := strings.LastIndex(hostPort, ":"); i >= 0 && !containsVariable(hostPort[i:]) {
			if port := hostPort[i+1:]; port != "" && isAllDigits(port) {
				host = hostPort[:i]
				out["port"] = mustRaw(port)
			}
		}
		if host != "" {
			out["host"] = mustRaw(hostSegments(host))
		}
	}

	if path != "" {
		out["path"] = mustRaw(strings.Split(path, "/"))
	}

	if pairs := parseQuery(parts.Query); len(pairs) > 0 {
		entries := make([]map[string]string, 0, len(pairs))
		for _, p := range pairs {
			entries = append(entries, map[string]string{"key": p.Key, "value": p.Value})
		}
		out["query"] = mustRaw(entries)
	}

	if parts.Hash != "" {
		out["hash"] = mustRaw(parts.Hash)
	}
	return out
}

// hostSegments splits a host on '.', except when it carries a {{variable}} — `{{baseUrl}}` is one
// segment, not three (D8).
func hostSegments(host string) []string {
	if containsVariable(host) {
		return []string{host}
	}
	return strings.Split(host, ".")
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ImportParamDescriptions extracts a query param's own description from Postman's
// url.query array — the one place a per-param description can arrive, since this app has no
// native params row of its own. raw is the same `url` member ImportURL reads; a bare
// string (no query array at all) yields an empty, non-nil map.
func ImportParamDescriptions(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	obj := decodeObject(raw)
	if obj == nil {
		return out
	}
	for _, entry := range decodeArray(obj["query"]) {
		row := decodeObject(entry)
		if row == nil {
			continue
		}
		key, _ := decodeScalarString(row["key"])
		if key == "" {
			continue
		}
		if d := decodeDescription(row["description"]); d != "" {
			out[key] = d
		}
	}
	return out
}

// BuildWithDescriptions is Build plus each query param's own description, attached to
// the matching `query` entry when one exists — a separate entry point rather than widening
// Build's own signature, since every existing caller (this package's own tests included) calls
// Build with exactly the one argument Import(Build(s)) == s already depends on.
func BuildWithDescriptions(raw string, descriptions map[string]string) map[string]json.RawMessage {
	out := Build(raw)
	if len(descriptions) == 0 {
		return out
	}
	queryRaw, ok := out["query"]
	if !ok {
		return out
	}
	var entries []map[string]any
	if err := json.Unmarshal(queryRaw, &entries); err != nil {
		return out
	}
	changed := false
	for _, e := range entries {
		key, _ := e["key"].(string)
		if d, ok := descriptions[key]; ok && d != "" {
			e["description"] = d
			changed = true
		}
	}
	if changed {
		out["query"] = mustRaw(entries)
	}
	return out
}

// ImportURL turns Postman's `url` member — a string or an object (F2) — back into this app's raw
// URL string. An object's own `raw` wins when it is present and non-empty, which is the common
// case and which preserves `:pathVariable` segments and `{{baseUrl}}` references exactly as typed.
func ImportURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := decodeString(raw); ok {
		return s
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if s, ok := decodeString(obj["raw"]); ok && s != "" {
		return s
	}
	return reconstructURL(obj)
}

// reconstructURL rebuilds the string from protocol/host/port/path/query/hash, with `host` and
// `path` each handling both their string and array forms. A `query` entry with
// `disabled: true` is omitted — a disabled param has no representation in a URL-authoritative
// model — and survives in origin regardless.
func reconstructURL(obj map[string]json.RawMessage) string {
	var b strings.Builder
	if protocol, ok := decodeString(obj["protocol"]); ok && protocol != "" {
		b.WriteString(protocol)
		b.WriteString("://")
	}
	if host := joinStringOrArray(obj["host"], "."); host != "" {
		b.WriteString(host)
	}
	if port, ok := decodeScalarString(obj["port"]); ok && port != "" {
		b.WriteString(":")
		b.WriteString(port)
	}
	if path := joinStringOrArray(obj["path"], "/"); path != "" {
		if !strings.HasPrefix(path, "/") {
			b.WriteString("/")
		}
		b.WriteString(path)
	}
	if query := reconstructQuery(obj["query"]); query != "" {
		b.WriteString("?")
		b.WriteString(query)
	}
	if hash, ok := decodeString(obj["hash"]); ok && hash != "" {
		b.WriteString("#")
		b.WriteString(hash)
	}
	return b.String()
}

func reconstructQuery(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if disabled, ok := decodeBool(e["disabled"]); ok && disabled {
			continue
		}
		key, _ := decodeScalarString(e["key"])
		value, hasValue := decodeScalarString(e["value"])
		if !hasValue {
			parts = append(parts, key)
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "&")
}

// joinStringOrArray handles F2's `host`/`path` oneOf: a bare string, or an array whose entries are
// strings or path-variable segment objects ({"type":"string","value":":id"}), which contribute
// their `value`.
func joinStringOrArray(raw json.RawMessage, sep string) string {
	if len(raw) == 0 {
		return ""
	}
	if s, ok := decodeString(raw); ok {
		return s
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		if s, ok := decodeString(e); ok {
			parts = append(parts, s)
			continue
		}
		var seg map[string]json.RawMessage
		if err := json.Unmarshal(e, &seg); err != nil {
			continue
		}
		if v, ok := decodeScalarString(seg["value"]); ok {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, sep)
}
