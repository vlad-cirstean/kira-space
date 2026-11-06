package adapterhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
)

// This file is bridge/stream.go's real subject: the data plane stops being a
// byte forwarder and becomes a server. A frame from the renderer is parsed just enough to decide
// routing (op, and for a connection-scoped op, its connectionId) — never fully, and never at all
// for a frame the router only relays. Requests arrive as JSON text; responses, errors and
// events leave through frame.go as FlatBuffers frames.

// AttachStream makes conn the current renderer connection: a Session (A18's single writer)
// subscribed to the Go cache's own stats-changed notifications (D12), which is what feeds the
// status bar's cache readout. detach supersedes this session — call it when the renderer's side
// closes.
func (r *Router) AttachStream(conn StreamSession) (session *Session, detach func()) {
	session = newSession(conn)
	unsubscribeStats := r.cache.OnStatsChanged(func(stats enginecache.CacheStats) {
		r.pushCacheStats(session, stats)
	})
	return session, func() {
		unsubscribeStats()
		session.Close()
	}
}

// pushCacheStats emits an unsolicited cache:stats event frame — the exact shape port.ts's
// handleMessage dispatches on and data.onCacheStats consumes.
func (r *Router) pushCacheStats(session *Session, stats enginecache.CacheStats) {
	body, err := encodeEvent("cache:stats", stats)
	if err != nil {
		return
	}
	session.enqueueLocal(body)
}

