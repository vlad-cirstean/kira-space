package ade

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// logFlushInterval batches log writes and pushes (gitprepare's batch cadence is 100 ms; this
// store-bound cadence is 250 ms).
const logFlushInterval = 250 * time.Millisecond

// Log streams on the wire.
const (
	logStdout = "stdout"
	logStderr = "stderr"
	logEvent  = "event"
)

// logSink buffers one log's lines, stores them as a batch and pushes the stored chunks on
// kira:adetask:log. Safe for concurrent add.
type logSink struct {
	b                *TaskBoard
	kind, id, taskID string

	// mask hides the secret values of the app's agent settings in stored lines; nil masks nothing.
	mask *secrets.Masker

	flushMu sync.Mutex // keeps stored seq order equal to push order
	mu      sync.Mutex
	pending []repos.AdeLogChunk
	timer   *time.Timer
	lastErr string
}

func (b *TaskBoard) newLogSink(kind, id, taskID string) *logSink {
	return &logSink{b: b, kind: kind, id: id, taskID: taskID}
}

// setMask sets the mask lines pass through, before the run starts.
func (s *logSink) setMask(m *secrets.Masker) {
	s.mu.Lock()
	s.mask = m
	s.mu.Unlock()
}

func (s *logSink) add(stream, text string) {
	s.mu.Lock()
	text = s.mask.Mask(text)
	s.pending = append(s.pending, repos.AdeLogChunk{At: s.b.deps.Now().UnixMilli(), Stream: stream, Text: text})
	if stream == logStderr && strings.TrimSpace(text) != "" {
		s.lastErr = text
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(logFlushInterval, s.flush)
	}
	s.mu.Unlock()
}

// lastStderr is the last non-blank stderr line added.
func (s *logSink) lastStderr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// flush stores and pushes what is pending; also the final flush when a run ends.
func (s *logSink) flush() {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	s.mu.Lock()
	batch := s.pending
	s.pending = nil
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	stored, err := s.b.deps.Logs.Append(s.kind, s.id, batch)
	if err != nil {
		slog.Warn("ade log append", "scope", "ade", "kind", s.kind, "id", s.id, "err", err)
		return
	}
	if s.b.deps.OnLog != nil {
		s.b.deps.OnLog(adewire.LogEvent{Kind: s.kind, ID: s.id, Chunks: toWireChunks(stored)})
	}
}

func toWireChunks(in []repos.AdeLogChunk) []adewire.LogChunk {
	out := make([]adewire.LogChunk, len(in))
	for i, c := range in {
		out[i] = adewire.LogChunk{Seq: c.Seq, At: c.At, Stream: c.Stream, Text: c.Text}
	}
	return out
}
