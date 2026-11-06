package httpflow

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
)

// Same scenario as tests/ui/http-curl.spec.ts "an imported curl sends as written, and copy-as-curl
// yields the contract command": both halves read tests/contract/api-curl.json. The command is
// generated in the renderer, so this half owns the expected string and proves it replays as the
// request the app sends; the UI half proves the generator still produces it.
func TestCurlReplayMatchesSend(t *testing.T) {
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl binary not installed")
	}
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
	const sc = "api-curl"
	const body = `{"sku":"A-1","qty":3}`
	opts := studioflowharness.Replace(srv.URL, "<http>")

	imported := fmt.Sprintf(`curl -X POST '%s/echo?src=curl' -H 'Content-Type: application/json' -H 'X-Flow-Probe: p249' -d '%s'`, srv.URL, body)
	app.Contract(t, sc, "curl:import", imported, opts)

	args := get(srv.URL + "/echo?src=curl")
	args.Method = "POST"
	args.Headers = []httpclient.Header{{Name: "Content-Type", Value: "application/json"}, {Name: "X-Flow-Probe", Value: "p249"}}
	args.Body = httpclient.Body{Mode: "code", Code: body, CodeLanguage: "json", URLEncoded: []httpclient.Field{}, FormData: []httpclient.FormField{}}
	app.Contract(t, sc, "args:HttpService.Send", args, studioflowharness.Mask("opId", "tabId"), opts)
	res := send(t, app, args)
	app.Contract(t, sc, "HttpService.Send", res, opts, studioflowharness.Mask("elapsedMs", "bodyBytes"), studioflowharness.Omit("body", "headers", "timeline", "wire"))

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
