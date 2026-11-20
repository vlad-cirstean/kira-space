package mobileterm

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// WebSocket close codes; the phone stops reconnecting on any of them.
const (
	CloseReleased  = 4000
	CloseReclaimed = 4001
	CloseReplaced  = 4002
	CloseExit      = 4003
	CloseTimeout   = 4004
	CloseRevoked   = 4005
)

const (
	graceDur = 60 * time.Second
	idleDur  = 15 * time.Minute
)

// Registry is the slice of the terminal registry the broker drives.
type Registry interface {
	Write(id string, b []byte) error
	Resize(id string, cols, rows uint16) error
}

// Timer is what AfterFunc returns; tests inject a fake clock through Deps.AfterFunc.
type Timer interface{ Stop() bool }

type Deps struct {
	Registry Registry
	// Sessions resolves an ADE session id to its live agent terminal.
	Sessions  func(sessionID string) (terminalID string, ok bool)
	Now       func() time.Time
	AfterFunc func(d time.Duration, f func()) Timer
	OnChange  func([]Hold)
}

// Hold is one terminal a phone currently controls (or just lost and may regain).
type Hold struct {
	TerminalID string `json:"terminalId"`
	SessionID  string `json:"sessionId"`
	DeviceID   string `json:"deviceId"`
	Label      string `json:"label"`
	Connected  bool   `json:"connected"`
	Since      int64  `json:"since"`
	// ReturnsAt is when an offline hold falls back to the desktop; 0 while connected.
	ReturnsAt int64 `json:"returnsAt"`
}

type state int

const (
	stateDesktop state = iota
	statePhone
	stateOffline
)

type entry struct {
	mu sync.Mutex
	// writeMu serialises phone PTY writes so e.mu is never held across a blocking write.
	writeMu     sync.Mutex
	id          string
	ring        *Ring
	ringBase    int64 // End of the last dropped ring
	state       state
	sessionID   string
	deviceID    string
	label       string
	conn        *phoneConn
	desktopCols int
	desktopRows int
	phoneCols   int
	phoneRows   int
	since       time.Time
	returnsAt   time.Time
	grace, idle Timer
	gen         uint64 // bumps on every state change so stale timers do nothing
}

// phoneConn is one attached socket. The broker ends it with finish; Serve closes the WebSocket
// with the recorded code.
type phoneConn struct {
	e      *entry
	device string
	notify chan struct{}
	done   chan struct{}
	once   sync.Once
	code   int
	reason string
}

func (c *phoneConn) finish(code int, reason string) {
	c.once.Do(func() {
		c.code, c.reason = code, reason
		close(c.done)
	})
}

// Done is closed once the broker ended this connection; Code and Reason are then readable.
func (c *phoneConn) Done() <-chan struct{} { return c.done }
func (c *phoneConn) Code() int             { return c.code }
func (c *phoneConn) Reason() string        { return c.reason }

func (c *phoneConn) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

// Broker implements terminal.Arbiter for agent terminals and the phone side attach.
type Broker struct {
	deps Deps

	mu      sync.Mutex
	entries map[string]*entry
	// recording gates the replay ring: a phone attaches only while phone access is on, so with it off
	// no output is kept.
	recording atomic.Bool
}

func New(d Deps) *Broker {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.AfterFunc == nil {
		d.AfterFunc = func(dur time.Duration, f func()) Timer { return time.AfterFunc(dur, f) }
	}
	if d.OnChange == nil {
		d.OnChange = func([]Hold) {}
	}
	return &Broker{deps: d, entries: map[string]*entry{}}
}

// SetRecording turns replay capture on or off. Off drops the ring of every terminal no phone holds.
func (b *Broker) SetRecording(on bool) {
	b.recording.Store(on)
	if on {
		return
	}
	b.mu.Lock()
	all := make([]*entry, 0, len(b.entries))
	for _, e := range b.entries {
		all = append(all, e)
	}
	b.mu.Unlock()
	for _, e := range all {
		e.mu.Lock()
		if e.conn == nil {
			e.dropRing()
		}
		e.mu.Unlock()
	}
}

// dropRing needs e.mu. Offsets stay monotonic across drops so a stale phone offset reads as overrun.
func (e *entry) dropRing() {
	if e.ring != nil {
		e.ringBase = e.ring.End()
		e.ring = nil
	}
}

func (b *Broker) get(id string) *entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.entries[id]
}

// getOrCreate covers output that arrives before Opened.
func (b *Broker) getOrCreate(id string) *entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[id]
	if e == nil {
		e = &entry{id: id}
		b.entries[id] = e
	}
	return e
}

func (b *Broker) Opened(id string, cols, rows int) {
	e := b.getOrCreate(id)
	e.mu.Lock()
	e.desktopCols, e.desktopRows = cols, rows
	e.mu.Unlock()
}

