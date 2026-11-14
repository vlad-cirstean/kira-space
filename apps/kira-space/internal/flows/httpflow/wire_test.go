package httpflow

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

func secretVar(t *testing.T, app *flowharness.App, name, value string) (collectionID, envID string) {
	t.Helper()
	col := app.SeedCollection(t, "c-"+name)
	env := app.SeedEnvironment(t, "e-"+name, flowharness.APIVar{Name: name, Value: value, Secret: true})
	return col.ID, env.ID
}

func TestAuthHeaders(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)

	if e := get(srv.URL + "/basic"); send(t, app, e).Status != 401 {
		t.Error("/basic without credentials did not answer 401")
	}

	colID, envID := secretVar(t, app, "creds", "kira:secret")
	basic := get(srv.URL + "/basic")
	basic.CollectionID, basic.EnvironmentID = colID, envID
	basic.Headers = []httpclient.Header{{Name: "Authorization", Value: "Basic {{creds | base64}}"}}
	res := send(t, app, basic)
	if res.Status != 200 {
		t.Fatalf("/basic with transformed secret = %d, want 200", res.Status)
	}
	reqs := srv.Requests()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("kira:secret"))
	if got := reqs[len(reqs)-1].Header.Get("Authorization"); got != want {
		t.Errorf("server Authorization = %q, want %q", got, want)
	}
	mustNotContain(t, "wire request", res.Wire.Request, "a2lyYTpzZWNyZXQ=")
	if !strings.Contains(res.Wire.Request, "{{creds | base64}}") {
		t.Errorf("wire request lost the piped placeholder:\n%s", res.Wire.Request)
	}

	colID2, envID2 := secretVar(t, app, "bearer", "b-4e1d8c")
	bearer := get(srv.URL + "/bearer?expect=b-4e1d8c")
	bearer.CollectionID, bearer.EnvironmentID = colID2, envID2
	bearer.Headers = []httpclient.Header{{Name: "Authorization", Value: "Bearer {{bearer}}"}}
	if got := send(t, app, bearer); got.Status != 200 {
		t.Errorf("/bearer with secret = %d, want 200", got.Status)
	}
}

