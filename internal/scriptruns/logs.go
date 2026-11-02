package scriptruns

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/internal/termtext"
)

const (
	logFlushInterval = 250 * time.Millisecond
	maxLogLineBytes  = 2 << 10
	tailLines        = 40
)

// LogPush is what a run's log pushes carry: the chunks just stored.
type LogPush struct {
	RunID  string     `json:"runId"`
	Chunks []LogChunk `json:"chunks"`
}

// logSink buffers one run's lines, stores them as a batch and pushes the stored chunks. Safe for
// concurrent add.
type logSink struct {
	svc   *Service
	runID string
	mask  *secrets.Masker

	flushMu sync.Mutex // keeps stored seq order equal to push order
	mu      sync.Mutex
	seq     int
	pending []LogChunk
	timer   *time.Timer
	lastErr string
	tail    []string
}

func newLogSink(svc *Service, runID string, mask *secrets.Masker) *logSink {
	return &logSink{svc: svc, runID: runID, mask: mask}
}

func (l *logSink) add(stream, text string) {
	text = l.mask.Mask(text)
	if len(text) > maxLogLineBytes {
		text = strings.ToValidUTF8(text[:maxLogLineBytes], "") + "…"
	}
	l.mu.Lock()
	l.tail = append(l.tail, text)
	if len(l.tail) > tailLines {
		l.tail = l.tail[1:]
	}
	l.seq++
	l.pending = append(l.pending, LogChunk{Seq: l.seq, Stream: stream, Text: text})
	if stream == "stderr" && strings.TrimSpace(text) != "" {
		l.lastErr = text
	}
	if l.timer == nil {
		l.timer = time.AfterFunc(logFlushInterval, l.flush)
	}
	l.mu.Unlock()
}

// lastStderr is the last non-blank stderr line added.
func (l *logSink) lastStderr() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

// outputTail is the last added lines, sanitized, blank ones dropped.
func (l *logSink) outputTail() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.tail))
	for _, t := range l.tail {
		if t = strings.TrimSpace(termtext.SanitizeLine(t)); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

// flush stores and pushes what is pending; also the final flush when a run ends.
func (l *logSink) flush() {
	l.flushMu.Lock()
	defer l.flushMu.Unlock()
	l.mu.Lock()
	batch := l.pending
	l.pending = nil
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	l.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := l.svc.Runs.AppendLogs(l.runID, batch); err != nil {
		slog.Warn("scriptruns: append log", "scope", "scriptruns", "run", l.runID, "err", err)
		return
	}
	if l.svc.EmitLog != nil {
		l.svc.EmitLog(LogPush{RunID: l.runID, Chunks: batch})
	}
}
