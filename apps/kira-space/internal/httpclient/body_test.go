package httpclient

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sendViaBuildBody drives buildBody the same way client.go's Send will once wired in
// (the guard: client.go stays untouched here) — headers loop, the Content-Type precedence,
// GetBody/ContentLength assigned explicitly onto the request, exactly what client_test.go
// exercises through the real Send instead.
func sendViaBuildBody(t *testing.T, url, method string, headers []Header, b Body) *http.Response {
	t.Helper()
	userContentType, hasUserCT := "", false
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Content-Type") {
			userContentType, hasUserCT = h.Value, true
		}
	}
	formBoundary := ""
	if b.Mode == string(BodyFormData) {
		if hasUserCT {
			if mt, params, err := mime.ParseMediaType(userContentType); err == nil && mt == "multipart/form-data" {
				formBoundary = params["boundary"]
			}
		}
		if formBoundary == "" {
			formBoundary = mintBoundary()
		}
	}
	body, getBody, length, defaultContentType, err := buildBody(b, formBoundary)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Body = body
		req.GetBody = getBody
		req.ContentLength = length
	}
	for _, h := range headers {
		req.Header.Add(h.Name, h.Value)
	}
	switch {
	case !hasUserCT:
		if defaultContentType != "" {
			req.Header.Set("Content-Type", defaultContentType)
		}
	case b.Mode == string(BodyFormData):
		if mt, params, err := mime.ParseMediaType(userContentType); err == nil && mt == "multipart/form-data" {
			if _, ok := params["boundary"]; !ok {
				req.Header.Set("Content-Type", userContentType+"; boundary="+formBoundary)
			}
		}
	}
	client := &http.Client{CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	return resp
}

// buildBodyErr calls buildBody directly (no server, no request) for the refusal cases.
func buildBodyErr(t *testing.T, b Body) error {
	t.Helper()
	_, _, _, _, err := buildBody(b, "")
	return err
}

// §6.3 case 1: form-data with a real temp file — the server's own ParseMultipartForm sees both
// the text field and the file part, the file's bytes are byte-identical to what was on disk, and
// the part's own Content-Type is the row's override when set and application/octet-stream when
// not.
func TestBuildBody_FormDataRealFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "report.csv")
	fileContent := []byte("a,b,c\n1,2,3\n")
	if err := os.WriteFile(filePath, fileContent, 0o644); err != nil {
		t.Fatal(err)
	}
	filePath2 := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(filePath2, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotValue string
	var gotBytes []byte
	var gotContentType, gotContentType2 string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotValue = r.FormValue("title")
		file, header, err := r.FormFile("upload")
		if err != nil {
			t.Fatalf("FormFile upload: %v", err)
		}
		defer file.Close()
		gotBytes, _ = io.ReadAll(file)
		gotContentType = header.Header.Get("Content-Type")

		_, header2, err := r.FormFile("upload2")
		if err != nil {
			t.Fatalf("FormFile upload2: %v", err)
		}
		gotContentType2 = header2.Header.Get("Content-Type")
	}))
	defer srv.Close()

	resp := sendViaBuildBody(t, srv.URL, "POST", nil, Body{
		Mode: "formdata",
		FormData: []FormField{
			{Name: "title", Kind: "text", Value: "hello"},
			{Name: "upload", Kind: "file", Path: filePath, ContentType: "text/csv"},
			{Name: "upload2", Kind: "file", Path: filePath2},
		},
	})
	resp.Body.Close()

	if gotValue != "hello" {
		t.Errorf("title = %q, want hello", gotValue)
	}
	if !bytes.Equal(gotBytes, fileContent) {
		t.Errorf("file bytes = %q, want %q", gotBytes, fileContent)
	}
	if gotContentType != "text/csv" {
		t.Errorf("upload Content-Type = %q, want text/csv", gotContentType)
	}
	if gotContentType2 != "application/octet-stream" {
		t.Errorf("upload2 Content-Type = %q, want application/octet-stream", gotContentType2)
	}
}

// §6.3 case 2: the multipart Content-Length is exact and the request is not chunked — the server
// sees a Content-Length equal to the bytes it actually reads, and TransferEncoding is empty.
func TestBuildBody_MultipartContentLengthExact(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "data.bin")
	content := bytes.Repeat([]byte("x"), 5000)
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	var gotContentLength int64
	var gotTransferEncoding []string
	var gotBodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength
		gotTransferEncoding = r.TransferEncoding
		data, _ := io.ReadAll(r.Body)
		gotBodyLen = len(data)
	}))
	defer srv.Close()

	resp := sendViaBuildBody(t, srv.URL, "POST", nil, Body{
		Mode: "formdata", FormData: []FormField{{Name: "file", Kind: "file", Path: filePath}},
	})
	resp.Body.Close()

	if gotContentLength <= 0 {
		t.Fatalf("ContentLength = %d, want > 0", gotContentLength)
	}
	if int64(gotBodyLen) != gotContentLength {
		t.Errorf("read %d bytes, Content-Length said %d", gotBodyLen, gotContentLength)
	}
	if len(gotTransferEncoding) != 0 {
		t.Errorf("TransferEncoding = %v, want none (not chunked)", gotTransferEncoding)
	}
}

