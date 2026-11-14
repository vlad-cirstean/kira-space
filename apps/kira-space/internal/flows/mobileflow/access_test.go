package mobileflow_test

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/flowtest/fakeclock"
	"github.com/kirathecat/kira-studio/internal/testx"
)

func portOf(t *testing.T, appURL string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(strings.TrimSuffix(strings.TrimPrefix(appURL, "http://"), "/"))
	if err != nil {
		t.Fatal(err)
	}
	n, _ := strconv.Atoi(port)
	return n
}

func unreachable(t *testing.T, appURL string) {
	t.Helper()
	testx.WaitUntil(t, waitFor, func() bool {
		conn, err := net.DialTimeout("tcp4", strings.TrimSuffix(strings.TrimPrefix(appURL, "http://"), "/"), time.Second)
		if err == nil {
			conn.Close()
			return false
		}
		return true
	})
}

func TestEnableTrustPort(t *testing.T) {
	app := flowharness.New(t)
	st := app.W.Mobile.Status()
	if st.Enabled || st.Running || st.Trusted != nil || st.Current == nil {
		t.Fatalf("initial status = %+v, want off, untrusted, with the current network shown", st)
	}

	port := freePort(t)
	if _, err := app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: port}); err != nil {
		t.Fatal(err)
	}
	mark := app.Events.Mark()
	st, err := app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Enabled || st.Running || st.StopReason != "notTrusted" {
		t.Fatalf("enabled without trust = %+v, want stopped notTrusted", st)
	}

	st, err = app.W.Mobile.TrustCurrentNetwork()
	if err != nil {
		t.Fatal(err)
	}
	wantURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	if !st.Running || st.AppURL != wantURL || st.StopReason != "" || st.Trusted == nil || st.TrustedAt == 0 {
		t.Fatalf("after trust = %+v, want running at %s", st, wantURL)
	}
	if st.Trusted.RouterMAC != st.Current.RouterMAC || st.Trusted.Subnet != st.Current.Subnet {
		t.Fatalf("trusted %+v differs from current %+v", st.Trusted, st.Current)
	}
	app.Events.WaitAfter(t, mark, bridge.ChannelMobileStatus, func(e flowharness.Event) bool {
		var got bridge.MobileStatus
		e.Decode(t, &got)
		return got.Running
	}, waitFor)

	p := newPhone(t, st.AppURL)
	if r := p.get("/"); r.Status != http.StatusOK || !strings.Contains(string(r.Body), "<title>phone</title>") {
		t.Fatalf("GET / = %d %q, want the embedded phone page", r.Status, r.Body)
	}
	if r := p.get("/api/me"); r.Status != http.StatusUnauthorized || r.code(t) != "E_UNAUTHORIZED" {
		t.Fatalf("unpaired GET /api/me = %d %s", r.Status, r.Body)
	}

	// SetPort moves the listener.
	next := freePort(t)
	st, err = app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: next})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || portOf(t, st.AppURL) != next {
		t.Fatalf("after SetPort = %+v, want running on %d", st, next)
	}
	unreachable(t, wantURL)
	if r := newPhone(t, st.AppURL).get("/"); r.Status != http.StatusOK {
		t.Fatalf("GET / on the new port = %d", r.Status)
	}

	// A busy port leaves the server stopped with the reason; a free one recovers it.
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	st, err = app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: busy.Addr().(*net.TCPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || !strings.Contains(st.Error, "in use") {
		t.Fatalf("on a busy port = %+v, want stopped with an in-use error", st)
	}
	if st, err = app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err = app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: next}); err != nil {
		t.Fatal(err)
	}
	if st, err = app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: true}); err != nil || !st.Running || st.Error != "" {
		t.Fatalf("re-enabled on a free port = %+v, %v", st, err)
	}

	st, err = app.W.Mobile.ForgetNetwork()
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || st.StopReason != "notTrusted" || st.Trusted != nil || !st.Enabled {
		t.Fatalf("after forget = %+v, want stopped notTrusted but still enabled", st)
	}
	unreachable(t, st.Current.Address+":"+strconv.Itoa(next))

	if _, err := app.W.Mobile.TrustCurrentNetwork(); err != nil {
		t.Fatal(err)
	}
	if st = app.W.Mobile.Status(); !st.Running {
		t.Fatalf("trusting again does not restart the server: %+v", st)
	}
	st, err = app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: false})
	if err != nil || st.Enabled || st.Running {
		t.Fatalf("disabled = %+v, %v", st, err)
	}
	unreachable(t, "127.0.0.1:"+strconv.Itoa(next))

	t.Run("a new port starts a server stopped by a busy one", func(t *testing.T) {
		if _, err := app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: busy.Addr().(*net.TCPAddr).Port}); err != nil {
			t.Fatal(err)
		}
		st, err := app.W.Mobile.SetPort(bridge.MobileSetPortArgs{Port: next})
		if err != nil || !st.Running || st.Error != "" {
			t.Fatalf("SetPort off a busy port = %+v, %v, want running with no error", st, err)
		}
	})
}

