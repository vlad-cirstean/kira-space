// Package graphplane serves the git graph bulk data plane: one connection carrying many
// credit-gated chunk streams, JSON text requests in, binary blob frames out. It never learns what a
// chunk means; the OpenFunc supplies the stream body.
//
// Inbound text frames:
//
//	{"t":"open","id":n,"windowKey":"...","params":{...},"credit":2}
//	{"t":"credit","id":n,"n":1}
//	{"t":"cancel","id":n}
//
// Outbound frames: a chunk is 0x00 | uint32BE headerLen | header JSON | blob, header
// {"t":"chunk","id":n,"chunk":{...}}; a stream ends with {"t":"end","id":n} plus an "error"
// {code,message} on failure.
package graphplane

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Conn is one renderer connection carrying whole frames; *application.StreamConn satisfies it.
type Conn interface {
	Send(frame []byte) error
	Receive() ([]byte, error)
}

// OpenFunc serves one opened stream, calling emit for each chunk. emit waits for a credit, so it
// returns the stream's ctx error once the stream is cancelled; the caller must end on any error.
type OpenFunc func(ctx context.Context, windowKey string, params json.RawMessage, emit func(payload any, blob []byte) error) error

const (
	sendQueueCap  = 16
	blobDiscrimin = 0x00
)

type inFrame struct {
	T         string          `json:"t"`
	ID        int             `json:"id"`
	WindowKey string          `json:"windowKey"`
	Params    json.RawMessage `json:"params"`
	Credit    int             `json:"credit"`
	N         int             `json:"n"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type outHeader struct {
	T     string          `json:"t"`
	ID    int             `json:"id"`
	Chunk json.RawMessage `json:"chunk,omitempty"`
	Error *wireError      `json:"error,omitempty"`
}

type stream struct {
	cancel context.CancelFunc
	gate   *creditGate
}

type plane struct {
	conn     Conn
	open     OpenFunc
	maxFrame int
	ctx      context.Context

	sendCh chan []byte
	done   chan struct{}
	wg     sync.WaitGroup

	mu      sync.Mutex
	streams map[int]*stream
}

// Serve runs one connection until its peer closes or ctx ends, then cancels every stream and waits
// for them. maxFrameBytes caps one encoded chunk frame; zero means unbounded.
func Serve(ctx context.Context, conn Conn, open OpenFunc, maxFrameBytes int) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := &plane{
		conn:     conn,
		open:     open,
		maxFrame: maxFrameBytes,
		ctx:      ctx,
		sendCh:   make(chan []byte, sendQueueCap),
		done:     make(chan struct{}),
		streams:  map[int]*stream{},
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		p.writeLoop()
	}()
	for {
		raw, err := conn.Receive()
		if err != nil {
			break
		}
		p.handle(raw)
	}
	cancel()
	p.wg.Wait()
	close(p.done)
	<-writerDone
}

func (p *plane) writeLoop() {
	for {
		select {
		case b := <-p.sendCh:
			_ = p.conn.Send(b) // a write failure means the peer is going; Receive observes it.
		case <-p.done:
			return
		}
	}
}

func (p *plane) enqueue(b []byte) {
	select {
	case p.sendCh <- b:
	case <-p.done:
	}
}

func (p *plane) handle(raw []byte) {
	var f inFrame
	if err := json.Unmarshal(raw, &f); err != nil {
		return
	}
	switch f.T {
	case "open":
		p.handleOpen(f)
	case "credit":
		p.mu.Lock()
		s := p.streams[f.ID]
		p.mu.Unlock()
		if s != nil {
			s.gate.grant(f.N)
		}
	case "cancel":
		p.mu.Lock()
		s := p.streams[f.ID]
		delete(p.streams, f.ID)
		p.mu.Unlock()
		if s != nil {
			s.cancel()
		}
	}
}

func (p *plane) handleOpen(f inFrame) {
	ctx, cancel := context.WithCancel(p.ctx)
	s := &stream{cancel: cancel, gate: newCreditGate()}
	p.mu.Lock()
	if _, live := p.streams[f.ID]; live {
		p.mu.Unlock()
		cancel()
		p.sendEnd(f.ID, ipcerr.Contract("graphplane: stream id is already live"))
		return
	}
	p.streams[f.ID] = s
	p.mu.Unlock()
	if f.Credit > 0 {
		s.gate.grant(f.Credit)
	}
	p.wg.Add(1)
	go p.run(ctx, f, s)
}

func (p *plane) run(ctx context.Context, f inFrame, s *stream) {
	defer p.wg.Done()
	emit := func(payload any, blob []byte) error {
		if err := s.gate.acquire(ctx); err != nil {
			return err
		}
		chunk, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		b, err := encodeChunk(outHeader{T: "chunk", ID: f.ID, Chunk: chunk}, blob)
		if err != nil {
			return err
		}
		if p.maxFrame > 0 && len(b) > p.maxFrame {
			return ipcerr.T(ipcerr.CodeFrameTooLarge, "backend.space.gitGraph.frameTooLarge.body", nil)
		}
		p.enqueue(b)
		return nil
	}
	err := p.open(ctx, f.WindowKey, f.Params, emit)
	s.cancel()

	p.mu.Lock()
	live := p.streams[f.ID] == s
	if live {
		delete(p.streams, f.ID)
	}
	p.mu.Unlock()
	if !live {
		return // cancelled by the client, which no longer waits for an end
	}
	p.sendEnd(f.ID, err)
}

func (p *plane) sendEnd(id int, err error) {
	h := outHeader{T: "end", ID: id}
	if err != nil {
		h.Error = wireErrorFrom(err)
	}
	b, merr := json.Marshal(h)
	if merr != nil {
		return
	}
	p.enqueue(b)
}

// encodeChunk lays out 0x00 | uint32BE headerLen | header | blob, or plain header JSON for a nil blob.
func encodeChunk(h outHeader, blob []byte) ([]byte, error) {
	header, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	if blob == nil {
		return header, nil
	}
	out := make([]byte, 0, 1+4+len(header)+len(blob))
	out = append(out, blobDiscrimin)
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(header)))
	out = append(out, n[:]...)
	out = append(out, header...)
	return append(out, blob...), nil
}

func wireErrorFrom(err error) *wireError {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return &wireError{Code: ie.Code, Message: ie.Message}
	}
	return &wireError{Code: string(ipcerr.CodeInternal), Message: err.Error()}
}
