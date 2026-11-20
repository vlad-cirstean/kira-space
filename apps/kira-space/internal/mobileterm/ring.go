// Package mobileterm lets a phone drive a agent terminal over a WebSocket while the
// desktop window keeps its own view. One Broker is the terminal.Arbiter for those terminals.
package mobileterm

// ringCap is the replay window per terminal.
const ringCap = 1 << 20

// Ring keeps the last ringCap bytes of a terminal's output under absolute offsets, so a
// reconnecting phone resumes from where it stopped. Not safe for concurrent use.
type Ring struct {
	buf []byte // grows with output up to cap
	end int64 // relative to base
	cap int64
	// base is the absolute offset of the first byte, so a ring rebuilt after a drop continues the
	// old numbering and a stale resume offset reads as overrun.
	base int64
}

func newRing(capacity int, base int64) *Ring {
	return &Ring{cap: int64(capacity), base: base}
}

func (r *Ring) start() int64 { return max(0, r.end-r.cap) }

// End is the offset one past the newest byte.
func (r *Ring) End() int64 { return r.base + r.end }

// Append stores b, dropping the oldest bytes once full.
func (r *Ring) Append(b []byte) {
	if int64(len(b)) > r.cap {
		r.buf = append(r.buf, make([]byte, r.cap-int64(len(r.buf)))...)
		r.end += int64(len(b)) - r.cap
		b = b[int64(len(b))-r.cap:]
	}
	for len(b) > 0 {
		pos := int(r.end % r.cap)
		if pos == len(r.buf) && int64(len(r.buf)) < r.cap {
			n := min(int64(len(b)), r.cap-int64(len(r.buf)))
			r.buf = append(r.buf, b[:n]...)
			r.end += n
			b = b[n:]
			continue
		}
		n := copy(r.buf[pos:], b)
		r.end += int64(n)
		b = b[n:]
	}
}

// ReadFrom returns up to limit bytes from off. When off fell out of the window it starts at the
// oldest byte and reports overrun. next is the offset after the returned data.
func (r *Ring) ReadFrom(off int64, limit int) (data []byte, next int64, overrun bool) {
	off -= r.base
	defer func() { next += r.base }()
	if off < 0 {
		off, overrun = 0, true
	}
	if s := r.start(); off < s {
		off, overrun = s, true
	}
	if off > r.end {
		off = r.end
	}
	n := min(int64(limit), r.end-off)
	if n <= 0 {
		return nil, off, overrun
	}
	data = make([]byte, 0, n)
	for int64(len(data)) < n {
		pos := int((off + int64(len(data))) % r.cap)
		chunk := min(n-int64(len(data)), r.cap-int64(pos))
		data = append(data, r.buf[pos:pos+int(chunk)]...)
	}
	return data, off + n, overrun
}

// Snapshot returns the whole window and the offset after it.
func (r *Ring) Snapshot() ([]byte, int64) {
	data, next, _ := r.ReadFrom(r.base, int(r.cap))
	return data, next
}