func TestPhonePairing(t *testing.T) {
	app := flowharness.New(t)
	st := serve(t, app)
	a, b := newPhone(t, st.AppURL), newPhone(t, st.AppURL)

	if r := a.get("/api/ade/board"); r.Status != http.StatusUnauthorized {
		t.Fatalf("unpaired board read = %d", r.Status)
	}
	if r := a.send(http.MethodPost, "/api/pair", map[string]string{"label": "x", "code": "12"}, ""); r.Status != http.StatusBadRequest {
		t.Fatalf("pairing with a short code = %d, want 400", r.Status)
	}

	mark := app.Events.Mark()
	done, pending := a.requestPairing(app, "Ana's phone", "4821")
	if pending.Code != "4821" || pending.RemoteIP != "127.0.0.1" || pending.ClientID == "" {
		t.Fatalf("prompt = %+v", pending)
	}
	ev := app.Events.WaitAfter(t, mark, bridge.ChannelMobilePairing, func(e flowharness.Event) bool {
		var snap bridge.MobilePairingSnapshot
		e.Decode(t, &snap)
		return snap.Pending != nil && snap.Pending.RequestID == pending.RequestID
	}, waitFor)
	app.Contract(t, "mobile-pairing", "event:"+ev.Channel+"#request", ev.Data, flowharness.Mask("expiresAtMs"))
	if res, err := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pending.RequestID}); err != nil || res.Result != "resolved" {
		t.Fatalf("Approve = %+v, %v", res, err)
	}
	if res, _ := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pending.RequestID}); res.Result != "alreadyResolved" {
		t.Fatalf("second Approve = %+v, want alreadyResolved", res)
	}
	out := <-done
	if out.err != nil || out.reply.Status != http.StatusOK {
		t.Fatalf("approved pairing = %+v", out)
	}
	app.Contract(t, "mobile-pairing", "http:POST /api/pair#approved", out.reply.raw(t))
	var paired struct{ DeviceID, Label string }
	out.reply.json(t, &struct {
		DeviceID *string `json:"deviceId"`
		Label    *string `json:"label"`
	}{&paired.DeviceID, &paired.Label})
	if paired.DeviceID == "" || paired.Label != "Ana's phone" {
		t.Fatalf("pairing body = %+v", paired)
	}
	var me struct {
		DeviceID    string `json:"deviceId"`
		Permissions struct {
			Write      bool `json:"write"`
			AgentInput bool `json:"agentInput"`
		} `json:"permissions"`
		AgentInputGlobal bool `json:"agentInputGlobal"`
	}
	r := a.get("/api/me")
	if r.Status != http.StatusOK {
		t.Fatalf("paired GET /api/me = %d %s", r.Status, r.Body)
	}
	r.json(t, &me)
	app.Contract(t, "mobile-pairing", "http:GET /api/me#paired", r.raw(t))
	if me.DeviceID != paired.DeviceID || me.Permissions.AgentInput || me.AgentInputGlobal {
		t.Fatalf("me = %+v, want agent input off", me)
	}
	if r.Header.Get("Cache-Control") != "no-store" || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v, want no-store and nosniff", r.Header)
	}
	devices, err := app.W.Mobile.Devices()
	if err != nil || len(devices) != 1 || devices[0].ID != paired.DeviceID {
		t.Fatalf("Devices = %+v, %v", devices, err)
	}

	// A second phone is denied and gets no cookie.
	doneB, pendingB := b.requestPairing(app, "Bo's phone", "7310")
	if res, err := app.W.Mobile.Deny(bridge.MobileIDArgs{ID: pendingB.RequestID}); err != nil || res.Result != "resolved" {
		t.Fatalf("Deny = %+v, %v", res, err)
	}
	denied := <-doneB
	if denied.err != nil || denied.reply.Status != http.StatusForbidden || denied.reply.code(t) != "E_PAIRING_DENIED/denied" {
		t.Fatalf("denied pairing = %+v %s", denied.reply.Status, denied.reply.Body)
	}
	if r := b.get("/api/me"); r.Status != http.StatusUnauthorized {
		t.Fatalf("denied phone GET /api/me = %d", r.Status)
	}

	// Revoking ends the open event stream and every later request.
	stream := a.events()
	if err := app.W.Mobile.Revoke(bridge.MobileIDArgs{ID: paired.DeviceID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream.closed:
	case <-time.After(waitFor):
		t.Fatal("event stream still open after Revoke")
	}
	if r := a.get("/api/me"); r.Status != http.StatusUnauthorized || r.code(t) != "E_REVOKED" {
		t.Fatalf("revoked GET /api/me = %d %s", r.Status, r.Body)
	}
	devices, _ = app.W.Mobile.Devices()
	if len(devices) != 1 || devices[0].RevokedAt == nil {
		t.Fatalf("Devices after Revoke = %+v, want the row marked revoked", devices)
	}
}

