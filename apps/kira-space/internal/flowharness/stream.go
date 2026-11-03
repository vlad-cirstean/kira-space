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

// Chunk is one graph.stream chunk: the JSON payload plus the out-of-band blob.
type Chunk struct {
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


// planeFrame is a graph data plane frame without its blob: the header both directions share.
type planeFrame struct {
	T         string          `json:"t"`
	ID        int             `json:"id,omitempty"`
	WindowKey string          `json:"windowKey,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Credit    int             `json:"credit,omitempty"`
	N         int             `json:"n,omitempty"`
	Chunk     json.RawMessage `json:"chunk,omitempty"`
	Error     *WireError      `json:"error,omitempty"`
}

type frameIn struct {
	planeFrame
	blob []byte
	raw  []byte
}

// GitStream is a client of one window's git connection: the bound GitService for every request,
// the graph data plane (bridge.ServeGitGraph over an in-memory connection) for graph.stream.
type GitStream struct {
	t    testing.TB
	app  *App
	key  string
	mark int

	closed sync.Once
	wg     sync.WaitGroup

	mu      sync.Mutex
	plane   *pipeConn
	nextID  int
	waiters map[int]chan frameIn
	// held lists repos opened bound through this stream; Close releases them, as the page's transport does.
	held []string
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
		nextID:  1,
		waiters: map[int]chan frameIn{},
	}
	a.t.Cleanup(s.Close)
	return s
}

// Key is the window key the stream's connection belongs to.
func (s *GitStream) Key() string { return s.key }

// Fire sends a request without waiting for its reply and returns a cancel for it, as a UI abandoning
// a call would. The reply, if any, is dropped.
func (s *GitStream) Fire(method string, params any) (cancel func()) {
	s.t.Helper()
	call := s.mustBound(method)
	ctx, stop := context.WithCancel(context.Background())
	raw := rawParams(s.t, params)
	go func() { _, _ = call(ctx, s.key, raw) }()
	return stop
}

// openPlane starts the graph data plane connection on first use.
func (s *GitStream) openPlane() *pipeConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plane != nil {
		return s.plane
	}
	client := &pipeConn{in: make(chan []byte, 64), out: make(chan []byte, 64)}
	server := &pipeConn{in: client.out, out: client.in}
	s.plane = client
	s.wg.Add(2)
	go func() {
		defer s.wg.Done()
		bridge.ServeGitGraph(context.Background(), s.app.W.GitWindows, server)
		server.closeOut()
	}()
	go func() {
		defer s.wg.Done()
		s.readLoop(client)
	}()
	return client
}

// ClosePlane drops the data plane connection, as a closing socket does; the next Stream redials.
func (s *GitStream) ClosePlane() {
	s.mu.Lock()
	plane := s.plane
	s.plane = nil
	s.mu.Unlock()
	if plane != nil {
		plane.closeOut()
	}
}

// Close releases the stream's holds, ends the plane connection and waits for its loops to return.
func (s *GitStream) Close() {
	s.closed.Do(func() {
		s.mu.Lock()
		held := s.held
		s.held = nil
		plane := s.plane
		s.mu.Unlock()
		for _, id := range held {
			_, _ = s.app.W.GitSvc.RepoClose(context.Background(), bridge.GitRepoCloseArgs{WindowKey: s.key, Params: gitrpc.RepoCloseParams{RepoID: id}})
		}
		if plane != nil {
			plane.closeOut()
		}
		s.app.W.GitWindows.Close(s.key)
	})
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.t.Error("git stream did not shut down within 10s")
	}
}

func (s *GitStream) readLoop(conn *pipeConn) {
	for raw := range conn.in {
		var blob []byte
		body := raw
		if len(raw) > 0 && raw[0] == 0x00 {
			n := int(binary.BigEndian.Uint32(raw[1:5]))
			body, blob = raw[5:5+n], raw[5+n:]
		}
		var f planeFrame
		if err := json.Unmarshal(body, &f); err != nil {
			s.t.Errorf("git stream: bad frame: %v", err)
			continue
		}
		s.mu.Lock()
		w := s.waiters[f.ID]
		s.mu.Unlock()
		if w != nil {
			w <- frameIn{planeFrame: f, blob: blob, raw: raw}
		}
	}
}

func (s *GitStream) send(f planeFrame) {
	body, err := json.Marshal(f)
	if err != nil {
		s.t.Fatal(err)
	}
	s.openPlane().out <- body
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

// mustBound returns the bound call for method; a method outside GitService fails the test.
func (s *GitStream) mustBound(method string) boundFn {
	s.t.Helper()
	call, ok := s.boundCall(method)
	if !ok {
		s.t.Fatalf("%s: not a GitService method", method)
	}
	return call
}

// Request sends one request through GitService and decodes the result into out (nil to discard).
// A server refusal comes back as *WireError.
func (s *GitStream) Request(method string, params, out any) error {
	s.t.Helper()
	return s.requestBound(s.mustBound(method), method, params, out)
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

// Stream opens a graph.stream on the data plane with an initial credit.
func (s *GitStream) Stream(method string, params any, credit int) *StreamCall {
	s.t.Helper()
	if method != "graph.stream" {
		s.t.Fatalf("%s: only graph.stream rides the data plane", method)
	}
	id, ch := s.register()
	s.send(planeFrame{T: "open", ID: id, WindowKey: s.key, Params: rawParams(s.t, params), Credit: credit})
	return &StreamCall{s: s, id: id, ch: ch}
}

// Credit grants n more chunks.
func (c *StreamCall) Credit(n int) { c.s.send(planeFrame{T: "credit", ID: c.id, N: n}) }

// Cancel aborts the stream.
func (c *StreamCall) Cancel() {
	c.s.send(planeFrame{T: "cancel", ID: c.id})
	c.s.unregister(c.id)
}

// Next returns the next chunk; ok is false once the stream ended, with err the stream's failure.
func (c *StreamCall) Next() (chunk Chunk, ok bool, err error) {
	c.s.t.Helper()
	f, ok, err := c.nextFrame()
	if !ok {
		return Chunk{}, false, err
	}
	return Chunk{JSON: f.Chunk, Blob: f.blob}, true, nil
}

// NextRaw is Next returning the chunk frame exactly as the plane sent it, blob header included.
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

// Quiet reports whether no frame arrives within d; any frame that does arrive is left unread.
func (c *StreamCall) Quiet(d time.Duration) bool {
	c.s.t.Helper()
	select {
	case f := <-c.ch:
		c.ch <- f
		return false
	case <-time.After(d):
		return true
	}
}

// OpenAgain sends a second open with this stream's id and returns the end error the plane answers
// it with, leaving the first stream live.
func (c *StreamCall) OpenAgain(params any) error {
	c.s.t.Helper()
	c.s.send(planeFrame{T: "open", ID: c.id, WindowKey: c.s.key, Params: rawParams(c.s.t, params), Credit: 1})
	select {
	case f := <-c.ch:
		if f.T != "end" || f.Error == nil {
			c.s.t.Fatalf("duplicate open answered %+v, want an end error", f.planeFrame)
		}
		return f.Error
	case <-time.After(requestTimeout):
		c.s.t.Fatalf("stream %d: no answer to the duplicate open within %s", c.id, requestTimeout)
		return nil
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
