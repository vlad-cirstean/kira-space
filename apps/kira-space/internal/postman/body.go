// body.go is the translation table, both directions. It exists because this app's body-mode
// vocabulary is not Postman's: Postman has one `raw` mode with an `options.raw.language` sub-field
// covering Text/JavaScript/JSON/HTML/XML plus a `graphql` mode; this app has
// none|raw|code|urlencoded|formdata|file where `raw` is plain text only, `code` owns the other
// four languages via its own `codeLanguage`, and there is no GraphQL mode at all.
package postman

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// A map[string]bool literal, read as plain text by
// tests/unit/go-ts-vocabulary-parity.spec.ts against CODE_LANGUAGES (http.ts) — do not turn this
// into a switch or derive it from another table. If a fifth language is ever added on the
// TypeScript side, the importer would otherwise silently stop recognising it (importing that
// language's bodies as plain `raw`) and the exporter would silently stop emitting it.
var postmanCodeLanguages = map[string]bool{
	"javascript": true, "json": true, "html": true, "xml": true,
}

// savedBody is the body half of a SavedRequest — the exact set of fields D6's unchanged-⇒-verbatim
// comparison covers, split out so import and the comparison agree by construction.
type savedBody struct {
	BodyMode     string
	Body         string
	Code         string
	CodeLanguage string
	URLEncoded   []model.SavedField
	FormData     []model.SavedFormField
	BinaryFile   *model.SavedFile
}

func (b savedBody) applyTo(r *model.SavedRequest) {
	r.BodyMode = b.BodyMode
	r.Body = b.Body
	r.Code = b.Code
	r.CodeLanguage = b.CodeLanguage
	r.URLEncoded = b.URLEncoded
	r.FormData = b.FormData
	r.BinaryFile = b.BinaryFile
}

func bodyOf(r model.SavedRequest) savedBody {
	return savedBody{
		BodyMode:     r.BodyMode,
		Body:         r.Body,
		Code:         r.Code,
		CodeLanguage: r.CodeLanguage,
		URLEncoded:   r.URLEncoded,
		FormData:     r.FormData,
		BinaryFile:   r.BinaryFile,
	}
}

// defaultBody is what an absent, null or mode-less `body` member imports as. codeLanguage defaults
// to 'json' the same way httpRequestTabStateSchema's own field does, so a Validate() over a
// never-had-a-body request still passes.
func defaultBody() savedBody {
	return savedBody{BodyMode: "none", CodeLanguage: "json", URLEncoded: []model.SavedField{}, FormData: []model.SavedFormField{}}
}

// ImportBody is D7's import half. rep is optional — Parse passes its own so the warnings are
// counted; the D6 comparison passes nil, since re-deriving a stored body must not re-report.
func importBody(raw json.RawMessage, rep *Report) savedBody {
	out := defaultBody()
	obj := decodeObject(raw)
	if obj == nil {
		return out
	}
	if disabled, ok := decodeBool(obj["disabled"]); ok && disabled && rep != nil {
		rep.warn(WarnDisabledBody)
	}
	mode, _ := decodeString(obj["mode"])
	switch mode {
	case "raw":
		text, _ := decodeString(obj["raw"])
		language := rawLanguage(obj["options"])
		if postmanCodeLanguages[language] {
			out.BodyMode, out.Code, out.CodeLanguage = "code", text, language
			return out
		}
		// Absent, "text", or an unrecognised value: `options` is untyped in the schema (F3), so
		// the value is free-form and must degrade rather than fail. The original language
		// survives in origin, so an unedited export restores it.
		out.BodyMode, out.Body = "raw", text
		return out
	case "urlencoded":
		out.BodyMode, out.URLEncoded = "urlencoded", importURLEncoded(obj["urlencoded"])
		return out
	case "formdata":
		out.BodyMode, out.FormData = "formdata", importFormData(obj["formdata"], rep)
		return out
	case "file":
		out.BodyMode = "file"
		out.BinaryFile = importFileBody(obj["file"], rep)
		return out
	case "graphql":
		// Import as code·json carrying the GraphQL-over-HTTP envelope this app would have sent, so
		// the imported request stays runnable. The round trip stays lossless while it is untouched
		// (re-emits mode:'graphql' verbatim); the moment the user edits that body it stops being a
		// GraphQL body and exports as raw + json, which is honest — an app with no GraphQL mode
		// cannot claim to have edited a GraphQL body.
		out.BodyMode, out.CodeLanguage = "code", "json"
		out.Code = graphqlEnvelope(obj["graphql"])
		if rep != nil {
			rep.warn(WarnGraphQLBody)
		}
		return out
	}
	return out
}

// rawLanguage reads options.raw.language — a documented Postman convention the published schema
// does not type at all (F3), so it is a free-form string with five known values, never an enum.
func rawLanguage(raw json.RawMessage) string {
	options := decodeObject(raw)
	if options == nil {
		return ""
	}
	rawOpts := decodeObject(options["raw"])
	if rawOpts == nil {
		return ""
	}
	language, _ := decodeString(rawOpts["language"])
	return language
}

