package kubernetes

import (
	"bytes"
	"context"
	"io"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	corev1 "k8s.io/api/core/v1"
)

// mcpLogBytes caps one MCP log read.
const mcpLogBytes = 256 << 10

// ListCapped is List with a caller-chosen row cap.
func (s *Service) ListCapped(ctx context.Context, args ListArgs, max int) (ListResult, error) {
	return s.list(ctx, args, max)
}

// LogsReadArgs is ReadLogs' argument: a one-shot, non-follow read.
type LogsReadArgs struct {
	ClusterID  string
	Namespace  string
	Pod        string
	Container  string
	Previous   bool
	Timestamps bool
	Tail       int
}

// LogsReadResult holds the framed lines; Truncated means the byte limit dropped the oldest lines.
type LogsReadResult struct {
	Lines     []LogLine `json:"lines"`
	Truncated bool      `json:"truncated"`
}

// ReadLogs reads the last Tail lines of one container, keeping the newest mcpLogBytes of them.
func (s *Service) ReadLogs(ctx context.Context, args LogsReadArgs) (LogsReadResult, error) {
	switch {
	case args.ClusterID == "":
		return LogsReadResult{}, ipcerr.Required("clusterId")
	case args.Namespace == "":
		return LogsReadResult{}, ipcerr.Required("namespace")
	case args.Pod == "":
		return LogsReadResult{}, ipcerr.Required("pod")
	case args.Tail <= 0:
		return LogsReadResult{}, ipcerr.Contract("tail must be positive")
	}
	var res LogsReadResult
	err := s.record(ctx, args.ClusterID, false, func(n string) string { return LogsCommand(n, args, false) }, func(ctx context.Context, note *OpNote) error {
		var err error
		if res, err = s.readLogs(ctx, args); err == nil {
			note.SetRows(len(res.Lines))
		}
		return err
	})
	if err != nil {
		return LogsReadResult{}, err
	}
	return res, nil
}

func (s *Service) readLogs(ctx context.Context, args LogsReadArgs) (LogsReadResult, error) {
	cs, err := s.client(args.ClusterID)
	if err != nil {
		return LogsReadResult{}, mapErr(err)
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	tail := int64(args.Tail)
	body, err := cs.core.Pods(args.Namespace).GetLogs(args.Pod, &corev1.PodLogOptions{
		Container: args.Container, Previous: args.Previous, Timestamps: args.Timestamps, TailLines: &tail,
	}).Stream(cctx)
	if err != nil {
		return LogsReadResult{}, mapErr(err)
	}
	defer body.Close()
	raw, truncated, err := readTail(body, mcpLogBytes)
	if err != nil {
		return LogsReadResult{}, mapErr(err)
	}
	framer := newLineFramer(args.Timestamps)
	lines := append(framer.write(raw), framer.flush()...)
	if lines == nil {
		lines = []LogLine{}
	}
	return LogsReadResult{Lines: lines, Truncated: truncated}, nil
}

// readTail reads r to its end and keeps the last max bytes, starting at a whole line. The API server's
// LimitBytes would keep the oldest bytes instead, so the cap is applied here.
func readTail(r io.Reader, max int) (kept []byte, dropped bool, err error) {
	buf := make([]byte, 0, max)
	chunk := make([]byte, 32<<10)
	for {
		n, rerr := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if len(buf) > 2*max {
			buf, dropped = append(buf[:0], buf[len(buf)-max:]...), true
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, false, rerr
		}
	}
	if len(buf) > max {
		buf, dropped = buf[len(buf)-max:], true
	}
	if dropped {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return buf, dropped, nil
}
