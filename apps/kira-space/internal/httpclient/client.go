// Package httpclient is the whole outbound HTTP path for the request builder: one exported
// Send(ctx, Request) (Response, error), self-contained and dependency-free (no adapters,
// adapterhost, storage or Wails import — drivable from a plain httptest server).
// Every default it takes on the caller's behalf is named and explained below; nothing here
// silently rewrites what the user asked for.
package httpclient

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/buildinfo"
)

var validMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true,
	"OPTIONS": true,
}

// Header is one request or response header. Response headers come back sorted by name, one entry
// per value so duplicates survive — F19's honest substitute for net/http's order-losing,
// key-canonicalising map; there is no stdlib access to the bytes as received.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Request is the wire shape control.ts's httpSend sends. Headers/body/method go out unmodified —
// the one design requirement: no client library sits between this struct and the wire.
// Body was Body string/HasBody bool (the only two modes); it is now one tagged union
// (body.go) covering every mode Postman's own format exposes.
type Request struct {
	Method  string   `json:"method"`
	URL     string   `json:"url"`
	Headers []Header `json:"headers"`
	Body    Body     `json:"body"`
}

// RedirectHop is one followed redirect: the status that was returned, and the URL that returned
// it (not the URL it redirected to — that is either the next hop's URL, or FinalURL for the last
// one).
type RedirectHop struct {
	Status int    `json:"status"`
	URL    string `json:"url"`
}

// Response is the wire shape Send returns. Body's meaning depends on BodyEncoding: "utf8" is the
// bytes as a string, "base64" is the raw bytes — Go's encoding/json would otherwise replace
// invalid UTF-8 with U+FFFD and silently corrupt a binary response (D4).
type Response struct {
	Status        int           `json:"status"`
	StatusText    string        `json:"statusText"`
	Proto         string        `json:"proto"`
	Headers       []Header      `json:"headers"`
	Body          string        `json:"body"`
	BodyEncoding  string        `json:"bodyEncoding"` // "utf8" | "base64"
	BodyBytes     int           `json:"bodyBytes"`
	BodyTruncated bool          `json:"bodyTruncated"`
	ElapsedMs     int           `json:"elapsedMs"`
	FinalURL      string        `json:"finalUrl"`
	Redirects     []RedirectHop `json:"redirects"`
	// Wire is P9 D2/D7's rendered exchange — a pointer with omitempty so a stored snapshot's JSON
	// (repos/response_history.go strips it to nil before marshalling, D7/F12) carries no "wire" key
	// at all, not just a null one. Never fatal to compute: a dump error simply leaves this nil.
	Wire *WireExchange `json:"wire,omitempty"`
	// Timeline is P10's full chronological sequence of what happened while the request ran (D2,
	// D10) — always populated for a response Send actually returns; unlike Wire, it is small
	// enough (F17) that repos/response_history.go keeps it rather than stripping it.
	Timeline Timeline `json:"timeline"`
	// Sent is what went out per hop, captured
	// from httptrace's WroteHeaderField (the jar injects Cookie inside net/http, after
	// CheckRedirect, so there is nowhere else to read it). Received is every hop's Set-Cookie,
	// parsed by the stdlib. Populated regardless of whether the cookie jar itself is on — these
	// reflect the literal wire, not the jar's own retained state.
	SentCookies     []Cookie `json:"sentCookies,omitempty"`
	ReceivedCookies []Cookie `json:"receivedCookies,omitempty"`
}

// redirectHeaderNamesCtxKey carries the user-supplied header names through context, sibling to
// timelineCtxKey, so checkRedirect (which only ever sees *http.Request/[]*http.Request per its
// stdlib-mandated signature) can strip them on a cross-host hop (P21 round 3 finding 5).
type redirectHeaderNamesCtxKey struct{}

// sameRedirectHost mirrors net/http's own shouldCopyHeaderOnRedirect host check
// (isDomainOrSubdomain): dest is allowed to be the same host as, or a subdomain of, from —
// anything else counts as a cross-origin hop for header-stripping purposes here. Deliberately
// stricter than "same registrable domain" (which would need a public suffix list this
// dependency-free package has no business importing) — a false "cross-origin" only costs a
// header re-add the transport doesn't need; a false "same-origin" would leak a secret.
func sameRedirectHost(from, dest *url.URL) bool {
	fh, dh := strings.ToLower(from.Hostname()), strings.ToLower(dest.Hostname())
	if fh == "" || dh == "" {
		return fh == dh
	}
	return dh == fh || strings.HasSuffix(dh, "."+fh)
}

