package flowharness

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitrpc"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitwire"
)

// pipeConn is the in-memory twin of application.StreamConn: whole frames both ways.
type pipeConn struct {
	in, out chan []byte
	mu      sync.RWMutex
	closed  bool
}

// Send drops the frame once closeOut ran, as a write to a dead socket would.
func (c *pipeConn) Send(frame []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil
	}
	c.out <- append([]byte(nil), frame...)
	return nil
}

// closeOut closes the outbound channel once, after in-flight sends finish.
func (c *pipeConn) closeOut() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.out)
	}
}

func (c *pipeConn) Receive() ([]byte, error) {
	b, ok := <-c.in
	if !ok {
		return nil, io.EOF
	}
	return b, nil
}

// WireError is a request or stream failure as the wire carries it.
type WireError struct{ Code, Message string }

func (e *WireError) Error() string { return e.Code + ": " + e.Message }

// Chunk is one graph.stream (or other stream) chunk: the JSON payload plus the out-of-band blob.
type Chunk struct {
	Seq  int
	JSON json.RawMessage
	Blob []byte
}

// GraphMeta is the JSON half of a graph.stream chunk.
type GraphMeta struct {
	RepoID    string `json:"repoId"`
	Seq       int    `json:"seq"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	Source    string `json:"source"`
	Remaining int    `json:"remaining"`
	Exhausted bool   `json:"exhausted"`
}

// GraphRow is one decoded commit row.
type GraphRow struct {
	// Sha is hex of the wire sha (full width unless the server abbreviates).
	Sha     string
	Parents []string
	Time    uint32
	Subject string
	Refs    []GraphRef
}

// GraphRef is a ref decoration on a row.
type GraphRef struct {
	Kind, Name string
	IsHead     bool
}

// Graph decodes a graph chunk into its metadata and rows (absolute index From+i).
func (c Chunk) Graph(t testing.TB) (GraphMeta, []GraphRow) {
	t.Helper()
	var meta GraphMeta
	if err := json.Unmarshal(c.JSON, &meta); err != nil {
		t.Fatalf("graph chunk meta: %v", err)
	}
	if !gitwire.FrameBufferHasIdentifier(c.Blob) {
		t.Fatal("graph chunk blob lacks the KIG1 identifier")
	}
	frame := gitwire.GetRootAsFrame(c.Blob, 0)
	if frame.PayloadType() != gitwire.PayloadPackedCommitChunk {
		t.Fatalf("graph chunk payload type %v", frame.PayloadType())
	}
	var tab flatbuffers.Table
	if !frame.Payload(&tab) {
		t.Fatal("graph chunk has no payload")
	}
	pc := new(gitwire.PackedCommitChunk)
	pc.Init(tab.Bytes, tab.Pos)

	width := int(pc.ShaWidthBytes())
	shas := pc.ShasBytes()
	rows := make([]GraphRow, 0, len(shas)/max(width, 1))
	u32 := func(raw []byte) []uint32 {
		out := make([]uint32, len(raw)/4)
		for i := range out {
			out[i] = binary.LittleEndian.Uint32(raw[i*4:])
		}
		return out
	}
	parentOffsets, times := u32(pc.ParentOffsetsBytes()), u32(pc.TimesBytes())
	subjectOffsets, subjectBytes := u32(pc.SubjectOffsetsBytes()), pc.SubjectBytesBytes()
	parentShas := pc.ParentShasBytes()
	for i := 0; width > 0 && i < len(shas)/width; i++ {
		row := GraphRow{Sha: hex.EncodeToString(shas[i*width : (i+1)*width]), Time: times[i*2]}
		for p := parentOffsets[i]; p < parentOffsets[i+1]; p++ {
			row.Parents = append(row.Parents, hex.EncodeToString(parentShas[int(p)*width:int(p+1)*width]))
		}
		row.Subject = string(subjectBytes[subjectOffsets[i]:subjectOffsets[i+1]])
		rows = append(rows, row)
	}
	for i := 0; i < pc.DecorationsLength(); i++ {
		var rd gitwire.RowDecorations
		pc.Decorations(&rd, i)
		for j := 0; j < rd.RefsLength(); j++ {
			var ref gitwire.DecorationRef
			rd.Refs(&ref, j)
			rows[rd.Row()].Refs = append(rows[rd.Row()].Refs, GraphRef{Kind: string(ref.Kind()), Name: string(ref.Name()), IsHead: ref.IsHead()})
		}
	}
	return meta, rows
}

type wireFrame struct {
	T       string          `json:"t"`
	ID      int             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *WireError      `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Seq     int             `json:"seq,omitempty"`
	Chunk   json.RawMessage `json:"chunk,omitempty"`
	N       int             `json:"n,omitempty"`
}