func TestPhoneExpiredDevice(t *testing.T) {
	app := flowharness.New(t)
	st := serve(t, app)
	a := newPhone(t, st.AppURL)
	a.pair(app, "Ana's phone")
	if r := a.get("/api/me"); r.Status != http.StatusOK {
		t.Fatalf("paired GET /api/me = %d", r.Status)
	}
	if _, err := app.DB().Exec(`UPDATE mobile_devices SET expires_at = ?`, time.Now().Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	r := a.get("/api/me")
	if r.Status != http.StatusUnauthorized || r.code(t) != "E_EXPIRED" {
		t.Fatalf("expired GET /api/me = %d %s", r.Status, r.Body)
	}
	app.Contract(t, "mobile-pairing", "http:GET /api/me#expired", r.raw(t))
}

// J-MOB-011: abuse of pairing and device tokens. Each attempt is refused, changes no state, and the
// rate limits bite. The phone server runs on a manual clock, so no test waits real minutes.
func TestPairingAbuse(t *testing.T) {
	clock := fakeclock.New(time.Now())
	app := flowharness.New(t, flowharness.WithMobileClock(clock.Now))
	st := serve(t, app)
	a, b := newPhone(t, st.AppURL), newPhone(t, st.AppURL)
	deviceCount := func() int {
		t.Helper()
		d, err := app.W.Mobile.Devices()
		if err != nil {
			t.Fatal(err)
		}
		return len(d)
	}
	noPrompt := func(what string) {
		t.Helper()
		if p := app.W.Mobile.PendingPairing(); p.Pending != nil || deviceCount() != 0 {
			t.Fatalf("%s left a prompt %+v or a device", what, p.Pending)
		}
	}

	// A code that is not four digits is refused before any prompt; the third refusal drains the
	// burst and the next request from the address is rate limited, with Retry-After.
	for i, code := range []string{"", "12345", "abcd", "1234"} {
		r := b.send(http.MethodPost, "/api/pair", map[string]string{"label": "x", "code": code}, "")
		if i < 3 && r.Status != http.StatusBadRequest {
			t.Fatalf("code %q = %d, want 400", code, r.Status)
		}
		if i == 3 {
			if r.Status != http.StatusTooManyRequests || r.Header.Get("Retry-After") == "" || r.code(t) != "E_RATE_LIMITED" {
				t.Fatalf("fourth pairing attempt = %d %s, want 429 with Retry-After", r.Status, r.Body)
			}
		}
	}
	noPrompt("malformed and rate limited attempts")
	clock.Advance(time.Minute)

	// A request nobody answers expires on the pairing clock: the phone is told, a late Approve finds
	// nothing, and no device or cookie exists.
	done, pending := b.requestPairing(app, "Late phone", "1234")
	clock.Advance(121 * time.Second)
	out := <-done
	if out.err != nil || out.reply.Status != http.StatusForbidden || out.reply.code(t) != "E_PAIRING_DENIED/timeout" {
		t.Fatalf("expired pairing = %+v", out.reply)
	}
	if res, _ := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pending.RequestID}); res.Result != "alreadyResolved" {
		t.Fatalf("Approve after expiry = %+v, want alreadyResolved", res)
	}
	noPrompt("an expired request")
	if r := b.get("/api/me"); r.Status != http.StatusUnauthorized {
		t.Fatalf("expired phone GET /api/me = %d", r.Status)
	}
	clock.Advance(time.Minute)

	// An approved request cannot be approved again: one device row, and a phone that already holds
	// its cookie gets its own device back instead of a second prompt.
	doneA, pendA := a.requestPairing(app, "Ana", "4821")
	if res, err := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pendA.RequestID}); err != nil || res.Result != "resolved" {
		t.Fatalf("Approve = %+v, %v", res, err)
	}
	if out := <-doneA; out.err != nil || out.reply.Status != http.StatusOK {
		t.Fatalf("pairing = %+v", out)
	}
	if res, _ := app.W.Mobile.Approve(bridge.MobileIDArgs{ID: pendA.RequestID}); res.Result != "alreadyResolved" {
		t.Fatalf("replayed Approve = %+v, want alreadyResolved", res)
	}
	if r := a.send(http.MethodPost, "/api/pair", map[string]string{"label": "again", "code": "9999"}, ""); r.Status != http.StatusOK {
		t.Fatalf("pairing with a valid cookie = %d %s", r.Status, r.Body)
	}
	if p := app.W.Mobile.PendingPairing(); p.Pending != nil || deviceCount() != 1 {
		t.Fatalf("after a replay: prompt %+v, %d devices, want none and 1", p.Pending, deviceCount())
	}

	// The first phone's cookie, carried by a second device after the first was revoked, is refused.
	devices, _ := app.W.Mobile.Devices()
	u, _ := url.Parse(a.base)
	b.client.Jar.SetCookies(u, a.client.Jar.Cookies(u))
	if r := b.get("/api/me"); r.Status != http.StatusOK {
		t.Fatalf("a cookie is the device: GET /api/me = %d, want 200 until revoked", r.Status)
	}
	if err := app.W.Mobile.Revoke(bridge.MobileIDArgs{ID: devices[0].ID}); err != nil {
		t.Fatal(err)
	}
	if r := b.get("/api/me"); r.Status != http.StatusUnauthorized || r.code(t) != "E_REVOKED" {
		t.Fatalf("revoked cookie on a second device = %d %s", r.Status, r.Body)
	}
	if r := b.post("/api/ade/later/items", map[string]string{"title": "sneak"}); r.Status != http.StatusUnauthorized {
		t.Fatalf("write with a revoked cookie = %d", r.Status)
	}
	if got := shelfTitles(t, app, adewire.ShelfLater, "sneak"); len(got) != 0 {
		t.Fatalf("a revoked cookie changed Later: %v", got)
	}

	// Guessing tokens locks the address out after ten failures, even for a paired phone, until the
	// limit refills.
	clock.Advance(time.Minute)
	c := newPhone(t, st.AppURL)
	c.pair(app, "Cy")
	forged := newPhone(t, st.AppURL)
	guess := func() reply {
		// A rejected credential is cleared from the jar, so each guess brings its own.
		forged.client.Jar.SetCookies(u, []*http.Cookie{{Name: "kira-space-device", Value: devices[0].ID + ".guess", Path: "/"}})
		return forged.get("/api/me")
	}
	for i := range 10 {
		if r := guess(); r.Status != http.StatusUnauthorized {
			t.Fatalf("guess %d = %d, want 401", i, r.Status)
		}
	}
	if r := guess(); r.Status != http.StatusTooManyRequests {
		t.Fatalf("guess 11 = %d, want 429", r.Status)
	}
	if r := c.get("/api/me"); r.Status != http.StatusTooManyRequests {
		t.Fatalf("a paired phone on a locked address = %d, want 429", r.Status)
	}
	clock.Advance(2 * time.Minute)
	if r := c.get("/api/me"); r.Status != http.StatusOK {
		t.Fatalf("paired phone after the lockout = %d, want 200", r.Status)
	}
}