func (b *Broker) Output(id string, data []byte) {
	if !b.recording.Load() {
		return
	}
	e := b.getOrCreate(id)
	e.mu.Lock()
	if e.ring == nil {
		// Re-check under e.mu: SetRecording(false) sweeps under it, so no ring outlives the sweep.
		if !b.recording.Load() {
			e.mu.Unlock()
			return
		}
		e.ring = newRing(ringCap, e.ringBase)
	}
	e.ring.Append(data)
	c := e.conn
	e.mu.Unlock()
	if c != nil {
		c.signal()
	}
}

func (b *Broker) Exited(id string) {
	b.mu.Lock()
	e := b.entries[id]
	delete(b.entries, id)
	b.mu.Unlock()
	if e == nil {
		return
	}
	e.mu.Lock()
	held := e.state != stateDesktop
	if e.conn != nil {
		e.conn.finish(CloseExit, "exit")
		e.conn = nil
	}
	stopTimers(e)
	e.state = stateDesktop
	e.gen++
	e.mu.Unlock()
	if held {
		b.changed()
	}
}

func (b *Broker) AllowWrite(id string) bool {
	e := b.get(id)
	if e == nil {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state == stateDesktop
}

// AllowResize records the window's size and reports whether it may apply it.
func (b *Broker) AllowResize(id string, cols, rows int) bool {
	e := b.get(id)
	if e == nil {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.desktopCols, e.desktopRows = cols, rows
	return e.state == stateDesktop
}

// Attach makes dev the terminal's controller. A second device is refused; the same device
// replaces its old socket.
func (b *Broker) Attach(sessionID string, dev repos.MobileDeviceRow, cols, rows int) (*phoneConn, error) {
	tid, ok := b.deps.Sessions(sessionID)
	if !ok {
		return nil, ipcerr.T(ipcerr.CodeNotFound, "backend.space.mobileterm.noLiveTerminal.body", nil)
	}
	e := b.get(tid)
	if e == nil {
		return nil, ipcerr.T(ipcerr.CodeNotFound, "backend.space.mobileterm.noLiveTerminal.body", nil)
	}
	e.mu.Lock()
	if e.state != stateDesktop && e.deviceID != dev.ID {
		e.mu.Unlock()
		return nil, ipcerr.T(ipcerr.CodeConflict, "backend.space.mobileterm.controlledElsewhere.body", nil)
	}
	if e.ring == nil {
		e.ring = newRing(ringCap, e.ringBase)
	}
	c := &phoneConn{e: e, device: dev.ID, notify: make(chan struct{}, 1), done: make(chan struct{})}
	if old := e.conn; old != nil {
		old.finish(CloseReplaced, "replaced")
	}
	if e.state == stateDesktop {
		e.since = b.deps.Now()
	}
	stopTimers(e)
	e.gen++
	e.conn, e.state = c, statePhone
	e.sessionID, e.deviceID, e.label = sessionID, dev.ID, dev.Label
	e.phoneCols, e.phoneRows = cols, rows
	e.returnsAt = time.Time{}
	b.armIdle(e)
	b.resize(e, cols, rows)
	e.mu.Unlock()
	c.signal()
	b.changed()
	return c, nil
}

// Snapshot and Read give Serve its view of the ring. The ring is nil once SetRecording(false) ran
// after the terminal went back to the desktop while pump still holds c: read as empty.
func (c *phoneConn) Snapshot() (data []byte, end int64) {
	c.e.mu.Lock()
	defer c.e.mu.Unlock()
	if c.e.ring == nil {
		return nil, 0
	}
	return c.e.ring.Snapshot()
}

func (c *phoneConn) ReadFrom(off int64, limit int) (data []byte, next int64, overrun bool) {
	c.e.mu.Lock()
	defer c.e.mu.Unlock()
	if c.e.ring == nil {
		return nil, 0, false
	}
	return c.e.ring.ReadFrom(off, limit)
}

func (c *phoneConn) End() int64 {
	c.e.mu.Lock()
	defer c.e.mu.Unlock()
	if c.e.ring == nil {
		return 0
	}
	return c.e.ring.End()
}

// Input writes phone keystrokes; false when c no longer controls the terminal.
func (b *Broker) Input(c *phoneConn, data []byte) bool {
	e := c.e
	e.mu.Lock()
	if e.conn != c {
		e.mu.Unlock()
		return false
	}
	b.armIdle(e)
	e.mu.Unlock()

	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.mu.Lock()
	held := e.conn == c
	e.mu.Unlock()
	if !held {
		return false
	}
	return b.deps.Registry.Write(e.id, data) == nil
}

func (b *Broker) Resize(c *phoneConn, cols, rows int) bool {
	e := c.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.conn != c {
		return false
	}
	e.phoneCols, e.phoneRows = cols, rows
	b.armIdle(e)
	b.resize(e, cols, rows)
	return true
}

// Disconnected starts the grace period: the phone may come back within it.
func (b *Broker) Disconnected(c *phoneConn) {
	e := c.e
	e.mu.Lock()
	if e.conn != c {
		e.mu.Unlock()
		return
	}
	e.conn, e.state = nil, stateOffline
	stopTimers(e)
	e.gen++
	gen := e.gen
	e.returnsAt = b.deps.Now().Add(graceDur)
	e.grace = b.deps.AfterFunc(graceDur, func() { b.expire(e, gen) })
	e.mu.Unlock()
	b.changed()
}

// Release is the phone saying it is done.
func (b *Broker) Release(c *phoneConn) {
	e := c.e
	e.mu.Lock()
	if e.conn != c {
		e.mu.Unlock()
		return
	}
	changed := b.toDesktop(e, CloseReleased, "released")
	e.mu.Unlock()
	if changed {
		b.changed()
	}
}

// Reclaim is the desktop taking the terminal back. Non-zero cols and rows become the desktop size.
func (b *Broker) Reclaim(id string, cols, rows int) {
	e := b.get(id)
	if e == nil {
		return
	}
	e.mu.Lock()
	if cols > 0 && rows > 0 {
		e.desktopCols, e.desktopRows = cols, rows
	}
	changed := b.toDesktop(e, CloseReclaimed, "reclaimed")
	if !changed && cols > 0 && rows > 0 {
		b.resize(e, cols, rows)
	}
	e.mu.Unlock()
	if changed {
		b.changed()
	}
}

func (b *Broker) ReleaseDevice(deviceID string) {
	b.releaseWhere("permission off", func(e *entry) bool { return e.deviceID == deviceID })
}

func (b *Broker) ReleaseAll(reason string) {
	b.releaseWhere(reason, func(*entry) bool { return true })
}

func (b *Broker) releaseWhere(reason string, match func(*entry) bool) {
	b.mu.Lock()
	all := make([]*entry, 0, len(b.entries))
	for _, e := range b.entries {
		all = append(all, e)
	}
	b.mu.Unlock()
	changed := false
	for _, e := range all {
		e.mu.Lock()
		if e.state != stateDesktop && match(e) && b.toDesktop(e, CloseRevoked, reason) {
			changed = true
		}
		e.mu.Unlock()
	}
	if changed {
		b.changed()
	}
}

// Holds lists terminals a phone controls, offline holds included.
func (b *Broker) Holds() []Hold {
	b.mu.Lock()
	all := make([]*entry, 0, len(b.entries))
	for _, e := range b.entries {
		all = append(all, e)
	}
	b.mu.Unlock()
	out := []Hold{}
	for _, e := range all {
		e.mu.Lock()
		if e.state != stateDesktop {
			h := Hold{
				TerminalID: e.id, SessionID: e.sessionID, DeviceID: e.deviceID, Label: e.label,
				Connected: e.state == statePhone, Since: e.since.UnixMilli(),
			}
			if !e.returnsAt.IsZero() {
				h.ReturnsAt = e.returnsAt.UnixMilli()
			}
			out = append(out, h)
		}
		e.mu.Unlock()
	}
	return out
}

func (b *Broker) changed() { b.deps.OnChange(b.Holds()) }

// toDesktop needs e.mu. It reports whether the state changed.
func (b *Broker) toDesktop(e *entry, code int, reason string) bool {
	if e.state == stateDesktop {
		return false
	}
	if e.conn != nil {
		e.conn.finish(code, reason)
		e.conn = nil
	}
	stopTimers(e)
	e.gen++
	e.state = stateDesktop
	if !b.recording.Load() {
		e.dropRing()
	}
	e.deviceID, e.label, e.returnsAt = "", "", time.Time{}
	if e.desktopCols > 0 && e.desktopRows > 0 && (e.desktopCols != e.phoneCols || e.desktopRows != e.phoneRows) {
		b.resize(e, e.desktopCols, e.desktopRows)
	}
	return true
}

// resize needs e.mu; holding it orders a phone resize against a racing Reclaim.
func (b *Broker) resize(e *entry, cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	_ = b.deps.Registry.Resize(e.id, uint16(cols), uint16(rows))
}

// armIdle needs e.mu.
func (b *Broker) armIdle(e *entry) {
	if e.idle != nil {
		e.idle.Stop()
	}
	gen := e.gen
	e.idle = b.deps.AfterFunc(idleDur, func() { b.expire(e, gen) })
}

func (b *Broker) expire(e *entry, gen uint64) {
	e.mu.Lock()
	changed := e.gen == gen && b.toDesktop(e, CloseTimeout, "timeout")
	e.mu.Unlock()
	if changed {
		b.changed()
	}
}

func stopTimers(e *entry) {
	if e.grace != nil {
		e.grace.Stop()
		e.grace = nil
	}
	if e.idle != nil {
		e.idle.Stop()
		e.idle = nil
	}
}
