package mobileweb

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/internal/appevent"
)

func recv(t *testing.T, s *Subscription) string {
	t.Helper()
	select {
	case b := <-s.C:
		return string(b)
	case <-time.After(time.Second):
		t.Fatal("no event")
		return ""
	}
}

func TestHub_AllowlistAndFanOut(t *testing.T) {
	t.Parallel()
	h := NewHub()
	a, _ := h.Subscribe("a")
	b, _ := h.Subscribe("b")
	h.Publish(appevent.ChannelTerminal, "secret keystrokes")
	h.Publish("kira:settings:changed", nil)
	h.Publish(adewire.ChannelBoard, nil)
	for _, s := range []*Subscription{a, b} {
		got := recv(t, s)
		if !strings.HasPrefix(got, "event: "+adewire.ChannelBoard+"\ndata: null\n\n") {
			t.Fatalf("got %q", got)
		}
		select {
		case extra := <-s.C:
			t.Fatalf("non-allowlisted channel leaked: %q", extra)
		default:
		}
	}
}

func TestHub_ScriptRunPushIsProjected(t *testing.T) {
	t.Parallel()
	h := NewHub()
	a, _ := h.Subscribe("a")
	h.Publish(appevent.ChannelScriptRunsChanged, map[string]any{
		"id": "r1", "state": "failed", "cwd": "/home/me/repo", "command": "make", "terminalId": "t1",
		"outcome": map[string]any{"status": "failed", "reason": "exit 2", "outputTail": "secret", "exitCode": 2},
	})
	got := recv(t, a)
	for _, leak := range []string{"cwd", "command", "terminalId", "outputTail", "secret"} {
		if strings.Contains(got, leak) {
			t.Errorf("push leaks %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, `"reason":"exit 2"`) {
		t.Errorf("push dropped the outcome reason: %s", got)
	}
}

func TestHub_SlowSubscriberIsDroppedWithoutBlocking(t *testing.T) {
	t.Parallel()
	h := NewHub()
	slow, _ := h.Subscribe("slow")
	fast, _ := h.Subscribe("fast")
	done := make(chan struct{})
	go func() {
		for i := 0; i < subBuffer+10; i++ {
			h.Publish(adewire.ChannelRuns, map[string]int{"i": i})
			select {
			case <-fast.C:
			default:
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a stalled subscriber blocked the publisher")
	}
	select {
	case <-slow.Done():
	default:
		t.Fatal("the full subscriber must be dropped")
	}
	select {
	case <-fast.Done():
		t.Fatal("a draining subscriber must stay")
	default:
	}
}

func TestHub_DisconnectDeviceEndsOnlyThatDevice(t *testing.T) {
	t.Parallel()
	h := NewHub()
	a1, _ := h.Subscribe("a")
	a2, _ := h.Subscribe("a")
	b, _ := h.Subscribe("b")
	h.DisconnectDevice("a")
	for _, s := range []*Subscription{a1, a2} {
		select {
		case <-s.Done():
		default:
			t.Fatal("device a stream must end")
		}
	}
	select {
	case <-b.Done():
		t.Fatal("device b stream must stay")
	default:
	}
}

func TestHub_StreamCaps(t *testing.T) {
	t.Parallel()
	h := NewHub()
	var subs []*Subscription
	for i := 0; i < maxStreamsPerDev; i++ {
		s, err := h.Subscribe("a")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, s)
	}
	if _, err := h.Subscribe("a"); err == nil {
		t.Fatal("per-device cap not enforced")
	}
	subs[0].Close()
	if _, err := h.Subscribe("a"); err != nil {
		t.Fatalf("closing a stream must free a slot: %v", err)
	}
	for i := 0; ; i++ {
		if _, err := h.Subscribe("dev" + portStr(i)); err != nil {
			break
		}
		if i > maxStreamsTotal+1 {
			t.Fatal("total cap not enforced")
		}
	}
}

func TestHub_ConcurrentPublishSubscribe(t *testing.T) {
	t.Parallel()
	h := NewHub()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				h.Publish(adewire.ChannelLog, map[string]int{"j": j})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if s, err := h.Subscribe("d"); err == nil {
					s.Close()
				}
			}
		}()
	}
	wg.Wait()
}
