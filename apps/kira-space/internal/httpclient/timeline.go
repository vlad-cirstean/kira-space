package httpclient

import (
	"crypto/tls"
	"math"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"sync"
	"time"
)

// maxHopHeaderBytes — http.Transport.MaxResponseHeaderBytes defaults to 10 MB and sharedClient
// leaves it unset, so a 10-hop redirect chain could otherwise carry ~100 MB of response headers
// into Response and into kira.sqlite. Capped here, with the truncation reported (HeadersElided)
// rather than hidden.
const maxHopHeaderBytes = 8 * 1024

// Phase is one measured interval within a hop. A nil *Phase (every use below is a pointer with
// `omitempty`) means the phase did not happen at all — a reused connection has no DNS/connect/TLS,
// a literal-IP URL has no DNS, a plain-http URL has no TLS — which is a different fact
// from a phase that took no measurable time.
type Phase struct {
	StartOffsetMs float64 `json:"startOffsetMs"` // from the send's t0, not the hop's own start
	DurationMs    float64 `json:"durationMs"`
}

// TimelineHop is one hop's full detail — every redirect hop plus the final one (D9). Method,
// StatusText and Headers are exactly what F13 found `checkRedirect` already has access to and
// today drops on the floor.
type TimelineHop struct {
	Index      int    `json:"index"`
	Method     string `json:"method"`
	URL        string `json:"url"`    // the URL that produced this hop's own response
	Status     int    `json:"status"` // 0 when this hop never got a response (a failed send, D15)
	StatusText string `json:"statusText"`
	Proto      string `json:"proto"`
	// Headers/HeadersElided are populated for intermediate hops only — the final hop's headers
	// are already Response.Headers, and duplicating the largest header set into both the live
	// response and (via D10) the stored snapshot buys nothing (D9).
	Headers       []Header `json:"headers,omitempty"`
	HeadersElided bool     `json:"headersElided,omitempty"`
	Reused        bool     `json:"reused"`
	IdleMs        float64  `json:"idleMs,omitempty"`
	// RemoteAddr is GotConnInfo.Conn.RemoteAddr() — the peer actually spoken to, which behind a
	// proxy is the proxy (F12) — never GetConn's own hostPort argument, which is derived from the
	// transport's internal connect-method key and is not a stable identifier for anything (D9).
	RemoteAddr string `json:"remoteAddr,omitempty"`
	// ConnAttempts counts GetConn calls within this one hop — normally 1; >1 means the transport
	// found its pooled connection dead and dialed again (F8/D7), never a phantom extra hop.
	ConnAttempts int `json:"connAttempts"`
	// Info1xx records any informational response seen before the final one (F9) — GotFirstResponseByte
	// fires on the first byte of a 1xx too, so the wait phase's own end can predate "the response"
	// in the everyday sense; this is what makes that legible rather than surprising.
	Info1xx       []int   `json:"info1xx,omitempty"`
	StartOffsetMs float64 `json:"startOffsetMs"` // from the send's t0
	TotalMs       float64 `json:"totalMs"`
	// Error is set only on the hop a failed send died on (D15) — every other hop that completed
	// normally leaves this empty.
	Error string `json:"error,omitempty"`

	DNS      *Phase `json:"dns,omitempty"`
	Connect  *Phase `json:"connect,omitempty"`
	TLS      *Phase `json:"tls,omitempty"`
	Wait     *Phase `json:"wait,omitempty"`
	Download *Phase `json:"download,omitempty"`
}

// Timeline is Response.Timeline — the full chronological sequence of what happened while the
// request ran. Hops[:len-1] are exactly the same hops Response.Redirects projects; the last hop
// is the one that produced Response's own Status/Headers/Body. TotalMs is measured directly
// (time.Since over the whole send), never summed from the hops' own totals — the same "never claim
// time it did not measure" rule for a hop's own phases.
type Timeline struct {
	Hops    []TimelineHop `json:"hops"`
	TotalMs float64       `json:"totalMs"`
}

type timelineCtxKey struct{}

