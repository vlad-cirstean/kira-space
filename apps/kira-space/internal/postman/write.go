// write.go turns a Tree back into a Collection v2.1 file under the one rule, applied per
// request item:
//
//	Start from origin. Set `name`. For each of `url`, `header` and `body`: if re-running the
//	*importer* over the origin's own member yields exactly what is stored, re-emit the origin
//	member byte-identically; otherwise write a freshly built member. `method` is always written
//	from the stored request. Every other member is re-emitted from origin, untouched.
//
// It is one rule, not four heuristics — `import(origin.X) == stored.X ? origin.X : build(stored.X)`
// — which is why it is mechanically checkable and why the round-trip corpus is nothing but this
// assertion at scale.
package postman

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sort"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Write emits t as a v2.1 collection. json.Encoder with SetEscapeHTML(false) is the same
// mustMarshalNoEscape discipline internal/ipcfixture/write.go applies for the same reason: Go's
// default escapes '<', '>' and '&', which would mangle an HTML body or a {{var}}-adjacent
// character in a way that survives re-import but reads as corruption in a diff.
func Write(w io.Writer, t *Tree) error {
	doc := cloneOrigin(t.Origin)
	delete(doc, "item")

	info := decodeObject(doc["info"])
	if info == nil {
		info = map[string]json.RawMessage{}
	} else {
		info = cloneOrigin(info)
	}
	info["name"] = mustRaw(t.Name)
	// D10: the one info member that is never re-emitted from origin — the file being written *is*
	// v2.1 by construction, whatever the imported file said.
	info["schema"] = mustRaw(SchemaURL)
	doc["info"] = mustRaw(info)
	doc["item"] = buildItems(t, RootParent)
	// D15: `variable` is the exporter's fourth owned member, unconditionally — the rows *are* the
	// collection's variables now, and origin no longer carries the member to compare against.
	if len(t.Variables) > 0 {
		doc["variable"] = buildVariables(t.Variables)
	} else {
		delete(doc, "variable")
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "\t")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("postman: write: %w", err)
	}
	return nil
}

// buildItems rebuilds one `item[]` array from the flat list, ordered by Order — the exact inverse
// of walkItems, with no sorting anywhere else in the pipeline. A protocol='grpc' item is
// skipped outright — the format has no representation for one, and CollectionsService.Export
// counts the same skip (scanning t.Items itself) to report it rather than silently dropping it.
func buildItems(t *Tree, parent int) json.RawMessage {
	indices := []int{}
	for i, item := range t.Items {
		if item.Parent == parent && item.Protocol != "grpc" {
			indices = append(indices, i)
		}
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return t.Items[indices[a]].Order < t.Items[indices[b]].Order
	})
	out := make([]json.RawMessage, 0, len(indices))
	for _, i := range indices {
		out = append(out, buildItem(t, i))
	}
	return mustRaw(out)
}

func buildItem(t *Tree, idx int) json.RawMessage {
	item := t.Items[idx]
	obj := cloneOrigin(item.Origin)
	obj["name"] = mustRaw(item.Name)
	if item.Kind == KindFolder {
		obj["item"] = buildItems(t, idx)
	} else {
		delete(obj, "item")
		obj["request"] = buildRequest(obj["request"], item.Request)
	}
	return mustRaw(obj)
}

// importedOriginForCompare re-imports origin — a request member's raw bytes, object or string
// form, nil included — through the one real importer, importRequest, so its alias
// rewrite ({{$guid}} -> {{fake.string.uuid}}) always runs before the result is compared against a
// stored (and therefore already-rewritten) model.SavedRequest.
//
// buildRequest's and ShedOrigin's object-form branches used to call ImportURL/importHeaders/
// importBody directly on origin's members, bypassing rewriteRequestAliases entirely — origin's
// alias spelling never matched saved's rewritten spelling, so every alias-bearing field compared
// "changed" even when the user never touched it. Export wrote a fresh member
// ({{fake.string.uuid}} replacing {{$guid}}) instead of the untouched origin bytes, and
// ShedOrigin dropped those members from origin on first save. Both callers now compare against
// this one rewritten baseline instead, so they cannot drift from each other again.
func importedOriginForCompare(origin json.RawMessage) model.SavedRequest {
	return importRequest(origin, nil)
}

// buildRequest applies D6's rule per member. The bare-string origin form gets the same rule one
// level up: a string request that still imports to exactly what is stored is re-emitted verbatim.
func buildRequest(origin json.RawMessage, saved model.SavedRequest) json.RawMessage {
	if _, isString := decodeString(origin); isString {
		if requestEqual(importRequest(origin, nil), saved) {
			return origin
		}
		origin = nil
	}
	members := decodeObject(origin)
	out := map[string]json.RawMessage{}
	if members != nil {
		out = cloneOrigin(members)
	}

	// method needs no origin path — there is nothing to preserve beyond the string itself, and
	// writing it unconditionally removes a comparison.
	out["method"] = mustRaw(saved.Method)

	imported := importedOriginForCompare(origin)

	// P22b D7: a param description can change with the URL string untouched, so the origin `url`
	// member has to be rebuilt on either change, not just the raw string comparison alone.
	urlChanged := imported.URL != saved.URL
	descriptionsChanged := !maps.Equal(imported.ParamDescriptions, saved.ParamDescriptions)
	if urlChanged || descriptionsChanged {
		if saved.URL == "" {
			delete(out, "url")
		} else {
			out["url"] = mustRaw(BuildWithDescriptions(saved.URL, saved.ParamDescriptions))
		}
	}

	if !headersEqual(imported.Headers, saved.Headers) {
		if len(saved.Headers) == 0 {
			delete(out, "header")
		} else {
			out["header"] = buildHeaders(saved.Headers)
		}
	}

	if !bodyEqual(bodyOf(imported), bodyOf(saved)) {
		if built := buildBody(bodyOf(saved)); built != nil {
			out["body"] = built
		} else {
			delete(out, "body")
		}
	}

	return mustRaw(out)
}

