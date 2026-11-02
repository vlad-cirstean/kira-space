package opencode

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/internal/harness"
)

func TestParseSessionList(t *testing.T) {
	dir := t.TempDir()
	since := time.UnixMilli(1000)
	q := harness.SessionQuery{Cwd: dir, Since: since}
	list := func(items ...map[string]any) []byte {
		b, _ := json.Marshal(items)
		return b
	}
	cases := []struct {
		name   string
		out    []byte
		want   string
		wantOK bool
		skip   []string
	}{
		{"one candidate in the folder", list(
			map[string]any{"id": "ses_old", "directory": dir, "created": 900},
			map[string]any{"id": "ses_new", "directory": dir, "created": 2500},
			map[string]any{"id": "ses_other", "directory": "/elsewhere", "created": 9000},
		), "ses_new", true, nil},
		{"two candidates are ambiguous", list(
			map[string]any{"id": "ses_a", "directory": dir, "created": 1500},
			map[string]any{"id": "ses_b", "directory": dir, "created": 2500},
		), "", true, nil},
		{"excluded id leaves one", list(
			map[string]any{"id": "ses_a", "directory": dir, "created": 1500},
			map[string]any{"id": "ses_b", "directory": dir, "created": 2500},
		), "ses_b", true, []string{"ses_a"}},
		{"older than the launch is skipped", list(map[string]any{"id": "ses_a", "directory": dir, "created": 900}), "", true, nil},
		{"rfc3339 created", list(map[string]any{"id": "ses_a", "directory": dir, "created": "1970-01-01T00:00:03Z"}), "ses_a", true, nil},
		{"foreign id prefix and unsafe id", list(
			map[string]any{"id": "x_a", "directory": dir, "created": 2000},
			map[string]any{"id": "ses_a b", "directory": dir, "created": 2000},
		), "", true, nil},
		{"not an array", []byte(`{"error":"x"}`), "", false, nil},
	}
	for _, c := range cases {
		qc := q
		qc.Exclude = c.skip
		got, ok := parseSessionList(c.out, qc)
		if got != c.want || ok != c.wantOK {
			t.Errorf("%s: got %q, %v want %q, %v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestParseEvent(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		ok      bool
		session string
		err     string
		text    string
	}{
		{"error data message", `{"type":"error","sessionID":"ses_a","error":{"name":"X","data":{"message":"boom"}}}`, true, "ses_a", "boom", ""},
		{"error name only", `{"type":"error","error":{"name":"NotFoundError"}}`, true, "", "NotFoundError", ""},
		{"text under part", `{"type":"text","sessionID":"ses_a","part":{"text":"hi\nthere"}}`, true, "ses_a", "", "hi\nthere"},
		{"session id under part", `{"type":"step_start","part":{"sessionID":"ses_b"}}`, true, "ses_b", "", ""},
		{"no type", `{"sessionID":"ses_a"}`, false, "", "", ""},
		{"not json", `plain text`, false, "", "", ""},
	}
	for _, c := range cases {
		ev, ok := parseEvent(c.line)
		if ok != c.ok || ev.SessionID != c.session || ev.Err != c.err || ev.Text != c.text {
			t.Errorf("%s: got %+v, %v", c.name, ev, ok)
		}
	}
}

func TestParseMCPList(t *testing.T) {
	out := "\x1b[0m┌  MCP Servers\n│\n•  \x1b[32m✓\x1b[0m docs connected\n|      https://mcp.example.com/sse\n│\n" +
		"•  ✗ local failed\n|      /usr/bin/node server.js\n│\n•  ⚠ wiki needs authentication\n|      https://wiki.example.com/mcp\n└  3 server(s)\n"
	got, ok := parseMCPList(out)
	want := []harness.UserServer{
		{Name: "docs", Transport: "http", Target: "mcp.example.com", Status: "connected"},
		{Name: "local", Transport: "stdio", Target: "node", Status: "failed"},
		{Name: "wiki", Transport: "http", Target: "wiki.example.com", Status: "needsAuth"},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v want %+v", got, ok, want)
	}
	if _, ok := parseMCPList("garbled"); ok {
		t.Error("garbled output parsed as a list")
	}
}

func TestPermissionDoc(t *testing.T) {
	enc := func(spec harness.HeadlessSpec) string {
		doc, err := permissionDoc(spec, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(doc)
		return string(b)
	}
	// Deny wins over allow for the same pattern; Bash patterns keep order, with `prefix:*` as a glob.
	got := enc(harness.HeadlessSpec{
		AllowedTools:    []string{"Read", "Bash(git diff:*)", "Bash(git *)"},
		DisallowedTools: []string{"Bash(git *)"},
	})
	want := `{"permission":{"bash":{"git diff*":"allow","git *":"deny"},"doom_loop":"deny","external_directory":"deny","read":"allow"}}`
	if got != want {
		t.Errorf("deny over allow:\n got %s\nwant %s", got, want)
	}
	// Isolated turns every other built-in off and a pattern-only allow list gets a deny-all first.
	got = enc(harness.HeadlessSpec{Isolated: true, AllowedTools: []string{"Bash(go test:*)"}})
	if !reflect.DeepEqual(permissionKeys(t, got, "bash"), []string{"*", "go test*"}) {
		t.Errorf("isolated bash rules = %s", got)
	}
	// An MCP server wildcard and a single tool translate to OpenCode keys.
	got = enc(harness.HeadlessSpec{AllowedTools: []string{"mcp__kira__finish_step", "mcp__docs"}})
	if want := `"docs_*":"allow"`; !contains(got, want) || !contains(got, `"kira_finish_step":"allow"`) {
		t.Errorf("mcp keys = %s", got)
	}
	// Unknown names and unsupported arguments fail closed, naming the entry.
	for _, bad := range []string{"NotATool", "Read(x)", "Bash()"} {
		if _, err := permissionDoc(harness.HeadlessSpec{AllowedTools: []string{bad}}, nil); err == nil || !contains(err.Error(), bad) {
			t.Errorf("entry %q: err = %v, want one naming it", bad, err)
		}
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// permissionKeys is the pattern order of one permission key.
func permissionKeys(t *testing.T, doc, key string) []string {
	t.Helper()
	var d struct {
		Permission map[string]json.RawMessage `json:"permission"`
	}
	if err := json.Unmarshal([]byte(doc), &d); err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(d.Permission[key]))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var v string
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}
