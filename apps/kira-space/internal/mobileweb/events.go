package mobileweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/appevent"
)

// eventChannels is the only set of app events a phone receives. Terminal data, settings, git and
// every window-addressed event are dropped in Publish. A non-nil value projects the payload before
// it reaches a phone.
var eventChannels = map[string]func(any) (any, error){
	adewire.ChannelBoard:              nil,
	adewire.ChannelWorkflows:          nil,
	adewire.ChannelRepos:              nil,
	adewire.ChannelRuns:               nil,
	adewire.ChannelLog:                nil,
	adewire.ChannelSessions:           nil,
	appevent.ChannelAgentSessions:     withoutCwd,
	appevent.ChannelAgentEvent:        withoutCwd,
	appevent.ChannelApprovalsChanged:  approvalsEvent,
	appevent.ChannelScriptRunsChanged: phoneRun,
}

// withoutCwd drops every "cwd" key: the phone never sees absolute paths.
func withoutCwd(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	dropCwd(out)
	return out, nil
}

func dropCwd(v any) {
	switch t := v.(type) {
	case map[string]any:
		delete(t, "cwd")
		for _, c := range t {
			dropCwd(c)
		}
	case []any:
		for _, c := range t {
			dropCwd(c)
		}
	}
}

const (
	subBuffer         = 256
	maxStreamsPerDev  = 4
	maxStreamsTotal   = 32
	keepAliveInterval = 25 * time.Second
	sseWriteTimeout   = 10 * time.Second
)

var errTooManyStreams = errors.New("mobileweb: too many event streams")

// Hub fans allowlisted app events out to connected phones. A subscriber that cannot keep up is
// dropped (it reconnects and refetches) instead of blocking publishers.
type Hub struct {
	mu   sync.Mutex
	subs map[*Subscription]struct{}
	dev  map[string]int
}

func NewHub() *Hub {
	return &Hub{subs: map[*Subscription]struct{}{}, dev: map[string]int{}}
}

// Subscription is one open event stream.
type Subscription struct {
	C        <-chan []byte
	ch       chan []byte
	done     chan struct{}
	deviceID string
	hub      *Hub
	once     sync.Once
}

// Done closes when the hub dropped this subscription (slow consumer, revoke, shutdown).
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Close unsubscribes; safe to call more than once.
func (s *Subscription) Close() { s.hub.remove(s) }

// Subscribe opens a stream for deviceID, refusing past the per-device and total caps.
func (h *Hub) Subscribe(deviceID string) (*Subscription, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs) >= maxStreamsTotal || h.dev[deviceID] >= maxStreamsPerDev {
		return nil, errTooManyStreams
	}
	ch := make(chan []byte, subBuffer)
	s := &Subscription{C: ch, ch: ch, done: make(chan struct{}), deviceID: deviceID, hub: h}
	h.subs[s] = struct{}{}
	h.dev[deviceID]++
	return s, nil
}

func (h *Hub) remove(s *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(s)
}

func (h *Hub) removeLocked(s *Subscription) {
	s.once.Do(func() {
		delete(h.subs, s)
		if h.dev[s.deviceID]--; h.dev[s.deviceID] <= 0 {
			delete(h.dev, s.deviceID)
		}
		close(s.done)
	})
}

// Publish matches appevent's Emit signature so a Tap can feed it. Channels outside the allowlist
// are dropped before any marshalling.
func (h *Hub) Publish(name string, data any) {
	project, ok := eventChannels[name]
	if !ok {
		return
	}
	h.mu.Lock()
	idle := len(h.subs) == 0
	h.mu.Unlock()
	if idle {
		return
	}
	var err error
	if project != nil {
		data, err = project(data)
	}
	var payload []byte
	if err == nil {
		payload, err = json.Marshal(data)
	}
	if err != nil {
		slog.Warn("mobileweb: marshal event", "scope", "mobileweb", "channel", name, "err", err)
		return
	}
	frame := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", name, payload))
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		select {
		case s.ch <- frame:
		default:
			h.removeLocked(s)
		}
	}
}

// DisconnectDevice ends every stream of a revoked device.
func (h *Hub) DisconnectDevice(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if s.deviceID == deviceID {
			h.removeLocked(s)
		}
	}
}

// CloseAll ends every stream (server shutdown).
func (h *Hub) CloseAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		h.removeLocked(s)
	}
}

// handleEvents is the SSE stream. EventSource sends the device cookie itself and reconnects after
// `retry`; the client refetches everything on each reconnect, so no replay buffer is kept.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	sub, err := s.cfg.Hub.Subscribe(d.ID)
	if err != nil {
		writeErrorT(w, http.StatusTooManyRequests, codeRateLimited, "backend.space.mobileweb.tooManyStreams.body")
		return
	}
	defer sub.Close()
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(b []byte) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout))
		if _, err := w.Write(b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !write([]byte("retry: 3000\nevent: ready\ndata: {}\n\n")) {
		return
	}
	tick := time.NewTicker(keepAliveInterval)
	defer tick.Stop()
	expiry := time.NewTimer(time.UnixMilli(d.ExpiresAt).Sub(s.cfg.Now()))
	defer expiry.Stop()
	for {
		select {
		case frame := <-sub.C:
			if !write(frame) {
				return
			}
		case <-tick.C:
			if !write([]byte(": ping\n\n")) {
				return
			}
		case <-sub.Done():
			return
		case <-expiry.C:
			return
		case <-r.Context().Done():
			return
		}
	}
}
