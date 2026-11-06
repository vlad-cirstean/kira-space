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
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studioflowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/bridge"
)

func secretVar(t *testing.T, app *studioflowharness.App, name, value string) (collectionID, envID string) {
	t.Helper()
	col := app.SeedCollection(t, "c-"+name)
	env := app.SeedEnvironment(t, "e-"+name, studioflowharness.APIVar{Name: name, Value: value, Secret: true})
	return col.ID, env.ID
}

func TestAuthHeaders(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

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
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)
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
		studioflowharness.Complete(t)
		args := get(srv.URL + "/bytes?n=209715200")
		args.Options.MaxResponseMb = ptr(300)
		res := send(t, app, args)
		if res.BodyBytes != 200<<20 || res.BodyTruncated {
			t.Errorf("bytes = %d truncated = %v, want %d", res.BodyBytes, res.BodyTruncated, 200<<20)
		}
	})
}

func TestTimeoutAndCancel(t *testing.T) {
	app := studioflowharness.New(t)
	srv := studioflowharness.HTTP(t)

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
	app := studioflowharness.New(t)

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
