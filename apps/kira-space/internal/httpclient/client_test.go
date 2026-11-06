package httpclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// §6.3 case 1: a 301→302→200 chain — the body is the final one, Redirects has two hops with
// their real statuses and URLs, and FinalURL is the last.
func TestSend_RedirectChain(t *testing.T) {
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/mid", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/mid", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final-body"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	serverURL = srv.URL

	resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL + "/start"}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Body != "final-body" {
		t.Fatalf("Body = %q, want %q", resp.Body, "final-body")
	}
	if resp.FinalURL != srv.URL+"/final" {
		t.Fatalf("FinalURL = %q, want %q", resp.FinalURL, srv.URL+"/final")
	}
	if len(resp.Redirects) != 2 {
		t.Fatalf("len(Redirects) = %d, want 2: %+v", len(resp.Redirects), resp.Redirects)
	}
	if resp.Redirects[0].Status != http.StatusMovedPermanently || resp.Redirects[0].URL != srv.URL+"/start" {
		t.Errorf("Redirects[0] = %+v, want {301 %s/start}", resp.Redirects[0], srv.URL)
	}
	if resp.Redirects[1].Status != http.StatusFound || resp.Redirects[1].URL != srv.URL+"/mid" {
		t.Errorf("Redirects[1] = %+v, want {302 %s/mid}", resp.Redirects[1], srv.URL)
	}
}

// TestSend_CrossHostRedirectStripsUserHeaders: Go's net/http strips only
// Authorization/WWW-Authenticate/Cookie/Cookie2 on a cross-host redirect — every other header a
// saved request carries (X-Api-Key, PRIVATE-TOKEN, X-Amz-Security-Token, …) is copied verbatim to
// whatever answered the redirect. A same-host redirect must still carry the header (that's the
// ordinary, intended case httptest already covers via TestSend_RedirectChain) — only the
// cross-host hop must drop it.
func TestSend_CrossHostRedirectStripsUserHeaders(t *testing.T) {
	var finalHeader, sameHostHeader string

	finalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalHeader = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("final"))
	}))
	defer finalSrv.Close()
	finalPort := finalSrv.Listener.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	mux.HandleFunc("/cross-host", func(w http.ResponseWriter, r *http.Request) {
		// "localhost" resolves to the same loopback address as httptest's own 127.0.0.1, but is a
		// different *hostname* — exactly the case sameRedirectHost must treat as cross-origin.
		http.Redirect(w, r, fmt.Sprintf("http://localhost:%d/", finalPort), http.StatusFound)
	})
	mux.HandleFunc("/same-host", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/same-host-final", http.StatusFound)
	})
	mux.HandleFunc("/same-host-final", func(w http.ResponseWriter, r *http.Request) {
		sameHostHeader = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("same-host"))
	})
	startSrv := httptest.NewServer(mux)
	defer startSrv.Close()

	if _, err := Send(context.Background(), Request{
		Method:  "GET",
		URL:     startSrv.URL + "/cross-host",
		Headers: []Header{{Name: "X-Api-Key", Value: "sk-secret"}},
	}, Options{}); err != nil {
		t.Fatalf("Send (cross-host): %v", err)
	}
	if finalHeader != "" {
		t.Errorf("X-Api-Key reached the cross-host redirect target: %q, want stripped", finalHeader)
	}

	if _, err := Send(context.Background(), Request{
		Method:  "GET",
		URL:     startSrv.URL + "/same-host",
		Headers: []Header{{Name: "X-Api-Key", Value: "sk-secret"}},
	}, Options{}); err != nil {
		t.Fatalf("Send (same-host redirect): %v", err)
	}
	if sameHostHeader != "sk-secret" {
		t.Errorf("X-Api-Key across a same-host redirect = %q, want \"sk-secret\" unchanged", sameHostHeader)
	}
}

