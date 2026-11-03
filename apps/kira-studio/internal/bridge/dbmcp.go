package bridge

import (
	"context"
	"github.com/kirathecat/kira-studio/internal/embedded"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/dbmcp"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// dbMcpServerName is the one name every Install call registers under — internal/dbmcp's own
// server identity (Implementation.Name, dbmcp/server.go).
const dbMcpServerName = "kira-db"

// dbMcpTokenName is mcpauth.PathNamed's own file-name argument — no repo-id slug (M1 §2.2): one DB
// MCP instance exists per app process per KIRA_HOME, with no second identity to key several apart
// by.
const dbMcpTokenName = "mcp-db"

// DbMcpService is the Database MCP section's whole surface: Status, SetEnabled,
// Regenerate, InstallAgent — it owns the embedded *dbmcp.Server's actual lifecycle,
// constructed and started when the setting turns on (or already is, at boot), stopped when it
// turns off or the app quits.
type DbMcpService struct {
	Deps      appcore.Deps
	Installer harness.MCPInstaller
	// Approvals is M2's prompt-mode broker — constructed once in main.go and outliving this
	// service's own server start/stop, so the event subscription wired at boot (Events.Attach)
	// stays valid across a restart of the embedded server.
	Approvals *dbmcp.ApprovalBroker
	// Proof backs the proof_* tools (P420).
	Proof dbmcp.ProofStore
	// Kube backs the kube_* tools (P440).
	Kube dbmcp.KubeReader

	embedded embedded.Service[*dbmcp.Server, DbMcpStatus]
}

// NewDbMcpService wires the embedded lifecycle's own start/stop/status closures once, here, so
// every other method can assume s.embedded is ready — a plain struct literal (main.go's own shape
// before T2-13) would leave them nil.
func NewDbMcpService(deps appcore.Deps, installer harness.MCPInstaller, approvals *dbmcp.ApprovalBroker, proofStore dbmcp.ProofStore, kube dbmcp.KubeReader) *DbMcpService {
	s := &DbMcpService{Deps: deps, Installer: installer, Approvals: approvals, Proof: proofStore, Kube: kube}
	s.embedded = embedded.Service[*dbmcp.Server, DbMcpStatus]{
		StartFn: func(mint bool) (*dbmcp.Server, error) {
			home := config.KiraHome()
			// The headersHelper script's own content depends only on the token file's path, never
			// its live value — ensure it once per start (F2), not on every statusFn/Install call.
			if _, err := mcpauth.EnsureHeaderHelperScript(home, mcpauth.HelperTokenPathNamed(home, dbMcpTokenName)); err != nil {
				return nil, err
			}
			srv, err := dbmcp.New(dbmcp.Config{
				Home:             home,
				Token:            dbMcpTokenProviderFor(home, mint),
				Conns:            s.Deps.Connections,
				Tree:             s.Deps.Tree,
				Query:            s.Deps.Router,
				Approvals:        s.Approvals,
				Proof:            s.Proof,
				Kube:             s.Kube,
				MaskRules:        s.Deps.MaskRules,
				ExplainThreshold: s.explainThreshold,
				PreviewResponses: s.previewResponses,
				Logger:           slog.Default(),
				Prompts:          s.Deps.Prompts,
			})
			if err != nil {
				return nil, err
			}
			go func() {
				if err := srv.Serve(); err != nil {
					slog.Warn("db mcp embedded server", "scope", "dbmcp", "err", err)
				}
			}()
			return srv, nil
		},
		// Approvals stays usable — a later re-enable within the same app run constructs a fresh
		// server against it. AbandonAll runs before Close, not after: a run_query
		// handler mid-flight can be parked in ApprovalBroker.Request waiting on a human
		// who will never answer once the server is going away. Close's own closeHTTP calls
		// http.Server.Shutdown, which waits for every in-flight handler to return — closing before
		// abandoning would have that handler, and Shutdown itself, both wait on each other with
		// nothing left to break the deadlock but closeHTTP's own backstop timeout. Abandoning first
		// lets the parked handler return immediately (ApprovalAbandoned), so Shutdown's ordinary
		// graceful drain finds nothing left in flight.
		StopFn: func(srv *dbmcp.Server) {
			s.Approvals.AbandonAll()
			_ = srv.Close()
		},
		StatusFn: func(srv *dbmcp.Server, startErr error) DbMcpStatus {
			inst := s.Installer.Status()
			st := DbMcpStatus{AgentAvailable: inst.Path != "", Probed: inst.Probed}
			if srv == nil {
				if startErr != nil {
					st.Error = startErr.Error()
				}
				return st
			}
			st.Running = true
			// Command is "" whenever the on-disk helper token mirror is missing or does not
			// verify against the currently-held record — never gated on whether *this run* minted
			// the token, which stayed false across every ordinary restart within the token's own
			// TTL even though the helper file (and the record it matches) both survive restarts
			// and stay valid.
			if helperTokenValid(srv) {
				helperPath := mcpauth.HeaderHelperScriptPath(config.KiraHome())
				st.Command = s.Installer.Command(dbMcpServerName, srv.URL(), helperPath)
			}
			if exp := srv.TokenExpiry(); !exp.IsZero() {
				st.ExpiresAt = exp.Format(time.RFC3339)
			}
			return st
		},
	}
	return s
}

// DbMcpStatus is the wire projection every method below returns.
type DbMcpStatus struct {
	Running        bool     `json:"running"`
	Command        string   `json:"command"`
	AgentAvailable bool     `json:"agentAvailable"`
	Probed         []string `json:"probed"`
	// ExpiresAt is the current token's own expiry, RFC 3339, "" when nothing is running or the
	// record has not yet been stamped.
	ExpiresAt string `json:"expiresAt"`
	// Error names why Running is false despite the setting being on — a bind failure. "" whenever
	// Running is true, or the setting is simply off.
	Error string `json:"error"`
}

// Status reads the embedded instance's current state — never starts or stops anything.
func (s *DbMcpService) Status() DbMcpStatus {
	return s.embedded.Status()
}

// dbMcpTokenProviderFor implements dbmcp.TokenProvider's own no-repoID signature with a mint/!mint
// split: mint always mints fresh and persists over any existing file (an explicit enable); !mint
// loads an existing token or mints only if none exists yet, or the existing one has lapsed (the
// app-boot-with-the-leaf-already-true path).
func dbMcpTokenProviderFor(home string, mint bool) dbmcp.TokenProvider {
	return func() (mcpauth.Record, string, bool, error) {
		path := mcpauth.PathNamed(home, dbMcpTokenName)
		helperPath := mcpauth.HelperTokenPathNamed(home, dbMcpTokenName)
		if mint {
			return remintDbMcpToken(path, helperPath)
		}
		plain, rec, minted, err := mcpauth.LoadOrMintTTL(path, mcpauth.TTL)
		if err != nil {
			return rec, plain, minted, err
		}
		if minted {
			// A fresh plaintext (minted, or the one-shot zero-ExpiresAt stamp — neither of which
			// LoadOrMintTTL distinguishes in its own return shape) only exists here when minted is
			// true (LoadOrMintTTL's own doc: plain is "" whenever it did not mint).
			if serr := mcpauth.SaveHelperToken(helperPath, plain); serr != nil {
				return rec, plain, minted, serr
			}
			return rec, plain, minted, nil
		}
		// A loaded (not minted) record must still have a helper mirror whose plaintext
		// verifies against it. A missing file (never written on some older run), or a mismatch
		// (Save succeeded then SaveHelperToken failed on a previous run, or a stale copy left
		// over some other way), would otherwise leave every client silently 401ing forever — this
		// app never re-derives a plaintext from a hash. Remint fresh, both files together, rather
		// than serve a record no client can match.
		if helperTokenMatchesRecord(helperPath, rec) {
			return rec, plain, minted, nil
		}
		return remintDbMcpToken(path, helperPath)
	}
}

// remintDbMcpToken mints a fresh token and persists it to both files together — the record
// (hash+salt) and the helper mirror (plaintext) must never go out of sync, since nothing here can
// recover one from the other.
func remintDbMcpToken(path, helperPath string) (mcpauth.Record, string, bool, error) {
	plain, rec, err := mcpauth.MintTTL(mcpauth.TTL)
	if err != nil {
		return mcpauth.Record{}, "", false, err
	}
	if err := mcpauth.Save(path, rec); err != nil {
		return mcpauth.Record{}, "", false, err
	}
	if err := mcpauth.SaveHelperToken(helperPath, plain); err != nil {
		return mcpauth.Record{}, "", false, err
	}
	return rec, plain, true, nil
}

// helperTokenMatchesRecord reports whether helperPath's own plaintext still verifies against rec —
// the shared check dbMcpTokenProviderFor's load branch and helperTokenValid below both need, over
// a Record read from two different places (a freshly loaded one here, a live server's
// TokenRecord() there).
func helperTokenMatchesRecord(helperPath string, rec mcpauth.Record) bool {
	plain, ok, err := mcpauth.LoadHelperToken(helperPath)
	if err != nil || !ok {
		return false
	}
	return mcpauth.Verify(plain, rec)
}

// helperTokenValid is the running server's own F8 gate: Command/Install must show only when the
// on-disk helper token mirror still verifies against the record the server is actually checking
// bearer tokens against right now — not whether this particular run minted a fresh token, which
// stayed false, wrongly, across every ordinary restart within the token's own 7-day TTL even
// though the helper file (and the record it matches) both survive a restart and stay valid.
func helperTokenValid(srv *dbmcp.Server) bool {
	helperPath := mcpauth.HelperTokenPathNamed(config.KiraHome(), dbMcpTokenName)
	return helperTokenMatchesRecord(helperPath, srv.TokenRecord())
}

// explainThreshold is dbmcp.Config.ExplainThreshold's real backend: settingsState.advanced.
// expensiveQueryRows's own Go leaf, read fresh on every call — never cached, since a stale
// threshold would silently mis-flag every query after the user changes it. A settings
// read failure must not break a query; it falls back to the documented default and logs once.
func (s *DbMcpService) explainThreshold() int {
	settings, err := s.Deps.Repos.Settings.GetAll()
	if err != nil {
		slog.Warn("db mcp: read expensive-query threshold, using default", "scope", "dbmcp", "err", err)
		return model.DefaultSettings().Advanced.ExpensiveQueryRows
	}
	return settings.Advanced.ExpensiveQueryRows
}

// previewResponses is dbmcp.Config.PreviewResponses' real backend, read fresh on every call. A
// read failure fails closed (true): sending unreviewed data is the unsafe direction.
func (s *DbMcpService) previewResponses() bool {
	settings, err := s.Deps.Repos.Settings.GetAll()
	if err != nil {
		slog.Warn("db mcp: read preview setting, holding results for review", "scope", "dbmcp", "err", err)
		return true
	}
	return settings.DbMcp.PreviewResponses
}

// startIfEnabled is main.go's own boot-time call: a failure (a bind conflict) is logged, never
// fatal — the app boots regardless.
//
// Unexported, reached only through StartDbMcpIfEnabled below: Wails binds every exported method of
// a registered service, and a wire-callable Start would let a stray call bypass the settings leaf.
func (s *DbMcpService) startIfEnabled() {
	s.embedded.StartIfEnabled("dbmcp", func() (bool, error) {
		settings, err := s.Deps.Repos.Settings.GetAll()
		if err != nil {
			return false, err
		}
		return settings.DbMcp.ServerEnabled, nil
	})
}

// stop is main.go's own shutdown call — see startIfEnabled's own note on why this is unexported
// and reached only through StopDbMcp.
func (s *DbMcpService) stop() {
	s.embedded.Stop()
}

// StartDbMcpIfEnabled and StopDbMcp are main.go's own boot/shutdown hooks for the embedded
// instance, package-level functions rather than exported methods on DbMcpService — the identical
// reasoning startIfEnabled's own doc comment above documents.
func StartDbMcpIfEnabled(s *DbMcpService) { s.startIfEnabled() }
func StopDbMcp(s *DbMcpService)           { s.stop() }

// DbMcpSetEnabledArgs is SetEnabled's own argument shape.
type DbMcpSetEnabledArgs struct {
	Enabled bool `json:"enabled"`
}

// SetEnabled patches the settings leaf and starts or stops the embedded instance in the same call
// (the toggle bypasses the dialog's draft/Save flow entirely, an instant action). Turning on always
// mints a fresh token, even if a file already exists for a previous enable — a deliberate, explicit
// re-enable is exactly the event token rotation ties regeneration to.
func (s *DbMcpService) SetEnabled(args DbMcpSetEnabledArgs) (DbMcpStatus, error) {
	merged, err := s.Deps.Repos.Settings.Set(model.SettingsPatch{
		DbMcp: &model.DbMcpPatch{ServerEnabled: &args.Enabled},
	})
	if err != nil {
		return DbMcpStatus{}, ipcerr.InternalErr(err)
	}
	s.Deps.Events.Emit(ChannelSettingsChanged, merged)

	st, err := s.embedded.SetRunning(args.Enabled)
	if err != nil {
		slog.Warn("db mcp: start on enable", "scope", "dbmcp", "err", err)
	}
	return st, nil
}

// Regenerate mints a fresh token for the already-running embedded instance without touching the
// setting or its lifecycle (an app restart loaded the existing hash+salt but has no plaintext to
// show). A no-op, returning the current status unchanged, when nothing is running.
func (s *DbMcpService) Regenerate() DbMcpStatus {
	s.embedded.Mu.Lock()
	defer s.embedded.Mu.Unlock()
	if s.embedded.Server == nil {
		return s.embedded.StatusLocked()
	}
	path := mcpauth.PathNamed(config.KiraHome(), dbMcpTokenName)
	plain, rec, err := mcpauth.MintTTL(mcpauth.TTL)
	if err != nil {
		slog.Warn("db mcp: regenerate token", "scope", "dbmcp", "err", err)
		return s.embedded.StatusLocked()
	}
	if err := mcpauth.Save(path, rec); err != nil {
		slog.Warn("db mcp: persist regenerated token", "scope", "dbmcp", "err", err)
		return s.embedded.StatusLocked()
	}
	// The helper script's own live source (F2) — rotation is exactly rewriting this file, no
	// re-registration, so a stale mirror here would defeat the whole point of Regenerate.
	if err := mcpauth.SaveHelperToken(mcpauth.HelperTokenPathNamed(config.KiraHome(), dbMcpTokenName), plain); err != nil {
		slog.Warn("db mcp: persist regenerated helper token", "scope", "dbmcp", "err", err)
		return s.embedded.StatusLocked()
	}
	s.embedded.Server.SetToken(rec)
	return s.embedded.StatusLocked()
}

// DbMcpInstallResult is harness.InstallResult's wire projection — a domain package's plain Go struct
// never crosses the wire directly, only this tagged copy of it.
type DbMcpInstallResult struct {
	Outcome string   `json:"outcome"`
	Detail  string   `json:"detail"`
	Probed  []string `json:"probed"`
}

func toWireDbMcpInstallResult(r harness.InstallResult) DbMcpInstallResult {
	return DbMcpInstallResult{Outcome: r.Outcome, Detail: r.Detail, Probed: r.Probed}
}

// InstallAgent never returns a Go error — MCPInstaller.Install's own contract, following
// connections.Service.Reveal's own precedent. A no-op result (outcome notFound) when nothing is
// running: there is nothing to register yet.
func (s *DbMcpService) InstallAgent(ctx context.Context) DbMcpInstallResult {
	// The lock covers only the state check: the spawn below can take up to 30 s and names a fixed
	// URL and helper path, so holding mu would block Status, the toggle and app quit behind it.
	s.embedded.Mu.Lock()
	srv := s.embedded.Server
	// F8: same gate statusFn uses — the on-disk helper mirror must exist and verify against the
	// live record, not just "this run minted a fresh token".
	if srv == nil || !helperTokenValid(srv) {
		s.embedded.Mu.Unlock()
		return DbMcpInstallResult{Outcome: harness.OutcomeNotFound}
	}
	url := srv.URL()
	s.embedded.Mu.Unlock()
	helperPath := mcpauth.HeaderHelperScriptPath(config.KiraHome())
	return toWireDbMcpInstallResult(s.Installer.Install(ctx, dbMcpServerName, url, helperPath))
}

// dbMcpApprovalPlanIssuesCap bounds DbMcpApprovalPlan.Issues on the wire — the dbmcp package's own
// approvalPlanFrom already caps at this same figure, so this is a defense-in-depth
// re-application at the wire boundary. Unlike the statement text below it, eliding some of these
// issues never hides part of the statement itself from the human approving it.
const dbMcpApprovalPlanIssuesCap = 10

// DbMcpApprovalPlanIssue is dbmcp.ApprovalPlanIssue's wire projection.
type DbMcpApprovalPlanIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// DbMcpApprovalPlan is dbmcp.ApprovalPlan's wire projection — M3's own plan evidence the approval
// dialog renders, present only on a request that carries one.
type DbMcpApprovalPlan struct {
	EstimatedRowsRead *float64                 `json:"estimatedRowsRead"`
	ThresholdRows     int                      `json:"thresholdRows"`
	OverThreshold     bool                     `json:"overThreshold"`
	Issues            []DbMcpApprovalPlanIssue `json:"issues"`
	IssuesOmitted     int                      `json:"issuesOmitted"`
}

func toWireApprovalPlan(p *dbmcp.ApprovalPlan) *DbMcpApprovalPlan {
	if p == nil {
		return nil
	}
	issues := p.Issues
	omitted := p.IssuesOmitted
	if len(issues) > dbMcpApprovalPlanIssuesCap {
		omitted += len(issues) - dbMcpApprovalPlanIssuesCap
		issues = issues[:dbMcpApprovalPlanIssuesCap]
	}
	wireIssues := make([]DbMcpApprovalPlanIssue, len(issues))
	for i, iss := range issues {
		wireIssues[i] = DbMcpApprovalPlanIssue(iss)
	}
	return &DbMcpApprovalPlan{
		EstimatedRowsRead: p.EstimatedRowsRead, ThresholdRows: p.ThresholdRows,
		OverThreshold: p.OverThreshold, Issues: wireIssues, IssuesOmitted: omitted,
	}
}

// DbMcpApprovalTarget is dbmcp.ApprovalTarget's wire projection; Path is never nil.
type DbMcpApprovalTarget struct {
	Color    string              `json:"color"`
	ReadOnly bool                `json:"readOnly"`
	Host     string              `json:"host"`
	Database string              `json:"database"`
	Path     []model.PathSegment `json:"path"`
}

func toWireApprovalTarget(t dbmcp.ApprovalTarget) DbMcpApprovalTarget {
	path := t.Path
	if path == nil {
		path = []model.PathSegment{}
	}
	return DbMcpApprovalTarget{Color: t.Color, ReadOnly: t.ReadOnly, Host: t.Host, Database: t.Database, Path: path}
}

// DbMcpApprovalCluster is dbmcp.ApprovalCluster's wire projection.
type DbMcpApprovalCluster struct {
	Context   string `json:"context"`
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Operation string `json:"operation"`
}

func toWireApprovalCluster(c *dbmcp.ApprovalCluster) *DbMcpApprovalCluster {
	if c == nil {
		return nil
	}
	return &DbMcpApprovalCluster{Context: c.Context, Server: c.Server, Namespace: c.Namespace, Operation: c.Operation}
}

// DbMcpApprovalResponse is the held result's metadata.
type DbMcpApprovalResponse struct {
	IsError bool `json:"isError"`
	Bytes   int  `json:"bytes"`
}

// DbMcpApprovalRequest is dbmcp.ApprovalRequest's wire projection — an absolute deadline (epoch
// ms) instead of a time.Time, so a client-side countdown never drifts from the server's own clock.
type DbMcpApprovalRequest struct {
	RequestID      string `json:"requestId"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Kind           string `json:"kind"`
	Class          string `json:"class"`
	Statement      string `json:"statement"`
	ExpiresAtMs    int64  `json:"expiresAtMs"`
	// Reason is M3's own "permission" | "heavy" — every M2-era request is "permission" (M3 §6.2).
	Reason string              `json:"reason"`
	Plan   *DbMcpApprovalPlan  `json:"plan"`
	Target DbMcpApprovalTarget `json:"target"`
	// Stage is "query" (may this statement run) or "response" (may its result go to the client).
	Stage string `json:"stage"`
	// Response is set only for stage "response". It carries metadata only: the result text can be
	// MB-sized and snapshots fan out to every window, so PendingResponse serves the text.
	Response *DbMcpApprovalResponse `json:"response"`
	// Cluster is set for a Kubernetes read (Target is then empty), null for a database request.
	Cluster *DbMcpApprovalCluster `json:"cluster"`
}

// DbMcpApprovalSnapshot is dbmcp.ApprovalSnapshot's wire projection.
type DbMcpApprovalSnapshot struct {
	Pending *DbMcpApprovalRequest `json:"pending"`
	Queued  int                   `json:"queued"`
}

func toWireApprovalResponse(r *dbmcp.ApprovalResponse) *DbMcpApprovalResponse {
	if r == nil {
		return nil
	}
	return &DbMcpApprovalResponse{IsError: r.IsError, Bytes: r.Bytes}
}

func toWireApprovalSnapshot(snap dbmcp.ApprovalSnapshot) DbMcpApprovalSnapshot {
	out := DbMcpApprovalSnapshot{Queued: snap.Queued}
	if snap.Pending != nil {
		out.Pending = &DbMcpApprovalRequest{
			RequestID: snap.Pending.RequestID, ConnectionID: snap.Pending.ConnectionID,
			ConnectionName: snap.Pending.ConnectionName, Kind: snap.Pending.Kind,
			// Statement carries the full text, uncapped (a byte cap that elided the middle let a
			// hidden clause sit entirely inside the omitted span; the dialog's read-only editor
			// (ApprovalStatement.vue) scrolls and wraps, so nothing here bounds the DOM by hiding
			// text a human is being asked to approve).
			Class: string(snap.Pending.Class), Statement: snap.Pending.Statement,
			ExpiresAtMs: snap.Pending.ExpiresAt.UnixMilli(),
			Reason:      string(snap.Pending.Reason),
			Plan:        toWireApprovalPlan(snap.Pending.Plan),
			Target:      toWireApprovalTarget(snap.Pending.Target),
			Stage:       string(snap.Pending.Stage),
			Response:    toWireApprovalResponse(snap.Pending.Response),
			Cluster:     toWireApprovalCluster(snap.Pending.Cluster),
		}
	}
	return out
}

// PendingApprovals is the snapshot a newly opened window fetches on mount, the same boot-time
// hydration this app's other push-backed state applies to the approval queue.
func (s *DbMcpService) PendingApprovals() DbMcpApprovalSnapshot {
	return toWireApprovalSnapshot(s.Approvals.Pending())
}

// DbMcpApprovalArgs is shared by ApproveQuery/DenyQuery — nothing but the request id.
type DbMcpApprovalArgs struct {
	RequestID string `json:"requestId"`
}

// ApproveQuery and DenyQuery never return a Go error — a decision is a value — and return the
// current snapshot rather than an action-result enum, so the clicking window updates immediately
// instead of waiting for its own broadcast to arrive.
func (s *DbMcpService) ApproveQuery(args DbMcpApprovalArgs) (DbMcpApprovalSnapshot, error) {
	if args.RequestID == "" {
		return DbMcpApprovalSnapshot{}, ipcerr.Required("requestId")
	}
	s.Approvals.Approve(args.RequestID)
	return toWireApprovalSnapshot(s.Approvals.Pending()), nil
}

func (s *DbMcpService) DenyQuery(args DbMcpApprovalArgs) (DbMcpApprovalSnapshot, error) {
	if args.RequestID == "" {
		return DbMcpApprovalSnapshot{}, ipcerr.Required("requestId")
	}
	s.Approvals.Deny(args.RequestID)
	return toWireApprovalSnapshot(s.Approvals.Pending()), nil
}

// DbMcpPendingResponse is the held run_query result a response-stage request asks about.
type DbMcpPendingResponse struct {
	RequestID string `json:"requestId"`
	Text      string `json:"text"`
	IsError   bool   `json:"isError"`
}

// PendingResponse returns the exact text the AI client receives if the user sends the result of
// requestID. NotFound once the request resolved, or when it is not a response-stage request.
func (s *DbMcpService) PendingResponse(args DbMcpApprovalArgs) (DbMcpPendingResponse, error) {
	if args.RequestID == "" {
		return DbMcpPendingResponse{}, ipcerr.Required("requestId")
	}
	r, ok := s.Approvals.ResponseFor(args.RequestID)
	if !ok {
		return DbMcpPendingResponse{}, ipcerr.NotFoundNamed("result under review")
	}
	return DbMcpPendingResponse{RequestID: args.RequestID, Text: r.Text, IsError: r.IsError}, nil
}
