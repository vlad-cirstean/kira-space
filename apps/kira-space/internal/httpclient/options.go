package httpclient

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Options is one send's resolved client configuration. Every field is a pointer and
// nil means "this package's own default" — never the type's zero value, which for SSLVerify would
// mean "verification off". bridge/http.go always passes a fully-populated Options (resolved
// against model.ApiSettings, packages/shared/domain/settings.ts's own apiSettingsSchema); the nil
// handling exists for this package's own tests and for any future direct caller.
type Options struct {
	HTTPVersion      *string `json:"httpVersion,omitempty"` // "1.1" | "2"
	RequestTimeoutMs *int    `json:"requestTimeoutMs,omitempty"`
	MaxResponseMb    *int    `json:"maxResponseMb,omitempty"`
	SSLVerify        *bool   `json:"sslVerify,omitempty"`
	FollowRedirects  *bool   `json:"followRedirects,omitempty"`
	MaxRedirects     *int    `json:"maxRedirects,omitempty"`
	DisableCookieJar *bool   `json:"disableCookieJar,omitempty"`
	// Ephemeral routes this send's cookies to a throwaway jar instead of the shared one — set by
	// bridge/http.go for an incognito tab (cookies.go). Never surfaced in Settings.
	Ephemeral bool `json:"ephemeral,omitempty"`
	// WireMask: bridge/http.go builds this from the secrets stage 2 substituted (apivars.UsedSecret
	// has already been resolved by the time it calls Send) and passes it in so wire.go's
	// capWireText can mask a body BEFORE truncating it to the cap, never after. Masking only after
	// capping (the old order — mask the whole already-built WireExchange.Request in
	// bridge/http.go's maskSecrets) can leave an unmasked secret prefix when the secret straddles
	// the cut: the kept bytes no longer match the full registered pattern, so the replacer can't
	// find it. httpclient never sees apivars.UsedSecret directly — only this already-built stdlib
	// Replacer — so this package takes on no dependency on apivars. Never populated from IPC JSON.
	WireMask *strings.Replacer `json:"-"`
	// RedirectGuard vets each redirect target before it is followed; an error stops the chain and
	// fails the send. Only the agent request path sets it. Never populated from IPC JSON.
	RedirectGuard func(next *url.URL) error `json:"-"`
}

// resolved is Options with every field decided and clamped. Nothing below this line reads Options.
type resolved struct {
	http1            bool
	timeout          time.Duration // 0 = none
	maxResponseBytes int64         // 0 = unlimited
	sslVerify        bool
	followRedirects  bool
	maxRedirects     int
	useJar           bool
	ephemeral        bool
	redirectGuard    func(next *url.URL) error
}

// normalize resolves every field to a concrete value, clamping to the same bounds
// packages/shared/domain/settings.ts's REQUEST_TIMEOUT_MS_RANGE/MAX_RESPONSE_MB_RANGE/
// MAX_REDIRECTS_RANGE state — a value out of range reaches here only via a hand-edited or
// stored tab row, never the dialog's own bounded inputs, so clamping (not rejecting) is
// what keeps a send working rather than failing on stale local state.
//
// Every default below must equal model.DefaultSettings().Api field for field — internal/storage/
// model/settings.go states the same coupling back at this file, since neither package may import
// the other.
func (o Options) normalize() resolved {
	r := resolved{
		http1:            false,
		timeout:          0,
		maxResponseBytes: 5 * 1024 * 1024,
		sslVerify:        true,
		followRedirects:  true,
		maxRedirects:     10,
		useJar:           false,
		ephemeral:        o.Ephemeral,
		redirectGuard:    o.RedirectGuard,
	}
	if o.HTTPVersion != nil {
		r.http1 = *o.HTTPVersion == "1.1"
	}
	if o.RequestTimeoutMs != nil {
		ms := min(max(*o.RequestTimeoutMs, 0), 3_600_000)
		if ms == 0 {
			r.timeout = 0
		} else {
			r.timeout = time.Duration(ms) * time.Millisecond
		}
	}
	if o.MaxResponseMb != nil {
		mb := min(max(*o.MaxResponseMb, 0), 2048)
		if mb == 0 {
			r.maxResponseBytes = 0
		} else {
			r.maxResponseBytes = int64(mb) * 1024 * 1024
		}
	}
	if o.SSLVerify != nil {
		r.sslVerify = *o.SSLVerify
	}
	if o.FollowRedirects != nil {
		r.followRedirects = *o.FollowRedirects
	}
	if o.MaxRedirects != nil {
		r.maxRedirects = min(max(*o.MaxRedirects, 0), 100)
	}
	if o.DisableCookieJar != nil {
		r.useJar = !*o.DisableCookieJar
	}
	return r
}

// transportKey is the only two fields of a send's resolved configuration that change how a
// *http.Transport itself is built — everything else (redirects, jar, timeout) varies per
// *http.Client instead, so connection reuse (sharedClient's whole reason to exist) survives
// per-send Options.
// maxResponseHeaderBytes bounds one response's header block.
const maxResponseHeaderBytes = 1 << 20

