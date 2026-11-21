package keptvars

import (
	"encoding/json"
	"regexp"
)

// ScriptSite is one literal kept.get("name"), kept.set("name", ...) or kept.setSecret("name", ...) call in a script. From and To are
// UTF-16 code units of the source, the unit an editor indexes by.
type ScriptSite struct {
	Name     string
	Op       string // "get", "set" or "setSecret"
	From, To int
}

var scriptCallRE = regexp.MustCompile(`\bkept\.(get|setSecret|set)\(\s*(?:"([A-Za-z][A-Za-z0-9_]{0,39})"|'([A-Za-z][A-Za-z0-9_]{0,39})')`)

// ScanScript lists the literal kept calls of src. Comments and strings count too: over-listing is the safe
// side, because the host functions refuse every name the scan did not find.
func ScanScript(src string) []ScriptSite {
	matches := scriptCallRE.FindAllStringSubmatchIndex(src, -1)
	out := make([]ScriptSite, 0, len(matches))
	for _, m := range matches {
		start, end := m[4], m[5]
		if start < 0 {
			start, end = m[6], m[7]
		}
		out = append(out, ScriptSite{
			Name: src[start:end], Op: src[m[2]:m[3]],
			From: ToUTF16(src, m[0]), To: ToUTF16(src, m[1]),
		})
	}
	return out
}

// Describe reports what a value is without storing it: its compact size, JSON type and item count. The
// error is the one Put would give.
func Describe(raw json.RawMessage) (typ string, items, bytes int, err error) {
	compact, typ, items, err := analyze(raw)
	return typ, items, len(compact), err
}