// hop is one bucket under construction: TimelineHop plus the raw timestamps its phases are
// computed from. The embedded timestamps are unexported and so never marshaled — only the
// finished TimelineHop values (timeline.snapshot) ever leave this file.
type hop struct {
	TimelineHop
	openedAt time.Time

	dnsStart, dnsDone         time.Time
	connectStart, connectDone time.Time
	tlsStart, tlsDone         time.Time
	wroteRequest, firstByte   time.Time

	// sentCookieValues is every Cookie header value httptrace.WroteHeaderField saw written on this
	// hop — the jar injects Cookie inside net/http, after CheckRedirect, so there is nowhere else
	// to read what went out (client.go's own Send never sees the final wire headers, only the ones
	// it built before net/http added the jar's own cookies).
	sentCookieValues []string
}

// timeline is one Send call's collector, installed once on sendCtx and inherited across every
// hop because net/http's redirect path builds each subsequent request with ctx: ireq.ctx.
// checkRedirect closes the current hop and opens the next — it is the only
// delimiter that cannot invent a phantom hop from a mid-hop connection retry.
//
// The mutex is required, not defensive: httptrace hooks run on the transport's own dial and read
// goroutines while checkRedirect and Send run on the caller's — 8 concurrent sends
// of a 3-redirect chain ran under -race clean with exactly this locking shape.
type timeline struct {
	mu    sync.Mutex
	start time.Time
	hops  []*hop

	// receivedCookies is every Set-Cookie parsed from a hop's raw response headers, as
	// they arrive — closeHop and finishFinal both append to this directly from resp.Header, before
	// capHopHeaders (below) ever elides anything from the hop's own displayed Headers, so a long
	// Set-Cookie never silently vanishes from the Cookies tab the way a capped header would.
	receivedCookies []Cookie
}

// newTimeline opens hop 0 with the original request's own method/URL already known — Send calls
// this after resolveURL succeeds, so there is no window where hop 0 is open but unaddressed.
func newTimeline(method, url string) *timeline {
	now := time.Now()
	return &timeline{
		start: now,
		hops:  []*hop{{TimelineHop: TimelineHop{Index: 0, Method: method, URL: url}, openedAt: now}},
	}
}

// current is the open (last) hop — never nil: newTimeline always seeds hop 0, and every close
// immediately opens the next.
func (tl *timeline) current() *hop {
	return tl.hops[len(tl.hops)-1]
}

// with runs f against the open hop under the collector's lock — every trace hook's own body.
func (tl *timeline) with(f func(h *hop)) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	f(tl.current())
}

func round3(ms float64) float64 {
	return math.Round(ms*1000) / 1000
}

// offsetMs is every StartOffsetMs's own definition: milliseconds from the send's t0, the same
// origin for every hop and every phase inside it (D4/D9) — never relative to the hop's own start,
// which is what lets the pane place a hop's bar and its phases inside one shared, full-width
// track.
func (tl *timeline) offsetMs(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return round3(t.Sub(tl.start).Seconds() * 1000)
}

func (tl *timeline) phase(from, to time.Time) *Phase {
	return &Phase{StartOffsetMs: tl.offsetMs(from), DurationMs: round3(to.Sub(from).Seconds() * 1000)}
}

// capHopHeaders is F21/D9's 8 KiB cap, summed by rendered "name: value" length — truncating the
// list once exceeded rather than copying an adversarial server's megabytes of headers into
// Response and, via D10, into kira.sqlite.
func capHopHeaders(headers []Header) ([]Header, bool) {
	size := 0
	for i, h := range headers {
		size += len(h.Name) + len(h.Value) + 2
		if size > maxHopHeaderBytes {
			return headers[:i], true
		}
	}
	return headers, false
}

// fillConnectionPhases is the one piece shared by a normally-closed hop and a failed one (D15):
// DNS/connect/TLS are measured exactly the same way whether or not a response ever arrived.
func (tl *timeline) fillConnectionPhases(h *hop) {
	if !h.dnsStart.IsZero() && !h.dnsDone.IsZero() {
		h.DNS = tl.phase(h.dnsStart, h.dnsDone)
	}
	if !h.connectStart.IsZero() && !h.connectDone.IsZero() {
		h.Connect = tl.phase(h.connectStart, h.connectDone)
	}
	if !h.tlsStart.IsZero() && !h.tlsDone.IsZero() {
		h.TLS = tl.phase(h.tlsStart, h.tlsDone)
	}
}

