package mobileflow_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

var ctx = context.Background()

const waitFor = 20 * time.Second

func freePort(t *testing.T) int {
	t.Helper()
	// Below the ephemeral range: parallel packages' outgoing connections cannot take the port.
	for range 200 {
		port := 20000 + rand.IntN(10000)
		ln, err := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			continue
		}
		ln.Close()
		return port
	}
	t.Fatal("no free port below the ephemeral range")
	return 0
}

// serve trusts the loopback network and starts the phone server on a free port. Another test
// process can take the probed port before the listen, so a port-in-use stop retries on a fresh port.
func serve(t *testing.T, app *flowharness.App) bridge.MobileStatus {
	t.Helper()
	if _, err := app.W.Mobile.TrustCurrentNetwork(); err != nil {
		t.Fatal(err)
	}
	var st bridge.MobileStatus
	for range 20 {
		if _, err := app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: freePort(t)}); err != nil {
			t.Fatal(err)
		}
		var err error
		st, err = app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		if st.Running || !strings.Contains(st.Error, "in use") {
			break
		}
	}
	if !st.Running || st.AppURL == "" {
		t.Fatalf("phone server not running: %+v", st)
	}
	return st
}

// phone is one paired-or-not phone browser: a cookie jar and same-origin headers.
type phone struct {
	t      *testing.T
	base   string
	client *http.Client
}

func newPhone(t *testing.T, appURL string) *phone {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &phone{t: t, base: strings.TrimSuffix(appURL, "/"), client: &http.Client{Jar: jar}}
}

func (p *phone) origin() string { return p.base }

func (p *phone) host() string {
	u, _ := url.Parse(p.base)
	return u.Host
}

type reply struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r reply) json(t *testing.T, out any) {
	t.Helper()
	if err := json.Unmarshal(r.Body, out); err != nil {
		t.Fatalf("response %d is not JSON %q: %v", r.Status, r.Body, err)
	}
}

func (r reply) raw(t *testing.T) any {
	t.Helper()
	var v any
	r.json(t, &v)
	return v
}

func (r reply) code(t *testing.T) string {
	t.Helper()
	var e struct {
		Code   string `json:"code"`
		Reason string `json:"reason"`
	}
	r.json(t, &e)
	if e.Reason != "" {
		return e.Code + "/" + e.Reason
	}
	return e.Code
}

func (p *phone) get(path string) reply { return p.send(http.MethodGet, path, nil, "") }

// post sends a JSON write with a fresh Idempotency-Key.
func (p *phone) post(path string, body any) reply {
	return p.send(http.MethodPost, path, body, uuid.NewString())
}

func (p *phone) send(method, path string, body any, idemKey string) reply {
	p.t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			p.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rd)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Origin", p.origin())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return reply{Status: resp.StatusCode, Header: resp.Header, Body: raw}
}

type pairResult struct {
	reply reply
	err   error
}

// startPairing parks a POST /api/pair; the result arrives once the desktop answers.
func (p *phone) startPairing(label, code string) <-chan pairResult {
	done := make(chan pairResult, 1)
	go func() {
		raw, _ := json.Marshal(map[string]string{"label": label, "code": code})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.base+"/api/pair", bytes.NewReader(raw))
		req.Header.Set("Origin", p.origin())
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			done <- pairResult{err: err}
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		done <- pairResult{reply: reply{Status: resp.StatusCode, Header: resp.Header, Body: body}}
	}()
	return done
}

// requestPairing starts a parked POST /api/pair and returns the desktop prompt that appears for it.
func (p *phone) requestPairing(app *flowharness.App, label, code string) (<-chan pairResult, bridge.MobilePairingRequest) {
	p.t.Helper()
	done := p.startPairing(label, code)
	var pending bridge.MobilePairingRequest
	testx.WaitUntil(p.t, waitFor, func() bool {
		snap := app.W.Mobile.PendingPairing()
		if snap.Pending != nil && snap.Pending.Label == label {
			pending = *snap.Pending
			return true
		}
		return false
	})
	return done, pending
}

// pair runs a full approved pairing and returns the new device id.
func (p *phone) pair(app *flowharness.App, label string) string {
	p.t.Helper()
	done, pending := p.requestPairing(app, label, "4821")
	if res, err := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pending.RequestID}); err != nil || res.Result != "resolved" {
		p.t.Fatalf("Approve = %+v, %v", res, err)
	}
	out := <-done
	if out.err != nil || out.reply.Status != http.StatusOK {
		p.t.Fatalf("pairing = %+v", out)
	}
	var body struct {
		DeviceID string `json:"deviceId"`
	}
	out.reply.json(p.t, &body)
	return body.DeviceID
}

// sse is an open /api/events stream.
type sse struct {
	frames chan sseFrame
	closed chan struct{}
}

type sseFrame struct{ Event, Data string }

func (p *phone) events() *sse {
	p.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/api/events", nil)
	if err != nil {
		p.t.Fatal(err)
	}
	req.Header.Set("Origin", p.origin())
	resp, err := p.client.Do(req) //nolint:bodyclose // the reader goroutine closes it
	if err != nil {
		p.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		p.t.Fatalf("GET /api/events = %d", resp.StatusCode)
	}
	s := &sse{frames: make(chan sseFrame, 256), closed: make(chan struct{})}
	var once sync.Once
	go func() {
		defer resp.Body.Close()
		defer once.Do(func() { close(s.closed) })
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		var cur sseFrame
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				cur.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.Data = strings.TrimPrefix(line, "data: ")
			case line == "" && cur.Event != "":
				s.frames <- cur
				cur = sseFrame{}
			}
		}
	}()
	s.wait(p.t, "ready", waitFor)
	return s
}

func (s *sse) wait(t *testing.T, event string, timeout time.Duration) sseFrame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f := <-s.frames:
			if f.Event == event {
				return f
			}
		case <-s.closed:
			t.Fatalf("event stream closed while waiting for %q", event)
		case <-deadline:
			t.Fatalf("no %q event within %s", event, timeout)
		}
	}
}
