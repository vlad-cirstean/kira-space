package mobileflow_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mobileterm"
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/terminal"
	"github.com/kirathecat/kira-studio/internal/testx"
)

type termConn struct {
	ws  *websocket.Conn
	out chan []byte
	end chan websocket.StatusCode
}

// attach opens the terminal WebSocket the way the phone page does.
func (p *phone) attach(t *testing.T, sessionID string) *termConn {
	t.Helper()
	dctx, cancel := context.WithTimeout(ctx, waitFor)
	defer cancel()
	ws, _, err := websocket.Dial( //nolint:bodyclose // a handshake response body is not read after a successful upgrade
		dctx, "ws://"+p.host()+"/api/agent/sessions/"+sessionID+"/terminal?cols=80&rows=24", &websocket.DialOptions{
			HTTPClient: p.client, HTTPHeader: http.Header{"Origin": []string{p.origin()}},
		})
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	ws.SetReadLimit(1 << 20)
	c := &termConn{ws: ws, out: make(chan []byte, 256), end: make(chan websocket.StatusCode, 1)}
	go func() {
		for {
			typ, data, err := ws.Read(ctx)
			if err != nil {
				c.end <- websocket.CloseStatus(err)
				return
			}
			if typ == websocket.MessageBinary {
				c.out <- data
			}
		}
	}()
	t.Cleanup(func() { _ = ws.CloseNow() })
	return c
}

func (c *termConn) waitOutput(t *testing.T, want string) {
	t.Helper()
	var seen bytes.Buffer
	deadline := time.After(waitFor)
	for !strings.Contains(seen.String(), want) {
		select {
		case b := <-c.out:
			seen.Write(b)
		case <-deadline:
			t.Fatalf("terminal output %q never contained %q", seen.String(), want)
		}
	}
}

