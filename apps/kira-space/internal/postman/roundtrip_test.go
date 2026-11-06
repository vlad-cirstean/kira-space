// internal/postman is CLAUDE.md's named category — a parser with several interacting
// rules over a real wire format, where every failure mode is silent (a dropped event[] is
// invisible until someone opens the export in Postman; a scrambled sort_order looks like the
// user's own ordering). The one assertion that matters is the rule at scale:
// import(origin.X) == stored.X ? origin.X : build(stored.X).
package postman_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func parseFile(t *testing.T, name string) *postman.Tree {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer f.Close()
	tree, err := postman.Parse(f)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return tree
}

// exported writes tree and decodes the result, so a test asserts against the real emitted bytes.
func exported(t *testing.T, tree *postman.Tree) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := postman.Write(&buf, tree); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("re-decode written collection: %v\n%s", err, buf.String())
	}
	return out
}

func decodeFile(t *testing.T, name string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return out
}

// items indexes an exported collection's item[] array by name, at any depth.
func items(root map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	var walk func(v any)
	walk = func(v any) {
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for _, entry := range arr {
			obj, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if name, ok := obj["name"].(string); ok {
				out[name] = obj
			}
			walk(obj["item"])
		}
	}
	walk(root["item"])
	return out
}

func requestOf(t *testing.T, item map[string]any) map[string]any {
	t.Helper()
	req, ok := item["request"].(map[string]any)
	if !ok {
		t.Fatalf("item %v has no object request", item["name"])
	}
	return req
}

// itemIndex finds the one item with this name in a parsed tree.
func itemIndex(t *testing.T, tree *postman.Tree, name string) int {
	t.Helper()
	for i, item := range tree.Items {
		if item.Name == name {
			return i
		}
	}
	t.Fatalf("no item named %q", name)
	return -1
}

// ---- 1. Nesting and order ----

