package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	corev1 "k8s.io/api/core/v1"
)

const (
	maxLogLine       = 64 * 1024
	truncationMarker = "…"
	logsCoalesce     = 16 * time.Millisecond
	logsCoalesceMax  = 500
	maxBacklogBytes  = 8 << 20
	followTailAll    = 20000
	readChunk        = 32 * 1024
)

// LogLine is one framed log line.
type LogLine struct {
	TS   string `json:"ts,omitempty"`
	Text string `json:"text"`
}

// LogsEvent is ChannelLogs' payload: a batch of lines for one stream, or its end.
type LogsEvent struct {
	StreamID string    `json:"streamId"`
	Lines    []LogLine `json:"lines"`
	Ended    bool      `json:"ended"`
	Error    string    `json:"error,omitempty"`
}

// lineFramer turns raw chunks into lines: it keeps the partial line across reads, strips one
// trailing CR, truncates a line at maxLogLine and splits the RFC3339Nano timestamp prefix.
type lineFramer struct {
	timestamps bool
	buf        []byte
	skipping   bool // dropping the rest of an over-long line
}

func newLineFramer(timestamps bool) *lineFramer { return &lineFramer{timestamps: timestamps} }

func (f *lineFramer) write(p []byte) []LogLine {
	var out []LogLine
	for len(p) > 0 {
		nl := bytes.IndexByte(p, '\n')
		chunk := p
		if nl >= 0 {
			chunk = p[:nl]
		}
		if !f.skipping {
			f.buf = append(f.buf, chunk...)
			if len(f.buf) > maxLogLine {
				out = append(out, f.line(f.buf[:maxLogLine], true))
				f.buf, f.skipping = nil, true
			}
		}
		if nl < 0 {
			break
		}
		if f.skipping {
			f.skipping = false
		} else {
			out = append(out, f.line(f.buf, false))
			f.buf = nil
		}
		p = p[nl+1:]
	}
	return out
}

// flush emits the trailing partial line at EOF.
func (f *lineFramer) flush() []LogLine {
	if len(f.buf) == 0 {
		return nil
	}
	l := f.line(f.buf, false)
	f.buf = nil
	return []LogLine{l}
}

func (f *lineFramer) line(raw []byte, truncated bool) LogLine {
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	var ts string
	if f.timestamps {
		if sp := bytes.IndexByte(raw, ' '); sp > 0 {
			if _, err := time.Parse(time.RFC3339Nano, string(raw[:sp])); err == nil {
				ts, raw = string(raw[:sp]), raw[sp+1:]
			}
		}
	}
	text := strings.ToValidUTF8(string(raw), "�")
	if truncated {
		text += truncationMarker
	}
	return LogLine{TS: ts, Text: text}
}

type logStream struct {
	windowKey string
	clusterID string
	cancel    context.CancelFunc
}

type logRegistry struct {
	mu      sync.Mutex
	streams map[string]*logStream
}

func newLogRegistry() *logRegistry { return &logRegistry{streams: map[string]*logStream{}} }

func (r *logRegistry) add(id string, s *logStream) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.streams[id]; dup {
		return false
	}
	r.streams[id] = s
	return true
}

func (r *logRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.streams, id)
}

