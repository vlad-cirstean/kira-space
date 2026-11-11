package mobileweb

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Permission failure codes on the wire; the phone names the desktop pane that fixes each.
const (
	codeWriteOff      = string(ipcerr.CodeWriteOff)
	codeAgentInputOff = string(ipcerr.CodeAgentInputOff)
	codeUnavailable   = string(ipcerr.CodeUnavailable)
	maxWriteBody      = 64 << 10
	// writeCallLimit bounds one service call. A write detaches from the phone's connection so a
	// dropped request cannot abort it half way; the idempotency key lets the phone retry.
	writeCallLimit = 80 * time.Second
	// seenCallLimit bounds the best-effort seen mark an attach makes.
	seenCallLimit = 5 * time.Second
)

type permissionsBody struct {
	Write      bool `json:"write"`
	AgentInput bool `json:"agentInput"`
}

type meBody struct {
	DeviceID    string          `json:"deviceId"`
	Label       string          `json:"label"`
	Permissions permissionsBody `json:"permissions"`
	// AgentInputGlobal is the desktop's global switch, so the phone can say which switch is off.
	AgentInputGlobal bool `json:"agentInputGlobal"`
}

func (s *Server) agentInputGlobal() bool {
	return s.cfg.AgentInputEnabled != nil && s.cfg.AgentInputEnabled()
}

func (s *Server) handleMe(w http.ResponseWriter, _ *http.Request, d repos.MobileDeviceRow) {
	global := s.agentInputGlobal()
	writeJSON(w, http.StatusOK, meBody{
		DeviceID: d.ID, Label: d.Label, AgentInputGlobal: global,
		Permissions: permissionsBody{Write: d.CanWrite, AgentInput: d.CanAgentInput && global},
	})
}

// requirePerm refuses a device that lacks the row's permission. Agent input needs the device flag
// and the global switch: both are set on the desktop.
func (s *Server) requirePerm(rt route, next deviceHandler) deviceHandler {
	return func(w http.ResponseWriter, r *http.Request, dev repos.MobileDeviceRow) {
		switch rt.perm {
		case permWrite:
			if !dev.CanWrite {
				writeErrorT(w, http.StatusForbidden, codeWriteOff, "backend.space.mobileweb.writeOff.body")
				return
			}
		case permAgentInput:
			if !s.agentInputGlobal() {
				writeJSON(w, http.StatusForbidden, errorBody{Code: codeAgentInputOff, Reason: "global",
					Message: i18n.T("backend.space.mobileweb.agentInputOffGlobal.body", nil)})
				return
			}
			if !dev.CanAgentInput {
				writeJSON(w, http.StatusForbidden, errorBody{Code: codeAgentInputOff, Reason: "device",
					Message: i18n.T("backend.space.mobileweb.agentInputOffDevice.body", nil)})
				return
			}
		}
		next(w, r, dev)
	}
}

// writeLimit rate limits a write per device; an attach has its own, slower bucket.
func (s *Server) writeLimit(rt route, next deviceHandler) deviceHandler {
	return func(w http.ResponseWriter, r *http.Request, dev repos.MobileDeviceRow) {
		limiter := s.writeRate
		if rt.kind == kindUpgrade {
			limiter = s.attachRate
		}
		if !limiter.allow(dev.ID) {
			writeRateLimited(w)
			return
		}
		next(w, r, dev)
	}
}

type auditKey struct{}

// auditFields collects the ids a handler names for its audit line. Message text and keystrokes
// are never added.
type auditFields struct {
	mu sync.Mutex
	kv []any
}

func auditAdd(r *http.Request, kv ...any) {
	if f, ok := r.Context().Value(auditKey{}).(*auditFields); ok {
		f.mu.Lock()
		f.kv = append(f.kv, kv...)
		f.mu.Unlock()
	}
}

// logWrite is the one audit line of a phone write, attach, release or reclaim.
// markSeen clears a session's unseen marker. A failure only costs a stale dot, so it logs and goes on.
func (s *Server) markSeen(ctx context.Context, device, sessionID string) {
	if s.cfg.Writer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), seenCallLimit)
	defer cancel()
	if _, err := s.cfg.Writer.MarkSessionSeen(ctx, adewire.MarkSessionSeenArgs{SessionID: sessionID}); err != nil {
		slog.Warn("mobileweb: mark session seen", "scope", "mobileweb", "device", device, "err", err)
	}
}

func logWrite(device, action string, kv ...any) {
	audit := slog.With("scope", "mobileweb", "device", device, "action", action)
	audit.Info("mobileweb: write", kv...)
}

func (s *Server) audited(rt route, next deviceHandler) deviceHandler {
	return func(w http.ResponseWriter, r *http.Request, dev repos.MobileDeviceRow) {
		f := &auditFields{}
		rec := newRecorder(w, false)
		r = r.WithContext(context.WithValue(r.Context(), auditKey{}, f))
		next(rec, r, dev)
		f.mu.Lock()
		defer f.mu.Unlock()
		logWrite(dev.ID, rt.action, append(f.kv, "status", rec.status)...)
	}
}