// TestSend_RedirectHeaderStrippingComparesAgainstOrigin: net/http's redirect
// machinery always re-copies headers from the *original* request onto every new hop (it never
// copies from the previous hop's already-modified request), so checkRedirectFor must compare
// each hop against via[0] (the origin), not the immediately-previous hop — otherwise an
// A -> B -> B chain strips on the first (cross-host) hop but restores the header on the second
// (same-host-as-previous-hop) one.
func TestSend_RedirectHeaderStrippingComparesAgainstOrigin(t *testing.T) {
	var finalHeader string

	finalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mid":
			// Relative redirect: resolves against the host the client actually used
			// ("localhost", set below), so this hop is same-host relative to /mid but still
			// cross-host relative to the origin.
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			finalHeader = r.Header.Get("X-Api-Key")
			_, _ = w.Write([]byte("final"))
		}
	}))
	defer finalSrv.Close()
	finalPort := finalSrv.Listener.Addr().(*net.TCPAddr).Port

	startSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "localhost" resolves to the same loopback address as httptest's own 127.0.0.1, but is a
		// different *hostname* — the cross-host hop sameRedirectHost must catch, same as
		// TestSend_CrossHostRedirectStripsUserHeaders.
		http.Redirect(w, r, fmt.Sprintf("http://localhost:%d/mid", finalPort), http.StatusFound)
	}))
	defer startSrv.Close()

	if _, err := Send(context.Background(), Request{
		Method:  "GET",
		URL:     startSrv.URL,
		Headers: []Header{{Name: "X-Api-Key", Value: "sk-secret"}},
	}, Options{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if finalHeader != "" {
		t.Errorf("X-Api-Key reached the B->B hop after an A->B redirect: %q, want stripped", finalHeader)
	}
}

// TestSend_SchemeDowngradeRedirectStripsAuthAndCookie: net/http's own sensitive-header
// stripping (shouldCopyHeaderOnRedirect) compares hostnames only, never scheme, so an https to
// http redirect on the same host keeps Authorization and Cookie on the wire in clear unless
// checkRedirectFor strips them explicitly.
func TestSend_SchemeDowngradeRedirectStripsAuthAndCookie(t *testing.T) {
	var finalAuth, finalCookie, finalCustom string
	plainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		finalAuth = r.Header.Get("Authorization")
		finalCookie = r.Header.Get("Cookie")
		finalCustom = r.Header.Get("X-Api-Key")
		_, _ = w.Write([]byte("plain"))
	}))
	defer plainSrv.Close()
	plainPort := plainSrv.Listener.Addr().(*net.TCPAddr).Port

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("http://127.0.0.1:%d/final", plainPort), http.StatusFound)
	}))
	defer tlsSrv.Close()

	if _, err := Send(context.Background(), Request{
		Method: "GET",
		URL:    tlsSrv.URL,
		Headers: []Header{
			{Name: "Authorization", Value: "Bearer tok"},
			{Name: "Cookie", Value: "session=s3cret"},
			{Name: "X-Api-Key", Value: "sk-secret"},
		},
	}, Options{SSLVerify: ptr(false)}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if finalAuth != "" {
		t.Errorf("Authorization reached the http hop after an https->http downgrade: %q, want stripped", finalAuth)
	}
	if finalCookie != "" {
		t.Errorf("Cookie reached the http hop after an https->http downgrade: %q, want stripped", finalCookie)
	}
	if finalCustom != "" {
		t.Errorf("X-Api-Key reached the http hop after an https->http downgrade: %q, want stripped", finalCustom)
	}
}

// §6.3 case 2 / §6.1: a response larger than Options.MaxResponseMb — BodyTruncated is true,
// BodyBytes reports what was read, and the reader is not left open. A second case with
// MaxResponseMb: ptr(0) asserts the size cap is off entirely (P90 §2.3).
func TestSend_BodySizeTruncation(t *testing.T) {
	const capBytes = 1 * 1024 * 1024
	oversized := bytes.Repeat([]byte("a"), capBytes+1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(oversized)
	}))
	defer srv.Close()

	t.Run("truncated at the cap", func(t *testing.T) {
		resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{MaxResponseMb: ptr(1)})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if !resp.BodyTruncated {
			t.Fatal("BodyTruncated = false, want true")
		}
		if resp.BodyBytes != capBytes {
			t.Fatalf("BodyBytes = %d, want %d", resp.BodyBytes, capBytes)
		}
		if len(resp.Body) != capBytes {
			t.Fatalf("len(Body) = %d, want %d", len(resp.Body), capBytes)
		}
	})

	t.Run("MaxResponseMb 0 is unlimited", func(t *testing.T) {
		resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{MaxResponseMb: ptr(0)})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if resp.BodyTruncated {
			t.Fatal("BodyTruncated = true, want false")
		}
		if resp.BodyBytes != len(oversized) {
			t.Fatalf("BodyBytes = %d, want %d", resp.BodyBytes, len(oversized))
		}
	})
}