func TestBodies(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	last := func() flowharnessRequest {
		reqs := srv.Requests()
		return reqs[len(reqs)-1]
	}

	t.Run("json", func(t *testing.T) {
		args := get(srv.URL + "/echo")
		args.Method = "POST"
		args.Body = httpclient.Body{Mode: "code", Code: `{"a":1}`, CodeLanguage: "json"}
		send(t, app, args)
		r := last()
		if r.Header.Get("Content-Type") != "application/json" || string(r.Body) != `{"a":1}` {
			t.Errorf("server saw %q %q", r.Header.Get("Content-Type"), r.Body)
		}
	})

	t.Run("urlencoded reserved characters", func(t *testing.T) {
		args := get(srv.URL + "/echo")
		args.Method = "POST"
		args.Body = httpclient.Body{Mode: "urlencoded", URLEncoded: []httpclient.Field{
			{Name: "q", Value: "a b&c=d/é"}, {Name: "x y", Value: "+%"},
		}}
		send(t, app, args)
		r := last()
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		form, err := url.ParseQuery(string(r.Body))
		if err != nil || form.Get("q") != "a b&c=d/é" || form.Get("x y") != "+%" {
			t.Errorf("server form = %v (%v) from %q", form, err, r.Body)
		}
	})

	file := make([]byte, 64*1024)
	for i := range file {
		file[i] = byte(i % 251)
	}
	path := writeFile(t, "blob.bin", file)
	sum := sha256.Sum256(file)
	fileHash := hex.EncodeToString(sum[:])

	t.Run("multipart with file", func(t *testing.T) {
		args := get(srv.URL + "/echo")
		args.Method = "POST"
		args.Body = httpclient.Body{Mode: "formdata", FormData: []httpclient.FormField{
			{Name: "note", Kind: "text", Value: "hello"},
			{Name: "upload", Kind: "file", Path: path, ContentType: "application/octet-stream"},
		}}
		send(t, app, args)
		r := last()
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "multipart/form-data" {
			t.Fatalf("content type = %q (%v)", r.Header.Get("Content-Type"), err)
		}
		mr := multipart.NewReader(bytes.NewReader(r.Body), params["boundary"])
		got := map[string]string{}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(p)
			h := sha256.Sum256(data)
			if p.FormName() == "upload" {
				got["upload"] = hex.EncodeToString(h[:])
			} else {
				got[p.FormName()] = string(data)
			}
		}
		if got["note"] != "hello" || got["upload"] != fileHash {
			t.Errorf("parts = %v, want note=hello and upload=%s", got, fileHash)
		}
	})

	t.Run("binary file", func(t *testing.T) {
		args := get(srv.URL + "/echo")
		args.Method = "PUT"
		args.Body = httpclient.Body{Mode: "file", File: path}
		send(t, app, args)
		if r := last(); r.BodySHA256 != fileHash || r.BodyLen != len(file) {
			t.Errorf("server body = %d bytes sha %s, want %d bytes sha %s", r.BodyLen, r.BodySHA256, len(file), fileHash)
		}
	})

	t.Run("gzip response decoded", func(t *testing.T) {
		res := send(t, app, get(srv.URL+"/gzip"))
		if strings.TrimSpace(res.Body) != `{"hello":"gzip"}` {
			t.Errorf("body = %q, want the decoded JSON", res.Body)
		}
	})

	t.Run("response size cap", func(t *testing.T) {
		args := get(srv.URL + "/bytes?n=2097152")
		args.Options.MaxResponseMb = ptr(1)
		res := send(t, app, args)
		if !res.BodyTruncated || res.BodyBytes != 1<<20 {
			t.Errorf("truncated = %v, bytes = %d; want true and %d", res.BodyTruncated, res.BodyBytes, 1<<20)
		}
		full := send(t, app, get(srv.URL+"/bytes?n=1048576"))
		if full.BodyTruncated {
			t.Error("a body under the default cap was truncated")
		}
	})

	t.Run("200 MB body under a raised cap", func(t *testing.T) {
		flowharness.Complete(t)
		args := get(srv.URL + "/bytes?n=209715200")
		args.Options.MaxResponseMb = ptr(300)
		res := send(t, app, args)
		if res.BodyBytes != 200<<20 || res.BodyTruncated {
			t.Errorf("bytes = %d truncated = %v, want %d", res.BodyBytes, res.BodyTruncated, 200<<20)
		}
	})
}

func TestTimeoutAndCancel(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)

	to := get(srv.URL + "/slow?ms=10000")
	to.Options.RequestTimeoutMs = ptr(200)
	e := sendErr(t, app, to)
	if e.Code != string(httpclient.CodeTimeout) {
		t.Errorf("timeout error code = %s, want E_TIMEOUT", e.Code)
	}
	if op := waitOp(t, app, to.OpID); op.Status != "error" || op.Error == nil || !strings.Contains(*op.Error, "timed out") {
		t.Errorf("timeout op = %s %v, want error naming the timeout", op.Status, op.Error)
	}

	cancelled := get(srv.URL + "/slow?ms=10000")
	done := make(chan *errBox, 1)
	go func() {
		_, err := app.W.Http.Send(ctx, cancelled)
		done <- &errBox{err}
	}()
	waitUntil(t, "slow request to reach the server", func() bool { return len(srv.Requests()) >= 2 })
	time.Sleep(100 * time.Millisecond)
	if err := app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: cancelled.OpID}); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if code := ipcCode(r.err); code != string(httpclient.CodeCancelled) {
			t.Errorf("cancelled send error = %v, want E_CANCELLED", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Send did not return after Ops.Cancel")
	}
	if op := waitOp(t, app, cancelled.OpID); op.Status != "cancelled" {
		t.Errorf("cancelled op status = %s, want cancelled", op.Status)
	}
	waitUntil(t, "server to see both connections close", func() bool {
		reqs := srv.Requests()
		return len(reqs) == 2 && reqs[0].Canceled && reqs[1].Canceled
	})
}