// J-MOB-010: a paired phone loses the server (network gone, app relaunched), comes back and is the
// same device while its 30 days last. After they pass it is refused and told to pair again.
func TestPairedPhoneReconnectsThenExpires(t *testing.T) {
	clock := fakeclock.New(time.Now())
	app := flowharness.New(t, flowharness.WithMobileClock(clock.Now))
	st := serve(t, app)
	a := newPhone(t, st.AppURL)
	deviceID := a.pair(app, "Ana's phone")
	stream := a.events()

	if _, err := app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stream.closed:
	case <-time.After(waitFor):
		t.Fatal("event stream still open after the server stopped")
	}
	unreachable(t, st.AppURL)

	app.Restart()
	st2, err := app.W.Mobile.SetEnabled(bridge.MobileSetEnabledArgs{Enabled: true})
	if err != nil || !st2.Running {
		t.Fatalf("SetEnabled after relaunch = %+v, %v", st2, err)
	}
	a.base = strings.TrimSuffix(st2.AppURL, "/")
	var me struct {
		DeviceID string `json:"deviceId"`
	}
	r := a.get("/api/me")
	if r.Status != http.StatusOK {
		t.Fatalf("reconnect GET /api/me = %d %s", r.Status, r.Body)
	}
	r.json(t, &me)
	if me.DeviceID != deviceID {
		t.Fatalf("reconnected as %q, want the same device %q", me.DeviceID, deviceID)
	}
	resumed := a.events()
	if _, err := app.W.AdeTask.CaptureTask(ctx, adewire.CaptureTaskArgs{Title: "after reconnect"}); err != nil {
		t.Fatal(err)
	}
	resumed.wait(t, adewire.ChannelBoard, waitFor)

	clock.Advance(30*24*time.Hour - time.Minute)
	if r := a.get("/api/me"); r.Status != http.StatusOK {
		t.Fatalf("one minute before expiry = %d %s", r.Status, r.Body)
	}
	clock.Advance(2 * time.Minute)
	r = a.get("/api/me")
	if r.Status != http.StatusUnauthorized || r.code(t) != "E_EXPIRED" {
		t.Fatalf("after expiry = %d %s, want E_EXPIRED", r.Status, r.Body)
	}
	if r := a.get("/api/ade/board"); r.Status != http.StatusUnauthorized {
		t.Fatalf("board after expiry = %d, want 401", r.Status)
	}
}