func TestPhoneTerminalAttach(t *testing.T) {
	f := newFixture(t)
	app := f.app
	p, deviceID := pairedPhone(t, f)
	launch := f.startSession(t)

	// Both agent-input switches gate the send and the attach.
	send := func(msg string) reply {
		return p.post("/api/agent/sessions/"+launch.SessionID+"/send", map[string]string{"message": msg})
	}
	if r := send("early"); r.Status != http.StatusForbidden || r.code(t) != "E_AGENT_INPUT_OFF/global" {
		t.Fatalf("send with the global switch off = %d %s", r.Status, r.Body)
	}
	if _, err := app.W.Mobile.SetAgentInputEnabled(bridge.MobileSetAgentInputArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if r := send("early"); r.Status != http.StatusForbidden || r.code(t) != "E_AGENT_INPUT_OFF/device" {
		t.Fatalf("send with the phone switch off = %d %s", r.Status, r.Body)
	}
	if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: true, AgentInput: true}); err != nil {
		t.Fatal(err)
	}

	if r := send("hello from the phone"); r.Status != http.StatusOK {
		t.Fatalf("send = %d %s", r.Status, r.Body)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-2.prompt"))
		return strings.Contains(string(raw), "hello from the phone")
	})

	mark := app.Events.Mark()
	term := p.attach(t, launch.SessionID)
	term.waitOutput(t, strings.TrimSpace(f.banner))
	holds := app.W.Mobile.TerminalHolds()
	if len(holds) != 1 || holds[0].SessionID != launch.SessionID || holds[0].TerminalID != launch.TerminalID ||
		holds[0].DeviceID != deviceID || !holds[0].Connected || holds[0].Label != "Ana's phone" {
		t.Fatalf("TerminalHolds = %+v, want the phone's hold on the session", holds)
	}
	held := app.Events.WaitAfter(t, mark, bridge.ChannelMobileTerminals, func(e flowharness.Event) bool {
		var got []mobileterm.Hold
		e.Decode(t, &got)
		return len(got) == 1
	}, waitFor)
	app.Contract(t, "mobile-terminal", "event:"+held.Channel+"#held", held.Data, flowharness.Mask("since", "returnsAt"))

	// Typed bytes reach the agent's terminal.
	if err := term.ws.Write(ctx, websocket.MessageBinary, []byte("typed on the phone\n")); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool {
		raw, _ := os.ReadFile(filepath.Join(app.FakeDir, "api-2.prompt"))
		return strings.Contains(string(raw), "typed on the phone")
	})

	// The desktop takes it back: the socket closes as reclaimed and the hold is gone.
	mark = app.Events.Mark()
	if err := app.W.Mobile.ReclaimTerminal(bridge.MobileReclaimArgs{TerminalID: launch.TerminalID, Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-term.end:
		if code != websocket.StatusCode(mobileterm.CloseReclaimed) {
			t.Fatalf("socket closed with %d, want %d (reclaimed)", code, mobileterm.CloseReclaimed)
		}
	case <-time.After(waitFor):
		t.Fatal("socket still open after ReclaimTerminal")
	}
	if holds := app.W.Mobile.TerminalHolds(); len(holds) != 0 {
		t.Fatalf("holds after reclaim = %+v", holds)
	}
	released := app.Events.WaitAfter(t, mark, bridge.ChannelMobileTerminals, func(e flowharness.Event) bool {
		var got []mobileterm.Hold
		e.Decode(t, &got)
		return len(got) == 0
	}, waitFor)
	app.Contract(t, "mobile-terminal", "event:"+released.Channel+"#released", released.Data)

	// Turning agent input off ends a fresh attach.
	again := p.attach(t, launch.SessionID)
	again.waitOutput(t, strings.TrimSpace(f.banner))
	if _, err := app.W.Mobile.SetAgentInputEnabled(bridge.MobileSetAgentInputArgs{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-again.end:
	case <-time.After(waitFor):
		t.Fatal("socket still open after agent input was turned off")
	}
	f.release(t)
}

// desktopSaw reports whether the desktop window's terminal stream has carried want.
func desktopSaw(app *flowharness.App, terminalID, want string) bool {
	var sb strings.Builder
	for _, ev := range app.Events.Since(0, appevent.ChannelTerminal) {
		te, ok := ev.Data.(terminal.Event)
		if !ok || te.TerminalID != terminalID {
			continue
		}
		raw, _ := base64.StdEncoding.DecodeString(te.Data)
		sb.Write(raw)
	}
	return strings.Contains(sb.String(), want)
}

// Replay exists only for output since phone access was turned on: a banner written while access was
// off is not kept, output after enabling is.
func TestPhoneTerminalReplayStartsAtAccess(t *testing.T) {
	f := newFixture(t)
	app := f.app
	launch := f.startSession(t)
	banner := strings.TrimSpace(f.banner)
	testx.WaitUntil(t, waitFor, func() bool { return desktopSaw(app, launch.TerminalID, banner) })

	p, deviceID := pairedPhone(t, f)
	if _, err := app.W.Mobile.SetAgentInputEnabled(bridge.MobileSetAgentInputArgs{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := app.W.Mobile.SetDevicePermissions(bridge.MobileDevicePermissionsArgs{ID: deviceID, Write: true, AgentInput: true}); err != nil {
		t.Fatal(err)
	}
	const marker = "after-access-marker"
	if err := app.W.Terminal.Write(terminal.WriteArgs{
		TerminalID: launch.TerminalID, Data: base64.StdEncoding.EncodeToString([]byte(marker + "\n")),
	}); err != nil {
		t.Fatal(err)
	}
	testx.WaitUntil(t, waitFor, func() bool { return desktopSaw(app, launch.TerminalID, marker) })

	term := p.attach(t, launch.SessionID)
	var seen bytes.Buffer
	deadline := time.After(waitFor)
	for !strings.Contains(seen.String(), marker) {
		select {
		case b := <-term.out:
			seen.Write(b)
		case <-deadline:
			t.Fatalf("replay %q never contained %q", seen.String(), marker)
		}
	}
	if strings.Contains(seen.String(), banner) {
		t.Fatalf("replay %q holds the banner written before access was on", seen.String())
	}
	f.release(t)
}