type transportKey struct {
	http1      bool
	skipVerify bool
}

var (
	transportsMu sync.Mutex
	transports   = map[transportKey]*http.Transport{}
)

// transportFor returns the cached transport for k, building it once lazily — at most four ever
// exist (2 HTTP-version choices x 2 TLS-verify choices).
func transportFor(k transportKey) *http.Transport {
	transportsMu.Lock()
	defer transportsMu.Unlock()
	if tr, ok := transports[k]; ok {
		return tr
	}
	// Clone keeps DefaultTransport's dial, TLS-handshake and idle timeouts; a bare Transport has none.
	tr := http.DefaultTransport.(*http.Transport).Clone()
	// A hostile endpoint can otherwise stream 10 MiB of headers (Set-Cookie) per hop into the
	// timeline and the history snapshot.
	tr.MaxResponseHeaderBytes = maxResponseHeaderBytes
	p := new(http.Protocols)
	p.SetHTTP1(true)
	if !k.http1 {
		p.SetHTTP2(true)
	}
	// SetUnencryptedHTTP2 stays off deliberately: HTTP/2 negotiates over ALPN on https:// only,
	// which is today's behaviour and what the setting means — do not "fix" this into h2c.
	tr.Protocols = p
	if k.skipVerify {
		// The only place in this app that turns certificate verification off. Reachable only from
		// an explicit per-request or global "SSL certificate verification: off", never a default,
		// and the request editor shows a warning chip while it is off (RequestSettingsPane.vue).
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	transports[k] = tr
	return tr
}

// checkRedirectFor is checkRedirect (client.go) closed over one send's resolved
// redirect policy — everything but the two branches below (the follow-redirects gate and the
// max-redirects bound, both newly variable per-send) is unchanged from before this phase, in the
// same order: closeHop, then the cross-host header-stripping block.
func checkRedirectFor(r resolved) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if !r.followRedirects {
			// Before closeHop below: net/http returns the 3xx itself as the response when this
			// error is returned, so this hop is the *final* hop and finishFinal (client.go) is
			// what closes it — closing it here too, via closeHop, would record it twice.
			return http.ErrUseLastResponse
		}
		if len(via) >= r.maxRedirects {
			return fmt.Errorf("httpclient: stopped after %d redirects", r.maxRedirects)
		}
		if r.redirectGuard != nil {
			if err := r.redirectGuard(req.URL); err != nil {
				return err
			}
		}
		if tl, _ := req.Context().Value(timelineCtxKey{}).(*timeline); tl != nil && req.Response != nil {
			// F13: the hop's own method, status text and headers are all readable here and nowhere
			// else; req is the next request about to be issued, so its method/URL address the hop
			// this call is opening (D9) before a single byte of it has gone out.
			tl.closeHop(req.Response, req.Method, req.URL.String())
		}

		// net/http's redirect machinery re-copies every header from the
		// *original* request onto each new hop (makeHeadersCopier closes over ireq, not the
		// previous hop's request), stripping only Authorization/WWW-Authenticate/Cookie/Cookie2
		// when the host changed from the original. A saved request's own custom headers —
		// X-Api-Key, PRIVATE-TOKEN, X-Amz-Security-Token and the like are at least as common in
		// this app's requests as Authorization — would otherwise silently follow a redirect to a
		// different host, replaying a secret to whatever answered the redirect. Drop every header
		// the user typed the moment a hop crosses hosts.
		//
		// Comparing only against the immediately-previous hop misses A -> B -> B — the second
		// hop is same-host relative to B, but the headers being carried were re-copied from the
		// *original* A request, so what matters is whether the current hop differs from the
		// original, not from the previous one. Since net/http always re-copies from the original,
		// comparing every hop against via[0] here is both correct and sufficient — no separate
		// "stay stripped once crossed" state needed.
		if len(via) > 0 {
			stripOnOriginChange(req, via[0].URL)
		}
		return nil
	}
}

// stripOnOriginChange drops what must not follow a redirect that leaves origin's host, port or
// scheme security level.
func stripOnOriginChange(req *http.Request, origin *url.URL) {
	crossHost := !sameRedirectHost(origin, req.URL)
	portChange := isPortChange(origin, req.URL)
	downgrade := isSchemeDowngrade(origin, req.URL)
	if crossHost || portChange || downgrade {
		if names, ok := req.Context().Value(redirectHeaderNamesCtxKey{}).([]string); ok {
			for _, name := range names {
				req.Header.Del(name)
			}
		}
	}
	if portChange {
		// Same hostname, other service: net/http keeps the credential headers.
		for _, h := range []string{"Authorization", "WWW-Authenticate", "Cookie", "Cookie2"} {
			req.Header.Del(h)
		}
	}
	if downgrade {
		// F3: an https -> http downgrade on the same host passes net/http's own
		// isDomainOrSubdomain check (it never looks at scheme), so Authorization and
		// Cookie survive onto the plaintext hop unless stripped here explicitly.
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
	}
}