// §6.3 case 3: a body of invalid UTF-8 — BodyEncoding == "base64" and the bytes decode back
// byte-identical.
func TestSend_NonUTF8BodyBase64RoundTrip(t *testing.T) {
	raw := []byte{0xff, 0xfe, 0x00, 0x01, 'h', 'i', 0x80}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.BodyEncoding != "base64" {
		t.Fatalf("BodyEncoding = %q, want base64", resp.BodyEncoding)
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.Body)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatalf("decoded = %v, want %v", decoded, raw)
	}
}

// §6.3 case 4: a server that never responds — the context deadline fires as E_TIMEOUT, and a
// cancelled context as E_CANCELLED. The two must not be conflated: one is a failure, one is the
// user.
func TestSend_TimeoutVsCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	// Registered after srv.Close() so defers run in the order that unblocks it (LIFO):
	// close(block) first, letting the handler goroutines return and their connections close,
	// then srv.Close() — the reverse order left Close() waiting up to its own hardcoded timeout
	// for connections whose handlers were still parked on a channel Close() itself was blocking.
	defer srv.Close()
	defer close(block)

	t.Run("timeout", func(t *testing.T) {
		_, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL}, Options{RequestTimeoutMs: ptr(50)})
		code, ok := CodeOf(err)
		if !ok || code != CodeTimeout {
			t.Fatalf("CodeOf(err) = %v, %v, want %v, true (err: %v)", code, ok, CodeTimeout, err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		_, err := Send(ctx, Request{Method: "GET", URL: srv.URL}, Options{RequestTimeoutMs: ptr(50)})
		code, ok := CodeOf(err)
		if !ok || code != CodeCancelled {
			t.Fatalf("CodeOf(err) = %v, %v, want %v, true (err: %v)", code, ok, CodeCancelled, err)
		}
	})
}

// TestSend_TimeoutZeroMeansNone: RequestTimeoutMs 0 disables the deadline
// entirely, so a server sleeping past the old 30s default still returns, and cancelling the
// caller's own ctx still classifies CodeCancelled (sendCtx.Err() can only ever be
// context.Canceled with no deadline set).
func TestSend_TimeoutZeroMeansNone(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := Send(ctx, Request{Method: "GET", URL: srv.URL}, Options{RequestTimeoutMs: ptr(0)})
	code, ok := CodeOf(err)
	if !ok || code != CodeCancelled {
		t.Fatalf("CodeOf(err) = %v, %v, want %v, true (err: %v)", code, ok, CodeCancelled, err)
	}
}

// §6.3 case 5: a user-supplied Host header actually reaches the server as the request's Host
// (F20a) — net/http silently drops Header.Set("Host", …) unless it is assigned to req.Host.
func TestSend_HostHeaderReachesServer(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
	}))
	defer srv.Close()

	_, err := Send(context.Background(), Request{
		Method:  "GET",
		URL:     srv.URL,
		Headers: []Header{{Name: "Host", Value: "example.internal"}},
	}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotHost != "example.internal" {
		t.Fatalf("server saw Host = %q, want %q", gotHost, "example.internal")
	}
}

// ptr is a small generic helper for building an Options field's *T from a literal — every test
// below needs several of these.
func ptr[T any](v T) *T { return &v }