// finish stamps a hop that got a real response — an intermediate redirect (headers non-nil) or
// the final one (headers nil, the do-not-duplicate rule). now is the hop's own end instant:
// checkRedirect's own moment for an intermediate hop (net/http drains and closes the redirect
// body just before it), or the moment io.ReadAll returns for the final hop.
func (tl *timeline) finish(
	h *hop, now time.Time, status int, statusText, proto string, headers []Header,
) {
	h.Status = status
	h.StatusText = statusText
	h.Proto = proto
	if headers != nil {
		h.Headers, h.HeadersElided = capHopHeaders(headers)
	}
	h.StartOffsetMs = tl.offsetMs(h.openedAt)
	h.TotalMs = round3(now.Sub(h.openedAt).Seconds() * 1000)
	tl.fillConnectionPhases(h)
	// Guarded against both of the measured cases — a server may answer before the request is
	// fully written (a 1xx, or a rejection mid-upload), in which case GotFirstResponseByte
	// precedes WroteRequest, or WroteRequest never fires at all because the transport abandoned
	// the body write. Either way there is no meaningful wait interval, and reporting a negative or
	// decades-long one is worse than reporting none.
	if !h.wroteRequest.IsZero() && !h.firstByte.IsZero() && !h.firstByte.Before(h.wroteRequest) {
		h.Wait = tl.phase(h.wroteRequest, h.firstByte)
	}
	// D5: download runs from the first response byte to the hop's own end — never absent for a
	// hop that got a response at all.
	if !h.firstByte.IsZero() {
		h.Download = tl.phase(h.firstByte, now)
	}
}

// openHop appends the next bucket, already addressed with the request that is about to run —
// known at this point because checkRedirect's own req argument (or, for hop 0, Send's already-
// resolved request) is exactly that request, so a failed later hop never has to guess which
// URL it was trying.
func (tl *timeline) openHop(method, url string) {
	tl.hops = append(tl.hops, &hop{
		TimelineHop: TimelineHop{Index: len(tl.hops), Method: method, URL: url},
		openedAt:    time.Now(),
	})
}

// closeHop is checkRedirect's own call (F2/F13): prev is the hop that just finished (its method,
// URL and the redirect response that closed it), next is the request about to be issued.
func (tl *timeline) closeHop(resp *http.Response, nextMethod, nextURL string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	h := tl.current()
	tl.receivedCookies = append(tl.receivedCookies, receivedCookiesFromHeader(resp.Header, h.Index)...)
	tl.finish(h, time.Now(), resp.StatusCode, resp.Status, resp.Proto, flattenHeaders(resp.Header))
	tl.openHop(nextMethod, nextURL)
}

// finishFinal closes the timeline's last hop from the response Send already has (F13) and
// returns the whole timeline. now is the same instant Send's own `elapsed` is computed at
// (§1.1/D5), so the final hop's download phase is genuinely "how long the body took".
func (tl *timeline) finishFinal(now time.Time, resp *http.Response) Timeline {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	h := tl.current()
	tl.receivedCookies = append(tl.receivedCookies, receivedCookiesFromHeader(resp.Header, h.Index)...)
	tl.finish(h, now, resp.StatusCode, resp.Status, resp.Proto, nil)
	return tl.snapshotLocked(now)
}

// finishFailed closes the timeline's open hop with the error the send died on — used only
// when Send returns before a response ever arrived for this hop (a refused connect, a DNS
// failure, a mid-body read failure). There is no status, no proto and no wait/download phase,
// since none of those were ever measured; DNS/connect/TLS are filled exactly as they would be for
// a successful hop, because those are real and complete right up to the failure.
func (tl *timeline) finishFailed(now time.Time, errMsg string) Timeline {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	h := tl.current()
	h.Error = errMsg
	h.StartOffsetMs = tl.offsetMs(h.openedAt)
	h.TotalMs = round3(now.Sub(h.openedAt).Seconds() * 1000)
	tl.fillConnectionPhases(h)
	return tl.snapshotLocked(now)
}

// snapshotLocked copies every hop's finished TimelineHop into the wire Timeline. Called only from
// inside a method that already holds tl.mu.
func (tl *timeline) snapshotLocked(now time.Time) Timeline {
	hops := make([]TimelineHop, len(tl.hops))
	for i, h := range tl.hops {
		hops[i] = h.TimelineHop
	}
	return Timeline{Hops: hops, TotalMs: round3(now.Sub(tl.start).Seconds() * 1000)}
}