type frameIn struct {
	wireFrame
	blob []byte
	raw  []byte
}

// GitStream is a client of one window's git connection: the bound GitService for the methods it
// carries, the real bridge.ServeGitStream over an in-memory connection for the rest.
type GitStream struct {
	t      testing.TB
	app    *App
	key    string
	mark   int
	conn   *pipeConn
	wg     sync.WaitGroup
	closed sync.Once

	mu      sync.Mutex
	nextID  int
	waiters map[int]chan frameIn
}

var gitStreamSeq atomic.Int64

// OpenGitStream opens a stream on a window of its own, so each stream keeps its own connection.
func (a *App) OpenGitStream() *GitStream {
	return a.OpenGitStreamFor(fmt.Sprintf("flow-git-%d", gitStreamSeq.Add(1)))
}

// OpenGitStreamFor is OpenGitStream for a connection opened by window windowKey; streams on one
// key share its connection.
func (a *App) OpenGitStreamFor(windowKey string) *GitStream {
	a.t.Helper()
	s := &GitStream{
		t:       a.t,
		app:     a,
		key:     windowKey,
		mark:    a.Events.Mark(),
		conn:    &pipeConn{in: make(chan []byte, 64), out: make(chan []byte, 64)},
		nextID:  1,
		waiters: map[int]chan frameIn{},
	}
	server := &pipeConn{in: s.conn.out, out: s.conn.in}
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		bridge.ServeGitStream(a.W.GitWindows, server, windowKey)
		server.closeOut()
	}()
	go func() {
		defer s.wg.Done()
		s.readLoop()
	}()
	a.t.Cleanup(s.Close)
	return s
}

// Key is the window key the stream's connection belongs to.
func (s *GitStream) Key() string { return s.key }

// Fire sends a request without waiting for its reply and returns a cancel for it, as a UI abandoning
// a call would. The reply, if any, is dropped.
func (s *GitStream) Fire(method string, params any) (cancel func()) {
	s.t.Helper()
	if call, ok := s.boundCall(method); ok {
		ctx, stop := context.WithCancel(context.Background())
		raw := rawParams(s.t, params)
		go func() { _, _ = call(ctx, s.key, raw) }()
		return stop
	}
	id, _ := s.register()
	s.send(wireFrame{T: "req", ID: id, Method: method, Params: rawParams(s.t, params)})
	return func() {
		s.send(wireFrame{T: "cancel", ID: id})
		s.unregister(id)
	}
}

// Close ends the connection and waits for the server loop to return.
func (s *GitStream) Close() {
	s.closed.Do(s.conn.closeOut)
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.t.Error("git stream did not shut down within 10s")
	}
}

