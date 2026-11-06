// body.go is the file: one serializer per request-body mode, turning a Body into
// what net/http needs — a reader, a working GetBody for 307/308 replay, an exact
// Content-Length so nothing here is ever sent chunked, and the mode's default Content-Type
// (client.go's Send applies the precedence over it). No dependency: mime/multipart's own
// deterministic framing for a fixed boundary is what makes the multipart dry run possible.
package httpclient

import (
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// BodyMode is this app's own `mode` spelling, originally Postman's own (D2) before the raw/code
// split — "file" is what the UI calls binary.
type BodyMode string

const (
	BodyNone       BodyMode = "none"
	BodyRaw        BodyMode = "raw"
	BodyCode       BodyMode = "code"
	BodyURLEncoded BodyMode = "urlencoded"
	BodyFormData   BodyMode = "formdata"
	// BodyFile — one local file as the entire body; the UI calls this **binary**.
	BodyFile BodyMode = "file"
)

// validBodyModes — D12's parity guard reads this exact map[string]bool literal as plain text
// (extractGoStringSet), so it stays a literal rather than being derived from the consts above.
var validBodyModes = map[string]bool{
	"none": true, "raw": true, "code": true, "urlencoded": true, "formdata": true, "file": true,
}

// contentTypeByCodeLanguage — the default Content-Type per `code` mode sub-language. A
// map[string]string literal, not a switch, specifically so the parity test
// (extractGoStringMap) can read it as plain text against CONTENT_TYPE_BY_CODE_LANGUAGE (http.ts)
// — do not "simplify" this into a switch. Plain `raw` always sends text/plain and needs no table.
var contentTypeByCodeLanguage = map[string]string{
	"javascript": "application/javascript",
	"json":       "application/json",
	"html":       "text/html",
	"xml":        "application/xml",
}

// Field is one urlencoded row (D5).
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FormField is one form-data row. Kind == "file" means Path is an absolute local path
// and Value is ignored; Kind == "text" means Value is the field's content and Path is ignored.
// ContentType is a per-part override; "" means the per-mode default (the row's own Content type
// field when set, else "application/octet-stream" for a file part, nothing for a text part).
type FormField struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
}

// Body is a tagged union: Mode selects which member is meaningful and every other member is
// ignored. Raw is the plain-text buffer (raw mode only); Code/CodeLanguage are the
// syntax-highlighted buffer and its language (code mode only) — two separate buffers so switching
// between the two modes never loses either one's text.
type Body struct {
	Mode         string      `json:"mode"`
	Raw          string      `json:"raw"`
	Code         string      `json:"code"`
	CodeLanguage string      `json:"codeLanguage"`
	URLEncoded   []Field     `json:"urlEncoded"`
	FormData     []FormField `json:"formData"`
	File         string      `json:"file"`
}

// buildBody turns a Body into what net/http needs. contentType is the default this mode implies;
// length is exact, never -1, so nothing here is ever sent chunked. formBoundary is the boundary
// a formdata body must use — client.go's Content-Type precedence resolves it (the user's own
// header's boundary parameter, or a freshly minted one) before calling this; every other mode
// ignores it.
func buildBody(b Body, formBoundary string) (
	body io.ReadCloser, getBody func() (io.ReadCloser, error), length int64, contentType string, err error,
) {
	mode := b.Mode
	if mode == "" {
		// The zero-value Body{} (every httpclient.Request built with no Body field set at all,
		// e.g. every pre-P3 test in this package) means "no body" — the same default HasBody's
		// own zero value (false) gave P2.
		mode = string(BodyNone)
	}
	if !validBodyModes[mode] {
		return nil, nil, 0, "", newError(CodeContract, "unknown body mode: "+b.Mode, nil)
	}
	switch BodyMode(mode) {
	case BodyNone:
		return nil, nil, 0, "", nil
	case BodyRaw:
		return buildRaw(b.Raw)
	case BodyCode:
		return buildCode(b.Code, b.CodeLanguage)
	case BodyURLEncoded:
		return buildURLEncoded(b.URLEncoded)
	case BodyFile:
		return buildFile(b.File)
	case BodyFormData:
		return buildFormData(b.FormData, formBoundary)
	}
	return nil, nil, 0, "", newError(CodeContract, "unknown body mode: "+b.Mode, nil)
}

// buildRaw is plain text only — no sub-language, always text/plain.
func buildRaw(raw string) (io.ReadCloser, func() (io.ReadCloser, error), int64, string, error) {
	reader := func() io.ReadCloser { return io.NopCloser(strings.NewReader(raw)) }
	return reader(), func() (io.ReadCloser, error) { return reader(), nil },
		int64(len(raw)), "text/plain", nil
}