// HandleDataFrame is rpc.ts's dispatch, reimagined as a router: ping is answered in-process — the
// engine is this process now; cache:stats/cache:clear are not connection-scoped;
// every other op is served in-process too, since every kind has been native
// (deleted the Node child this used to forward a non-native kind's op to).
func (r *Router) HandleDataFrame(session *Session, frame []byte) {
	var probe struct {
		Kind    string          `json:"kind"`
		ID      int             `json:"id"`
		Op      string          `json:"op"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(frame, &probe); err != nil || probe.Kind != "req" {
		// Matches stdio-main.ts's own "dropped an unparseable frame" / kind-check no-op.
		return
	}

	// P2 R2: host.go's safeRun (P58 D16) only wraps a call already routed through Host.RunOp — but
	// Dispatcher.Preview deliberately never goes through RunOp (it must not appear in the op log,
	// data.go's own doc comment on Preview), and several other paths here (Invalidate, the cache
	// fast paths around Read/Count, every RunOp result's own `any` type assertion) never do either.
	// Each of those runs on this frame's own goroutine (HandleDataFrameAsync), so an unrecovered
	// panic here previously took the whole process down instead of failing just this one request.
	// This mirrors safeRun's own contract (recover, log, answer with E_INTERNAL) at the one point
	// that actually covers every op dispatched from a frame, known-request-id included.
	defer func() {
		if rec := recover(); rec != nil {
			r.deps.Log("error", fmt.Sprintf("data frame panic: %v\n%s", rec, debug.Stack()))
			r.respondError(session, probe.ID, adapters.Internal(fmt.Sprintf("internal error: %v", rec), nil))
		}
	}()

	switch probe.Op {
	case "ping":
		r.respondPing(session, probe.ID)
		return
	case "cache:stats":
		r.respondCacheStats(session, probe.ID)
		return
	case "cache:clear":
		r.cache.Clear()
		r.respond(session, probe.ID, struct{}{}, nil)
		return
	}

	r.handleDataOp(session, probe.Op, probe.ID, probe.Payload)
}

// HandleDataFrameAsync is ServeEngineStream's own per-frame dispatch: HandleDataFrame runs on its
// own goroutine so a slow op never serialises behind it, but bounded — it blocks until a
// concurrency slot is free (or the session closes) before spawning, so a burst of frames can't grow
// goroutines/driver work without bound. Returns immediately (without spawning) once the session has
// closed, matching every other session-scoped operation's own no-op-after-close contract.
func (r *Router) HandleDataFrameAsync(session *Session, frame []byte) {
	if !session.acquireSlot() {
		return
	}
	go func() {
		defer session.releaseSlot()
		r.HandleDataFrame(session, frame)
	}()
}

// pingPayload is port.ts's PingPayload (packages/shared/protocol/port.ts:18-22), byte-compatible with
// what rpc.ts's own ping handler used to return — state/engine.ts and StatusBar.vue read it
// unchanged (P58f D11).
type pingPayload struct {
	Pong      bool  `json:"pong"`
	EnginePid int   `json:"enginePid"`
	At        int64 `json:"at"`
}

// respondPing answers the data plane's own health probe locally.
func (r *Router) respondPing(session *Session, id int) {
	r.respond(session, id, pingPayload{Pong: true, EnginePid: os.Getpid(), At: time.Now().UnixMilli()}, nil)
}

func decodeAndValidate[T interface{ Validate() error }](payload json.RawMessage, out *T) error {
	if err := json.Unmarshal(payload, out); err != nil {
		return adapters.New(adapters.CodeQuery, err.Error(), err)
	}
	if err := (*out).Validate(); err != nil {
		return adapters.New(adapters.CodeQuery, err.Error(), err)
	}
	return nil
}

func (r *Router) handleDataOp(session *Session, op string, id int, payload json.RawMessage) {
	// P2 R1: session.ctx, not context.Background() — an op derived from Background() has no way
	// to be bounded by anything but an explicit ops:cancel RPC, so it outlives its own session
	// (and the now-gone renderer request that started it) indefinitely once the session closes.
	ctx := session.ctx
	switch op {
	case "data:read":
		var req ReadRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.Read(ctx, req)
		r.respond(session, id, resp, err)
	case "data:count":
		var req CountRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.Count(ctx, req)
		r.respond(session, id, resp, err)
	case "data:invalidate":
		var req InvalidateRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		r.dispatcher.Invalidate(req)
		r.respond(session, id, struct{}{}, nil)
	case "data:preview":
		var req PreviewRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.Preview(req)
		r.respond(session, id, resp, err)
	case "data:mutate":
		var req MutateRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.Mutate(ctx, req)
		r.respond(session, id, resp, err)
	case "data:execute":
		var req ExecuteRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.Execute(ctx, req)
		r.respond(session, id, resp, err)
	case "data:objectDownload":
		var req ObjectDownloadRequestWire
		if err := decodeAndValidate(payload, &req); err != nil {
			r.respondError(session, id, err)
			return
		}
		resp, err := r.dispatcher.ObjectDownload(ctx, req)
		r.respond(session, id, resp, err)
	default:
		r.respondError(session, id, adapters.New(adapters.CodeUnsupported, "unknown op: "+op, nil))
	}
}

// maxResponsePayloadBytes leaves generous room for the frame's own envelope (kind/id/ok) below
// session.go's hard maxDataFrameBytes cap — resp is checked against this before ever reaching the
// queue, so a payload that would be silently unrepresentable there is turned into an error response
// instead of a frame that can never be delivered (a dropped response has no other way to
// ever settle its pending request, per enqueueResponse's own doc comment).
const maxResponsePayloadBytes = maxDataFrameBytes - 4096

// internalErrorFrame is respondError's answer for the vanishingly unlikely case that encodeError
// itself panics (the flatbuffers builder's own internal limits) — built once, with id 0, so the
// fallback path cannot also fail. Replaces the hand-written JSON literal the base64 wire format
// used for the same purpose.
var internalErrorFrame []byte

func init() {
	internalErrorFrame = encodeError(0, "internal error", "")
}

// safeEncodeError calls encodeError, substituting internalErrorFrame if it panics — still must not
// leave id's pending request unanswered forever.
func safeEncodeError(id int, message, code string) (body []byte) {
	defer func() {
		if recover() != nil {
			body = internalErrorFrame
		}
	}()
	return encodeError(id, message, code)
}

func (r *Router) respond(session *Session, id int, payload any, err error) {
	if err != nil {
		r.respondError(session, id, err)
		return
	}
	// A page's own byte size is already known before encoding (it's exactly what estimateFrameSize
	// below uses to pre-size the FlatBuffers builder, docs/PERF.md) — an oversized page used to be
	// fully built into a frame (~1.06x the page's own size, tens of MB for a pathological one)
	// purely to be discarded by the post-encode check further down. The post-encode check stays as
	// the correctness backstop for every payload type this estimate doesn't cover (or
	// underestimates).
	if oversizedPagePayload(payload) {
		r.deps.Log("error", "response frame exceeds the size limit, substituting an error response")
		r.respondError(session, id, adapters.T(adapters.CodeQuery, "backend.studio.adapter.responseTooLarge.body", nil, nil))
		return
	}
	body, encErr := encodeResponse(id, payload)
	if encErr != nil {
		r.deps.Log("error", "failed to encode a response frame: "+encErr.Error())
		r.respondError(session, id, adapters.Contract("failed to encode the response", nil))
		return
	}
	if len(body) > maxResponsePayloadBytes {
		r.deps.Log("error", "response frame exceeds the size limit, substituting an error response")
		r.respondError(session, id, adapters.T(adapters.CodeQuery, "backend.studio.adapter.responseTooLarge.body", nil, nil))
		return
	}
	session.enqueueResponse(body)
}

// oversizedPagePayload answers the size question estimateFrameSize already knows how to compute,
// before encodePayload ever runs — the same two payload shapes (ReadResponse/ExecuteResponse)
// that shape already special-cases, since those are the ones whose size the app doesn't have to
// guess at (page.Page.Size() is measured, not estimated, per docs/PERF.md).
func oversizedPagePayload(payload any) bool {
	switch v := payload.(type) {
	case ReadResponse:
		return pageTooLarge(v.Page.Size())
	case ExecuteResponse:
		total := 0
		for _, p := range v.Pages {
			total += p.Size()
		}
		return pageTooLarge(total)
	default:
		return false
	}
}

func pageTooLarge(rawBytes int) bool {
	return pageSizeEstimate(rawBytes) > maxResponsePayloadBytes
}

// maxErrorMessageBytes caps an error frame's text: a driver can echo a huge statement back, and an
// error frame over the Wails frame limit would otherwise kill the writer (F5, P168 Part 5).
const maxErrorMessageBytes = 64 << 10

func capErrorMessage(msg string) string {
	if len(msg) <= maxErrorMessageBytes {
		return msg
	}
	return strings.ToValidUTF8(msg[:maxErrorMessageBytes], "") + "…"
}

func (r *Router) respondError(session *Session, id int, err error) {
	code := ""
	var ae *adapters.Error
	if errors.As(err, &ae) {
		code = string(ae.Code)
	}
	session.enqueueResponse(safeEncodeError(id, capErrorMessage(err.Error()), code))
}

func (r *Router) respondCacheStats(session *Session, id int) {
	body, err := encodeResponse(id, r.cache.Stats())
	if err != nil {
		r.respondError(session, id, adapters.Contract("failed to encode the response", nil))
		return
	}
	session.enqueueResponse(body)
}