func (s *GitStream) readLoop() {
	for raw := range s.conn.in {
		var blob []byte
		body := raw
		if len(raw) > 0 && raw[0] == 0x00 {
			n := int(binary.BigEndian.Uint32(raw[1:5]))
			body, blob = raw[5:5+n], raw[5+n:]
		}
		var env struct {
			Version int       `json:"version"`
			Body    wireFrame `json:"body"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			s.t.Errorf("git stream: bad frame: %v", err)
			continue
		}
		f := frameIn{wireFrame: env.Body, blob: blob, raw: raw}
		s.mu.Lock()
		w := s.waiters[f.ID]
		s.mu.Unlock()
		if w != nil {
			w <- f
		}
	}
}

func (s *GitStream) send(f wireFrame) {
	body, err := json.Marshal(map[string]any{"version": gitrpc.ContractVersion, "body": f})
	if err != nil {
		s.t.Fatal(err)
	}
	s.conn.out <- body
}

func (s *GitStream) register() (int, chan frameIn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan frameIn, 256)
	s.waiters[id] = ch
	return id, ch
}

func (s *GitStream) unregister(id int) {
	s.mu.Lock()
	delete(s.waiters, id)
	s.mu.Unlock()
}

// rawParams marshals params; nil sends an empty object.
func rawParams(t testing.TB, params any) json.RawMessage {
	t.Helper()
	if params == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return b
}

const requestTimeout = 60 * time.Second

// Request sends one request and decodes the result into out (nil to discard). A server refusal
// comes back as *WireError.
func (s *GitStream) Request(method string, params, out any) error {
	s.t.Helper()
	if call, ok := s.boundCall(method); ok {
		return s.requestBound(call, method, params, out)
	}
	id, ch := s.register()
	defer s.unregister(id)
	s.send(wireFrame{T: "req", ID: id, Method: method, Params: rawParams(s.t, params)})
	select {
	case f := <-ch:
		if f.OK != nil && !*f.OK {
			return f.Error
		}
		if out != nil {
			if err := json.Unmarshal(f.Result, out); err != nil {
				return fmt.Errorf("%s: decode result %s: %w", method, f.Result, err)
			}
		}
		return nil
	case <-time.After(requestTimeout):
		s.t.Fatalf("%s: no response within %s", method, requestTimeout)
		return nil
	}
}

// MustRequest is Request failing the test on error.
func (s *GitStream) MustRequest(method string, params, out any) {
	s.t.Helper()
	if err := s.Request(method, params, out); err != nil {
		s.t.Fatalf("%s: %v", method, err)
	}
}

// ErrorCode returns the wire error code of err, or "" when err is not a *WireError.
func ErrorCode(err error) string {
	var we *WireError
	if errors.As(err, &we) {
		return we.Code
	}
	return ""
}

// StreamCall is one open stream.
type StreamCall struct {
	s  *GitStream
	id int
	ch chan frameIn
}

// Stream opens a stream (graph.stream) with an initial credit.
func (s *GitStream) Stream(method string, params any, credit int) *StreamCall {
	s.t.Helper()
	id, ch := s.register()
	s.send(wireFrame{T: "open", ID: id, Method: method, Params: rawParams(s.t, params)})
	c := &StreamCall{s: s, id: id, ch: ch}
	if credit > 0 {
		c.Credit(credit)
	}
	return c
}

// Credit grants n more chunks.
func (c *StreamCall) Credit(n int) { c.s.send(wireFrame{T: "credit", ID: c.id, N: n}) }

// Cancel aborts the stream.
func (c *StreamCall) Cancel() {
	c.s.send(wireFrame{T: "cancel", ID: c.id})
	c.s.unregister(c.id)
}

// Next returns the next chunk; ok is false once the stream ended, with err the stream's failure.
func (c *StreamCall) Next() (chunk Chunk, ok bool, err error) {
	c.s.t.Helper()
	f, ok, err := c.nextFrame()
	if !ok {
		return Chunk{}, false, err
	}
	return Chunk{Seq: f.Seq, JSON: f.Chunk, Blob: f.blob}, true, nil
}

// NextRaw is Next returning the chunk frame body exactly as ServeGitStream sent it, blob header
// included.
func (c *StreamCall) NextRaw() (raw []byte, ok bool, err error) {
	c.s.t.Helper()
	f, ok, err := c.nextFrame()
	if !ok {
		return nil, false, err
	}
	return f.raw, true, nil
}

func (c *StreamCall) nextFrame() (frameIn, bool, error) {
	c.s.t.Helper()
	select {
	case f := <-c.ch:
		if f.T == "end" {
			c.s.unregister(c.id)
			if f.Error != nil {
				return frameIn{}, false, f.Error
			}
			return frameIn{}, false, nil
		}
		return f, true, nil
	case <-time.After(requestTimeout):
		c.s.t.Fatalf("stream %d: no frame within %s", c.id, requestTimeout)
		return frameIn{}, false, nil
	}
}

// Drain reads chunks to the end, granting credit as it goes, and returns them.
func (c *StreamCall) Drain() []Chunk {
	c.s.t.Helper()
	var out []Chunk
	for {
		ch, ok, err := c.Next()
		if err != nil {
			c.s.t.Fatalf("stream %d ended with error: %v", c.id, err)
		}
		if !ok {
			return out
		}
		out = append(out, ch)
		c.Credit(1)
	}
}

// Events returns the git events pushed to this stream's window since it opened for method
// (repo.changed, ...); "" returns all. Data is the raw JSON payload.
func (s *GitStream) Events(method string) []Event {
	var out []Event
	for _, e := range s.app.Events.Since(s.mark, "") {
		name, ok := s.eventName(e)
		if ok && (method == "" || name == method) {
			out = append(out, Event{Seq: len(out), Channel: name, Data: s.rawData(e)})
		}
	}
	return out
}

func (s *GitStream) eventName(e Event) (string, bool) {
	if e.Window != s.key || !strings.HasPrefix(e.Channel, bridge.GitEventPrefix) {
		return "", false
	}
	return strings.TrimPrefix(e.Channel, bridge.GitEventPrefix), true
}

func (s *GitStream) rawData(e Event) json.RawMessage {
	raw, err := json.Marshal(e.Data)
	if err != nil {
		s.t.Fatalf("git event %s: marshal: %v", e.Channel, err)
	}
	return raw
}

// WaitEvent blocks until a pushed event of method satisfies pred (nil matches any).
func (s *GitStream) WaitEvent(method string, pred func(json.RawMessage) bool, timeout time.Duration) json.RawMessage {
	s.t.Helper()
	e := s.app.Events.WaitAfter(s.t, s.mark, bridge.GitEventPrefix+method, func(e Event) bool {
		return e.Window == s.key && (pred == nil || pred(s.rawData(e)))
	}, timeout)
	return s.rawData(e)
}