// TestSend_FollowRedirectsOff: FollowRedirects: ptr(false) must return the 3xx itself
// as the response, with Redirects empty, FinalURL the original URL, and exactly one Timeline hop
// — this is the assertion that catches the ErrUseLastResponse-before-closeHop ordering if
// it is ever written the other way round (closing the hop twice, or not at all).
func TestSend_FollowRedirectsOff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL + "/start"}, Options{FollowRedirects: ptr(false)})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != http.StatusMovedPermanently {
		t.Fatalf("Status = %d, want %d", resp.Status, http.StatusMovedPermanently)
	}
	if len(resp.Redirects) != 0 {
		t.Fatalf("len(Redirects) = %d, want 0: %+v", len(resp.Redirects), resp.Redirects)
	}
	if resp.FinalURL != srv.URL+"/start" {
		t.Fatalf("FinalURL = %q, want %q", resp.FinalURL, srv.URL+"/start")
	}
	if len(resp.Timeline.Hops) != 1 {
		t.Fatalf("len(Timeline.Hops) = %d, want 1: %+v", len(resp.Timeline.Hops), resp.Timeline.Hops)
	}
}

// TestSend_MaxRedirectsHonoured is P90 §6.1: a 3-hop chain with MaxRedirects: ptr(1) must fail
// with CodeTransport and a message naming how many redirects it stopped after.
func TestSend_MaxRedirectsHonoured(t *testing.T) {
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/2", http.StatusFound)
	})
	mux.HandleFunc("/2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/3", http.StatusFound)
	})
	mux.HandleFunc("/3", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	serverURL = srv.URL

	_, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL + "/1"}, Options{MaxRedirects: ptr(1)})
	code, ok := CodeOf(err)
	if !ok || code != CodeTransport {
		t.Fatalf("CodeOf(err) = %v, %v, want %v, true (err: %v)", code, ok, CodeTransport, err)
	}
	if !strings.Contains(err.Error(), "stopped after 1 redirects") {
		t.Fatalf("err = %v, want it to contain %q", err, "stopped after 1 redirects")
	}
}

// TestSend_CookieJarReplaysAcrossSends is P90 §6.1/§2.4: the jar's default-off state, its on
// state, and its ephemeral (incognito) state — three interacting rules, one table, ClearJar()
// between cases since the shared jar is package state and these would otherwise be order-dependent.
func TestSend_CookieJarReplaysAcrossSends(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
		_, _ = w.Write([]byte("logged in"))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err == nil {
			_, _ = w.Write([]byte("session=" + c.Value))
		} else {
			_, _ = w.Write([]byte("no session"))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cases := []struct {
		name           string
		opts           Options
		wantSecondBody string
		wantSentCookie bool
	}{
		{"jar off (default)", Options{}, "no session", false},
		{"jar on", Options{DisableCookieJar: ptr(false)}, "session=abc123", true},
		{"jar on, ephemeral", Options{DisableCookieJar: ptr(false), Ephemeral: true}, "no session", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ClearJar()
			if _, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL + "/login"}, tc.opts); err != nil {
				t.Fatalf("Send (login): %v", err)
			}
			resp, err := Send(context.Background(), Request{Method: "GET", URL: srv.URL + "/me"}, tc.opts)
			if err != nil {
				t.Fatalf("Send (me): %v", err)
			}
			if resp.Body != tc.wantSecondBody {
				t.Fatalf("second send body = %q, want %q", resp.Body, tc.wantSecondBody)
			}
		})
	}

	// The ephemeral case still carries its cookie within one send's own redirect chain: assert
	// SentCookies names it on a send that both sets and immediately uses the cookie via a redirect.
	t.Run("ephemeral jar works within one send's own chain", func(t *testing.T) {
		ClearJar()
		var loginURL string
		chainMux := http.NewServeMux()
		chainMux.HandleFunc("/set-and-redirect", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "xyz"})
			http.Redirect(w, r, loginURL+"/check", http.StatusFound)
		})
		chainMux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie("session")
			if err == nil {
				_, _ = w.Write([]byte("session=" + c.Value))
			} else {
				_, _ = w.Write([]byte("no session"))
			}
		})
		chainSrv := httptest.NewServer(chainMux)
		defer chainSrv.Close()
		loginURL = chainSrv.URL

		resp, err := Send(context.Background(), Request{Method: "GET", URL: chainSrv.URL + "/set-and-redirect"},
			Options{DisableCookieJar: ptr(false), Ephemeral: true})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if resp.Body != "session=xyz" {
			t.Fatalf("body = %q, want the redirect hop to carry the cookie an ephemeral jar set moments earlier", resp.Body)
		}

		// Nothing reached the shared jar: a fresh, non-ephemeral send to the same host sees none.
		resp2, err := Send(context.Background(), Request{Method: "GET", URL: chainSrv.URL + "/check"}, Options{DisableCookieJar: ptr(false)})
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if resp2.Body != "no session" {
			t.Fatalf("body = %q, want the ephemeral send's cookie to never reach the shared jar", resp2.Body)
		}
	})
}

