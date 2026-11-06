package postman

import (
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// The rewrite earns a test where the rest of this batch does not — it edits user data at
// import across five field families, and the rules interact (whole-name match, an unmapped alias
// left alone, an unterminated reference, several per string). The alias table itself is guarded
// against drifting from its TypeScript twin by go-ts-api-parity.spec.ts, not from here.
func TestRewriteAliases(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no reference at all", "https://api.example.com/users", "https://api.example.com/users"},
		{"a plain variable is untouched", "{{base_url}}/users", "{{base_url}}/users"},
		{"a mapped alias is rewritten", "{{$guid}}", "{{fake.string.uuid}}"},
		{
			"two aliases in one string, both rewritten, surrounding text preserved",
			"/orders/{{$guid}}?t={{$timestamp}}&x=1",
			"/orders/{{fake.string.uuid}}?t={{fake.date.timestamp}}&x=1",
		},
		{
			"mapped and plain references interleave",
			"{{base_url}}/{{$randomEmail}}/{{tenant}}",
			"{{base_url}}/{{fake.internet.email}}/{{tenant}}",
		},
		// catalog.ts excludes these deliberately (no single faker call produces them), so they have
		// no entry here and must survive verbatim — resolution still accepts the $ spelling.
		{"an unmapped alias is left verbatim", "{{$randomStreetName}}", "{{$randomStreetName}}"},
		// Whole-name match: a longer name that merely starts with a mapped one is a different name.
		{"a longer name sharing a prefix is not rewritten", "{{$guidly}}", "{{$guidly}}"},
		// Postman does not permit inner spaces, so this is not a reference to rewrite.
		{"inner spaces are not a reference", "{{ $guid }}", "{{ $guid }}"},
		{"an unterminated reference does not consume the tail", "a{{$guid", "a{{$guid"},
		{
			"an unterminated reference after a rewritten one keeps the earlier rewrite",
			"{{$guid}} then {{$time",
			"{{fake.string.uuid}} then {{$time",
		},
		{"case is significant, as it is in Postman", "{{$GUID}}", "{{$GUID}}"},
		{"an empty dynamic reference is left alone", "{{$}}", "{{$}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteAliases(tc.in); got != tc.want {
				t.Fatalf("rewriteAliases(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteRequestAliasesCoversEveryFieldThatCanCarryAReference(t *testing.T) {
	req := model.SavedRequest{
		Method: "POST",
		URL:    "https://api.example.com/o/{{$guid}}",
		Headers: []model.SavedHeader{
			{Name: "X-{{$guid}}-Trace", Value: "{{$randomEmail}}", Description: "{{$guid}} in prose"},
		},
		Body:       `{"id":"{{$guid}}"}`,
		Code:       `console.log("{{$randomInt}}")`,
		URLEncoded: []model.SavedField{{Name: "u{{$guid}}", Value: "{{$randomUUID}}"}},
		FormData:   []model.SavedFormField{{Name: "f{{$guid}}", Value: "{{$randomBoolean}}"}},
	}
	rewriteRequestAliases(&req)

	if req.URL != "https://api.example.com/o/{{fake.string.uuid}}" {
		t.Errorf("URL = %q", req.URL)
	}
	if req.Headers[0].Name != "X-{{fake.string.uuid}}-Trace" {
		t.Errorf("header name = %q", req.Headers[0].Name)
	}
	if req.Headers[0].Value != "{{fake.internet.email}}" {
		t.Errorf("header value = %q", req.Headers[0].Value)
	}
	// A description is prose about the request, never part of what gets sent — deliberately not
	// rewritten, so this asserts the alias is still there.
	if req.Headers[0].Description != "{{$guid}} in prose" {
		t.Errorf("header description was rewritten: %q", req.Headers[0].Description)
	}
	if req.Body != `{"id":"{{fake.string.uuid}}"}` {
		t.Errorf("body = %q", req.Body)
	}
	if !strings.Contains(req.Code, "{{fake.number.int}}") {
		t.Errorf("code = %q", req.Code)
	}
	if req.URLEncoded[0].Name != "u{{fake.string.uuid}}" ||
		req.URLEncoded[0].Value != "{{fake.string.uuid}}" {
		t.Errorf("urlencoded = %+v", req.URLEncoded[0])
	}
	if req.FormData[0].Name != "f{{fake.string.uuid}}" ||
		req.FormData[0].Value != "{{fake.datatype.boolean}}" {
		t.Errorf("formdata = %+v", req.FormData[0])
	}
	// Method is a verb, never a reference site.
	if req.Method != "POST" {
		t.Errorf("method = %q", req.Method)
	}
}