// importKeyValueRows decodes a Postman key/value array — the shared row shape a url-encoded body
// row and a header row both use (name/value/disabled/description) — into SavedHeader values.
// SavedHeader and SavedField have identical fields, so a caller wanting SavedField converts
// each row with a plain Go struct conversion.
func importKeyValueRows(raw json.RawMessage) []model.SavedHeader {
	out := []model.SavedHeader{}
	for _, entry := range decodeArray(raw) {
		row := decodeObject(entry)
		if row == nil {
			continue
		}
		key, _ := decodeScalarString(row["key"])
		value, _ := decodeScalarString(row["value"])
		disabled, _ := decodeBool(row["disabled"])
		description := decodeDescription(row["description"])
		out = append(out, model.SavedHeader{Name: key, Value: value, Enabled: !disabled, Description: description})
	}
	return out
}

func importURLEncoded(raw json.RawMessage) []model.SavedField {
	rows := importKeyValueRows(raw)
	out := make([]model.SavedField, len(rows))
	for i, row := range rows {
		out[i] = model.SavedField(row)
	}
	return out
}

func importFormData(raw json.RawMessage, rep *Report) []model.SavedFormField {
	out := []model.SavedFormField{}
	for _, entry := range decodeArray(raw) {
		row := decodeObject(entry)
		if row == nil {
			continue
		}
		key, _ := decodeScalarString(row["key"])
		contentType, _ := decodeScalarString(row["contentType"])
		disabled, _ := decodeBool(row["disabled"])
		description := decodeDescription(row["description"])
		kind, _ := decodeString(row["type"])
		if kind != "file" {
			// `type` is optional in the schema's anyOf; absent means text.
			value, _ := decodeScalarString(row["value"])
			out = append(out, model.SavedFormField{
				Name: key, Kind: "text", Value: value, ContentType: contentType, Enabled: !disabled,
				Description: description,
			})
			continue
		}
		paths := fileSources(row["src"])
		if len(paths) == 0 {
			// src: null — the picker shows "No file chosen", and prepareFormParts already
			// refuses the send with a legible message.
			paths = []string{""}
		}
		for _, path := range paths {
			// An imported src is attacker-controlled input, not a path
			// this app resolved itself — a collection can be shared or handed over by anyone, and
			// the schema itself calls src a *name*, not a path. Carrying it straight into a live,
			// sendable Path would let a crafted collection name an arbitrary local file (an SSH
			// key, say) and stream it to a URL the user pasted in without reading, the moment they
			// press Send. So the row is left in the same "no file chosen" state prepareFormParts
			// already refuses with a legible message — the imported name is kept only for display
			// (FileName), never as something Send can open, until the user re-picks the file
			// through FilesService.ChooseOpen.
			if path != "" && rep != nil {
				rep.warn(WarnUnresolvedFile)
			}
			out = append(out, model.SavedFormField{
				Name: key, Kind: "file", Path: "", FileName: baseName(path),
				ContentType: contentType, Enabled: !disabled, Description: description,
			})
		}
	}
	return out
}

// fileSources handles formdata's `src` oneOf [string, null, array]. An array expands to N rows
// with the same key — lossless for sending (repeated names are legal in a multipart form) and
// legal for exporting (Postman accepts repeated formdata keys); no collapse heuristic is needed
// on export, because D6 re-emits the original array verbatim for an untouched body.
func fileSources(raw json.RawMessage) []string {
	if s, ok := decodeString(raw); ok {
		return []string{s}
	}
	entries := decodeArray(raw)
	if entries == nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if s, ok := decodeString(e); ok {
			out = append(out, s)
		}
	}
	return out
}

// importFileBody handles the `file` mode. The schema calls `src` a *name, not a path*, so
// treating it as one is optimistic by design and reported. A null src with only inline `content`
// writes no temp file — that would put an app-created file on disk with no owner, no lifetime and
// no cleanup story — the content is preserved in origin and reported as unresolvable.
func importFileBody(raw json.RawMessage, rep *Report) *model.SavedFile {
	obj := decodeObject(raw)
	if obj == nil {
		return nil
	}
	src, ok := decodeString(obj["src"])
	if !ok || src == "" {
		if _, hasContent := decodeString(obj["content"]); hasContent && rep != nil {
			rep.warn(WarnInlineFileContent)
		}
		return nil
	}
	if rep != nil {
		rep.warn(WarnUnresolvedFile)
	}
	// Same reasoning as formdata's file rows just above: src is attacker-controlled input from an
	// imported collection, not a path this app resolved — Path stays empty (the send path already
	// refuses an empty Path with "no file chosen for the request body") until the user re-picks the
	// file themselves. Name is kept only for display.
	return &model.SavedFile{Path: "", Name: baseName(src)}
}

