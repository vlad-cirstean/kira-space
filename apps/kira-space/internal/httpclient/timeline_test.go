package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Seven cases guarding the bucketing (order-dependent across two goroutines) and the
// two measured phase-arithmetic traps — not CRUD round trips. Explicitly not tested:
// that httptrace fires at all (stdlib), that a float64 rounds, that an absent phase marshals to a
// missing key (a json tag) — each restates a short function body (CLAUDE.md).

// threeHopChain serves a same-host 301->302->307->200 chain — F1's own shape, which is also what
// makes hops 1-3 connection-reused (§6.2 case 2 rides on this same server).
func threeHopChain(t *testing.T) (base string) {
	t.Helper()
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/h0", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/h1", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/h1", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/h2", http.StatusFound)
	})
	mux.HandleFunc("/h2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/final", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("final-body"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	serverURL = srv.URL
	return serverURL
}

// ---- 1. Bucketing across a real 3-redirect chain (F1) ----
//
// The single most valuable test here: exactly four hops, each with the right status/url/method,
// and Redirects derived from them identical to what the pre-phase code produced (D3).

func TestTimeline_BucketsPerHop(t *testing.T) {
	base := threeHopChain(t)

	resp, err := Send(context.Background(), Request{Method: "GET", URL: base + "/h0"}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Body != "final-body" {
		t.Fatalf("Body = %q, want final-body", resp.Body)
	}

	hops := resp.Timeline.Hops
	if len(hops) != 4 {
		t.Fatalf("len(Timeline.Hops) = %d, want 4: %+v", len(hops), hops)
	}
	want := []struct {
		method string
		url    string
		status int
	}{
		{"GET", base + "/h0", http.StatusMovedPermanently},
		{"GET", base + "/h1", http.StatusFound},
		{"GET", base + "/h2", http.StatusTemporaryRedirect},
		{"GET", base + "/final", http.StatusOK},
	}
	for i, w := range want {
		h := hops[i]
		if h.Index != i || h.Method != w.method || h.URL != w.url || h.Status != w.status {
			t.Errorf("hops[%d] = {Index:%d Method:%q URL:%q Status:%d}, want {%d %q %q %d}",
				i, h.Index, h.Method, h.URL, h.Status, i, w.method, w.url, w.status)
		}
	}

	// D3: Redirects is a projection of the same hops — byte-identical to the pre-phase output.
	if len(resp.Redirects) != 3 {
		t.Fatalf("len(Redirects) = %d, want 3: %+v", len(resp.Redirects), resp.Redirects)
	}
	for i := 0; i < 3; i++ {
		if resp.Redirects[i].Status != hops[i].Status || resp.Redirects[i].URL != hops[i].URL {
			t.Errorf("Redirects[%d] = %+v, want {%d %q}", i, resp.Redirects[i], hops[i].Status, hops[i].URL)
		}
	}
	if resp.FinalURL != base+"/final" {
		t.Errorf("FinalURL = %q, want %q", resp.FinalURL, base+"/final")
	}
}

// ---- 2. A reused hop reports no phases (F3) ----
//
// Asserted as nil, not zero — that distinction is the SPEC's own requirement (D4).

func TestTimeline_ReusedHopHasNoPhases(t *testing.T) {
	base := threeHopChain(t)

	resp, err := Send(context.Background(), Request{Method: "GET", URL: base + "/h0"}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	hops := resp.Timeline.Hops
	if len(hops) != 4 {
		t.Fatalf("len(Timeline.Hops) = %d, want 4", len(hops))
	}

	if hops[0].Reused {
		t.Errorf("hops[0].Reused = true, want false (the first hop always dials fresh)")
	}
	if hops[0].DNS == nil && hops[0].Connect == nil {
		t.Errorf("hops[0] has neither DNS nor Connect — the fresh connection was not measured at all")
	}

	for i := 1; i < 4; i++ {
		h := hops[i]
		if !h.Reused {
			t.Errorf("hops[%d].Reused = false, want true (F1: a same-host redirect reuses the pooled connection)", i)
		}
		if h.DNS != nil {
			t.Errorf("hops[%d].DNS = %+v, want nil (reused connections fire no DNS hook, F3)", i, h.DNS)
		}
		if h.Connect != nil {
			t.Errorf("hops[%d].Connect = %+v, want nil", i, h.Connect)
		}
		if h.TLS != nil {
			t.Errorf("hops[%d].TLS = %+v, want nil", i, h.TLS)
		}
	}
}

// ---- 3. A cross-host hop reports its own full phase set (F4) ----

func localhostURL(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", srv.URL, err)
	}
	return "http://localhost:" + u.Port()
}

func TestTimeline_CrossHostHopHasOwnPhaseSet(t *testing.T) {
	var bURL string
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("b-body"))
	}))
	t.Cleanup(b.Close)
	bURL = localhostURL(t, b)

	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, bURL+"/x", http.StatusFound)
	}))
	t.Cleanup(a.Close)
	aURL := localhostURL(t, a)

	resp, err := Send(context.Background(), Request{Method: "GET", URL: aURL}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Body != "b-body" {
		t.Fatalf("Body = %q, want b-body", resp.Body)
	}
	hops := resp.Timeline.Hops
	if len(hops) != 2 {
		t.Fatalf("len(Timeline.Hops) = %d, want 2: %+v", len(hops), hops)
	}
	if hops[1].Reused {
		t.Errorf("hops[1].Reused = true, want false — a different host dials fresh (F4)")
	}
	// Addressed via "localhost" (not the raw 127.0.0.1 httptest gives) so a real resolver lookup
	// runs — F4's own reason DNS fires at all here.
	if hops[1].DNS == nil {
		t.Errorf("hops[1].DNS = nil, want non-nil — a cross-host redirect must resolve its own name")
	}
	if hops[1].Connect == nil {
		t.Errorf("hops[1].Connect = nil, want non-nil — a cross-host redirect must dial its own connection")
	}
}

