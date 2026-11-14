package httpflow

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/httpclient"
)

// J-HTTP-041: typed headers (secret-looking or not) never follow a redirect that changes origin.
func TestRedirectOriginChangeStripsTypedHeaders(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)
	tls := flowharness.HTTPS(t)

	secrets := []httpclient.Header{
		{Name: "X-Api-Key", Value: "k-secret"}, {Name: "PRIVATE-TOKEN", Value: "p-secret"},
		{Name: "Authorization", Value: "Bearer a-secret"}, {Name: "Cookie", Value: "sid=c-secret"},
	}
	leaked := func(t *testing.T, r flowharness.Request) {
		t.Helper()
		for _, h := range secrets {
			if v := r.Header.Get(h.Name); v != "" {
				t.Errorf("hop to %s kept %s = %q", r.Host, h.Name, v)
			}
		}
	}

	t.Run("host change then same-host hop stays stripped", func(t *testing.T) {
		before := len(srv.Requests())
		// A (127.0.0.1) -> B (localhost) /redirect?n=1 -> B /echo
		hop := srv.AltURL + "/redirect?n=1&code=302"
		args := get(srv.URL + "/redirect?n=0&code=302&to=" + url.QueryEscape(hop))
		args.Headers = secrets
		send(t, app, args)
		reqs := srv.Requests()[before:]
		if len(reqs) != 4 {
			t.Fatalf("server saw %d requests, want 4", len(reqs))
		}
		if reqs[0].Header.Get("X-Api-Key") != "k-secret" {
			t.Error("origin request lost its typed header")
		}
		for _, r := range reqs[1:] {
			if strings.HasPrefix(r.Host, "localhost:") {
				leaked(t, r)
			}
		}
		last := reqs[len(reqs)-1]
		if last.Header.Get("User-Agent") == "" || last.Header.Get("Accept-Encoding") == "" {
			t.Errorf("default headers dropped: %v", last.Header)
		}
	})

	t.Run("port change strips", func(t *testing.T) {
		other := flowharness.HTTP(t)
		args := get(srv.URL + "/redirect?n=0&code=302&to=" + url.QueryEscape(other.URL+"/echo"))
		args.Headers = secrets
		send(t, app, args)
		reqs := other.Requests()
		if len(reqs) != 1 {
			t.Fatalf("other server saw %d requests, want 1", len(reqs))
		}
		leaked(t, reqs[0])
	})

	t.Run("https to http downgrade strips", func(t *testing.T) {
		args := get(tls.URL + "/redirect?n=0&code=302&to=" + url.QueryEscape(srv.URL+"/echo"))
		args.Options.SSLVerify = ptr(false)
		args.Headers = secrets
		before := len(srv.Requests())
		send(t, app, args)
		reqs := srv.Requests()[before:]
		if len(reqs) != 1 {
			t.Fatalf("http server saw %d requests, want 1", len(reqs))
		}
		leaked(t, reqs[0])
	})

	t.Run("same-origin hop keeps typed headers", func(t *testing.T) {
		before := len(srv.Requests())
		args := get(srv.URL + "/redirect?n=2&code=302")
		args.Headers = secrets
		send(t, app, args)
		for _, r := range srv.Requests()[before:] {
			if r.Header.Get("X-Api-Key") != "k-secret" || r.Header.Get("Cookie") != "sid=c-secret" {
				t.Errorf("same-origin hop %s lost headers: %v", r.Path, r.Header)
			}
		}
	})
}

// J-HTTP-044: redirect bound edges; the bound clamps to 0..100.
func TestRedirectLimits(t *testing.T) {
	app := flowharness.New(t)
	srv := flowharness.HTTP(t)

	for _, tc := range []struct {
		name    string
		n, max  int
		wantErr bool
	}{
		{"max 0 with a hop fails", 1, 0, true},
		{"max 1 with one hop", 1, 1, false},
		{"max 1 with two hops fails", 2, 1, true},
		{"max 10 with ten hops", 10, 10, false},
		{"max 100 with 100 hops", 100, 100, false},
		{"max 101 clamps to 100", 101, 101, true},
		{"default 10 with ten hops", 10, -1, false},
		{"default 10 with eleven hops fails", 11, -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := get(srv.URL + "/redirect?n=" + strconv.Itoa(tc.n-1) + "&code=302")
			if tc.max >= 0 {
				args.Options.MaxRedirects = ptr(tc.max)
			}
			if tc.wantErr {
				e := sendErr(t, app, args)
				if !strings.Contains(e.Message, "redirect") {
					t.Errorf("error %q does not name redirects", e.Message)
				}
				return
			}
			if res := send(t, app, args); res.Status != 200 {
				t.Errorf("status = %d, want 200", res.Status)
			}
		})
	}
}
