// wire.go is the whole rendering: "raw" is rendered in Go from the real *http.Request and
// *http.Response, never captured from the wire (measured capture unavailable) and never
// reconstructed in the renderer (measured the dump reflects transport-added facts a
// TypeScript reconstruction cannot know). The request half is exact; the response half is an
// explicitly labelled reconstruction — the fidelity value says which.
package httpclient

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxWireBodyBytes — D4: 128 KiB. Distinct from P8's 256 KiB storage cap and the 10 MiB transfer
// cap — this bounds a per-send bridge payload paid on *every* send, for a view nobody may open, not
// storage or transfer.
const maxWireBodyBytes = 128 * 1024

// WireExchange is P9 D13's Go source of truth, mirrored at packages/shared/domain/http.ts's
// HttpWireExchange. Every field is documented there; not restated here.
type WireExchange struct {
	Request           string `json:"request"`
	ResponseHead      string `json:"responseHead"`
	Fidelity          string `json:"fidelity"`
	MaskedSecrets     int    `json:"maskedSecrets"`
	RequestBodyElided bool   `json:"requestBodyElided"`
}

// wireProxyFunc — http.ProxyFromEnvironment, the exact function sharedClient.Transport.Proxy
// already is (client.go:44). A var, not a bare call, only so wire_test.go can substitute a fake
// proxy function for D3's classification cases without setting a real HTTPS_PROXY.
var wireProxyFunc = http.ProxyFromEnvironment

// classifyFidelity is computed after Do returns because both resp.ProtoMajor and whether a
// proxy applied are only known then. The proxy check calls wireProxyFunc directly — the same
// function the transport itself consults — so the classification can never disagree with what the
// transport did.
func classifyFidelity(resp *http.Response, httpReq *http.Request) string {
	if resp.ProtoMajor >= 2 {
		return "http2"
	}
	if proxyURL, err := wireProxyFunc(httpReq); err == nil && proxyURL != nil {
		return "proxied"
	}
	return "exact"
}

// capWireText applies the cap to a free-form buffer (raw/code/urlencoded bodies) — truncated with
// a visible marker naming exactly how much was cut, never silently.
//
// mask runs BEFORE the cap, never after. A secret straddling the cut byte would
// otherwise leave the kept prefix not matching the replacer's full registered pattern, so masking
// the already-truncated text (the old call order, in bridge/http.go's maskSecrets) could leave an
// unmasked secret prefix on the wire pane. mask is nil in every context with no secret to mask
// (the common case) or in wire_test.go's own
// direct calls.
func capWireText(s string, mask *strings.Replacer) (string, bool) {
	if mask != nil {
		s = mask.Replace(s)
	}
	if len(s) <= maxWireBodyBytes {
		return s, false
	}
	// Cut on a rune boundary — a byte cap can otherwise split a multi-byte UTF-8 rune, producing
	// invalid UTF-8 in the kept prefix.
	cut := maxWireBodyBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n[… %d more bytes …]", len(s)-cut), true
}

// binaryMarker is D4's elision marker, shared by a `file` body and a formdata file part — the
// header's own Content-Length always carries the real size (D4's "never lies about size" rule), so
// this only ever has to say which bytes it is standing in for.
func binaryMarker(n int64, name string) string {
	if name == "" {
		name = "file"
	}
	return fmt.Sprintf("[… %d bytes of %s …]", n, name)
}

// renderRequestBody is the per-mode dispatch, run over the exact same Body/formBoundary Send just
// used to build the real request — never a resolved or renderer-derived value. fileContentLength is
// httpReq.ContentLength for a `file` body: buildFile's own os.Stat already resolved it once: the
// "the Content-Length in the head is always the real one" invariant extended to the marker's
// count, rather than a second, possibly-racing os.Stat here.
func renderRequestBody(body Body, formBoundary string, fileContentLength int64, mask *strings.Replacer) (text string, elided bool, err error) {
	mode := body.Mode
	if mode == "" {
		mode = string(BodyNone)
	}
	switch BodyMode(mode) {
	case BodyNone:
		return "", false, nil
	case BodyRaw:
		text, truncated := capWireText(body.Raw, mask)
		return text, truncated, nil
	case BodyCode:
		text, truncated := capWireText(body.Code, mask)
		return text, truncated, nil
	case BodyURLEncoded:
		// D4: the *encoded* string buildURLEncoded produced, not a re-derivation — url.QueryEscape's
		// '+'-for-space is exactly what a raw view exists to confirm.
		encoded, encErr := encodeURLEncodedFields(body.URLEncoded)
		if encErr != nil {
			return "", false, encErr
		}
		text, truncated := capWireText(encoded, mask)
		return text, truncated, nil
	case BodyFile:
		return binaryMarker(fileContentLength, filepath.Base(strings.TrimSpace(body.File))), true, nil
	case BodyFormData:
		return renderFormDataBody(body.FormData, formBoundary, mask)
	}
	return "", false, nil
}