func TestTransportErrors(t *testing.T) {
	app := flowharness.New(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	cases := []struct {
		name     string
		args     bridge.HttpSendArgs
		wantCode string
		wantMsg  string
	}{
		{"refused", get(closedURL + "/"), "E_TRANSPORT", "refused"},
		{"dns", get("http://flow-nohost.invalid/"), "E_TRANSPORT", "no such host"},
		{"bad url", get("http://[::1"), "E_BAD_REQUEST", "Invalid URL"},
		{"bad scheme", get("ftp://example.com/"), "E_BAD_REQUEST", "Unsupported URL scheme"},
		{"bad method", func() bridge.HttpSendArgs { a := get(closedURL); a.Method = "FETCH"; return a }(), "E_BAD_REQUEST", "Unsupported method"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := sendErr(t, app, tc.args)
			if e.Code != tc.wantCode || !strings.Contains(e.Message, tc.wantMsg) {
				t.Errorf("error = %s %q, want %s containing %q", e.Code, e.Message, tc.wantCode, tc.wantMsg)
			}
			if op := waitOp(t, app, tc.args.OpID); op.Status != "error" {
				t.Errorf("op status = %s, want error", op.Status)
			}
		})
	}
}

// J-HTTP-049: a form-data file is read at send time, so the second send carries the changed bytes and
// a path that stopped being a readable file fails before any request leaves.
func TestFormDataFileChangesBetweenSends(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	path := writeFile(t, "report.csv", []byte("id,qty\n1,3\n"))

	partSHA := func(r flowharnessRequest) string {
		t.Helper()
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("content type %q: %v", r.Header.Get("Content-Type"), err)
		}
		mr := multipart.NewReader(bytes.NewReader(r.Body), params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				t.Fatalf("no upload part: %v", err)
			}
			if p.FormName() == "upload" {
				data, _ := io.ReadAll(p)
				sum := sha256.Sum256(data)
				return hex.EncodeToString(sum[:])
			}
		}
	}
	args := func() bridge.HttpSendArgs {
		a := get(srv.URL + "/echo")
		a.Method = "POST"
		a.Body = httpclient.Body{Mode: "formdata", FormData: []httpclient.FormField{{Name: "upload", Kind: "file", Path: path, ContentType: "text/csv"}}}
		return a
	}
	want := func(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
	last := func() flowharnessRequest { r := srv.Requests(); return r[len(r)-1] }

	send(t, app, args())
	if got := partSHA(last()); got != want([]byte("id,qty\n1,3\n")) {
		t.Fatalf("first send part sha = %s", got)
	}
	changed := []byte("id,qty\n1,3\n2,9\n")
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	send(t, app, args())
	if got := partSHA(last()); got != want(changed) {
		t.Errorf("second send part sha = %s, want the appended file's", got)
	}

	before := len(srv.Requests())
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if e := sendErr(t, app, args()); !strings.Contains(e.Message, "folder") {
		t.Errorf("directory error = %q, want it to say the path is a folder", e.Message)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if e := sendErr(t, app, args()); e.Message == "" {
		t.Error("missing file gave an empty error")
	}
	if len(srv.Requests()) != before {
		t.Errorf("a request left for an unreadable file (%d new)", len(srv.Requests())-before)
	}
}

// J-HTTP-045: timeout 0 is no timeout, a per-request value beats the global, and the response cap
// is exact at its edge (0 = no cap).
func TestTimeoutAndSizeSettings(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)

	t.Run("timeout 0 stays alive until cancel", func(t *testing.T) {
		a := get(srv.URL + "/slow?ms=90000")
		a.Options.RequestTimeoutMs = ptr(0)
		done := make(chan *errBox, 1)
		go func() { _, err := app.W.Http.Send(ctx, a); done <- &errBox{err} }()
		waitUntil(t, "the slow request to arrive", func() bool { return len(srv.Requests()) >= 1 })
		select {
		case r := <-done:
			t.Fatalf("send ended before any cancel: %v", r.err)
		case <-time.After(700 * time.Millisecond):
		}
		if err := app.W.Ops.Cancel(bridge.OpsCancelArgs{OpID: a.OpID}); err != nil {
			t.Fatal(err)
		}
		select {
		case r := <-done:
			if code := ipcCode(r.err); code != string(httpclient.CodeCancelled) {
				t.Errorf("error = %v, want E_CANCELLED", r.err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("Send did not return after cancel")
		}
		waitUntil(t, "the server to see the disconnect", func() bool {
			r := srv.Requests()
			return r[len(r)-1].Canceled
		})
	})

	t.Run("per-request timeout beats the global and removing it restores the global", func(t *testing.T) {
		setApi(t, app, model.ApiPatch{RequestTimeoutMs: ptr(200)})
		if e := sendErr(t, app, get(srv.URL+"/slow?ms=1000")); e.Code != string(httpclient.CodeTimeout) {
			t.Errorf("global 200 ms: code = %s, want E_TIMEOUT", e.Code)
		}
		over := get(srv.URL + "/slow?ms=1000")
		over.Options.RequestTimeoutMs = ptr(5000)
		if res := send(t, app, over); res.Status != 200 {
			t.Errorf("override 5000 ms: status = %d, want 200", res.Status)
		}
		if e := sendErr(t, app, get(srv.URL+"/slow?ms=1000")); e.Code != string(httpclient.CodeTimeout) {
			t.Errorf("after the override: code = %s, want E_TIMEOUT from the global", e.Code)
		}
		setApi(t, app, model.ApiPatch{RequestTimeoutMs: ptr(0)})
	})

	t.Run("response cap edges", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			n         int
			capMb     *int
			wantBytes int
			wantTrunc bool
		}{
			{"1 MB cap, exactly 1 MB", 1 << 20, ptr(1), 1 << 20, false},
			{"1 MB cap, 1 MB + 1 byte", 1<<20 + 1, ptr(1), 1 << 20, true},
			{"default 5 MiB cap, exactly 5 MiB", 5 << 20, nil, 5 << 20, false},
			{"default 5 MiB cap, 5 MiB + 1", 5<<20 + 1, nil, 5 << 20, true},
			{"cap 0 is no cap", 6 << 20, ptr(0), 6 << 20, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				a := get(srv.URL + "/bytes?n=" + strconv.Itoa(tc.n))
				a.Options.MaxResponseMb = tc.capMb
				res := send(t, app, a)
				if res.BodyBytes != tc.wantBytes || res.BodyTruncated != tc.wantTrunc {
					t.Errorf("bytes = %d truncated = %v, want %d and %v", res.BodyBytes, res.BodyTruncated, tc.wantBytes, tc.wantTrunc)
				}
			})
		}
	})
}

// J-HTTP-045: bodies that are not text come back flagged, never as mangled text.
func TestOddResponseBodies(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)

	for _, tc := range []struct {
		name, hex, ct string
		wantEncoding  string
	}{
		{"valid utf-8", hex.EncodeToString([]byte("héllo")), "text/plain; charset=utf-8", "utf8"},
		{"invalid utf-8 under a text type", "68690aff fe 21", "text/plain", "base64"},
		{"binary type", "00010203fffe", "application/octet-stream", "base64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := send(t, app, get(srv.URL+"/raw?ct="+url.QueryEscape(tc.ct)+"&hex="+strings.ReplaceAll(tc.hex, " ", "")))
			if res.BodyEncoding != tc.wantEncoding {
				t.Errorf("encoding = %q, want %q (body %q)", res.BodyEncoding, tc.wantEncoding, res.Body)
			}
			if tc.wantEncoding == "base64" {
				raw, err := base64.StdEncoding.DecodeString(res.Body)
				if err != nil || res.BodyBytes != len(raw) {
					t.Errorf("base64 body = %q (%v), bodyBytes %d", res.Body, err, res.BodyBytes)
				}
			}
		})
	}
}