// effectivePort is u's explicit port, else the scheme default ("" for an unknown scheme).
func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	}
	return ""
}

// isPortChange reports a hop to another service on the same host. net/http compares hostnames
// only, so it keeps Authorization/Cookie across a port change. The default-port http to https
// upgrade counts as same-origin: it targets the same service and never downgrades.
func isPortChange(from, dest *url.URL) bool {
	fp, dp := effectivePort(from), effectivePort(dest)
	if fp == dp {
		return false
	}
	return !(fp == "80" && dp == "443" && from.Port() == "" && dest.Port() == "")
}

// transportErrText renders err without the request URL. net/url.Error quotes the URL with %q,
// which escapes quotes and backslashes in an inlined secret so the plaintext secret masker never
// matches. The URL already sits in the (masked) timeline, so the text keeps the operation and
// the underlying cause only.
func transportErrText(err error) string {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err.Error()
	}
	inner := ue.Err
	for {
		var nested *url.Error
		if !errors.As(inner, &nested) {
			break
		}
		inner = nested.Err
	}
	return ue.Op + ": " + inner.Error()
}

// isSchemeDowngrade reports whether dest drops from https to http on an otherwise same-host hop.
// net/http's own sensitive-header stripping (shouldCopyHeaderOnRedirect) compares hostnames
// only, never scheme, so an https-to-http redirect keeps Authorization/Cookie on the wire in
// clear even when checkRedirectFor's own host check says "same host" — callers must strip those
// two explicitly on a downgrade rather than relying on net/http to have done it.
func isSchemeDowngrade(from, dest *url.URL) bool {
	return strings.EqualFold(from.Scheme, "https") && strings.EqualFold(dest.Scheme, "http")
}

// HasScheme reports whether s already begins with "<scheme>://" — deliberately stricter than
// url.Parse's own Scheme field, which never populates for a bare "api.example.com/path" (no
// "//") in the first place, so resolveURL has to know to prepend one before parsing at all. An
// RFC 3986 scheme (leading letter, then letters/digits/+/-/., up to "://") — postman's own url.go
// kept a second, near-identical copy of this.
func HasScheme(s string) bool {
	i := strings.Index(s, "://")
	if i <= 0 {
		return false
	}
	for j, c := range s[:i] {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '.' || c == '-'):
		default:
			return false
		}
	}
	return true
}

// resolveURL applies D4's scheme default (a URL with no scheme resolves to https://, fails safe
// rather than quietly going out in the clear) and refuses everything E_BAD_REQUEST should refuse
// before a single byte goes out.
func resolveURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, newErrorT(CodeBadRequest, "backend.studio.http.urlRequired.body", nil, nil)
	}
	if !HasScheme(trimmed) {
		trimmed = "https://" + trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, newErrorT(CodeBadRequest, "backend.studio.http.urlInvalid.error", map[string]string{"detail": transportErrText(err)}, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, newErrorT(CodeBadRequest, "backend.studio.http.urlSchemeUnsupported.body", map[string]string{"scheme": u.Scheme}, nil)
	}
	if u.Host == "" {
		return nil, newErrorT(CodeBadRequest, "backend.studio.http.urlHostMissing.body", nil, nil)
	}
	return u, nil
}

// classifySendErr distinguishes a timeout from a cancellation (one is a failure, the other is the
// user) by reading sendCtx.Err() rather than the wrapped error itself —
// net/http wraps a context error inside a *url.Error, and this way there is exactly one place
// that has to know that. tl carries what was measured before the send died, closed by the caller
// (finishFailed) before this is called.
func classifySendErr(sendCtx context.Context, err error, tl Timeline) *Error {
	var e *Error
	switch {
	case sendCtx.Err() == context.DeadlineExceeded:
		e = newErrorT(CodeTimeout, "backend.studio.http.requestTimedOut.body", nil, err)
	case sendCtx.Err() != nil:
		e = newErrorT(CodeCancelled, "backend.studio.http.requestCancelled.body", nil, err)
	default:
		e = newError(CodeTransport, transportErrText(err), err)
	}
	e.Timeline = &tl
	return e
}

