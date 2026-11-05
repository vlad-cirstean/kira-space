package prompts

import (
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/desknotify"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Windows is the window registry the router reads.
type Windows interface {
	MainKey() (string, bool)
	Has(key string) bool
	Focused(key string) bool
	Focus(key string) bool
}

// Deps are the seams the Router reads.
type Deps struct {
	Windows Windows
	// Emit broadcasts the full routed list.
	Emit func([]Routed)
	// Reveal tells one window to show prompt id first.
	Reveal func(windowKey, id string)
	// Reopen opens a window when none is; nil in flow tests.
	Reopen func()
	// Enabled reports advanced.notifyPrompts; nil means on.
	Enabled func() bool
	// App is "Studio" or "Space".
	App string
	Now func() time.Time
	// RaiseOnOpen reports whether opening a prompt of kind k brings its window to the front; nil means never.
	RaiseOnOpen func(k Kind) bool
}

// raiseQuiet spans the macOS Space-switch animation, when IsFocused may still read false.
const raiseQuiet = 2 * time.Second

type entry struct {
	Prompt
	claimed string
}

// Router is safe for concurrent use.
type Router struct {
	d  Deps
	em notify.OrderedEmitter[[]Routed]

	mu      sync.Mutex
	entries map[string]*entry
	sink    desknotify.Sink
	shown   map[Kind]bool
	raised  map[Kind]time.Time

	// sinkMu orders OS calls the way the state changes happened; taken under mu, released after the calls.
	sinkMu sync.Mutex
}

func New(d Deps) *Router {
	if d.Now == nil {
		d.Now = time.Now
	}
	r := &Router{d: d, entries: map[string]*entry{}, shown: map[Kind]bool{}, raised: map[Kind]time.Time{}}
	if d.Emit != nil {
		r.em.Subscribe(d.Emit)
	}
	return r
}

// SetSink installs the OS sink; nil drops every notification.
func (r *Router) SetSink(s desknotify.Sink) {
	r.mu.Lock()
	r.sink = s
	r.mu.Unlock()
}

// App is "Studio" or "Space".
func (r *Router) App() string { return r.d.App }

func (r *Router) enabled() bool { return r.d.Enabled == nil || r.d.Enabled() }

// target is the window that shows e: the claimed one, else the origin, else the main window, each
// only while registered. "" queues the prompt.
func (r *Router) target(e *entry) string {
	for _, k := range []string{e.claimed, e.Origin} {
		if k != "" && r.d.Windows.Has(k) {
			return k
		}
	}
	if k, ok := r.d.Windows.MainKey(); ok {
		return k
	}
	return ""
}

func (r *Router) listLocked() []Routed {
	out := make([]Routed, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, Routed{Prompt: e.Prompt, Target: r.target(e)})
	}
	slices.SortFunc(out, func(a, b Routed) int {
		if a.CreatedAt != b.CreatedAt {
			return int(a.CreatedAt - b.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// List is every open prompt with its target, oldest first.
func (r *Router) List() []Routed {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listLocked()
}

// note is the OS call a state change needs.
type note struct {
	send   *desknotify.Note
	remove string
}

const (
	noteSource = "prompt"
	thread     = "kira-prompts"
)

func noteID(k Kind) string { return "prompt:" + string(k) }

// countTitle is the grouped notification title for n open prompts of kind k.
func countTitle(k Kind, n int) string {
	count := strconv.Itoa(n)
	switch k {
	case KindSchedule:
		return i18n.T("backend.prompt.count.schedule.title", map[string]string{"count": count})
	case KindApproval:
		return i18n.T("backend.prompt.count.approval.title", map[string]string{"count": count})
	case KindGitCredential:
		return i18n.T("backend.prompt.count.gitCredential.title", map[string]string{"count": count})
	case KindHandover:
		return i18n.T("backend.prompt.count.handover.title", map[string]string{"count": count})
	case KindMobilePairing:
		return i18n.T("backend.prompt.count.mobilePairing.title", map[string]string{"count": count})
	case KindUpdate:
		return i18n.T("backend.prompt.count.update.title", map[string]string{"count": count})
	case KindProof:
		return i18n.T("backend.prompt.count.proof.title", map[string]string{"count": count})
	}
	return ""
}

func (r *Router) noteFor(k Kind, title string, n int) *desknotify.Note {
	if n > 1 {
		title = countTitle(k, n)
	}
	return &desknotify.Note{
		ID: noteID(k), Title: title, Body: i18n.T("backend.prompt.open.body", map[string]string{"app": r.d.App}), Thread: thread,
		Data: map[string]string{"source": noteSource, "kind": string(k)},
	}
}

func (r *Router) kindLocked(k Kind) (n int, oldest *entry) {
	for _, e := range r.entries {
		if e.Kind != k {
			continue
		}
		n++
		if oldest == nil || e.CreatedAt < oldest.CreatedAt || (e.CreatedAt == oldest.CreatedAt && e.ID < oldest.ID) {
			oldest = e
		}
	}
	return n, oldest
}

// commit publishes the state change and runs its OS call. Call with mu held; it releases mu.
func (r *Router) commit(n note) {
	seq, list, sink := r.em.NextSeq(), r.listLocked(), r.sink
	osCall := sink != nil && (n.send != nil || n.remove != "")
	if osCall {
		r.sinkMu.Lock()
		defer r.sinkMu.Unlock()
	}
	r.mu.Unlock()
	r.em.Emit(seq, list)
	if !osCall {
		return
	}
	if n.send != nil {
		_ = sink.Send(*n.send)
	} else {
		_ = sink.Remove(n.remove)
	}
}

// Open registers a popup; a repeated id is a no-op.
func (r *Router) Open(p Prompt) {
	r.mu.Lock()
	if _, dup := r.entries[p.ID]; dup {
		r.mu.Unlock()
		return
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = r.d.Now().UnixMilli()
	}
	e := &entry{Prompt: p}
	r.entries[p.ID] = e
	var n note
	target := r.target(e)
	focused := r.d.Windows.Focused(target)
	if r.sink != nil && r.enabled() && !focused {
		count, _ := r.kindLocked(p.Kind)
		n.send = r.noteFor(p.Kind, p.Title, count)
		r.shown[p.Kind] = true
	}
	raise := false
	if now := r.d.Now(); !focused && r.d.RaiseOnOpen != nil && r.d.RaiseOnOpen(p.Kind) && now.Sub(r.raised[p.Kind]) >= raiseQuiet {
		r.raised[p.Kind] = now
		raise = true
	}
	r.commit(n)
	if raise {
		slog.Info("prompt raise", "scope", "prompt", "kind", p.Kind, "target", target)
		// Focus blocks on the main thread; callers of Open may hold their own locks.
		go r.RevealKind(p.Kind)
	}
}

// Close withdraws a popup; an absent id is a no-op.
func (r *Router) Close(id string) {
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	delete(r.entries, id)
	var n note
	if r.shown[e.Kind] {
		if count, oldest := r.kindLocked(e.Kind); count == 0 {
			n.remove = noteID(e.Kind)
			delete(r.shown, e.Kind)
		} else {
			n.send = r.noteFor(e.Kind, oldest.Title, count)
		}
	}
	r.commit(n)
}

// Claim moves a popup to windowKey, for as long as that window is registered.
func (r *Router) Claim(id, windowKey string) error {
	if id == "" || windowKey == "" {
		return ipcerr.Contract("id and windowKey are required")
	}
	r.mu.Lock()
	e, ok := r.entries[id]
	if !ok {
		r.mu.Unlock()
		return ipcerr.T(ipcerr.CodeNotFound, "backend.core.prompt.gone.body", nil)
	}
	e.claimed = windowKey
	r.commit(note{})
	return nil
}

// Reroute republishes the list after the window set changed.
func (r *Router) Reroute() {
	r.mu.Lock()
	r.commit(note{})
}

// RevealKind handles a notification click: it shows the kind's oldest popup, reopening a window
// when none is open.
func (r *Router) RevealKind(k Kind) {
	r.mu.Lock()
	_, e := r.kindLocked(k)
	var tgt string
	if e != nil {
		tgt = r.target(e)
	}
	r.mu.Unlock()
	if e == nil {
		return
	}
	if tgt == "" && r.d.Reopen != nil {
		r.d.Reopen()
		r.mu.Lock()
		tgt = r.target(e)
		r.mu.Unlock()
	}
	if tgt == "" {
		return
	}
	r.d.Windows.Focus(tgt)
	if r.d.Reveal != nil {
		r.d.Reveal(tgt, e.ID)
	}
}

// SendTest posts a test notification, ignoring focus; the setting still gates it.
func (r *Router) SendTest() {
	r.mu.Lock()
	sink := r.sink
	if sink == nil || !r.enabled() {
		r.mu.Unlock()
		return
	}
	r.sinkMu.Lock()
	defer r.sinkMu.Unlock()
	r.mu.Unlock()
	_ = sink.Send(desknotify.Note{
		ID: "prompt:test", Title: i18n.T("backend.prompt.test.title", map[string]string{"app": r.d.App}), Body: i18n.T("backend.prompt.test.body", nil),
		Thread: thread, Data: map[string]string{"source": noteSource, "kind": "test"},
	})
}