// buildCode is the syntax-highlighted sibling of buildRaw — same shape, its Content-Type depends
// on which of the four code languages was chosen.
func buildCode(code, codeLanguage string) (io.ReadCloser, func() (io.ReadCloser, error), int64, string, error) {
	reader := func() io.ReadCloser { return io.NopCloser(strings.NewReader(code)) }
	return reader(), func() (io.ReadCloser, error) { return reader(), nil },
		int64(len(code)), contentTypeByCodeLanguage[codeLanguage], nil
}

// buildURLEncoded hand-writes the encoder rather than reusing url.Values.Encode: that
// sorts keys, silently rewriting a user's field order, and cannot express a disabled row (the
// renderer already dropped those before this ever runs). url.QueryEscape encodes a space as
// '+', which is correct in an application/x-www-form-urlencoded body — the deliberate opposite of
// views/httprequest/url.ts's buildQuery, which encodeURIComponents a query-string space as '%20'.
// The two encoders differ on purpose; neither is "fixed" into the other.
// encodeURLEncodedFields is buildURLEncoded's own encoder, factored out so wire.go's rendering
// can render the *encoded* string — the one that goes out — without a second
// encoder that could drift from this one.
func encodeURLEncodedFields(fields []Field) (string, error) {
	parts := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.TrimSpace(f.Name) == "" {
			return "", newErrorT(CodeBadRequest, "backend.studio.http.urlencodedFieldNameMissing.body", nil, nil)
		}
		parts = append(parts, url.QueryEscape(f.Name)+"="+url.QueryEscape(f.Value))
	}
	return strings.Join(parts, "&"), nil
}

func buildURLEncoded(fields []Field) (io.ReadCloser, func() (io.ReadCloser, error), int64, string, error) {
	encoded, err := encodeURLEncodedFields(fields)
	if err != nil {
		return nil, nil, 0, "", err
	}
	reader := func() io.ReadCloser { return io.NopCloser(strings.NewReader(encoded)) }
	return reader(), func() (io.ReadCloser, error) { return reader(), nil },
		int64(len(encoded)), "application/x-www-form-urlencoded", nil
}

// buildFile is the binary body: one whole local file, streamed by Go and never by the renderer.
// Postman sets no Content-Type for this mode, and neither do we. Re-`os.Stat`s at send
// time rather than trusting a stored size — the file may have changed or vanished since it was
// picked.
func buildFile(path string) (io.ReadCloser, func() (io.ReadCloser, error), int64, string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil, 0, "", newErrorT(CodeBadRequest, "backend.studio.http.bodyFileMissing.body", nil, nil)
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return nil, nil, 0, "", newErrorT(CodeBadRequest, "backend.studio.http.localFileUnreadable.error", map[string]string{"path": trimmed, "detail": err.Error()}, err)
	}
	if info.IsDir() {
		return nil, nil, 0, "", newErrorT(CodeBadRequest, "backend.studio.http.pathIsDirectory.body", map[string]string{"path": trimmed}, nil)
	}
	f, err := os.Open(trimmed)
	if err != nil {
		return nil, nil, 0, "", newErrorT(CodeBadRequest, "backend.studio.http.localFileUnreadable.error", map[string]string{"path": trimmed, "detail": err.Error()}, err)
	}
	getBody := func() (io.ReadCloser, error) {
		reopened, openErr := os.Open(trimmed)
		if openErr != nil {
			return nil, openErr
		}
		return reopened, nil
	}
	return f, getBody, info.Size(), "", nil
}

// formPart pairs a FormField with the file size resolved once, at prepare time — reused for both
// the dry-run count and the real stream (and any later GetBody replay), so a redirect's replayed
// body reports the same Content-Length the first attempt did even if the file changed size in
// between.
type formPart struct {
	field FormField
	size  int64 // meaningful only when field.Kind == "file"
}

func prepareFormParts(fields []FormField) ([]formPart, error) {
	parts := make([]formPart, 0, len(fields))
	for _, f := range fields {
		if strings.TrimSpace(f.Name) == "" {
			return nil, newErrorT(CodeBadRequest, "backend.studio.http.formFieldNameMissing.body", nil, nil)
		}
		if f.Kind != "file" {
			parts = append(parts, formPart{field: f})
			continue
		}
		trimmed := strings.TrimSpace(f.Path)
		if trimmed == "" {
			return nil, newErrorT(CodeBadRequest, "backend.studio.http.formFieldFileMissing.body", map[string]string{"name": f.Name}, nil)
		}
		info, err := os.Stat(trimmed)
		if err != nil {
			return nil, newErrorT(CodeBadRequest, "backend.studio.http.localFileUnreadable.error", map[string]string{"path": trimmed, "detail": err.Error()}, err)
		}
		if info.IsDir() {
			return nil, newErrorT(CodeBadRequest, "backend.studio.http.pathIsDirectory.body", map[string]string{"path": trimmed}, nil)
		}
		f.Path = trimmed
		parts = append(parts, formPart{field: f, size: info.Size()})
	}
	return parts, nil
}