// graphqlEnvelope builds `{"query":…,"variables":…,"operationName":…}` — byte-for-byte what this
// app's own GraphQL serializer built before GraphQL was removed, and what Content-Type:
// application/json plus a GraphQL-over-HTTP endpoint expects. The schema types `graphql` as a bare
// object and nothing more, so `variables` is handled in both the shapes anyone has described:
// an object is carried through, a string is parsed as JSON when valid and carried as a JSON string
// otherwise.
func graphqlEnvelope(raw json.RawMessage) string {
	obj := decodeObject(raw)
	if obj == nil {
		return ""
	}
	envelope := map[string]json.RawMessage{}
	if query, ok := decodeString(obj["query"]); ok {
		envelope["query"] = mustRaw(query)
	}
	if name, ok := decodeString(obj["operationName"]); ok && name != "" {
		envelope["operationName"] = mustRaw(name)
	}
	if vars := obj["variables"]; len(vars) > 0 {
		if s, ok := decodeString(vars); ok {
			if json.Valid([]byte(s)) && strings.TrimSpace(s) != "" {
				envelope["variables"] = json.RawMessage(s)
			} else {
				envelope["variables"] = vars
			}
		} else if !isJSONNull(vars) {
			envelope["variables"] = vars
		}
	}
	if len(envelope) == 0 {
		return ""
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return ""
	}
	return string(encoded)
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

// baseName is filepath.Base over a value that may well have come off another machine's OS —
// a Windows-style backslash path is handled too, since filepath.Base on Linux would not.
func baseName(path string) string {
	if path == "" {
		return ""
	}
	if i := strings.LastIndexAny(path, `\`); i >= 0 && !strings.Contains(path, "/") {
		return path[i+1:]
	}
	return filepath.Base(path)
}

// buildBody is D7's export half, taken under D6's `else` branch (the body actually changed).
// A `none` body returns nil, which omits the member entirely.
func buildBody(b savedBody) json.RawMessage {
	switch b.BodyMode {
	case "raw":
		// language:"text" is written explicitly rather than omitted, so a Postman import shows the
		// Text sub-selector instead of relying on a default this app does not control.
		return mustRaw(map[string]any{
			"mode":    "raw",
			"raw":     b.Body,
			"options": map[string]any{"raw": map[string]any{"language": "text"}},
		})
	case "code":
		language := b.CodeLanguage
		if !postmanCodeLanguages[language] {
			language = "text"
		}
		return mustRaw(map[string]any{
			"mode":    "raw",
			"raw":     b.Code,
			"options": map[string]any{"raw": map[string]any{"language": language}},
		})
	case "urlencoded":
		rows := make([]map[string]any, 0, len(b.URLEncoded))
		for _, f := range b.URLEncoded {
			row := map[string]any{"key": f.Name, "value": f.Value}
			// `disabled` is emitted only when true, matching the schema's own default: false.
			if !f.Enabled {
				row["disabled"] = true
			}
			if f.Description != "" {
				row["description"] = f.Description
			}
			rows = append(rows, row)
		}
		return mustRaw(map[string]any{"mode": "urlencoded", "urlencoded": rows})
	case "formdata":
		rows := make([]map[string]any, 0, len(b.FormData))
		for _, f := range b.FormData {
			row := map[string]any{"key": f.Name}
			if f.Kind == "file" {
				row["type"] = "file"
				// f.Path is a real absolute local filesystem path this app
				// resolved (FilesService.ChooseOpen) — export is a share artefact, and writing it
				// out verbatim disclosed the OS username, directory layout and often a client or
				// project name to whoever received the file. Import already refuses to trust a
				// collection's own src as a path for exactly the symmetric reason (body.go's
				// importFormdataBody above); baseName here is the same treatment on the way out —
				// and it is also all the recipient's own re-import can use anyway (it strips the
				// path down to the name regardless of what export sent).
				row["src"] = baseName(f.Path)
			} else {
				row["type"] = "text"
				row["value"] = f.Value
			}
			if f.ContentType != "" {
				row["contentType"] = f.ContentType
			}
			if !f.Enabled {
				row["disabled"] = true
			}
			if f.Description != "" {
				row["description"] = f.Description
			}
			rows = append(rows, row)
		}
		return mustRaw(map[string]any{"mode": "formdata", "formdata": rows})
	case "file":
		if b.BinaryFile == nil || b.BinaryFile.Path == "" {
			return mustRaw(map[string]any{"mode": "file", "file": map[string]any{"src": nil}})
		}
		// P21 round 3 finding 9: same reasoning as the formdata file rows above — baseName only.
		return mustRaw(map[string]any{
			"mode": "file",
			"file": map[string]any{"src": baseName(b.BinaryFile.Path)},
		})
	}
	return nil
}