// buildVariables emits `[{key, value, type?}]` — a secret's `value` is always "" in the
// written file, regardless of what LoadTree read (which is itself always "" for a secret row,
// the CHECK constraint), stated here rather than relied upon implicitly. `type` is "secret"
// for any secret entry (even one created in this app, which never had an original Postman `type`
// string) and otherwise the original string verbatim, when there was one.
func buildVariables(vars []Variable) json.RawMessage {
	rows := make([]map[string]any, 0, len(vars))
	for _, v := range vars {
		row := map[string]any{"key": v.Name}
		switch {
		case v.Secret:
			row["value"] = ""
			row["type"] = "secret"
		default:
			row["value"] = v.Value
			if v.Type != "" {
				row["type"] = v.Type
			}
		}
		// P17 D14: re-emitted as a plain string when non-empty, regardless of the shape it
		// arrived in — round-tripping an opaque object through a column the user can edit is not
		// honest (a description that arrived as an object's "content" member becomes that string).
		if v.Description != "" {
			row["description"] = v.Description
		}
		rows = append(rows, row)
	}
	return mustRaw(rows)
}

func buildHeaders(headers []model.SavedHeader) json.RawMessage {
	rows := make([]map[string]any, 0, len(headers))
	for _, h := range headers {
		row := map[string]any{"key": h.Name, "value": h.Value}
		if !h.Enabled {
			row["disabled"] = true
		}
		// P22b D6: buildVariables' own rule above — a plain string, only when non-empty.
		if h.Description != "" {
			row["description"] = h.Description
		}
		rows = append(rows, row)
	}
	return mustRaw(rows)
}

// ShedOrigin is called by CollectionsRepo.SaveRequest: it deletes each member the user has changed
// from the stored origin, so an edited request stops carrying a stale duplicate of its own body
// and export's rule degenerates correctly — with the member gone from origin, the `else` branch is
// taken.
func ShedOrigin(origin map[string]json.RawMessage, saved model.SavedRequest) map[string]json.RawMessage {
	out := cloneOrigin(origin)
	raw, ok := out["request"]
	if !ok {
		return out
	}
	if _, isString := decodeString(raw); isString {
		if !requestEqual(importRequest(raw, nil), saved) {
			delete(out, "request")
		}
		return out
	}
	members := decodeObject(raw)
	if members == nil {
		return out
	}
	request := cloneOrigin(members)
	imported := importedOriginForCompare(raw)
	// P22b D7: a param description can change with the URL string untouched (see buildRequest's
	// own comment above) — the origin url member must be shed on either change.
	if imported.URL != saved.URL ||
		!maps.Equal(imported.ParamDescriptions, saved.ParamDescriptions) {
		delete(request, "url")
	}
	if !headersEqual(imported.Headers, saved.Headers) {
		delete(request, "header")
	}
	if !bodyEqual(bodyOf(imported), bodyOf(saved)) {
		delete(request, "body")
	}
	out["request"] = mustRaw(request)
	return out
}

// ---- the comparisons D6's rule is made of ----
//
// Each compares only what Postman actually carries for that member: FileName/FileSize are this
// app's own display conveniences and re-picking the same file must not rewrite an untouched
// `formdata` member.

func requestEqual(a, b model.SavedRequest) bool {
	return a.Method == b.Method && a.URL == b.URL &&
		headersEqual(a.Headers, b.Headers) && bodyEqual(bodyOf(a), bodyOf(b)) &&
		maps.Equal(a.ParamDescriptions, b.ParamDescriptions)
}

func headersEqual(a, b []model.SavedHeader) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bodyEqual(a, b savedBody) bool {
	if a.BodyMode != b.BodyMode {
		return false
	}
	switch a.BodyMode {
	case "raw":
		return a.Body == b.Body
	case "code":
		return a.Code == b.Code && a.CodeLanguage == b.CodeLanguage
	case "urlencoded":
		return fieldsEqual(a.URLEncoded, b.URLEncoded)
	case "formdata":
		return formFieldsEqual(a.FormData, b.FormData)
	case "file":
		return filePathOf(a.BinaryFile) == filePathOf(b.BinaryFile)
	}
	return true
}

func fieldsEqual(a, b []model.SavedField) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formFieldsEqual(a, b []model.SavedFormField) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Name != y.Name || x.Kind != y.Kind || x.Value != y.Value ||
			x.Path != y.Path || x.ContentType != y.ContentType || x.Enabled != y.Enabled {
			return false
		}
	}
	return true
}

func filePathOf(f *model.SavedFile) string {
	if f == nil {
		return ""
	}
	return f.Path
}