// ---- 4. WroteRequest may never fire (F9/D8) ----
//
// A real 8 MiB upload the server rejects with 413 after reading 1 KiB — F9's own second measured
// case, reproduced end to end rather than asserted from a synthetic timestamp (the acceptance
// checklist's own requirement).

func TestTimeline_NoWaitWhenServerAnswersEarly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		_, _ = io.ReadFull(r.Body, buf)
		w.WriteHeader(http.StatusRequestEntityTooLarge)
	}))
	t.Cleanup(srv.Close)

	oversized := strings.Repeat("x", 8*1024*1024)
	resp, err := Send(context.Background(), Request{
		Method: "POST", URL: srv.URL,
		Body: Body{Mode: string(BodyRaw), Raw: oversized},
	}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("Status = %d, want %d", resp.Status, http.StatusRequestEntityTooLarge)
	}
	hops := resp.Timeline.Hops
	if len(hops) != 1 {
		t.Fatalf("len(Timeline.Hops) = %d, want 1: %+v", len(hops), hops)
	}
	h := hops[0]
	if h.Wait != nil {
		t.Errorf("Wait = %+v, want nil — the server answered before the request finished writing (F9/D8)", h.Wait)
	}
	if h.Download == nil {
		t.Errorf("Download = nil, want non-nil — a response, even an early one, still has a download phase")
	}
}

// ---- 5. Per-hop method and headers (F13) ----

func TestTimeline_PerHopMethodAndHeaders(t *testing.T) {
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		http.Redirect(w, r, serverURL+"/b", http.StatusSeeOther) // 303: POST -> GET
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, serverURL+"/c", http.StatusTemporaryRedirect) // 307: preserves GET
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	serverURL = srv.URL

	resp, err := Send(context.Background(), Request{Method: "POST", URL: serverURL + "/a"}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	hops := resp.Timeline.Hops
	if len(hops) != 3 {
		t.Fatalf("len(Timeline.Hops) = %d, want 3: %+v", len(hops), hops)
	}
	if hops[0].Method != "POST" {
		t.Errorf("hops[0].Method = %q, want POST", hops[0].Method)
	}
	if hops[1].Method != "GET" {
		t.Errorf("hops[1].Method = %q, want GET — a 303 converts POST to GET", hops[1].Method)
	}

	cookies := map[string]bool{}
	for _, h := range hops[0].Headers {
		if strings.EqualFold(h.Name, "Set-Cookie") {
			cookies[h.Value] = true
		}
	}
	if !cookies["a=1"] || !cookies["b=2"] {
		t.Errorf("hops[0].Headers Set-Cookie values = %v, want both a=1 and b=2 (duplicates must survive)", cookies)
	}
}

// ---- 6. The 8 KiB header cap (F21/D9) ----

func TestTimeline_HeaderCapTruncatesAndFlagsElision(t *testing.T) {
	var serverURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		// 64 KiB+ of response headers — comfortably over the 8 KiB cap.
		for i := 0; i < 64; i++ {
			w.Header().Add(fmt.Sprintf("X-Pad-%d", i), strings.Repeat("a", 1024))
		}
		http.Redirect(w, r, serverURL+"/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	serverURL = srv.URL

	resp, err := Send(context.Background(), Request{Method: "GET", URL: serverURL + "/big"}, Options{})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	hops := resp.Timeline.Hops
	if len(hops) != 2 {
		t.Fatalf("len(Timeline.Hops) = %d, want 2: %+v", len(hops), hops)
	}
	if !hops[0].HeadersElided {
		t.Fatal("hops[0].HeadersElided = false, want true — 64 KiB of headers exceeds the 8 KiB cap")
	}
	var size int
	for _, h := range hops[0].Headers {
		size += len(h.Name) + len(h.Value) + 2
	}
	if size > maxHopHeaderBytes {
		t.Errorf("capped Headers still sum to %d bytes, want <= %d", size, maxHopHeaderBytes)
	}
}

// ---- 7. Concurrency ----
//
// 16 concurrent sends of the same 3-redirect chain, each asserting four hops with the right
// statuses — the collector's own proof that it is race-free (run this file with -race, per
// CLAUDE.md's testing rules for concurrency: ordering, backpressure, cancellation, races).

func TestTimeline_ConcurrentSendsDoNotRace(t *testing.T) {
	base := threeHopChain(t)

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := Send(context.Background(), Request{Method: "GET", URL: base + "/h0"}, Options{})
			if err != nil {
				errs <- fmt.Errorf("Send: %w", err)
				return
			}
			if len(resp.Timeline.Hops) != 4 {
				errs <- fmt.Errorf("len(Timeline.Hops) = %d, want 4: %+v", len(resp.Timeline.Hops), resp.Timeline.Hops)
				return
			}
			wantStatus := []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusOK}
			for i, h := range resp.Timeline.Hops {
				if h.Status != wantStatus[i] {
					errs <- fmt.Errorf("hops[%d].Status = %d, want %d", i, h.Status, wantStatus[i])
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