func TestNestingAndOrderSurviveARoundTrip(t *testing.T) {
	tree := parseFile(t, "nesting.json")

	// Deliberately non-alphabetical siblings, at three depths.
	type row struct {
		name  string
		depth int
		order int
	}
	depth := func(i int) int {
		d := 0
		for tree.Items[i].Parent != postman.RootParent {
			i = tree.Items[i].Parent
			d++
		}
		return d
	}
	got := []row{}
	for i, item := range tree.Items {
		got = append(got, row{item.Name, depth(i), item.Order})
	}
	want := []row{
		{"zulu", 0, 0}, {"mike", 1, 0}, {"delta", 2, 0}, {"alpha", 2, 1},
		{"charlie", 1, 1}, {"bravo", 0, 1}, {"yankee", 0, 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed tree shape:\n got %v\nwant %v", got, want)
	}

	// Written back, the arrays come out in the same order at the same depths — an unordered or
	// map-shaped model would silently scramble a collection on its first round trip (F1).
	out := exported(t, tree)
	root, _ := out["item"].([]any)
	names := []string{}
	for _, entry := range root {
		names = append(names, entry.(map[string]any)["name"].(string))
	}
	if want := []string{"zulu", "bravo", "yankee"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("root order: got %v, want %v", names, want)
	}
	zulu := root[0].(map[string]any)["item"].([]any)
	inner := []string{zulu[0].(map[string]any)["name"].(string), zulu[1].(map[string]any)["name"].(string)}
	if want := []string{"mike", "charlie"}; !reflect.DeepEqual(inner, want) {
		t.Fatalf("zulu's order: got %v, want %v", inner, want)
	}
	mike := zulu[0].(map[string]any)["item"].([]any)
	leaves := []string{mike[0].(map[string]any)["name"].(string), mike[1].(map[string]any)["name"].(string)}
	if want := []string{"delta", "alpha"}; !reflect.DeepEqual(leaves, want) {
		t.Fatalf("mike's order: got %v, want %v", leaves, want)
	}
}

// ---- 2. Every oneOf from F2 in one file ----
//
// This is the file that would fail against the library D1 declined, which is why it exists: four
// of these shapes are an unmarshal *error* there, i.e. a whole-file import failure on shapes the
// published schema explicitly allows.

func TestEveryOneOfParsesAndSurvives(t *testing.T) {
	tree := parseFile(t, "oneofs.json")
	in := decodeFile(t, "oneofs.json")
	out := exported(t, tree)
	inItems, outItems := items(in), items(out)

	t.Run("a string request is the URL, with GET assumed", func(t *testing.T) {
		got := tree.Items[itemIndex(t, tree, "string request")].Request
		if got.Method != "GET" || got.URL != "https://api.example.com/string-request" {
			t.Fatalf("got %+v", got)
		}
		// Untouched, so it is re-emitted verbatim — the whole-member form of D6's rule.
		if outItems["string request"]["request"] != "https://api.example.com/string-request" {
			t.Fatalf("string request was rewritten: %#v", outItems["string request"]["request"])
		}
	})

	t.Run("a string header block splits into rows", func(t *testing.T) {
		got := tree.Items[itemIndex(t, tree, "string header block")].Request.Headers
		want := []model.SavedHeader{
			{Name: "X-First", Value: "1", Enabled: true},
			{Name: "X-Second", Value: "two", Enabled: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		if h := requestOf(t, outItems["string header block"])["header"]; h != "X-First: 1\nX-Second: two" {
			t.Fatalf("header block was rewritten: %#v", h)
		}
	})

	t.Run("a string host and a string path reconstruct", func(t *testing.T) {
		got := tree.Items[itemIndex(t, tree, "string url and string host")].Request.URL
		if got != "api.example.com/plain/path" {
			t.Fatalf("got %q", got)
		}
		if !reflect.DeepEqual(
			requestOf(t, outItems["string url and string host"])["url"],
			requestOf(t, inItems["string url and string host"])["url"],
		) {
			t.Fatal("an untouched url object was rewritten")
		}
	})

	t.Run("a path-variable segment contributes its value, a disabled query param does not", func(t *testing.T) {
		req := tree.Items[itemIndex(t, tree, "path variable segment")].Request
		if req.URL != "https://api.example.com:8443/users/:id/orders?limit=10#section" {
			t.Fatalf("got %q", req.URL)
		}
		// P22b D7/F10: a query param's own description arrives from Postman's own url.query — the
		// one place it can, since this app has no params row of its own. "limit"'s survives;
		// "cursor" is disabled (dropped from the URL entirely, D9) and never a description key.
		wantDescriptions := map[string]string{"limit": "how many"}
		if !reflect.DeepEqual(req.ParamDescriptions, wantDescriptions) {
			t.Fatalf("ParamDescriptions = %#v, want %#v", req.ParamDescriptions, wantDescriptions)
		}
		// Untouched, so the whole url object — description included — is re-emitted verbatim.
		if !reflect.DeepEqual(
			requestOf(t, outItems["path variable segment"])["url"],
			requestOf(t, inItems["path variable segment"])["url"],
		) {
			t.Fatal("an untouched url object was rewritten")
		}
	})

	t.Run("object descriptions and a string script.exec survive; a numeric variable value re-exports as its promoted row's own string", func(t *testing.T) {
		// event/info are not decoded at all — they live in origin as raw bytes, which is exactly
		// why parsing cannot error on them.
		for _, member := range []string{"event", "info"} {
			if !reflect.DeepEqual(out[member], in[member]) {
				t.Fatalf("collection-level %q changed:\n got %#v\nwant %#v", member, out[member], in[member])
			}
		}
		// Collection-level `variable` is promoted, not inert — it round-trips through
		// VariablesRepo's own TEXT columns, so a numeric/boolean `value` (the untyped-value
		// finding) re-exports as the equivalent JSON string rather than byte-identically. This is
		// the same lossy-scalar handling `decodeScalarString` already applies to header/url values
		// elsewhere in this package, not a new corner cut for variables specifically.
		gotVars, ok := out["variable"].([]any)
		if !ok || len(gotVars) != 3 {
			t.Fatalf("out[\"variable\"] = %#v, want 3 promoted rows", out["variable"])
		}
		// "port"'s description arrived as an object (`{content: ...}, Postman's own
		// richer shape, mirroring info.description above) and re-exports as a plain string —
		// decodeDescription's own leniency, and buildVariables' own "never round-trip an opaque
		// object through a column the user can edit" rule. "baseUrl"'s arrived as a plain string
		// and survives unchanged. "secure" has no description at all and gets no member.
		want := []map[string]any{
			{"key": "port", "value": "8080", "type": "number", "description": "Port number"},
			{"key": "secure", "value": "true", "type": "boolean"},
			{"key": "baseUrl", "value": "https://api.example.com", "description": "The tenant's public base URL"},
		}
		for i, row := range gotVars {
			if !reflect.DeepEqual(row, want[i]) {
				t.Errorf("out[\"variable\"][%d] = %#v, want %#v", i, row, want[i])
			}
		}
		if !reflect.DeepEqual(outItems["path variable segment"]["description"], inItems["path variable segment"]["description"]) {
			t.Fatal("an item's object description changed")
		}
		if !reflect.DeepEqual(
			requestOf(t, outItems["path variable segment"])["header"],
			requestOf(t, inItems["path variable segment"])["header"],
		) {
			t.Fatal("a header row's own description was dropped")
		}
	})
}

// ---- 3. Every body mode round-trips untouched, byte-identically ----

func TestEveryBodyModeRoundTripsUntouched(t *testing.T) {
	tree := parseFile(t, "bodies.json")
	in, out := decodeFile(t, "bodies.json"), exported(t, tree)
	inItems, outItems := items(in), items(out)

	for name := range inItems {
		wantBody := requestOf(t, inItems[name])["body"]
		gotBody := requestOf(t, outItems[name])["body"]
		if !reflect.DeepEqual(gotBody, wantBody) {
			t.Errorf("%s: body changed\n got %#v\nwant %#v", name, gotBody, wantBody)
		}
	}
}

func TestImportTranslatesEveryBodyMode(t *testing.T) {
	tree := parseFile(t, "bodies.json")
	req := func(name string) model.SavedRequest { return tree.Items[itemIndex(t, tree, name)].Request }

	cases := []struct {
		item     string
		mode     string
		check    func(r model.SavedRequest) bool
		describe string
	}{
		{"raw text", "raw", func(r model.SavedRequest) bool { return r.Body == "plain <b>text</b> & more" }, "raw·text stays plain text"},
		{"raw no language", "raw", func(r model.SavedRequest) bool { return r.Body == "no options member at all" }, "an absent language is text"},
		{"raw javascript", "code", func(r model.SavedRequest) bool { return r.CodeLanguage == "javascript" && r.Code == "const x = 1;" }, "raw·javascript becomes code"},
		{"raw json", "code", func(r model.SavedRequest) bool { return r.CodeLanguage == "json" }, "raw·json becomes code"},
		{"raw html", "code", func(r model.SavedRequest) bool { return r.CodeLanguage == "html" }, "raw·html becomes code"},
		{"raw xml", "code", func(r model.SavedRequest) bool { return r.CodeLanguage == "xml" }, "raw·xml becomes code"},
		{"raw unknown language", "raw", func(r model.SavedRequest) bool { return r.Body == "SELECT 1" }, "an unrecognised language degrades to plain text"},
		{"file with src", "file", func(r model.SavedRequest) bool {
			// P21 round 1 finding 3: src is attacker-controlled input from an imported collection,
			// not a path this app resolved — Path stays empty (the send path already refuses an
			// empty Path) until the user re-picks the file themselves. Name is kept for display.
			return r.BinaryFile != nil && r.BinaryFile.Path == "" && r.BinaryFile.Name == "payload.bin"
		}, "a file src is never carried into a live, sendable path"},
		{"file with content only", "file", func(r model.SavedRequest) bool { return r.BinaryFile == nil }, "inline content writes no temp file"},
		{"no body at all", "none", func(r model.SavedRequest) bool { return true }, "an absent body is none"},
		{"disabled body", "raw", func(r model.SavedRequest) bool { return r.Body == "not sent by Postman" }, "a disabled body imports as its mode"},
	}
	for _, tc := range cases {
		got := req(tc.item)
		if got.BodyMode != tc.mode {
			t.Errorf("%s: mode = %q, want %q", tc.item, got.BodyMode, tc.mode)
			continue
		}
		if !tc.check(got) {
			t.Errorf("%s (%s): %+v", tc.item, tc.describe, got)
		}
	}

	t.Run("urlencoded keeps its disabled row", func(t *testing.T) {
		got := req("urlencoded").URLEncoded
		want := []model.SavedField{
			// P22b D6: the testdata's own grant_type row carries a Postman `description` —
			// importURLEncoded now decodes it rather than silently dropping it.
			{Name: "grant_type", Value: "password", Enabled: true, Description: "the oauth grant"},
			{Name: "scope", Value: "read", Enabled: false},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("a formdata src array expands to one row per entry", func(t *testing.T) {
		// P21 round 1 finding 3: an imported src is never carried into a live, sendable Path —
		// only FileName (for display) survives the import. Path stays empty until the user
		// re-picks the file themselves.
		got := req("formdata").FormData
		want := []model.SavedFormField{
			// P22b D6: the testdata's own caption row carries a Postman `description`.
			{Name: "caption", Kind: "text", Value: "a text row", Enabled: true, Description: "the caption"},
			{Name: "notype", Kind: "text", Value: "type is absent, so text", Enabled: true},
			{Name: "single", Kind: "file", FileName: "report.csv", ContentType: "text/csv", Enabled: true},
			{Name: "many", Kind: "file", FileName: "a.png", Enabled: true},
			{Name: "many", Kind: "file", FileName: "b.png", Enabled: true},
			{Name: "none", Kind: "file", Enabled: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v\nwant %+v", got, want)
		}
	})

	t.Run("a graphql body becomes the GraphQL-over-HTTP envelope as code·json", func(t *testing.T) {
		got := req("graphql")
		if got.BodyMode != "code" || got.CodeLanguage != "json" {
			t.Fatalf("got mode %q/%q", got.BodyMode, got.CodeLanguage)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(got.Code), &envelope); err != nil {
			t.Fatalf("envelope is not JSON: %v (%s)", err, got.Code)
		}
		want := map[string]any{
			"query":         "query Order($id: ID!) { order(id: $id) { id } }",
			"operationName": "Order",
			"variables":     map[string]any{"id": "42"},
		}
		if !reflect.DeepEqual(envelope, want) {
			t.Fatalf("got %#v, want %#v", envelope, want)
		}
	})

	t.Run("the report counts what it did on the user's behalf", func(t *testing.T) {
		for kind, want := range map[string]int{
			postman.WarnGraphQLBody:       1,
			postman.WarnInlineFileContent: 1,
			postman.WarnDisabledBody:      1,
			// 'single' plus two 'many' entries plus the file-mode src.
			postman.WarnUnresolvedFile: 4,
		} {
			if got := tree.Report.Warnings[kind]; got != want {
				t.Errorf("warning %s = %d, want %d", kind, got, want)
			}
		}
	})
}

// ---- 4. An edited body exports canonically, and its origin is shed ----

func TestAnEditedBodyExportsCanonicallyAndShedsItsOrigin(t *testing.T) {
	tree := parseFile(t, "bodies.json")

	// The graphql case is the one that changes identity: an app with no GraphQL mode cannot claim
	// to have edited a GraphQL body, so it exports as raw + language:"json".
	idx := itemIndex(t, tree, "graphql")
	tree.Items[idx].Request.Code = `{"query":"query { me { id } }"}`
	tree.Items[idx].Origin = postman.ShedOrigin(tree.Items[idx].Origin, tree.Items[idx].Request)

	rawIdx := itemIndex(t, tree, "raw text")
	tree.Items[rawIdx].Request.Body = "edited"
	tree.Items[rawIdx].Origin = postman.ShedOrigin(tree.Items[rawIdx].Origin, tree.Items[rawIdx].Request)

	outItems := items(exported(t, tree))

	gql := requestOf(t, outItems["graphql"])["body"].(map[string]any)
	if gql["mode"] != "raw" || gql["raw"] != `{"query":"query { me { id } }"}` {
		t.Fatalf("edited graphql body: %#v", gql)
	}
	language := gql["options"].(map[string]any)["raw"].(map[string]any)["language"]
	if language != "json" {
		t.Fatalf("edited graphql language = %v, want json", language)
	}

	raw := requestOf(t, outItems["raw text"])["body"].(map[string]any)
	if raw["mode"] != "raw" || raw["raw"] != "edited" {
		t.Fatalf("edited raw body: %#v", raw)
	}
	// language:"text" is written explicitly rather than omitted, so a Postman import shows the
	// Text sub-selector instead of relying on a default this app does not control.
	if raw["options"].(map[string]any)["raw"].(map[string]any)["language"] != "text" {
		t.Fatalf("edited raw options: %#v", raw["options"])
	}

	t.Run("shedding removes the stale origin body and nothing else", func(t *testing.T) {
		origin := tree.Items[rawIdx].Origin
		var request map[string]json.RawMessage
		if err := json.Unmarshal(origin["request"], &request); err != nil {
			t.Fatalf("origin request: %v", err)
		}
		if _, ok := request["body"]; ok {
			t.Fatal("the stale body is still in origin")
		}
		if _, ok := request["url"]; !ok {
			t.Fatal("an untouched url was shed too")
		}
	})
}

// TestAnEditedParamDescriptionExportsAndShedsItsOrigin is P22b D7's own edge case: a param
// description can change with the URL string itself untouched, which is exactly the case
// requestEqual/ShedOrigin's plain string comparisons would miss without their own
// paramDescriptionsEqual check.
func TestAnEditedParamDescriptionExportsAndShedsItsOrigin(t *testing.T) {
	tree := parseFile(t, "oneofs.json")
	idx := itemIndex(t, tree, "path variable segment")

	tree.Items[idx].Request.ParamDescriptions = map[string]string{"limit": "how many, edited"}
	tree.Items[idx].Origin = postman.ShedOrigin(tree.Items[idx].Origin, tree.Items[idx].Request)

	out := requestOf(t, items(exported(t, tree))["path variable segment"])
	query := out["url"].(map[string]any)["query"].([]any)
	limit := query[0].(map[string]any)
	if limit["key"] != "limit" || limit["description"] != "how many, edited" {
		t.Fatalf("exported query = %#v", query)
	}

	var request map[string]json.RawMessage
	if err := json.Unmarshal(tree.Items[idx].Origin["request"], &request); err != nil {
		t.Fatalf("origin request: %v", err)
	}
	if _, ok := request["url"]; ok {
		t.Fatal("the stale url (with its old description) is still in origin")
	}
}

// TestObjectFormAliasesRoundTripUntouchedAndSurviveShedOrigin: buildRequest's and
// ShedOrigin's object-form branches used to compare origin's raw, un-rewritten alias spelling
// ({{$guid}}) against the stored request's already-rewritten spelling ({{fake.string.uuid}}),
// so an object-form request using a Postman dynamic-variable alias in its url/header/body always
// compared "changed" even when nobody edited it — export wrote a fresh member with the fake.*
// spelling instead of origin's own bytes, and ShedOrigin dropped those members from origin on
// first save.
func TestObjectFormAliasesRoundTripUntouchedAndSurviveShedOrigin(t *testing.T) {
	tree := parseFile(t, "aliases-object-form.json")
	idx := itemIndex(t, tree, "untouched")

	// The stored request itself carries the rewritten fake.* spelling (P28 D15(c)) — Send resolves
	// against this app's own vocabulary, never Postman's.
	if !strings.Contains(tree.Items[idx].Request.URL, "fake.string.uuid") {
		t.Fatalf("Request.URL = %q, want the rewritten fake.string.uuid spelling", tree.Items[idx].Request.URL)
	}

	out := requestOf(t, items(exported(t, tree))["untouched"])
	// The origin url member was a plain string (no query-param descriptions to preserve), so an
	// untouched export keeps it exactly that shape — cloneOrigin's own verbatim-bytes rule.
	rawURL, _ := out["url"].(string)
	if !strings.Contains(rawURL, "{{$guid}}") {
		t.Fatalf("exported url = %#v, want the untouched origin bytes ({{$guid}} intact)", out["url"])
	}
	headers := out["header"].([]any)[0].(map[string]any)
	if headers["key"] != "X-Trace-{{$guid}}" || headers["value"] != "{{$timestamp}}" {
		t.Fatalf("exported header = %#v, want origin's own {{$guid}}/{{$timestamp}} bytes untouched", headers)
	}
	body := out["body"].(map[string]any)
	if body["raw"] != `{"id":"{{$guid}}"}` {
		t.Fatalf("exported body = %#v, want origin's own {{$guid}} bytes untouched", body)
	}

	t.Run("ShedOrigin keeps an untouched request's url/header/body members", func(t *testing.T) {
		origin := postman.ShedOrigin(tree.Items[idx].Origin, tree.Items[idx].Request)
		var request map[string]json.RawMessage
		if err := json.Unmarshal(origin["request"], &request); err != nil {
			t.Fatalf("origin request: %v", err)
		}
		for _, member := range []string{"url", "header", "body"} {
			if _, ok := request[member]; !ok {
				t.Fatalf("ShedOrigin dropped the untouched %q member — F7 regression", member)
			}
		}
	})
}

// TestExportStripsLocalFilePathsToTheirBaseName: buildBody (the "the body actually changed" branch)
// used to write a formdata file row's or a
// file-mode body's real absolute local Path straight into `src` — the exact opposite of import's
// own rule, applied a few lines above in this same file, that src is never trustworthy and Path
// stays empty on the way in. An export is a share artefact; the absolute path discloses the OS
// username, directory layout and often a client or project name to whoever receives the file, and
// is useless to them besides (their own re-import strips it back down to the basename anyway).
func TestExportStripsLocalFilePathsToTheirBaseName(t *testing.T) {
	tree := parseFile(t, "bodies.json")

	const sensitivePath = "/Users/jane.doe/Documents/clients/acme-migration/payload.json"
	formIdx := itemIndex(t, tree, "formdata")
	tree.Items[formIdx].Request.FormData = []model.SavedFormField{
		{Name: "upload", Kind: "file", Path: sensitivePath, FileName: "payload.json", Enabled: true},
	}
	tree.Items[formIdx].Origin = postman.ShedOrigin(tree.Items[formIdx].Origin, tree.Items[formIdx].Request)

	const sensitiveBinaryPath = "/Users/jane.doe/Documents/clients/acme-migration/payload.bin"
	fileIdx := itemIndex(t, tree, "file with src")
	tree.Items[fileIdx].Request.BinaryFile = &model.SavedFile{Path: sensitiveBinaryPath, Name: "payload.bin"}
	tree.Items[fileIdx].Origin = postman.ShedOrigin(tree.Items[fileIdx].Origin, tree.Items[fileIdx].Request)

	outItems := items(exported(t, tree))

	formBody := requestOf(t, outItems["formdata"])["body"].(map[string]any)
	rows, ok := formBody["formdata"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("formdata body: %#v", formBody)
	}
	row := rows[0].(map[string]any)
	if src, _ := row["src"].(string); src != "payload.json" {
		t.Fatalf("formdata file src = %q, want basename-only %q — got the real local path leaked into the export", src, "payload.json")
	}

	fileBody := requestOf(t, outItems["file with src"])["body"].(map[string]any)
	fileObj, ok := fileBody["file"].(map[string]any)
	if !ok {
		t.Fatalf("file-mode body: %#v", fileBody)
	}
	if src, _ := fileObj["src"].(string); src != "payload.bin" {
		t.Fatalf("file-mode src = %q, want basename-only %q — got the real local path leaked into the export", src, "payload.bin")
	}
}

// ---- 5. The unchanged-⇒-verbatim rule holds per member ----

func TestTheVerbatimRuleIsPerMember(t *testing.T) {
	in := decodeFile(t, "oneofs.json")
	inItems := items(in)
	const name = "path variable segment"

	t.Run("editing only the URL rebuilds only the URL", func(t *testing.T) {
		tree := parseFile(t, "oneofs.json")
		idx := itemIndex(t, tree, name)
		tree.Items[idx].Request.URL = "https://api.example.com/moved"
		out := requestOf(t, items(exported(t, tree))[name])

		url, ok := out["url"].(map[string]any)
		if !ok {
			t.Fatalf("url is not an object: %#v", out["url"])
		}
		if url["raw"] != "https://api.example.com/moved" {
			t.Fatalf("url was not rebuilt: %#v", url)
		}
		// The old raw/host[]/path[]/query[] cannot linger and disagree with it.
		if _, stale := url["query"]; stale {
			t.Fatalf("a stale query survived the rebuild: %#v", url)
		}
		if !reflect.DeepEqual(out["header"], requestOf(t, inItems[name])["header"]) {
			t.Fatal("header changed when only the URL was edited")
		}
	})

	t.Run("editing only a header rebuilds only the header", func(t *testing.T) {
		tree := parseFile(t, "oneofs.json")
		idx := itemIndex(t, tree, name)
		tree.Items[idx].Request.Headers = []model.SavedHeader{{Name: "Accept", Value: "text/csv", Enabled: true}}
		out := requestOf(t, items(exported(t, tree))[name])

		if !reflect.DeepEqual(out["url"], requestOf(t, inItems[name])["url"]) {
			t.Fatal("url changed when only a header was edited")
		}
		header, ok := out["header"].([]any)
		if !ok || len(header) != 1 {
			t.Fatalf("header was not rebuilt: %#v", out["header"])
		}
		row := header[0].(map[string]any)
		if row["key"] != "Accept" || row["value"] != "text/csv" {
			t.Fatalf("rebuilt header row: %#v", row)
		}
	})
}

// ---- 6. Preserved but inert survives (D9) ----

func TestPreservedButInertSurvivesAFullCycle(t *testing.T) {
	tree := parseFile(t, "inert.json")
	in, out := decodeFile(t, "inert.json"), exported(t, tree)

	for _, member := range []string{"event", "protocolProfileBehavior"} {
		if !reflect.DeepEqual(out[member], in[member]) {
			t.Errorf("collection-level %q changed:\n got %#v\nwant %#v", member, out[member], in[member])
		}
	}
	// F8: unlike the members above, a collection-level `auth` block is not merely inert — it is a
	// plaintext credential, so it is stripped at import rather than surviving in origin. It must
	// never reappear in exported output.
	if out["auth"] != nil {
		t.Errorf("collection-level auth survived (F8): got %#v, want stripped", out["auth"])
	}
	// P5 D15: the collection level's own `variable` is promoted, not inert — it re-exports from
	// the rows (in order, names and values preserved) rather than surviving in origin untouched.
	if !reflect.DeepEqual(out["variable"], in["variable"]) {
		t.Errorf("promoted collection-level variable changed:\n got %#v\nwant %#v", out["variable"], in["variable"])
	}
	// info survives except the two members that are columns by construction.
	inInfo, outInfo := in["info"].(map[string]any), out["info"].(map[string]any)
	for _, member := range []string{"_postman_id", "description", "version"} {
		if !reflect.DeepEqual(outInfo[member], inInfo[member]) {
			t.Errorf("info.%s changed: got %#v, want %#v", member, outInfo[member], inInfo[member])
		}
	}
	if outInfo["schema"] != postman.SchemaURL {
		t.Errorf("info.schema = %v, want the v2.1 URL", outInfo["schema"])
	}

	inItems, outItems := items(in), items(out)
	for _, name := range []string{"Orders", "Create order"} {
		for _, member := range []string{"event", "protocolProfileBehavior", "response"} {
			if !reflect.DeepEqual(outItems[name][member], inItems[name][member]) {
				t.Errorf("%s.%s changed:\n got %#v\nwant %#v", name, member, outItems[name][member], inItems[name][member])
			}
		}
	}
	// F8: folder-level auth is stripped the same as collection-level auth (checked above).
	if outItems["Orders"]["auth"] != nil {
		t.Errorf("Orders.auth survived (F8): got %#v, want stripped", outItems["Orders"]["auth"])
	}
	// F8: a folder/item `variable[]` entry of type "secret" has its value blanked at import — its
	// plaintext must not reach origin_json — while a non-secret sibling entry stays untouched.
	outVars, ok := outItems["Orders"]["variable"].([]any)
	if !ok || len(outVars) != 2 {
		t.Fatalf("Orders.variable after export: %#v", outItems["Orders"]["variable"])
	}
	if folderVar, ok := outVars[0].(map[string]any); !ok ||
		!reflect.DeepEqual(folderVar, map[string]any{"key": "folderVar", "value": float64(7)}) {
		t.Errorf("Orders.variable[0] (folderVar) changed: got %#v", outVars[0])
	}
	if folderSecret, ok := outVars[1].(map[string]any); !ok ||
		folderSecret["key"] != "folderSecret" || folderSecret["type"] != "secret" || folderSecret["value"] != "" {
		t.Errorf("Orders.variable[1] (folderSecret) not blanked: got %#v", outVars[1])
	}
	for _, member := range []string{"description", "proxy", "certificate"} {
		got := requestOf(t, outItems["Create order"])[member]
		want := requestOf(t, inItems["Create order"])[member]
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Create order request.%s changed: got %#v, want %#v", member, got, want)
		}
	}
	// F8: a request-level auth block (nested under `request`, distinct from collection/folder-level
	// auth above) is stripped at import too — it never reaches origin_json or exported output.
	if got := requestOf(t, outItems["Create order"])["auth"]; got != nil {
		t.Errorf("Create order request.auth survived (F8): got %#v, want stripped", got)
	}

	if got := tree.Report.Warnings[postman.WarnScriptsInert]; got != 4 {
		t.Errorf("scripts_inert = %d, want 4", got)
	}
	if got := tree.Report.Warnings[postman.WarnAuthInert]; got != 2 {
		t.Errorf("auth_inert = %d, want 2", got)
	}
	// P5 D15/F6: the collection level's 2 variables are promoted (counted below), not inert —
	// only the "Orders" folder's own variables stay inert: folderVar plus the F8 fixture's
	// folderSecret (blanked above, but still counted as an inert folder variable).
	if got := tree.Report.Warnings[postman.WarnVariablesInert]; got != 2 {
		t.Errorf("variables_inert = %d, want 2", got)
	}
	if got := tree.Report.Warnings[postman.WarnVariablesImported]; got != 2 {
		t.Errorf("variables_imported = %d, want 2", got)
	}
}

// ---- 7. The version gate (D10) ----

func TestTheVersionGate(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "v2_0.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := postman.Parse(f); err == nil {
		t.Fatal("a v2.0.0 file was accepted")
	} else if !strings.Contains(err.Error(), "v2.0.0") {
		t.Fatalf("the refusal does not name the version found: %v", err)
	}

	// Hand-written and SDK-generated collections routinely omit the advisory URL; refusing a file
	// for that is the validator mistake D1 declined.
	tree := parseFile(t, "no_schema.json")
	if len(tree.Items) != 1 {
		t.Fatalf("a schema-less collection did not parse: %+v", tree.Items)
	}
}

// ---- 8. The malformed cases (D3) ----

func TestMalformedItemsImportWithDefaults(t *testing.T) {
	tree := parseFile(t, "malformed.json")

	names := []string{}
	for _, item := range tree.Items {
		names = append(names, item.Name)
	}
	want := []string{"neither a folder nor a request", "both members", "child", "nameless below", "/v1/derived/name", "/root-string", "custom method"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("items: got %v, want %v", names, want)
	}
	if got := tree.Report.Warnings[postman.WarnMalformedItem]; got != 1 {
		t.Errorf("malformed_item = %d, want 1", got)
	}
	if got := tree.Report.Warnings[postman.WarnMissingRequest]; got != 1 {
		t.Errorf("missing_request = %d, want 1", got)
	}
	if got := tree.Report.Warnings[postman.WarnUnsupportedMethod]; got != 1 {
		t.Errorf("unsupported_method = %d, want 1", got)
	}
	bare := tree.Items[itemIndex(t, tree, "neither a folder nor a request")]
	if bare.Kind != postman.KindRequest || bare.Request.Method != "GET" || bare.Request.URL != "" {
		t.Errorf("request-less item = %v %q %q, want an empty GET request", bare.Kind, bare.Request.Method, bare.Request.URL)
	}

	// Both members present: `item` wins, and the `request` member survives untouched in origin,
	// so the export re-emits exactly what came in.
	both := tree.Items[itemIndex(t, tree, "both members")]
	if both.Kind != postman.KindFolder {
		t.Fatalf("an item with both members is not a folder: %v", both.Kind)
	}
	outItems := items(exported(t, tree))
	inItems := items(decodeFile(t, "malformed.json"))
	if !reflect.DeepEqual(outItems["both members"]["request"], inItems["both members"]["request"]) {
		t.Fatal("the ill-formed item's own request member was not preserved")
	}

	// A custom method is stored verbatim; export restores it.
	if got := tree.Items[itemIndex(t, tree, "custom method")].Request.Method; got != "PROPFIND" {
		t.Fatalf("custom method = %q", got)
	}
	if got := requestOf(t, outItems["custom method"])["method"]; got != "PROPFIND" {
		t.Fatalf("exported custom method = %v", got)
	}
}

// ---- An export re-imports to the same tree ----
//
// The bar — "import that file into Postman" — needs real Postman. This is the
// half that can be checked here: whatever Write emits, Parse reads back as the same tree, so an
// export can never be a file this app itself would refuse or read differently. It runs over every
// fixture in the corpus rather than one, since each isolates a different shape.

func TestAnExportReImportsToTheSameTree(t *testing.T) {
	for _, name := range []string{"nesting.json", "oneofs.json", "bodies.json", "inert.json", "malformed.json"} {
		t.Run(name, func(t *testing.T) {
			first := parseFile(t, name)

			var written bytes.Buffer
			if err := postman.Write(&written, first); err != nil {
				t.Fatalf("Write: %v", err)
			}
			second, err := postman.Parse(bytes.NewReader(written.Bytes()))
			if err != nil {
				t.Fatalf("re-parsing our own export failed: %v", err)
			}

			if second.Name != first.Name {
				t.Errorf("name: got %q, want %q", second.Name, first.Name)
			}
			// malformed.json's request-less item is kept, exported with a request and re-imported
			// as the same request; its non-object entry is skipped both times.
			if len(second.Items) != len(first.Items) {
				t.Fatalf("item count: got %d, want %d", len(second.Items), len(first.Items))
			}
			for i := range first.Items {
				a, b := first.Items[i], second.Items[i]
				if a.Name != b.Name || a.Kind != b.Kind || a.Order != b.Order || a.Parent != b.Parent {
					t.Errorf("item %d: got {%s %s %d %d}, want {%s %s %d %d}",
						i, b.Name, b.Kind, b.Order, b.Parent, a.Name, a.Kind, a.Order, a.Parent)
				}
				if a.Kind == postman.KindRequest && !reflect.DeepEqual(a.Request, b.Request) {
					t.Errorf("item %d (%s): request changed\n got %+v\nwant %+v", i, a.Name, b.Request, a.Request)
				}
			}

			// And the exported file is v2.1 by construction, whatever the input said (D10).
			var doc map[string]any
			if err := json.Unmarshal(written.Bytes(), &doc); err != nil {
				t.Fatal(err)
			}
			if got := doc["info"].(map[string]any)["schema"]; got != postman.SchemaURL {
				t.Errorf("info.schema = %v", got)
			}
		})
	}
}

// ---- 10. url.go's splitter/builder ----

func TestURLSplitAndBuild(t *testing.T) {
	const raw = "{{baseUrl}}/users/:id?q=a+b#frag"

	split := postman.Split(raw)
	if split.Base != "{{baseUrl}}/users/:id" || split.Query != "q=a+b" || split.Hash != "frag" {
		t.Fatalf("Split: %+v", split)
	}

	built := postman.Build(raw)
	encoded, err := json.Marshal(built)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"raw": raw,
		// A {{variable}} in the host is one segment, not three — P5's own syntax survives.
		"host":  []any{"{{baseUrl}}"},
		"path":  []any{"users", ":id"},
		"query": []any{map[string]any{"key": "q", "value": "a+b"}},
		"hash":  "frag",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build:\n got %#v\nwant %#v", got, want)
	}

	// The whole point of writing `raw`: Import(Build(s)) == s by construction, for any s.
	for _, s := range []string{
		raw,
		"https://api.example.com:8443/v2/orders?a=1&b=2#top",
		"api.exa",
		"",
		"https://api.example.com",
	} {
		if got := postman.ImportURL(json.RawMessage(mustJSON(t, postman.Build(s)))); got != s {
			t.Errorf("ImportURL(Build(%q)) = %q", s, got)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
