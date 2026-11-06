package dbmcp

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/mcpcall"
)

// Audit statuses of a finished tool call.
const (
	AuditOK        = "ok"
	AuditError     = "error"
	AuditRefused   = "refused"
	AuditCancelled = "cancelled"
)

// AuditCall is a session-server tool call at its start. Args is the raw arguments JSON.
type AuditCall struct {
	ID       string
	Identity SessionIdentity
	Tool     string
	Args     []byte
	Started  time.Time
}

// AuditEnd is a call's verdict. ErrCode is set when the failure carried an adapter or ipcerr code;
// ErrText is then raw and must not be stored. Without a code, ErrText is dbmcp's own refusal wording.
type AuditEnd struct {
	ID      string
	Status  string
	ErrCode string
	ErrText string
	Ended   time.Time
}

// Auditor records the tool calls of agent sessions. Only the session server carries one.
type Auditor interface {
	CallStarted(AuditCall)
	CallFinished(AuditEnd)
}

// AuditErrorText is the fixed wording stored for a failure with a known code. The adapter message
// can echo the failing value, so it is never kept.
func AuditErrorText(code string) string { return code + ": failed (message not recorded)" }

var errCodePrefix = regexp.MustCompile(`^(E_[A-Z0-9_]+): `)

// auditHook is the mcpcall.Hook of the session server: it opens an audit row per tools/call and
// stamps its id on the request origin so approvals raised inside link to it.
func (s *Server) auditHook(ctx context.Context, c mcpcall.Call) (context.Context, func(bool, string, error)) {
	noop := func(bool, string, error) {}
	id, ok := SessionFrom(ctx)
	if !ok || id.TaskID == "" || s.cfg.Audit == nil {
		return ctx, noop
	}
	callID := ids.NewID()
	s.cfg.Audit.CallStarted(AuditCall{ID: callID, Identity: id, Tool: c.Tool, Args: c.Args, Started: time.Now()})
	origin := originFrom(ctx)
	origin.CallID = callID
	ctx = context.WithValue(ctx, originContextKey, origin)
	return ctx, func(isError bool, text string, err error) {
		end := AuditEnd{ID: callID, Ended: time.Now()}
		switch {
		case err != nil && !errors.Is(err, context.Canceled):
			end.Status, end.ErrText = AuditError, "internal error"
		case errors.Is(ctx.Err(), context.Canceled):
			end.Status = AuditCancelled
		case isError:
			if m := errCodePrefix.FindStringSubmatch(text); m != nil {
				end.Status, end.ErrCode = AuditError, m[1]
			} else {
				end.Status, end.ErrText = AuditRefused, text
			}
		default:
			end.Status = AuditOK
		}
		s.cfg.Audit.CallFinished(end)
	}
}