// headerValue reports the last value of a case-insensitively matching request header — mirrors
// the loop Send's own Host/User-Agent detection already does, reused by D7's Content-Type guard.
func headerValue(headers []Header, name string) (string, bool) {
	value, found := "", false
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			value, found = h.Value, true
		}
	}
	return value, found
}

func flattenHeaders(h http.Header) []Header {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	headers := make([]Header, 0, len(h))
	for _, k := range keys {
		for _, v := range h[k] {
			headers = append(headers, Header{Name: k, Value: v})
		}
	}
	return headers
}

func statusText(status string) string {
	if i := strings.IndexByte(status, ' '); i >= 0 {
		return status[i+1:]
	}
	return ""
}

// Send issues exactly one HTTP request and returns exactly one response — no retry, no second
// request, no rewriting of what the caller asked for. Both request send and body read happen
// under one deadline (opts' resolved timeout, layered onto ctx so the Stop button and the
// window-close abort both reach a body read in progress too) — unless that timeout resolves to
// 0/none, in which case only ctx's own cancellation applies.
func Send(ctx context.Context, req Request, opts Options) (Response, error) {
	if !validMethods[req.Method] {
		return Response{}, newErrorT(CodeBadRequest, "backend.studio.http.methodUnsupported.body", map[string]string{"method": req.Method}, nil)
	}
	u, err := resolveURL(req.URL)
	if err != nil {
		return Response{}, err
	}
	r := opts.normalize()

	var (
		sendCtx context.Context
		cancel  context.CancelFunc
	)
	if r.timeout > 0 {
		sendCtx, cancel = context.WithTimeout(ctx, r.timeout)
	} else {
		sendCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	// One trace, inherited across every redirect hop because net/http's redirect path
	// builds each subsequent request with ctx: ireq.ctx — replaces the old bare
	// *[]RedirectHop; Response.Redirects below is now a projection of tl.
	//
	// The ClientTrace itself is deliberately NOT attached to sendCtx here — httputil.DumpRequestOut
	// below clones httpReq and RoundTrips it through its own throwaway *http.Transport
	// (a fake in-memory dumpConn) to capture the wire bytes, and that fake RoundTrip fires every
	// hook on whatever ClientTrace the request's context carries. Attaching the trace only to
	// httpReq's context right before the real sharedClient.Do call (below) is what keeps that fake
	// connection from being recorded as a real one.
	tl := newTimeline(req.Method, u.String())
	sendCtx = context.WithValue(sendCtx, timelineCtxKey{}, tl)

	// P21 round 3 finding 5: the user-supplied header names, threaded through context the same
	// way tl is, so checkRedirect can strip them on a cross-host hop. Host is excluded — it is
	// never carried in http.Header (F20a assigns it to httpReq.Host directly instead).
	userHeaderNames := make([]string, 0, len(req.Headers))
	for _, h := range req.Headers {
		if strings.EqualFold(h.Name, "Host") {
			continue
		}
		userHeaderNames = append(userHeaderNames, h.Name)
	}
	sendCtx = context.WithValue(sendCtx, redirectHeaderNamesCtxKey{}, userHeaderNames)

	httpReq, formBoundary, defaultContentType, err := prepareRequest(sendCtx, req, u)
	if err != nil {
		return Response{}, err
	}
	applyHeaders(httpReq, req, formBoundary, defaultContentType)
	// The transport auto-adds Accept-Encoding: gzip and transparently decompresses only
	// when the caller didn't set that header itself. We never set it here, so a user-supplied
	// Accept-Encoding above is passed through untouched and the response is reported as-received
	// — no explicit "detect and skip decoding" branch is needed on top of that default.

	// Dumped from the request the transport is about to write, with body=false — this is safe for
	// a non-rewindable streaming body and body=true would buffer an entire file upload through
	// httputil.drainBody. The body is composed separately, under the cap, once Do returns.
	reqHead, dumpErr := httputil.DumpRequestOut(httpReq, false)

	// P10 D2: the trace goes on httpReq's own context only now, after the dump above — see the
	// comment where tl was created.
	httpReq = httpReq.WithContext(httptrace.WithClientTrace(sendCtx, tl.trace()))

	// One *http.Client per send, over a cached *http.Transport (transportFor) — an
	// http.Client value is three fields, so building one per send costs nothing and is what lets
	// CheckRedirect and Jar vary per-Options without a third context key; connection reuse across
	// sends is what the cached transport itself is for.
	client := &http.Client{
		Transport:     transportFor(transportKey{http1: r.http1, skipVerify: !r.sslVerify}),
		CheckRedirect: checkRedirectFor(r),
		Jar:           jarFor(r),
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, classifySendErr(sendCtx, err, tl.finishFailed(time.Now(), transportErrText(err)))
	}
	defer resp.Body.Close()

	data, truncated, readErr := readResponseBody(resp, r.maxResponseBytes)
	if readErr != nil {
		return Response{}, classifySendErr(sendCtx, readErr, tl.finishFailed(time.Now(), transportErrText(readErr)))
	}
	// now is also P10 D5's own "the hop's end" instant for the final hop's download phase — the
	// same instant elapsed is computed from, so download for the final hop is genuinely "how long
	// the body took", the number ElapsedMs has never separated out.
	now := time.Now()
	elapsed := now.Sub(start)

	encoding := "utf8"
	bodyStr := string(data)
	if !utf8.Valid(data) {
		encoding = "base64"
		bodyStr = base64.StdEncoding.EncodeToString(data)
	}

	finalURL := u.String()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	// finishFinal must run before sentCookies/receivedCookiesSnapshot below — it is what appends
	// the final hop's own Set-Cookie headers to the timeline's accumulator (timeline.go).
	timeline := tl.finishFinal(now, resp)

	return Response{
		Status:          resp.StatusCode,
		StatusText:      statusText(resp.Status),
		Proto:           resp.Proto,
		Headers:         flattenHeaders(resp.Header),
		Body:            bodyStr,
		BodyEncoding:    encoding,
		BodyBytes:       len(data),
		BodyTruncated:   truncated,
		ElapsedMs:       int(elapsed.Milliseconds()),
		FinalURL:        finalURL,
		Redirects:       tl.redirectHops(),
		Wire:            buildWireExchange(reqHead, dumpErr, httpReq, resp, req.Body, formBoundary, opts.WireMask),
		Timeline:        timeline,
		SentCookies:     tl.sentCookies(),
		ReceivedCookies: tl.receivedCookiesSnapshot(),
	}, nil
}

// prepareRequest resolves the multipart boundary (if any), builds the request body, and constructs
// the *http.Request through the body/GetBody/ContentLength assignment. A formdata body's boundary
// is resolved before buildBody runs, not after — a user-typed multipart/form-data Content-Type
// carrying its own boundary parameter must drive the actual multipart framing, and every other
// case mints one here. formBoundary and defaultContentType are returned for applyHeaders (and, for
// formBoundary, buildWireExchange later) to reuse without recomputing them.
func prepareRequest(sendCtx context.Context, req Request, u *url.URL) (httpReq *http.Request, formBoundary, defaultContentType string, err error) {
	userContentType, hasUserContentType := headerValue(req.Headers, "Content-Type")
	if req.Body.Mode == string(BodyFormData) {
		if hasUserContentType {
			if mt, params, mErr := mime.ParseMediaType(userContentType); mErr == nil && mt == "multipart/form-data" {
				formBoundary = params["boundary"]
			}
		}
		if formBoundary == "" {
			formBoundary = mintBoundary()
		}
	}

	bodyReader, getBody, length, defaultContentType, err := buildBody(req.Body, formBoundary)
	if err != nil {
		return nil, formBoundary, "", err
	}

	httpReq, err = http.NewRequestWithContext(sendCtx, req.Method, u.String(), nil)
	if err != nil {
		if bodyReader != nil {
			_ = bodyReader.Close()
		}
		return nil, formBoundary, defaultContentType, newErrorT(CodeBadRequest, "backend.studio.http.requestBuildFailed.error", map[string]string{"detail": err.Error()}, err)
	}
	// F4/F5: assigned explicitly rather than left to NewRequestWithContext's own auto-detection,
	// which only recognises *strings.Reader/*bytes.Reader/*bytes.Buffer — every streamed mode
	// (formdata, file) would otherwise get neither a working GetBody nor a known ContentLength.
	if bodyReader != nil {
		httpReq.Body = bodyReader
		httpReq.GetBody = getBody
		httpReq.ContentLength = length
	}
	return httpReq, formBoundary, defaultContentType, nil
}

// applyHeaders is F20a/P3 D7's own header-assignment pass: Host/User-Agent special-cased, every
// other header copied through, then Content-Type defaulted only when the user set none.
func applyHeaders(httpReq *http.Request, req Request, formBoundary, defaultContentType string) {
	// F20a: net/http silently ignores Header.Set("Host", …) — it writes req.Host/req.URL.Host
	// instead, so a user-typed Host: header must be assigned there explicitly or it does nothing.
	hasUserAgent := false
	for _, h := range req.Headers {
		if strings.EqualFold(h.Name, "Host") {
			httpReq.Host = h.Value
			continue
		}
		if strings.EqualFold(h.Name, "User-Agent") {
			hasUserAgent = true
		}
		httpReq.Header.Add(h.Name, h.Value)
	}
	if !hasUserAgent {
		// Go's default "Go-http-client/1.1" misrepresents the app; overridable by the user (D4).
		httpReq.Header.Set("User-Agent", "Kira Studio/"+buildinfo.Version)
	}

	// P3 D7: Content-Type is a default applied only when the user set none — matching Postman's
	// own "if you manually select a Content-Type header, that value takes precedence" (F3). The one
	// exception: a user-typed bare "multipart/form-data" (no boundary) can never be right, since the
	// boundary is minted here and unknowable to the user, so the boundary this send actually used
	// is appended to their value instead of left to silently mismatch the body.
	userContentType, hasUserContentType := headerValue(req.Headers, "Content-Type")
	switch {
	case !hasUserContentType:
		if defaultContentType != "" {
			httpReq.Header.Set("Content-Type", defaultContentType)
		}
	case req.Body.Mode == string(BodyFormData):
		if mt, params, mErr := mime.ParseMediaType(userContentType); mErr == nil && mt == "multipart/form-data" {
			if _, ok := params["boundary"]; !ok {
				httpReq.Header.Set("Content-Type", userContentType+"; boundary="+formBoundary)
			}
		}
	}
}

// readResponseBody reads the response body, capped at maxResponseBytes (D4's own cap; <= 0 means
// unbounded) and reporting whether it truncated.
func readResponseBody(resp *http.Response, maxResponseBytes int64) (data []byte, truncated bool, err error) {
	var reader io.Reader = resp.Body
	if maxResponseBytes > 0 {
		reader = io.LimitReader(resp.Body, maxResponseBytes+1)
	}
	data, err = io.ReadAll(reader)
	if err != nil {
		return nil, false, err
	}
	if maxResponseBytes > 0 && int64(len(data)) > maxResponseBytes {
		data = data[:maxResponseBytes]
		truncated = true
	}
	return data, truncated, nil
}

// buildWireExchange assembles the rendering from what Send already computed — never fatal
// (the rule: "a debugging view must never be the reason a send fails"), so any error along the
// way (the head dump, the body rendering, the response-head dump) is logged and leaves the
// exchange nil rather than propagated.
func buildWireExchange(
	reqHead []byte, dumpErr error, httpReq *http.Request, resp *http.Response, body Body, formBoundary string,
	mask *strings.Replacer,
) *WireExchange {
	if dumpErr != nil {
		slog.Warn("rendering the outgoing request failed", "scope", "httpclient", "err", dumpErr)
		return nil
	}
	bodyText, elided, bodyErr := renderRequestBody(body, formBoundary, httpReq.ContentLength, mask)
	if bodyErr != nil {
		slog.Warn("rendering the request body failed", "scope", "httpclient", "err", bodyErr)
		return nil
	}
	responseHead, headErr := renderResponseHead(resp)
	if headErr != nil {
		slog.Warn("rendering the response head failed", "scope", "httpclient", "err", headErr)
		return nil
	}
	return &WireExchange{
		Request:           string(reqHead) + bodyText,
		ResponseHead:      responseHead,
		Fidelity:          classifyFidelity(resp, httpReq),
		RequestBodyElided: elided,
	}
}
