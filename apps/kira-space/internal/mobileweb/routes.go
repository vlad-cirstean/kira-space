package mobileweb

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

type access int

const (
	accessPublic access = iota // reachable before pairing
	accessDevice               // requires a paired, unrevoked device
)

// perm is what a device row additionally needs: nothing, the device's write flag, or the agent
// input flag plus the global agent input switch.
type perm int

const (
	permNone perm = iota
	permWrite
	permAgentInput
)

// kind says how a row's connection lives: a plain request gets a write deadline, a stream (SSE,
// the pairing long-poll) sets its own, an upgrade is hijacked into a WebSocket.
type kind int

const (
	kindPlain kind = iota
	kindStream
	kindUpgrade
)

type deviceHandler = func(http.ResponseWriter, *http.Request, repos.MobileDeviceRow)

type route struct {
	method string
	path   string
	access access
	perm   perm
	kind   kind
	// action names the audit line of a write; reads leave it empty.
	action string
	handle deviceHandler
}

const (
	apiDeadline   = 30 * time.Second
	writeDeadline = 90 * time.Second
	flightLimit   = 25 * time.Second
)

func read(path string, h deviceHandler) route {
	return route{method: http.MethodGet, path: path, access: accessDevice, handle: h}
}

func write(path, action string, p perm, h deviceHandler) route {
	return route{method: http.MethodPost, path: path, access: accessDevice, perm: p, action: action, handle: h}
}

// routes is the complete app-port API. The mux registers exactly these, and a test walks them: a
// new endpoint is a deliberate row here. A row that changes state names its permission and audit
// action; appMux then adds the CSRF check, the device cookie, the permission gate, the write rate
// limit, the idempotency protocol and the audit line around it, so a row is only its handler.
func (s *Server) routes() []route {
	return []route{
		{method: http.MethodPost, path: "/api/pair", access: accessPublic, kind: kindStream,
			handle: func(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) { s.handlePair(w, r) }},
		read("/api/me", s.handleMe),
		{method: http.MethodGet, path: "/api/events", access: accessDevice, kind: kindStream,
			handle: func(w http.ResponseWriter, r *http.Request, d repos.MobileDeviceRow) { s.handleEvents(w, r, d) }},
		read("/api/ade/board", readJSON(s, "board", true, s.cfg.Reader.Board)),
		read("/api/ade/change-requests", readJSON(s, "change-requests", true, s.cfg.Reader.ChangeRequests)),
		read("/api/ade/sessions", readJSON(s, "sessions", false, func(ctx context.Context) (any, error) {
			v, err := s.cfg.Reader.Sessions(ctx)
			if err != nil {
				return nil, err
			}
			return withoutCwd(v)
		})),
		read("/api/ade/workflows", readJSON(s, "workflows", false, s.cfg.Reader.Workflows)),
		read("/api/ade/repos", readJSON(s, "repos", false, s.repoNames)),
		read("/api/ade/hosts", readJSON(s, "hosts", false, s.cfg.Reader.HostProviders)),
		read("/api/ade/log", s.handleLog),
		read("/api/ade/archived-task", s.handleArchivedTask),
		read("/api/agent/sessions", func(w http.ResponseWriter, _ *http.Request, _ repos.MobileDeviceRow) {
			v, err := withoutCwd(s.cfg.AgentSessions())
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, v)
		}),
		read("/api/agent/info", func(w http.ResponseWriter, _ *http.Request, _ repos.MobileDeviceRow) {
			if s.cfg.AgentInfo == nil {
				writeJSON(w, http.StatusOK, map[string]any{})
				return
			}
			writeJSON(w, http.StatusOK, s.cfg.AgentInfo())
		}),
		read("/api/runs", s.handleRuns),
		read("/api/computer", s.handleComputer),
		read("/api/approvals", s.handleApprovals),
		write("/api/ade/later/items", "later.add", permWrite, s.handleLaterAdd),
		write("/api/ade/shelf/move", "shelf.move", permWrite, s.handleShelfMove),
		write("/api/ade/tasks/step", "task.step", permWrite, s.handleTaskStep),
		write("/api/ade/tasks/run", "task.run", permWrite, s.handleTaskRun),
		write("/api/ade/tasks/launch", "task.launch", permWrite, s.handleTaskLaunch),
		write("/api/agent/sessions/{id}/send", "session.send", permAgentInput, s.handleSessionSend),
		write("/api/agent/sessions/{id}/take-over", "session.takeOver", permAgentInput, s.handleSessionTakeOver),
		write("/api/approvals/answer", "approval.answer", permAgentInput, s.handleApprovalAnswer),
		{method: http.MethodGet, path: "/api/agent/sessions/{id}/terminal", access: accessDevice,
			perm: permAgentInput, kind: kindUpgrade, action: "terminal.attach", handle: s.handleTerminal},
	}
}