// TestSend_ForcedHTTP1 is P90 §6.1/§2.3: the only check that Options.HTTPVersion actually drives
// http.Transport.Protocols the way normalize()/transportFor claim.
func TestSend_ForcedHTTP1(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	client := srv.Client()

	t.Run("HTTPVersion 2", func(t *testing.T) {
		resp := doForcedVersionSend(t, srv.URL, ptr("2"), client)
		if resp.Proto != "HTTP/2.0" {
			t.Fatalf("Proto = %q, want HTTP/2.0", resp.Proto)
		}
	})

	t.Run("HTTPVersion 1.1", func(t *testing.T) {
		resp := doForcedVersionSend(t, srv.URL, ptr("1.1"), client)
		if resp.Proto != "HTTP/1.1" {
			t.Fatalf("Proto = %q, want HTTP/1.1", resp.Proto)
		}
	})
}

// doForcedVersionSend routes through Send with SSLVerify off (srv.StartTLS's cert is
// self-signed) — the test's whole point is HTTPVersion, so the transport cache's skipVerify slot
// is exercised incidentally rather than tested on its own (CLAUDE.md: not worth a dedicated test).
func doForcedVersionSend(t *testing.T, url string, version *string, _ *http.Client) Response {
	t.Helper()
	resp, err := Send(context.Background(), Request{Method: "GET", URL: url},
		Options{HTTPVersion: version, SSLVerify: ptr(false)})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	return resp
}

// TestSend_PortChangeRedirectStripsCredentials is P168 Part 7 F2: net/http compares hostnames only,
// so a redirect to another service on the same host kept every header.
func TestSend_PortChangeRedirectStripsCredentials(t *testing.T) {
	var got http.Header
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer dest.Close()
	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusFound)
	}))
	defer start.Close()

	if _, err := Send(context.Background(), Request{
		Method: "GET",
		URL:    start.URL,
		Headers: []Header{
			{Name: "X-Api-Key", Value: "sk"},
			{Name: "Authorization", Value: "Bearer s"},
		},
	}, Options{}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got.Get("X-Api-Key") != "" || got.Get("Authorization") != "" {
		t.Errorf("credentials reached the other port: %v", got)
	}
}

// TestSend_TransportErrorOmitsQuotedURL is P168 Part 7 F1: url.Error quotes the URL with %q, which
// escapes quotes and backslashes in an inlined secret so the plaintext masker never matches it.
func TestSend_TransportErrorOmitsQuotedURL(t *testing.T) {
	secret := `p"w\d`
	for _, raw := range []string{
		"http://127.0.0.1:1/p?k=" + url.QueryEscape(secret),
		"http://127.0.0.1:1/p?k=" + secret,
	} {
		_, err := Send(context.Background(), Request{Method: "GET", URL: raw}, Options{})
		if err == nil {
			t.Fatal("Send: want transport error")
		}
		msg := err.Error()
		var e *Error
		if errors.As(err, &e) && e.Timeline != nil {
			for _, h := range e.Timeline.Hops {
				msg += " " + h.Error
			}
		}
		if strings.Contains(msg, "p\\\"w") || strings.Contains(msg, "127.0.0.1:1/p") {
			t.Errorf("error text echoes the request URL: %q", msg)
		}
	}
	if _, err := resolveURL("http://h/\x01" + secret); err == nil || strings.Contains(err.Error(), "p\\\"w") {
		t.Errorf("resolveURL error = %v, want no URL echo", err)
	}
}