// quoteEscaper mirrors mime/multipart's own unexported one (used by CreateFormFile) — the package
// gives no other way to build a Content-Disposition header for a caller-supplied Content-Type.
var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func formPartHeader(f FormField) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	if f.Kind == "file" {
		filename := filepath.Base(f.Path)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
			quoteEscaper.Replace(f.Name), quoteEscaper.Replace(filename)))
		ct := f.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h.Set("Content-Type", ct)
	} else {
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, quoteEscaper.Replace(f.Name)))
		if f.ContentType != "" {
			h.Set("Content-Type", f.ContentType)
		}
	}
	return h
}

// countWriter measures exactly what a real multipart.Writer pass would produce, without holding
// any of it in memory (F15).
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// multipartLength is the dry run (F15/D6): the identical boundary and part headers as the real
// pass, written to a counting writer; a file part adds its already-`os.Stat`'d size instead of
// being read. Same boundary, same headers, same terminator ⇒ the count the real pass will produce.
func multipartLength(parts []formPart, boundary string) (int64, error) {
	cw := &countWriter{}
	mw := multipart.NewWriter(cw)
	if err := mw.SetBoundary(boundary); err != nil {
		return 0, newErrorT(CodeBadRequest, "backend.studio.http.multipartBoundaryInvalid.error", map[string]string{"detail": err.Error()}, err)
	}
	for _, p := range parts {
		part, err := mw.CreatePart(formPartHeader(p.field))
		if err != nil {
			return 0, newErrorT(CodeTransport, "backend.studio.http.multipartBuildFailed.error", map[string]string{"detail": err.Error()}, err)
		}
		if p.field.Kind == "file" {
			cw.n += p.size
			continue
		}
		if _, err := part.Write([]byte(p.field.Value)); err != nil {
			return 0, newErrorT(CodeTransport, "backend.studio.http.multipartBuildFailed.error", map[string]string{"detail": err.Error()}, err)
		}
	}
	if err := mw.Close(); err != nil {
		return 0, newErrorT(CodeTransport, "backend.studio.http.multipartBuildFailed.error", map[string]string{"detail": err.Error()}, err)
	}
	return cw.n, nil
}

// streamFormData is the real pass: written into an io.Pipe by one goroutine so the caller can
// start reading before every part is ready, streaming each file straight from disk rather
// than buffering it. io.CopyN with the size prepareFormParts already resolved means a file that
// shrank in between fails loudly (io.ErrUnexpectedEOF, propagated via CloseWithError) and a file
// that grew is truncated to the counted size — either way the bytes sent match Content-Length.
// The goroutine never outlives the request: it exits on its own first write error, and
// cancelling the request closes pr, which turns the next write into io.ErrClosedPipe.
func streamFormData(parts []formPart, boundary string) *io.PipeReader {
	pr, pw := io.Pipe()
	go func() {
		var werr error
		defer func() { _ = pw.CloseWithError(werr) }()

		mw := multipart.NewWriter(pw)
		if err := mw.SetBoundary(boundary); err != nil {
			werr = err
			return
		}
		for _, p := range parts {
			part, err := mw.CreatePart(formPartHeader(p.field))
			if err != nil {
				werr = err
				return
			}
			if p.field.Kind == "file" {
				f, openErr := os.Open(p.field.Path)
				if openErr != nil {
					werr = openErr
					return
				}
				_, copyErr := io.CopyN(part, f, p.size)
				closeErr := f.Close()
				if copyErr != nil {
					werr = copyErr
					return
				}
				if closeErr != nil {
					werr = closeErr
					return
				}
				continue
			}
			if _, err := part.Write([]byte(p.field.Value)); err != nil {
				werr = err
				return
			}
		}
		werr = mw.Close()
	}()
	return pr
}

// mintBoundary hands back a fresh random boundary with no part written — multipart.NewWriter
// generates one immediately, before anything is written to the (discarded) underlying writer.
func mintBoundary() string {
	return multipart.NewWriter(io.Discard).Boundary()
}

func buildFormData(fields []FormField, boundary string) (
	io.ReadCloser, func() (io.ReadCloser, error), int64, string, error,
) {
	parts, err := prepareFormParts(fields)
	if err != nil {
		return nil, nil, 0, "", err
	}
	if boundary == "" {
		boundary = mintBoundary()
	}
	length, err := multipartLength(parts, boundary)
	if err != nil {
		return nil, nil, 0, "", err
	}
	contentType := "multipart/form-data; boundary=" + boundary
	getBody := func() (io.ReadCloser, error) { return streamFormData(parts, boundary), nil }
	return streamFormData(parts, boundary), getBody, length, contentType, nil
}