// appMux registers the route table plus the static shell. Everything under /api not in the table
// is a 404/405 from the mux or the static handler, never a handler.
func (s *Server) appMux() *http.ServeMux {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		var h http.Handler
		if rt.access == accessDevice {
			h = s.withDevice(s.gated(rt))
		} else {
			h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { rt.handle(w, r, repos.MobileDeviceRow{}) })
		}
		h = noStore(csrfGuard(rt, deadline(rt, h)))
		mux.Handle(rt.method+" "+rt.path, h)
	}
	mux.Handle("GET /", s.staticHandler())
	return mux
}

// gated wraps a device row's handler with everything a write needs, outermost first: permission,
// rate limit, idempotency (POST), audit. Reads pass through untouched.
func (s *Server) gated(rt route) deviceHandler {
	h := rt.handle
	if rt.perm == permNone {
		return h
	}
	if rt.kind != kindUpgrade {
		h = s.audited(rt, h)
		if rt.method == http.MethodPost {
			h = s.idempotent(rt, h)
		}
	}
	return s.requirePerm(rt, s.writeLimit(rt, h))
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// deadline bounds slow clients per route: the server has no global WriteTimeout because the SSE
// stream, the pairing long-poll and a hijacked terminal outlive any single value. Those set their
// own. A write may wait on git work or the desktop window, so it gets a longer bound.
func deadline(rt route, next http.Handler) http.Handler {
	if rt.kind != kindPlain {
		return next
	}
	limit := apiDeadline
	if rt.perm != permNone {
		limit = writeDeadline
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(limit))
		next.ServeHTTP(w, r)
	})
}

// readJSON serves one Reader call. Board and Prs each do git work per repo, so concurrent
// requests share one in-flight call (detached from the first caller's cancellation).
func readJSON[T any](s *Server, key string, coalesce bool, fetch func(context.Context) (T, error)) func(http.ResponseWriter, *http.Request, repos.MobileDeviceRow) {
	return func(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
		var (
			v   T
			err error
		)
		if coalesce {
			res, ferr, _ := s.flight.Do(key, func() (any, error) {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), flightLimit)
				defer cancel()
				return fetch(ctx)
			})
			err = ferr
			if err == nil {
				v = res.(T)
			}
		} else {
			v, err = fetch(r.Context())
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

type repoName struct {
	CodeRepoID string `json:"codeRepoId"`
	Name       string `json:"name"`
	Nickname   string `json:"nickname"`
}

type repoNamesResult struct {
	Repos []repoName `json:"repos"`
}

// repoNames projects the repo list to display names: the phone never sees paths or scripts.
func (s *Server) repoNames(ctx context.Context) (repoNamesResult, error) {
	res, err := s.cfg.Reader.Repos(ctx)
	if err != nil {
		return repoNamesResult{}, err
	}
	out := repoNamesResult{Repos: make([]repoName, 0, len(res.Repos))}
	for _, r := range res.Repos {
		out.Repos = append(out.Repos, repoName{CodeRepoID: r.CodeRepoID, Name: r.Name, Nickname: r.Nickname})
	}
	return out, nil
}

func (s *Server) handleLog(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
	q := r.URL.Query()
	args := adewire.ReadLogArgs{Kind: q.Get("kind"), ID: q.Get("id")}
	if raw := q.Get("afterSeq"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeErrorT(w, http.StatusBadRequest, string(ipcerr.CodeBadRequest), "backend.space.mobileweb.afterSeq.body")
			return
		}
		args.AfterSeq = n
	}
	page, err := s.cfg.Reader.ReadLog(r.Context(), args)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleArchivedTask(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
	v, err := s.cfg.Reader.ArchivedTask(r.Context(), adewire.TaskArgs{TaskID: r.URL.Query().Get("taskId")})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
