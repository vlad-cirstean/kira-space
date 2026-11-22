package terminal

import "errors"

// Move errors; BoundService maps each to E_CONTRACT.
var (
	ErrAlreadyMoving = errors.New("terminal is already moving")
	ErrNotHeld       = errors.New("terminal is not being moved")
	ErrNotOwner      = errors.New("terminal belongs to another window")
)

// AdoptResult is what a window adopting a moved terminal receives.
type AdoptResult struct {
	Data      string `json:"data"` // held bytes, base64
	End       int64  `json:"end"`  // cumulative count including Data
	Truncated bool   `json:"truncated"`
	Exited    bool   `json:"exited"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Error     string `json:"error,omitempty"`
}

// untrackOutput drops id's pump if it is still c.
func (r *Registry) untrackOutput(id string, c *outputCoalescer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.outs[id] == c {
		delete(r.outs, id)
	}
}

func (r *Registry) out(id string) *outputCoalescer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.outs[id]
}

// Reown moves id's session from window from to window to: byWindow, the session's own key and the
// output pump's target. From then closing from no longer kills it. false when id is not live under
// from.
func (r *Registry) Reown(id, from, to string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	sess := r.sessions[id]
	if sess == nil {
		// The session ended while held: its recorded exit still moves with the pump.
		if c := r.outs[id]; c != nil && c.reownEnded(from, to) {
			return true
		}
		return false
	}
	if _, ok := r.byWindow[from][id]; !ok {
		return false
	}
	delete(r.byWindow[from], id)
	if len(r.byWindow[from]) == 0 {
		delete(r.byWindow, from)
	}
	if r.byWindow[to] == nil {
		r.byWindow[to] = map[string]struct{}{}
	}
	r.byWindow[to][id] = struct{}{}
	sess.windowKey = to
	if c := r.outs[id]; c != nil {
		c.reown(to)
	}
	return true
}

// Hold stops id's output going to its window and buffers it. end is the byte count already
// emitted. live false: no live session, nothing held.
func (r *Registry) Hold(id string) (end int64, live bool, err error) {
	c := r.out(id)
	if c == nil {
		return 0, false, nil
	}
	end, err = c.hold()
	return end, err == nil, err
}

// Adopt ends id's hold in window key.
func (r *Registry) Adopt(id, key string) (AdoptResult, error) {
	c := r.out(id)
	if c == nil {
		return AdoptResult{}, ErrNotHeld
	}
	return c.adopt(key)
}

// Release ends id's hold in its current window, replaying the buffer there. A no-op when not held.
func (r *Registry) Release(id string) {
	if c := r.out(id); c != nil {
		c.release()
	}
}

// ForgetMoves drops every hold whose target window is key (it closed before adopting). Their
// sessions die in CloseWindow; this only frees the buffers and untracks pumps already done.
func (r *Registry) ForgetMoves(key string) {
	r.mu.Lock()
	type entry struct {
		id string
		c  *outputCoalescer
	}
	all := make([]entry, 0, len(r.outs))
	for id, c := range r.outs {
		all = append(all, entry{id, c})
	}
	r.mu.Unlock()
	for _, e := range all {
		if _, exited := e.c.forget(key); exited {
			r.untrackOutput(e.id, e.c)
		}
	}
}
