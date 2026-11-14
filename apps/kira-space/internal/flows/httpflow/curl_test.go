package httpflow

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os/exec"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
)

// Same scenario as tests/ui/http-curl.spec.ts "an imported curl sends as written, and copy-as-curl
// yields the contract command": both halves read tests/contract/api-curl.json. The command is
// generated in the renderer, so this half owns the expected string and proves it replays as the
// request the app sends; the UI half proves the generator still produces it.
func TestCurlReplayMatchesSend(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl binary not installed")
	}
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	const sc = "api-curl"
	const body = `{"sku":"A-1","qty":3}`
	opts := flowharness.Replace(srv.URL, "<http>")

	imported := fmt.Sprintf(`curl -X POST '%s/echo?src=curl' -H 'Content-Type: application/json' -H 'X-Flow-Probe: p249' -d '%s'`, srv.URL, body)
	app.Contract(t, sc, "curl:import", imported, opts)

	args := get(srv.URL + "/echo?src=curl")
	args.Method = "POST"
	args.Headers = []httpclient.Header{{Name: "Content-Type", Value: "application/json"}, {Name: "X-Flow-Probe", Value: "p249"}}
	args.Body = httpclient.Body{Mode: "code", Code: body, CodeLanguage: "json", URLEncoded: []httpclient.Field{}, FormData: []httpclient.FormField{}}
	app.Contract(t, sc, "args:HttpService.Send", args, flowharness.Mask("opId", "tabId"), opts)
	res := send(t, app, args)
	app.Contract(t, sc, "HttpService.Send", res, opts, flowharness.Mask("elapsedMs", "bodyBytes"), flowharness.Omit("body", "headers", "timeline", "wire"))

	generated := fmt.Sprintf("curl -L '%s/echo?src=curl' \\\n  -H 'Content-Type: application/json' \\\n  -H 'X-Flow-Probe: p249' \\\n  --data-raw '%s'", srv.URL, body)
	app.Contract(t, sc, "curl:generated", generated, opts)
	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", generated)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("curl replay: %v\n%s", err, out.String())
	}

	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("server saw %d requests, want the app's send and the replay", len(reqs))
	}
	a, b := reqs[0], reqs[1]
	if a.Method != b.Method || a.Query != b.Query || a.BodySHA256 != b.BodySHA256 {
		t.Errorf("replay differs: app %s ?%s %s, curl %s ?%s %s", a.Method, a.Query, a.BodySHA256, b.Method, b.Query, b.BodySHA256)
	}
	for _, h := range []string{"Content-Type", "X-Flow-Probe"} {
		if a.Header.Get(h) != b.Header.Get(h) {
			t.Errorf("header %s: app %q, curl %q", h, a.Header.Get(h), b.Header.Get(h))
		}
	}
}