// closeWhere cancels and forgets every stream match accepts.
func (r *logRegistry) closeWhere(match func(id string, s *logStream) bool) {
	r.mu.Lock()
	var cancels []context.CancelFunc
	for id, s := range r.streams {
		if match(id, s) {
			cancels = append(cancels, s.cancel)
			delete(r.streams, id)
		}
	}
	r.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

func (r *logRegistry) close(id string) {
	r.closeWhere(func(sid string, _ *logStream) bool { return sid == id })
}

func (r *logRegistry) closeWindow(key string) {
	r.closeWhere(func(_ string, s *logStream) bool { return s.windowKey == key })
}

func (r *logRegistry) closeCluster(id string) {
	r.closeWhere(func(_ string, s *logStream) bool { return s.clusterID == id })
}

func (r *logRegistry) closeAll() { r.closeWhere(func(string, *logStream) bool { return true }) }

type logsFinal struct{ err string }

// LogsOpenArgs is LogsOpen's wire shape.
type LogsOpenArgs struct {
	WindowKey  string `json:"windowKey"`
	StreamID   string `json:"streamId"`
	ClusterID  string `json:"clusterId"`
	Namespace  string `json:"namespace"`
	Pod        string `json:"pod"`
	Container  string `json:"container"`
	Previous   bool   `json:"previous"`
	Tail       int    `json:"tail"` // 100 | 1000 | 5000 | -1 (all)
	Timestamps bool   `json:"timestamps"`
	Follow     bool   `json:"follow"`
}

func (a LogsOpenArgs) validate() error {
	switch {
	case a.WindowKey == "":
		return ipcerr.Required("windowKey")
	case a.StreamID == "":
		return ipcerr.Required("streamId")
	case a.ClusterID == "":
		return ipcerr.Required("clusterId")
	case a.Namespace == "":
		return ipcerr.Required("namespace")
	case a.Pod == "":
		return ipcerr.Required("pod")
	}
	switch a.Tail {
	case 100, 1000, 5000, -1:
		return nil
	}
	return ipcerr.Contract("tail must be 100, 1000, 5000 or -1")
}

func (a LogsOpenArgs) options() *corev1.PodLogOptions {
	o := &corev1.PodLogOptions{Container: a.Container, Previous: a.Previous, Timestamps: a.Timestamps, Follow: a.Follow}
	tail := int64(a.Tail)
	switch {
	case a.Tail >= 0:
		o.TailLines = &tail
	case a.Follow:
		// LimitBytes would end the follow stream; the view keeps only the newest lines anyway.
		all := int64(followTailAll)
		o.TailLines = &all
	}
	if !a.Follow {
		limit := int64(maxBacklogBytes)
		o.LimitBytes = &limit
	}
	return o
}

// LogsOpen starts streaming one pod's logs to the window over ChannelLogs. Open errors (unknown
// container, no previous log) return here; later failures arrive as the stream's final event.
func (s *Service) LogsOpen(args LogsOpenArgs) error {
	if err := args.validate(); err != nil {
		return err
	}
	cs, err := s.client(args.ClusterID)
	if err != nil {
		return mapErr(err)
	}
	_, name, _, err := s.target(args.ClusterID)
	if err != nil {
		return err
	}
	base, stopBase := context.WithCancel(context.Background())
	if !s.logs.add(args.StreamID, &logStream{windowKey: args.WindowKey, clusterID: args.ClusterID, cancel: stopBase}) {
		stopBase()
		return ipcerr.Contract("streamId is already open")
	}
	cmd := LogsCommand(name, LogsReadArgs{
		Namespace: args.Namespace, Pod: args.Pod, Container: args.Container,
		Previous: args.Previous, Timestamps: args.Timestamps, Tail: args.Tail,
	}, args.Follow)
	opened := make(chan error, 1)
	var once sync.Once
	report := func(err error) { once.Do(func() { opened <- err }) }
	go func() {
		defer stopBase()
		reported := false
		err := s.rec.Record(context.Background(), cmd, func(runCtx context.Context, note *OpNote) error {
			return s.runLogStream(runCtx, base, cs, args, note, func(err error) {
				reported = true
				report(err)
			})
		})
		// Not reported: the recorder skipped the call or recovered a panic before the open result.
		if !reported {
			s.logs.remove(args.StreamID)
			if err == nil {
				err = errors.New("log stream ended before it opened")
			}
			report(err)
		}
	}()
	return <-opened
}

// runLogStream is the stream's recorded call: it reports the open result once via report, then pumps
// until the stream ends. stop is the registry's handle; runCtx ends with a panel Cancel.
func (s *Service) runLogStream(runCtx, stop context.Context, cs *clusterClient, args LogsOpenArgs, note *OpNote, report func(error)) error {
	ctx, cancel := context.WithCancel(runCtx)
	defer cancel()
	defer context.AfterFunc(stop, cancel)()
	body, err := openLogStream(ctx, cancel, cs, args)
	if err != nil {
		s.logs.remove(args.StreamID)
		err = mapErr(err)
		report(err)
		return err
	}
	report(nil)
	streamErr := s.pumpLogs(ctx, args, body, note)
	if runCtx.Err() != nil {
		return context.Canceled
	}
	return streamErr
}

// openLogStream opens the stream under ctx; cancel is ctx's own, fired if the open outlives callTimeout.
func openLogStream(ctx context.Context, cancel context.CancelFunc, cs *clusterClient, args LogsOpenArgs) (io.ReadCloser, error) {
	timer := time.AfterFunc(callTimeout, cancel)
	body, err := cs.core.Pods(args.Namespace).GetLogs(args.Pod, args.options()).Stream(ctx)
	if !timer.Stop() {
		if err == nil {
			_ = body.Close()
		}
		return nil, context.DeadlineExceeded
	}
	return body, err
}

// pumpLogs streams body to the window and returns the read error that ended it, nil on a normal end.
func (s *Service) pumpLogs(ctx context.Context, args LogsOpenArgs, body io.ReadCloser, note *OpNote) error {
	defer s.logs.remove(args.StreamID)
	defer body.Close()

	co := appevent.NewCoalescer(logsCoalesce, logsCoalesceMax, func(LogLine) int { return 1 },
		func(batch []LogLine, done bool, final logsFinal) {
			s.emit.EmitTo(args.WindowKey, ChannelLogs, LogsEvent{StreamID: args.StreamID, Lines: batch, Ended: done, Error: final.err})
		})
	framer := newLineFramer(args.Timestamps)
	buf := make([]byte, readChunk)
	var err error
	lines := 0
	for {
		var n int
		n, err = body.Read(buf)
		for _, l := range framer.write(buf[:n]) {
			co.Push(l)
			lines++
		}
		if err != nil {
			break
		}
	}
	for _, l := range framer.flush() {
		co.Push(l)
		lines++
	}
	note.SetRows(lines)
	final := logsFinal{}
	if ctx.Err() == nil && !errors.Is(err, io.EOF) {
		final.err = err.Error()
	}
	co.Finish(final)
	if final.err != "" {
		return err
	}
	return nil
}

// LogsClose ends one stream.
func (s *Service) LogsClose(streamID string) { s.logs.close(streamID) }

// CloseWindow ends every stream the window started.
func (s *Service) CloseWindow(windowKey string) { s.logs.closeWindow(windowKey) }

// Shutdown ends every stream.
func (s *Service) Shutdown() { s.logs.closeAll() }