// decodeBody reads one strict JSON object; false means the 400 is written.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxWriteBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErrorT(w, http.StatusBadRequest, string(ipcerr.CodeBadRequest), "backend.space.mobileweb.invalidBody.body")
		return false
	}
	return true
}

// call runs one Writer method for a decoded request on a context detached from the phone's
// connection.
func writeRoute[Req, Res any](s *Server, call func(ctx context.Context, wr Writer, r *http.Request, req Req) (Res, error)) deviceHandler {
	return func(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
		if s.cfg.Writer == nil {
			writeErrorT(w, http.StatusServiceUnavailable, codeUnavailable, "backend.space.mobileweb.changesUnavailable.body")
			return
		}
		var req Req
		if !decodeBody(w, r, &req) {
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), writeCallLimit)
		defer cancel()
		res, err := call(ctx, s.cfg.Writer, r, req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

type empty struct{}

// taskStepRequest is what a phone sends to act on a task: the step it saw, so the engine can
// refuse when the task has moved on. An empty value would skip the guard, so it is required.
type taskStepRequest struct {
	TaskID     string `json:"taskId"`
	FromStepID string `json:"fromStepId"`
}

var errFromStep = errors.New("fromStepId is required")

func (q taskStepRequest) validate() error {
	if q.FromStepID == "" {
		return errFromStep
	}
	return nil
}

type taskStepResult struct {
	TaskID string `json:"taskId"`
	StepID string `json:"stepId"`
}

type runResult struct {
	RunIDs []string `json:"runIds"`
}

type sendRequest struct {
	Message string `json:"message"`
}

type takeOverRequest struct{}

func badRequest(err error) error { return ipcerr.BadRequest(err.Error()) }

func (s *Server) handleLaterAdd(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req adewire.CaptureTaskArgs) (adewire.Task, error) {
		t, err := wr.CaptureTask(ctx, req)
		if err == nil {
			auditAdd(r, "task", t.ID)
		}
		return t, err
	})(w, r, d)
}

func (s *Server) handleShelfMove(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req adewire.ShelveTaskArgs) (empty, error) {
		auditAdd(r, "task", req.TaskID, "shelf", req.Shelf)
		return empty{}, wr.ShelveTask(ctx, req)
	})(w, r, d)
}

func (s *Server) handleTaskStep(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req adewire.SetTaskStepArgs) (taskStepResult, error) {
		if req.FromStepID == "" {
			return taskStepResult{}, badRequest(errFromStep)
		}
		auditAdd(r, "task", req.TaskID, "from", req.FromStepID, "to", req.StepID)
		t, err := wr.SetTaskStep(ctx, req)
		return taskStepResult{TaskID: t.ID, StepID: t.EntryStepID}, err
	})(w, r, d)
}

func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req taskStepRequest) (runResult, error) {
		if err := req.validate(); err != nil {
			return runResult{}, badRequest(err)
		}
		auditAdd(r, "task", req.TaskID, "step", req.FromStepID)
		res, err := wr.StartRun(ctx, adewire.StartRunArgs{
			TaskID: req.TaskID, FromStepID: req.FromStepID, BranchNames: map[string]string{},
		})
		return runResult{RunIDs: res.RunIDs}, err
	})(w, r, d)
}

func (s *Server) handleTaskLaunch(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req taskStepRequest) (LaunchResult, error) {
		if err := req.validate(); err != nil {
			return LaunchResult{}, badRequest(err)
		}
		auditAdd(r, "task", req.TaskID, "step", req.FromStepID)
		return wr.LaunchStep(ctx, adewire.LaunchStepArgs{TaskID: req.TaskID, FromStepID: req.FromStepID})
	})(w, r, d)
}

func (s *Server) handleSessionSend(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, req sendRequest) (empty, error) {
		id := r.PathValue("id")
		auditAdd(r, "session", id)
		return empty{}, wr.Send(ctx, adewire.SendArgs{SessionID: id, Message: req.Message})
	})(w, r, d)
}

func (s *Server) handleSessionTakeOver(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	writeRoute(s, func(ctx context.Context, wr Writer, r *http.Request, _ takeOverRequest) (LaunchResult, error) {
		id := r.PathValue("id")
		auditAdd(r, "session", id)
		return wr.TakeOver(ctx, adewire.TakeOverArgs{SessionID: id, StopIfRunning: true})
	})(w, r, d)
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) {
	if s.cfg.Terminals == nil {
		writeErrorT(w, http.StatusServiceUnavailable, codeUnavailable, "backend.space.mobileweb.terminalsUnavailable.body")
		return
	}
	id := r.PathValue("id")
	logWrite(d.ID, "terminal.attach", "session", id)
	var seen sync.Once
	s.cfg.Terminals.Serve(w, r, d, id, func() bool {
		row, found, err := s.cfg.Devices.ByID(d.ID)
		ok := err == nil && found && row.RevokedAt == nil && row.ExpiresAt > s.cfg.Now().UnixMilli() && row.CanAgentInput && s.agentInputGlobal()
		if ok {
			// Attaching is the phone's view of the session; best effort.
			seen.Do(func() { s.markSeen(r.Context(), d.ID, id) })
		}
		return ok
	})
}