// J-HTTP-065: copy-as-curl for every body mode. The generator runs in the renderer, so this half owns
// each expected command (contract api-curl, key curl:generated:<mode>; tests/ui/studio/http-curl.spec.ts
// proves the generator still yields it) and proves real curl replays it as the request the app sends.
func TestCurlCopyEveryBodyMode(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl binary not installed")
	}
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	csv := writeFile(t, "report.csv", []byte("id,qty\n1,3\n"))
	bin := writeFile(t, "blob.bin", []byte{0x00, 0x01, 0xfe, 0xff, 'k', 'i', 'r', 'a'})
	opts := []flowharness.ContractOption{flowharness.Replace(srv.URL, "<http>"), flowharness.Replace(csv, "<csv-file>"), flowharness.Replace(bin, "<blob-file>")}

	hdr := []httpclient.Header{{Name: "X-Mode", Value: ""}}
	cases := []struct {
		mode      string
		method    string
		body      httpclient.Body
		generated string
	}{
		{"none", "GET", httpclient.Body{Mode: "none"},
			"curl -L '%[1]s/echo?m=none' \\\n  -H 'X-Mode: none'"},
		{"raw", "POST", httpclient.Body{Mode: "raw", Raw: "it's a b&c=\"d\"\nline 2"},
			"curl -L '%[1]s/echo?m=raw' \\\n  -H 'X-Mode: raw' \\\n  -H 'Content-Type: text/plain' \\\n  --data-raw 'it'\"'\"'s a b&c=\"d\"\nline 2'"},
		{"code", "PUT", httpclient.Body{Mode: "code", Code: `{"a":[1,2],"b":"x y"}`, CodeLanguage: "json"},
			"curl -L -X PUT '%[1]s/echo?m=code' \\\n  -H 'X-Mode: code' \\\n  -H 'Content-Type: application/json' \\\n  --data-raw '{\"a\":[1,2],\"b\":\"x y\"}'"},
		{"urlencoded", "POST", httpclient.Body{Mode: "urlencoded", URLEncoded: []httpclient.Field{{Name: "a b", Value: "c&d"}, {Name: "k", Value: "é"}}},
			"curl -L '%[1]s/echo?m=urlencoded' \\\n  -H 'X-Mode: urlencoded' \\\n  -H 'Content-Type: application/x-www-form-urlencoded' \\\n  --data-raw 'a+b=c%%26d&k=%%C3%%A9'"},
		{"formdata", "POST", httpclient.Body{Mode: "formdata", FormData: []httpclient.FormField{
			{Name: "note", Kind: "text", Value: "@hello"},
			{Name: "meta", Kind: "text", Value: "{}", ContentType: "application/json"},
			{Name: "report", Kind: "file", Path: csv, ContentType: "text/csv"},
		}},
			"curl -L '%[1]s/echo?m=formdata' \\\n  -H 'X-Mode: formdata' \\\n  --form-string note=@hello \\\n  -F 'meta={};type=application/json' \\\n  -F 'report=@%[2]s;type=text/csv'"},
		{"file", "POST", httpclient.Body{Mode: "file", File: bin},
			"curl -L '%[1]s/echo?m=file' \\\n  -H 'X-Mode: file' \\\n  -H Content-Type: \\\n  --data-binary @%[3]s"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			generated := fmt.Sprintf(tc.generated, srv.URL, csv, bin)
			app.Contract(t, "api-curl", "curl:generated:"+tc.mode, generated, opts...)

			before := len(srv.Requests())
			args := get(srv.URL + "/echo?m=" + tc.mode)
			args.Method, args.Body = tc.method, tc.body
			args.Headers = []httpclient.Header{{Name: hdr[0].Name, Value: tc.mode}}
			send(t, app, args)

			var out bytes.Buffer
			cmd := exec.Command("sh", "-c", generated)
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Run(); err != nil {
				t.Fatalf("curl replay: %v\n%s", err, out.String())
			}
			reqs := srv.Requests()[before:]
			if len(reqs) != 2 {
				t.Fatalf("server saw %d requests, want the app's send and the replay", len(reqs))
			}
			a, b := reqs[0], reqs[1]
			if a.Method != b.Method || a.Query != b.Query || a.Path != b.Path {
				t.Errorf("replay differs: app %s %s?%s, curl %s %s?%s", a.Method, a.Path, a.Query, b.Method, b.Path, b.Query)
			}
			if a.Header.Get("X-Mode") != b.Header.Get("X-Mode") {
				t.Errorf("X-Mode: app %q, curl %q", a.Header.Get("X-Mode"), b.Header.Get("X-Mode"))
			}
			if tc.mode == "formdata" {
				if !samePartsOf(t, a, b) {
					t.Error("multipart parts differ between the app's send and the curl replay")
				}
				return
			}
			if a.Header.Get("Content-Type") != b.Header.Get("Content-Type") {
				t.Errorf("Content-Type: app %q, curl %q", a.Header.Get("Content-Type"), b.Header.Get("Content-Type"))
			}
			if a.BodySHA256 != b.BodySHA256 {
				t.Errorf("body sha: app %s, curl %s", a.BodySHA256, b.BodySHA256)
			}
		})
	}
}

// samePartsOf compares two multipart requests part by part: name, filename, part type and bytes.
// Boundaries differ per request, so the raw bodies never match.
func samePartsOf(t *testing.T, a, b flowharness.Request) bool {
	t.Helper()
	parts := func(r flowharness.Request) []string {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("Content-Type %q: %v", r.Header.Get("Content-Type"), err)
		}
		mr := multipart.NewReader(bytes.NewReader(r.Body), params["boundary"])
		var out []string
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				return out
			}
			if err != nil {
				t.Fatalf("multipart: %v", err)
			}
			data, _ := io.ReadAll(p)
			out = append(out, fmt.Sprintf("%s|%s|%s|%x", p.FormName(), p.FileName(), p.Header.Get("Content-Type"), data))
		}
	}
	pa, pb := parts(a), parts(b)
	if len(pa) != 3 {
		t.Fatalf("app sent %d parts, want 3: %v", len(pa), pa)
	}
	return strings.Join(pa, "\n") == strings.Join(pb, "\n")
}
