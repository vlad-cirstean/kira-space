// parse_limit_test.go is P21 round 1 architecture/security finding 11: Parse read a user-chosen
// file whole into memory with no bound. This pins that an implausibly large collection is refused
// with a named error rather than read in full.
package postman_test

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
)

func TestParse_RefusesAnImplausiblyLargeFile(t *testing.T) {
	// One real "info" object followed by padding well past the 64 MiB bound, inside a JSON string
	// so it stays a single well-formed (if oversized) document — Parse must refuse before decoding
	// it, not fail on malformed JSON for an unrelated reason.
	const oneMiB = 1 << 20
	padding := strings.Repeat("x", 65*oneMiB)
	var buf bytes.Buffer
	buf.WriteString(`{"info":{"name":"big","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[],"padding":"`)
	buf.WriteString(padding)
	buf.WriteString(`"}`)

	_, err := postman.Parse(&buf)
	if err == nil {
		t.Fatal("Parse: expected a refusal for an implausibly large file, got nil")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("Parse err = %v, want a named size-refusal", err)
	}
}

func TestParse_AcceptsAnOrdinarilySizedFile(t *testing.T) {
	const small = `{"info":{"name":"small","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},"item":[]}`
	if _, err := postman.Parse(strings.NewReader(small)); err != nil {
		t.Fatalf("Parse: unexpected error for an ordinary file: %v", err)
	}
}

func nestedCollection(depth int, payload string) string {
	var b strings.Builder
	b.WriteString(`{"info":{"name":"deep"},"item":[`)
	for range depth {
		b.WriteString(`{"name":"f","item":[`)
	}
	b.WriteString(`{"name":"leaf","request":{"method":"POST","url":"https://x.test","body":{"mode":"raw","raw":"`)
	b.WriteString(payload)
	b.WriteString(`"}}}`)
	for range depth {
		b.WriteString(`]}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

// P168 Part 7 F12: nesting past the cap is refused with a named error; nesting at the cap decodes
// in one pass, so allocation stays near the payload size instead of payload x depth.
func TestParse_FolderDepthCapAndSinglePassDecode(t *testing.T) {
	_, err := postman.Parse(strings.NewReader(nestedCollection(65, "x")))
	if err == nil || !strings.Contains(err.Error(), "nested deeper than 64") {
		t.Fatalf("Parse err = %v, want a depth refusal", err)
	}

	payload := strings.Repeat("x", 2<<20)
	doc := nestedCollection(63, payload)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	tree, err := postman.Parse(strings.NewReader(doc))
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatalf("Parse at the cap: %v", err)
	}
	if tree.Report.Folders != 63 || tree.Report.Requests != 1 {
		t.Fatalf("folders/requests = %d/%d, want 63/1", tree.Report.Folders, tree.Report.Requests)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
		t.Fatalf("Parse allocated %d MiB for a 2 MiB file, want a few copies (single pass)", alloc>>20)
	}
}

// P168 Part 7 F13: a saved example's originalRequest repeats the request's auth block; it must not
// reach origin like request.auth does not.
func TestParse_StripsAuthFromSavedExamples(t *testing.T) {
	const doc = `{"info":{"name":"c"},"item":[{"name":"r","request":{"method":"GET","url":"https://x.test",
"auth":{"type":"bearer","bearer":[{"key":"token","value":"REQSECRET"}]}},
"response":[{"name":"ok","code":200,"originalRequest":{"method":"GET","url":"https://x.test",
"auth":{"type":"bearer","bearer":[{"key":"token","value":"RESPSECRET"}]}}}]}]}`
	tree, err := postman.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	origin, _ := json.Marshal(tree.Items[0].Origin)
	if s := string(origin); strings.Contains(s, "REQSECRET") || strings.Contains(s, "RESPSECRET") {
		t.Fatalf("origin kept a credential: %s", s)
	}
	if !strings.Contains(string(origin), `"name":"ok"`) {
		t.Fatalf("origin lost the saved example: %s", origin)
	}
}
