package mobileweb

import (
	"bytes"
	"net/http"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// idemKeyHeader carries a per-intent UUID on every phone write. A retry after a lost response
// reuses it and gets the first result back instead of running the write twice (a reorder is
// index-based, so a second run would move the item again).
const (
	idemKeyHeader = "Idempotency-Key"
	idemTTL       = 10 * time.Minute
	idemCapacity  = 1024
	maxIdemBody   = 64 << 10
)

type idemEntry struct {
	route  string
	done   chan struct{}
	status int
	ctype  string
	body   []byte
}

type idemOutcome int

const (
	idemFresh idemOutcome = iota
	idemReplay
	idemMismatch
	idemInFlight
)

// idemStore keys entries by device and key. begin is check-then-add, so a mutex orders it.
type idemStore struct {
	mu sync.Mutex
	c  *expirable.LRU[string, *idemEntry]
}

func newIdemStore(ttl time.Duration) *idemStore {
	return &idemStore{c: expirable.NewLRU[string, *idemEntry](idemCapacity, nil, ttl)}
}

func idemID(device, key string) string { return device + "|" + key }

// begin claims the key for route, or reports why it cannot: a finished entry replays, an entry
// for another route is a client bug, an unfinished one is still running.
func (st *idemStore) begin(device, key, route string) (*idemEntry, idemOutcome) {
	st.mu.Lock()
	defer st.mu.Unlock()
	id := idemID(device, key)
	if e, ok := st.c.Get(id); ok {
		if e.route != route {
			return e, idemMismatch
		}
		select {
		case <-e.done:
			return e, idemReplay
		default:
			return e, idemInFlight
		}
	}
	e := &idemEntry{route: route, done: make(chan struct{})}
	st.c.Add(id, e)
	return e, idemFresh
}

// finish stores a result under 500, so a retry replays it. A 5xx, or a handler that wrote nothing,
// is dropped: the same key may run again.
func (st *idemStore) finish(device, key string, e *idemEntry, status int, ctype string, body []byte) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if status == 0 || status >= http.StatusInternalServerError {
		st.c.Remove(idemID(device, key))
		return
	}
	e.status, e.ctype, e.body = status, ctype, body
	close(e.done)
}

// recorder captures the status, and optionally the body, of a response while passing it through.
type recorder struct {
	http.ResponseWriter
	status  int
	capture bool
	buf     bytes.Buffer
}

func newRecorder(w http.ResponseWriter, capture bool) *recorder {
	return &recorder{ResponseWriter: w, capture: capture}
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.capture && r.buf.Len() < maxIdemBody {
		r.buf.Write(b)
	}
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// idempotent wraps a POST write handler with the key protocol.
func (s *Server) idempotent(rt route, next deviceHandler) deviceHandler {
	routeID := rt.method + " " + rt.path
	return func(w http.ResponseWriter, r *http.Request, dev repos.MobileDeviceRow) {
		key := r.Header.Get(idemKeyHeader)
		if !ids.IsUUID(key) {
			writeErrorT(w, http.StatusBadRequest, string(ipcerr.CodeIdempotencyKey), "backend.space.mobileweb.idempotencyKey.body")
			return
		}
		e, outcome := s.idem.begin(dev.ID, key, routeID)
		switch outcome {
		case idemReplay:
			w.Header().Set("Content-Type", e.ctype)
			w.Header().Set("Idempotent-Replay", "true")
			w.WriteHeader(e.status)
			_, _ = w.Write(e.body)
			return
		case idemMismatch:
			writeErrorT(w, http.StatusUnprocessableEntity, string(ipcerr.CodeIdempotencyMismatch), "backend.space.mobileweb.idempotencyMismatch.body")
			return
		case idemInFlight:
			writeErrorT(w, http.StatusConflict, string(ipcerr.CodeConflict), "backend.space.mobileweb.requestInFlight.body")
			return
		}
		rec := newRecorder(w, true)
		defer func() {
			s.idem.finish(dev.ID, key, e, rec.status, rec.Header().Get("Content-Type"), rec.buf.Bytes())
		}()
		next(rec, r, dev)
	}
}