// §6.3 case 3: a 307 redirect replays a file body (F4) — without GetBody this is the case that
// fails with "cannot retry request with non-replayable body".
func TestBuildBody_RedirectReplaysFileBody(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "payload.bin")
	content := []byte("redirect-me-verbatim")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	var secondHopBody []byte
	var secondHopHit bool
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/first", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/second", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/second", func(_ http.ResponseWriter, r *http.Request) {
		secondHopHit = true
		secondHopBody, _ = io.ReadAll(r.Body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	serverURL = srv.URL

	resp := sendViaBuildBody(t, srv.URL+"/first", "POST", nil, Body{Mode: "file", File: filePath})
	resp.Body.Close()

	if !secondHopHit {
		t.Fatal("second hop was never reached")
	}
	if !bytes.Equal(secondHopBody, content) {
		t.Errorf("second hop body = %q, want %q", secondHopBody, content)
	}
}

// §6.3 case 4: urlencoded preserves the user's own field order and encodes a space as '+' (F6).
func TestBuildBody_URLEncodedOrderAndSpaceEncoding(t *testing.T) {
	var gotBody, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		gotBody = string(data)
		gotContentType = r.Header.Get("Content-Type")
	}))
	defer srv.Close()

	resp := sendViaBuildBody(t, srv.URL, "POST", nil, Body{Mode: "urlencoded", URLEncoded: []Field{
		{Name: "b", Value: "2"},
		{Name: "a", Value: "hello world"},
	}})
	resp.Body.Close()

	if want := "b=2&a=hello+world"; gotBody != want {
		t.Errorf("body = %q, want %q", gotBody, want)
	}
	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotContentType)
	}
}

// §6.3 case 5: a binary body sets an exact Content-Length and no Content-Type (F3), and a missing
// path is CodeBadRequest with the path named in the message.
func TestBuildBody_BinaryBodyLengthAndNoContentType(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "image.bin")
	content := bytes.Repeat([]byte{0xAB}, 3333)
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	var gotContentLength int64
	var gotContentType string
	var gotBodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength
		gotContentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		gotBodyLen = len(data)
	}))
	defer srv.Close()

	resp := sendViaBuildBody(t, srv.URL, "POST", nil, Body{Mode: "file", File: filePath})
	resp.Body.Close()

	if gotContentLength != int64(len(content)) {
		t.Errorf("ContentLength = %d, want %d", gotContentLength, len(content))
	}
	if gotBodyLen != len(content) {
		t.Errorf("read %d bytes, want %d", gotBodyLen, len(content))
	}
	if gotContentType != "" {
		t.Errorf("Content-Type = %q, want none", gotContentType)
	}

	missing := filepath.Join(dir, "does-not-exist.bin")
	err := buildBodyErr(t, Body{Mode: "file", File: missing})
	code, ok := CodeOf(err)
	if !ok || code != CodeBadRequest {
		t.Fatalf("err code = %v (ok=%v), want CodeBadRequest", code, ok)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not mention the missing path %q", err.Error(), missing)
	}
}

// §6.3 case 6: Content-Type precedence (D7) — a user-set application/vnd.api+json beats code·json's
// default, and a user-set bare multipart/form-data gets the generated boundary appended so the
// body still parses server-side.
func TestBuildBody_ContentTypePrecedence(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	resp := sendViaBuildBody(t, srv.URL, "POST",
		[]Header{{Name: "Content-Type", Value: "application/vnd.api+json"}},
		Body{Mode: "code", CodeLanguage: "json", Code: `{"a":1}`})
	resp.Body.Close()
	if gotContentType != "application/vnd.api+json" {
		t.Errorf("Content-Type = %q, want application/vnd.api+json (user value wins)", gotContentType)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	var parsedOK bool
	srv2 := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		parsedOK = r.ParseMultipartForm(10<<20) == nil
	}))
	defer srv2.Close()

	resp = sendViaBuildBody(t, srv2.URL, "POST",
		[]Header{{Name: "Content-Type", Value: "multipart/form-data"}},
		Body{Mode: "formdata", FormData: []FormField{{Name: "f", Kind: "file", Path: filePath}}})
	resp.Body.Close()
	if !strings.HasPrefix(gotContentType, "multipart/form-data; boundary=") {
		t.Errorf("Content-Type = %q, want the generated boundary appended", gotContentType)
	}
	if !parsedOK {
		t.Error("server could not parse the multipart body")
	}
}