// renderFormDataBody is the dry-run trick a second time, with a strings.Builder standing in for
// countWriter: the identical boundary and formPartHeader output the real pass writes, so the
// framing this renders is exactly what went out. Each file part's payload is replaced by its
// marker (never read); each text part is kept verbatim under the cap.
func renderFormDataBody(fields []FormField, boundary string, mask *strings.Replacer) (string, bool, error) {
	parts, err := prepareFormParts(fields)
	if err != nil {
		return "", false, err
	}
	if boundary == "" {
		boundary = mintBoundary()
	}
	var b strings.Builder
	mw := multipart.NewWriter(&b)
	if err := mw.SetBoundary(boundary); err != nil {
		return "", false, newErrorT(CodeBadRequest, "backend.studio.http.multipartBoundaryInvalid.error", map[string]string{"detail": err.Error()}, err)
	}
	elided := false
	for _, p := range parts {
		part, partErr := mw.CreatePart(formPartHeader(p.field))
		if partErr != nil {
			return "", false, newErrorT(CodeTransport, "backend.studio.http.multipartRenderFailed.error", map[string]string{"detail": partErr.Error()}, partErr)
		}
		if p.field.Kind == "file" {
			if _, err := part.Write([]byte(binaryMarker(p.size, filepath.Base(p.field.Path)))); err != nil {
				return "", false, newErrorT(CodeTransport, "backend.studio.http.multipartRenderFailed.error", map[string]string{"detail": err.Error()}, err)
			}
			elided = true
			continue
		}
		text, truncated := capWireText(p.field.Value, mask)
		if truncated {
			elided = true
		}
		if _, err := part.Write([]byte(text)); err != nil {
			return "", false, newErrorT(CodeTransport, "backend.studio.http.multipartRenderFailed.error", map[string]string{"detail": err.Error()}, err)
		}
	}
	if err := mw.Close(); err != nil {
		return "", false, newErrorT(CodeTransport, "backend.studio.http.multipartRenderFailed.error", map[string]string{"detail": err.Error()}, err)
	}
	return b.String(), elided, nil
}

// renderRequest is the head verbatim from httputil.DumpRequestOut(httpReq, false) — exact,
// including the transport's own Accept-Encoding, the real Content-Length, Go's header ordering
// and a Host: override — plus the body, appended directly onto the dump's own trailing blank
// line (DumpRequestOut with body=false already ends the head at "\r\n\r\n").
func renderRequest(httpReq *http.Request, body Body, formBoundary string, mask *strings.Replacer) (text string, elided bool, err error) {
	head, dumpErr := httputil.DumpRequestOut(httpReq, false)
	if dumpErr != nil {
		return "", false, dumpErr
	}
	bodyText, bodyElided, bodyErr := renderRequestBody(body, formBoundary, httpReq.ContentLength, mask)
	if bodyErr != nil {
		return "", false, bodyErr
	}
	return string(head) + bodyText, bodyElided, nil
}

// renderResponseHead is the status line and headers only, from httputil.DumpResponse(resp,
// false) — the measured reconstruction (alphabetised, canonicalised, and for an h2 response an
// honest status line over HTTP/1.1-style header lines). The body is deliberately not included here
// — RawExchangePane.vue concatenates the Response object's own body locally rather than this
// duplicating it across the bridge.
func renderResponseHead(resp *http.Response) (string, error) {
	head, err := httputil.DumpResponse(resp, false)
	if err != nil {
		return "", err
	}
	return string(head), nil
}