// redirectHops projects every hop but the last into RedirectHop — the existing field is
// derived from the timeline rather than collected a second time, so the two can never drift
// apart. Byte-identical to what the pre-phase checkRedirect produced (client_test.go's own
// TestSend_RedirectChain passes unedited on this).
func (tl *timeline) redirectHops() []RedirectHop {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if len(tl.hops) <= 1 {
		return []RedirectHop{}
	}
	out := make([]RedirectHop, 0, len(tl.hops)-1)
	for _, h := range tl.hops[:len(tl.hops)-1] {
		out = append(out, RedirectHop{Status: h.Status, URL: h.URL})
	}
	return out
}

// sentCookies is P90 item 2's own projection: every Cookie header value seen written on any hop
// (sentCookieValues), parsed into individual cookies and tagged with the hop they went out on.
func (tl *timeline) sentCookies() []Cookie {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	var out []Cookie
	for _, h := range tl.hops {
		for _, v := range h.sentCookieValues {
			out = append(out, sentCookiesFromHeaderValue(v, h.Index)...)
		}
	}
	return out
}

// receivedCookiesSnapshot copies out what closeHop/finishFinal have accumulated so far.
func (tl *timeline) receivedCookiesSnapshot() []Cookie {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	if len(tl.receivedCookies) == 0 {
		return nil
	}
	out := make([]Cookie, len(tl.receivedCookies))
	copy(out, tl.receivedCookies)
	return out
}

// trace builds the httptrace.ClientTrace installed once on sendCtx. Every hook records a
// timestamp or a flag and returns — there is no error path, so a hook can never fail a send, and
// the cost (a handful of time.Now() calls and uncontended mutex acquisitions per hop) is
// unmeasurable against a request whose cheapest measured hop was 0.4 ms and whose realistic
// wait is tens of milliseconds.
func (tl *timeline) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			tl.with(func(h *hop) { h.ConnAttempts++ })
		},
		GotConn: func(info httptrace.GotConnInfo) {
			tl.with(func(h *hop) {
				// D7: the *last* GetConn/GotConn wins for Reused/RemoteAddr — it describes the
				// connection the request actually went out on, not a dead pooled one a retry
				// discarded (F8).
				h.Reused = info.Reused
				if info.WasIdle {
					h.IdleMs = round3(info.IdleTime.Seconds() * 1000)
				}
				if info.Conn != nil {
					h.RemoteAddr = info.Conn.RemoteAddr().String()
				}
			})
		},
		DNSStart: func(_ httptrace.DNSStartInfo) {
			tl.with(func(h *hop) {
				// D7: the *first* DNSStart/ConnectStart wins, so a mid-hop retry's wasted attempt
				// is included in the phase's span rather than invented as a second phase.
				if h.dnsStart.IsZero() {
					h.dnsStart = time.Now()
				}
			})
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			tl.with(func(h *hop) { h.dnsDone = time.Now() })
		},
		ConnectStart: func(_, _ string) {
			tl.with(func(h *hop) {
				if h.connectStart.IsZero() {
					h.connectStart = time.Now()
				}
			})
		},
		ConnectDone: func(_, _ string, _ error) {
			tl.with(func(h *hop) { h.connectDone = time.Now() })
		},
		TLSHandshakeStart: func() {
			tl.with(func(h *hop) {
				if h.tlsStart.IsZero() {
					h.tlsStart = time.Now()
				}
			})
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			tl.with(func(h *hop) { h.tlsDone = time.Now() })
		},
		WroteRequest: func(_ httptrace.WroteRequestInfo) {
			tl.with(func(h *hop) { h.wroteRequest = time.Now() })
		},
		// P90 item 2: the only place a Cookie header's actual sent value can be read — the jar (if
		// any) injects it inside net/http's own Client.send, strictly after CheckRedirect runs and
		// after this hop's request was built, so nothing upstream of the wire write ever sees it.
		WroteHeaderField: func(key string, value []string) {
			if textproto.CanonicalMIMEHeaderKey(key) != "Cookie" {
				return
			}
			tl.with(func(h *hop) { h.sentCookieValues = append(h.sentCookieValues, value...) })
		},
		GotFirstResponseByte: func() {
			tl.with(func(h *hop) {
				if h.firstByte.IsZero() {
					h.firstByte = time.Now()
				}
			})
		},
		Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
			tl.with(func(h *hop) { h.Info1xx = append(h.Info1xx, code) })
			return nil
		},
	}
}
