// Package dbmcp is the local DB MCP server (docs/HISTORY.md (v1.7 M1)): a protocol
// front end that lists this app's own connections, browses their metadata and runs a query —
// through the existing adapter layer (internal/adapterhost, internal/tree, internal/connections),
// never a parallel path. Transport is go-sdk/mcp over a loopback-only Streamable HTTP listener
// (DefaultPort 8766), with mcpauth minting and verifying its own bearer token and its own token
// file, independent of the rest of the app beyond the Space home itself.
//
// There is no headless binary: the DB server needs the app's own live adapters, keychain-backed
// secrets and op-log/throttle/cancel machinery (§3.1), none of which a second process could reach
// without duplicating them. It runs when Kira Space runs, embedded only (internal/bridge/dbmcp.go).
//
// This package imports nothing from internal/bridge (internal/layering_test.go enforces it).
package dbmcp

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/config"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mask"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/mcpcall"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultPort is the one port this instance ever binds — there is at most one instance per
// Space home in practice, and bindHTTP refuses to start rather than silently fall back to an
// ephemeral port when it is taken: every existing registration names this port, and a
// fallback would leave it pointing at whatever else is now listening there.
const DefaultPort = 8766

// EphemeralPort as Config.Port asks the OS for a free loopback port. Test seam: packages whose
// tests run in parallel under `go test` cannot share DefaultPort.
const EphemeralPort = -1

// TokenProvider resolves this instance's own 7-day-rotating token — no repo-id slug to key on: one
// DB MCP instance exists per app process per Space home, with no second identity to key several
// apart by (§2.2).
type TokenProvider func() (rec mcpauth.Record, plain string, minted bool, err error)

// ConnectionsReader is dbmcp's own consumer-declared interface (A11) over *connections.Service —
// only the three methods list_connections/access.go/run_query actually need.
type ConnectionsReader interface {
	List() ([]model.ConnectionSummary, error)
	StateOf(connectionID string) model.ConnectionState
	ConnectContext(ctx context.Context, connectionID string) (model.ConnectionState, error)
}

// MetadataReader is dbmcp's own consumer-declared interface over *tree.Service — list_children/
// describe's own backend.
type MetadataReader interface {
	Children(ctx context.Context, connectionID, path string, refresh bool) (tree.ChildrenResult, error)
	Describe(ctx context.Context, connectionID, path string, refresh bool, tabID *string) (tree.DescribeResult, error)
	SchemaColumns(ctx context.Context, connectionID, path string, refresh bool) (tree.SchemaColumnsResult, error)
}

// QueryRunner is dbmcp's own consumer-declared interface over *adapterhost.Router's Execute and
// ClassifyStatement seams — run_query's only path into the adapter layer, the same one the
// console's own run() uses.
type QueryRunner interface {
	Execute(ctx context.Context, req adapterhost.ExecuteRequestWire) (adapterhost.ExecuteResponse, error)
	// ExecuteRead runs a read-mode request on the connection's read lane: a second adapter on the
	// engine's read-only session. It never falls back to the main adapter.
	ExecuteRead(ctx context.Context, req adapterhost.ExecuteRequestWire) (adapterhost.ExecuteResponse, error)
	// Browse reads one structured page on the read lane (browse_table); it never writes.
	Browse(ctx context.Context, req adapterhost.BrowseRequest) (page.Page, error)
	// ClassifyStatement answers what one statement would do (M2's permission gate) — an adapter
	// error (or one that does not implement adapters.StatementClassifier) yields ClassUnknown, never
	// a fatal error the caller must special-case.
	ClassifyStatement(ctx context.Context, connectionID, statement string) (adapters.OpClass, error)
}

// laneCloser is the optional QueryRunner half that owns read lanes (*adapterhost.Router).
type laneCloser interface {
	CloseReadLane(ctx context.Context, connectionID string)
	CloseReadLanes(ctx context.Context)
}

// listWatcher is the optional ConnectionsReader half that reports list changes (*connections.Service).
type listWatcher interface {
	OnListChanged(fn func([]model.ConnectionSummary)) (unsubscribe func())
}

// MaskRules is dbmcp's own consumer-declared interface over *maskrules.Service. A nil
// Config.MaskRules is a construction error, not a default (New rejects it, the same way it rejects
// a nil Approvals): a silently-nil masker would mean every query returns unmasked data with
// nothing surfacing, exactly the failure mode ExplainThreshold's own comment above guards against
// for a different field.
type MaskRules interface {
	// MaskSetFor is run_query's render path's own seam (§4.2/§4.3) — the folded, per-column-name
	// rule map plus the connection's Masker.
	MaskSetFor(connectionID string) (mask.Set, error)
	// List is list_connections' own seam (§4.5) — the raw per-rule rows (table_name included,
	// unlike MaskSetFor's already-folded map) needed to report "table.column: kind" per rule.
	List(connectionID string) ([]model.MaskRule, error)
}

// ColumnReview is dbmcp's consumer-declared interface over *maskreview.Service (P534). A nil
// Config.ColumnReview is a construction error for the reason MaskRules' is: silently nil would return
// columns nobody reviewed.
type ColumnReview interface {
	// For is the gate's read: the withheld columns of one connection, checked inline when the last check is stale.
	For(ctx context.Context, summary model.ConnectionSummary) maskreview.Withheld
	// Recheck is For with a live column read of the statement's relations, for result columns the last check
	// cannot vouch for.
	Recheck(ctx context.Context, summary model.ConnectionSummary, req maskreview.Rescan) maskreview.Withheld
	// Cached is the last state without any op, for list_connections.
	Cached(connectionID string) (maskreview.Withheld, bool)
	// MarkStale makes the next For check again, after an MCP write or DDL statement.
	MarkStale(connectionID string)
}

// Config is everything New needs. Zero-value Home takes the documented default; every other field
// is required.
type Config struct {
	// Port overrides DefaultPort (test seam). Zero means DefaultPort; EphemeralPort an OS-chosen one.
	Port int
	// Home overrides the Space home (a debugging seam) — empty means config.KiraSpaceHome().
	Home string
	// Token resolves this instance's own auth record. Exactly one of Token and Sessions is set.
	Token TokenProvider
	// Sessions makes this the session server: it accepts only these per-launch tokens, never the
	// external one, and binds an ephemeral port only (SessionPool sets both).
	Sessions *SessionTokens
	// Conns/Tree/Query are the three backend seams (§3.3) — *connections.Service, *tree.Service and
	// *adapterhost.Router satisfy these structurally once Router.Execute exists.
	Conns ConnectionsReader
	Tree  MetadataReader
	Query QueryRunner
	// Approvals is the prompt-mode broker (M2 §5) — required, constructed once in main.go and
	// outliving Server start/stop so the event subscription wired at boot stays valid across a
	// server restart.
	Approvals *ApprovalBroker
	// MaskRules resolves a connection's own masking rules and correlation key (M5 §4.6) — required.
	MaskRules MaskRules
	// ColumnReview withholds columns that are new since the user reviewed masking (P534) — required.
	ColumnReview ColumnReview
	// ExplainThreshold is the expensive-query row threshold (advanced.expensiveQueryRows) — read
	// fresh on every call, never cached: a stale threshold silently mis-flags every query after the
	// user changes it. A plain func rather than an interface, so this package keeps its existing
	// backends and adds no new service dependency. Required — a silently-defaulted threshold would
	// make every heavy-query verdict wrong in a way nothing surfaces.
	ExplainThreshold func() int
	// PreviewResponses reports whether run_query results are held for the user's review before the
	// client gets them (dbMcp.previewResponses) — read fresh on every call. It is the default for connections whose review setting is inherit. Required for the same
	// reason as ExplainThreshold: a silently-defaulted switch would send unreviewed data.
	PreviewResponses func() bool
	// Proof backs the proof_* tools (P420); nil leaves them unregistered.
	Proof ProofStore
	// Kube backs the kube_* tools; its calls record their own op-log rows. Nil leaves them unregistered.
	Kube KubeReader
	// Ops backs operations; nil leaves both unregistered. Runs adds
	// script runs to them; nil means none are listed or stoppable.
	Ops  OpsControl
	Runs ScriptRunControl
	// Scripts backs the script tools; nil leaves them unregistered.
	Scripts ScriptControl
	// API backs the api_* tools; nil leaves them unregistered. APISettings is read fresh per call and
	// decides whether the tools are listed and whether a request waits for the user.
	API         APIRequester
	APISettings func() APISettings
	// Limits are the limits the server enforces on run_query, beside its built-ins. Nil means none.
	Limits []Limit
	// Prompts resolves the editable instruction and tool texts; nil means the registry defaults.
	Prompts llmprompts.Source
	// Audit records the tool calls of agent sessions; set on the session server only.
	Audit Auditor
	// Logger receives every operational log line. A nil Logger falls back to slog.Default().
	Logger *slog.Logger
}

// Server is the DB MCP server's one embedded instance — no index to build, so it needs no
// readiness gate, sync lock or watcher (§3.3): New binds and returns, Close shuts the listener
// down.
type Server struct {
	cfg Config
	log *slog.Logger

	// tokenMu guards token: Regenerate (bridge.DbMcpService's own restart-recovery action) mutates
	// it on a live, already-serving instance, concurrently with tokenVerifier reading it on every
	// in-flight request (http.go). The plaintext is never held here beyond New's own call —
	// bridge.dbMcpTokenProviderFor persists it to the helper token mirror file itself before
	// returning it, and the "can Command/Install show" gate reads that file back and verifies it
	// against TokenRecord() below, rather than an in-memory "this run minted a fresh token" flag
	// that stayed false, wrongly, across every restart within the token's own TTL.
	tokenMu sync.RWMutex
	token   mcpauth.Record

	// limits are the enforced run_query limits: built-ins, then cfg.Limits.
	limits []Limit
	// exemptions holds the ids limit refusals mint; it lives here so a prompt-edit rebuild keeps them.
	exemptions *exemptionStore

	// mcp is the tool set for revision mcpRev of the prompts; a prompt edit rebuilds it (server()).
	mcpMu  sync.Mutex
	mcp    *mcp.Server
	mcpRev uint64
	mcpAPI bool

	httpState // http.go's own fields (listener, *http.Server)

	// unwatchList stops the exposure watcher that closes read lanes of un-exposed connections.
	unwatchList func()

	closeOnce sync.Once
}

// serverVersion mirrors the app's own declared version — no ldflags plumbing invented for this
// phase alone.
const serverVersion = "0.0.0"

// New resolves cfg, mints or loads this instance's own token, builds the mcp.Server, and
// binds the HTTP listener (§3.2, DefaultPort only — see bindHTTP/F11) — but does not yet accept
// connections; call Serve for that.
func New(cfg Config) (*Server, error) {
	if cfg.Home == "" {
		cfg.Home = config.KiraSpaceHome()
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	if (cfg.Token == nil) == (cfg.Sessions == nil) {
		return nil, fmt.Errorf("dbmcp: exactly one of Config.Token and Config.Sessions is required")
	}
	if cfg.Sessions != nil && cfg.Port != EphemeralPort {
		return nil, fmt.Errorf("dbmcp: a session server binds an ephemeral port only")
	}
	if cfg.Conns == nil || cfg.Tree == nil || cfg.Query == nil {
		return nil, fmt.Errorf("dbmcp: Config.Conns, Config.Tree and Config.Query are required")
	}
	if cfg.Approvals == nil {
		return nil, fmt.Errorf("dbmcp: Config.Approvals is required")
	}
	if cfg.MaskRules == nil {
		return nil, fmt.Errorf("dbmcp: Config.MaskRules is required")
	}
	if cfg.ColumnReview == nil {
		return nil, fmt.Errorf("dbmcp: Config.ColumnReview is required")
	}
	if cfg.ExplainThreshold == nil {
		return nil, fmt.Errorf("dbmcp: Config.ExplainThreshold is required")
	}
	if cfg.PreviewResponses == nil {
		return nil, fmt.Errorf("dbmcp: Config.PreviewResponses is required")
	}

	// plain/minted served the old in-memory-plaintext gate (F8) — cfg.Token() (bridge's
	// dbMcpTokenProviderFor) already persists a fresh plaintext to the helper token mirror file
	// itself before returning here, so neither is needed beyond this call.
	var tokenRec mcpauth.Record
	if cfg.Token != nil {
		var err error
		if tokenRec, _, _, err = cfg.Token(); err != nil {
			return nil, fmt.Errorf("dbmcp: resolve token: %w", err)
		}
	}

	limits, err := validateLimits(cfg.Limits)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:        cfg,
		log:        log,
		token:      tokenRec,
		limits:     limits,
		exemptions: newExemptionStore(cfg.Approvals.now),
	}
	s.mcp, s.mcpRev, s.mcpAPI = s.buildMCPServer(), s.prompts().Revision(), s.apiEnabled()

	if err := s.bindHTTP(); err != nil {
		return nil, err
	}
	s.watchExposure()
	return s, nil
}

// builtinLimits are the limits every server enforces, ahead of Config.Limits.
func builtinLimits() []Limit { return []Limit{statementTimeoutLimit{}, expensiveQueryLimit{}} }

// validateLimits rejects a limit with no name, a duplicate name inside in, or neither PreLimit nor
// ExecLimit. It returns the built-ins followed by in; an entry named like a built-in replaces it
// (the test seam injects fakes this way).
func validateLimits(in []Limit) ([]Limit, error) {
	seen := map[string]bool{}
	for _, l := range in {
		name := l.Name()
		_, pre := l.(PreLimit)
		_, exec := l.(ExecLimit)
		switch {
		case name == "":
			return nil, fmt.Errorf("dbmcp: a limit has an empty name")
		case seen[name]:
			return nil, fmt.Errorf("dbmcp: duplicate limit %q", name)
		case !pre && !exec:
			return nil, fmt.Errorf("dbmcp: limit %q implements neither PreLimit nor ExecLimit", name)
		}
		seen[name] = true
	}
	out := make([]Limit, 0, len(in)+2)
	for _, b := range builtinLimits() {
		if !seen[b.Name()] {
			out = append(out, b)
		}
	}
	return append(out, in...), nil
}

// watchExposure closes the read lane of a connection the user stops exposing to MCP.
func (s *Server) watchExposure() {
	lanes, ok := s.cfg.Query.(laneCloser)
	watcher, watchable := s.cfg.Conns.(listWatcher)
	if !ok || !watchable {
		return
	}
	s.unwatchList = watcher.OnListChanged(func(list []model.ConnectionSummary) {
		for _, c := range list {
			if !c.McpEnabled {
				lanes.CloseReadLane(context.Background(), c.ID)
			}
		}
	})
}

// prompts is the source of the server's instruction and tool texts.
func (s *Server) prompts() llmprompts.Source {
	if s.cfg.Prompts == nil {
		return llmprompts.Defaults()
	}
	return s.cfg.Prompts
}

// server returns the tool set, rebuilt when a prompt edit moved the revision or the API tools were toggled: the HTTP transport is
// stateless, so the next request already sees the new texts.
func (s *Server) server() *mcp.Server {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()
	if rev, api := s.prompts().Revision(), s.apiEnabled(); rev != s.mcpRev || api != s.mcpAPI {
		s.mcp, s.mcpRev, s.mcpAPI = s.buildMCPServer(), rev, api
	}
	return s.mcp
}

// buildMCPServer constructs the mcp.Server (§4) — pure registration, no I/O of its own;
// every handler closes over s and calls into s.cfg.Conns/Tree/Query through access.go's gates.
func (s *Server) buildMCPServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "kira-studio",
		Title:   "Kira Studio",
		Version: serverVersion,
	}, &mcp.ServerOptions{Instructions: llmprompts.DbmcpInstructions.Text(s.prompts(), llmprompts.NoVars{})})

	src := s.prompts()
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_connections",
		Description: llmprompts.DbmcpToolListConnections.Text(src, llmprompts.NoVars{}),
	}, withPanicRecovery(s, "list_connections", s.listConnections))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_children",
		Description: llmprompts.DbmcpToolListChildren.Text(src, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[listChildrenArgs](src, llmprompts.DbmcpToolListChildren.ID(), nil), "connectionId"),
	}, withPanicRecovery(s, "list_children", s.listChildren))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "describe",
		Description: llmprompts.DbmcpToolDescribe.Text(src, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[describeArgs](src, llmprompts.DbmcpToolDescribe.ID(), nil), "connectionId", "path"),
	}, withPanicRecovery(s, "describe", s.describe))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "browse_table",
		Description: llmprompts.DbmcpToolBrowseTable.Text(src, llmprompts.NoVars{}),
		InputSchema: browseTableSchema(src),
	}, withPanicRecovery(s, "browse_table", s.browseTable))
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_query",
		Description: llmprompts.DbmcpToolRunQuery.Text(src, llmprompts.NoVars{}),
		InputSchema: runQuerySchema(src),
	}, withPanicRecovery(s, "run_query", s.runQuery))
	if s.cfg.Proof != nil {
		s.addProofTools(srv, src)
	}
	if s.cfg.Kube != nil {
		s.addKubeTools(srv, src)
	}
	if s.cfg.Ops != nil {
		s.addOpsTools(srv, src)
	}
	if s.cfg.Scripts != nil {
		s.addScriptTools(srv, src)
	}
	if s.apiEnabled() {
		s.addAPITools(srv, src)
	}

	if s.cfg.Audit != nil {
		mcpcall.Record(srv, s.auditHook)
	}
	return srv
}

// runQuerySchema is run_query's input schema: mode is required and limited to the three declared modes.
func runQuerySchema(src llmprompts.Source) *jsonschema.Schema {
	sch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[runQueryArgs](src, llmprompts.DbmcpToolRunQuery.ID(),
		map[string]any{"maxRows": llmprompts.LimitVars{Default: runQueryDefaultMaxRows, Max: runQueryMaxMaxRows}}), "connectionId", "sql", "mode")
	sch.Properties["mode"].Enum = []any{modeRead, modeWrite, modeDDL}
	ex := sch.Properties["exemption"]
	ex.Properties["id"].MinLength = jsonschema.Ptr(1)
	ex.Properties["reason"].MinLength = jsonschema.Ptr(1)
	ex.Properties["reason"].MaxLength = jsonschema.Ptr(maxExemptionReasonRunes)
	return sch
}

// browseScalarSchema is the schema of one filter value: a string, number or boolean.
func browseScalarSchema(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Types: []string{"string", "number", "boolean"}, Description: desc}
}

// browseTableSchema is browse_table's input schema: the ops and directions are enums, the counts are
// bounded, and a filter value is a scalar.
func browseTableSchema(src llmprompts.Source) *jsonschema.Schema {
	sch := llmprompts.RequireNonEmpty(llmprompts.InputSchema[browseTableArgs](src, llmprompts.DbmcpToolBrowseTable.ID(),
		map[string]any{"limit": llmprompts.LimitVars{Default: browseDefaultRows, Max: browseMaxRows}}), "connectionId", "path")
	sch.Properties["limit"].Minimum = jsonschema.Ptr(1.0)
	sch.Properties["limit"].Maximum = jsonschema.Ptr(float64(browseMaxRows))
	sch.Properties["offset"].Minimum = jsonschema.Ptr(0.0)
	sch.Properties["offset"].Maximum = jsonschema.Ptr(float64(browseMaxOffset))
	cols := sch.Properties["columns"]
	cols.MaxItems = jsonschema.Ptr(browseMaxColumns)
	filters := sch.Properties["filters"]
	filters.MaxItems = jsonschema.Ptr(browseMaxFilters)
	fi := filters.Items
	ops := make([]any, 0, len(browseOps))
	for op := range browseOps {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].(string) < ops[j].(string) })
	fi.Properties["op"].Enum = ops
	fi.Properties["column"].MinLength = jsonschema.Ptr(1)
	fi.Properties["value"] = browseScalarSchema(fi.Properties["value"].Description)
	fi.Properties["values"].Items = browseScalarSchema("")
	fi.Properties["values"].MaxItems = jsonschema.Ptr(browseMaxInValues)
	order := sch.Properties["order"]
	order.MaxItems = jsonschema.Ptr(browseMaxOrder)
	order.Items.Properties["direction"].Enum = []any{"asc", "desc"}
	order.Items.Properties["column"].MinLength = jsonschema.Ptr(1)
	ex := sch.Properties["exemption"]
	ex.Properties["id"].MinLength = jsonschema.Ptr(1)
	ex.Properties["reason"].MinLength = jsonschema.Ptr(1)
	ex.Properties["reason"].MaxLength = jsonschema.Ptr(maxExemptionReasonRunes)
	return sch
}

// withPanicRecovery wraps every MCP tool handler mcp.AddTool registers: go-sdk v1.8.0 runs
// each request in its own goroutine with no recover() anywhere in the module, so an unguarded nil
// deref or index panic in any handler — render.go scanning client SQL, queryplan parsing server
// EXPLAIN output, a mask transform, a tree.Service call — kills the whole desktop app, including
// every open editor and terminal, from an authenticated local client. Generic over the handler's
// own args type so every tool shares one wrapper rather than six hand-duplicated ones; every
// handler here returns `any` as its result type (tools.go), so that half is fixed. The recovered
// message never repeats the panic value: on a masked connection it could carry a row's own data
// (mirrors maskedToolError's posture) — only the stack goes to the log.
func withPanicRecovery[In any](s *Server, name string, h func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (res *mcp.CallToolResult, out any, err error) {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("dbmcp: tool handler panicked", "tool", name, "panic", fmt.Sprintf("%v", r), "stack", string(debug.Stack()))
				res, out, err = errResult("internal error handling this tool call")
			}
		}()
		ctx, cancel := bindRequestCancellation(ctx)
		defer cancel()
		return h(adapterhost.WithOpSource(ctx, adapterhost.OpSourceMCP), req, in)
	}
}

// TokenRecord returns the currently-held auth record (hash+salt+expiry, never the plaintext) —
// the gate seam: bridge.DbMcpService checks Command/Install availability by reading the
// helper token mirror file back and verifying it against this record, not an in-memory
// "this run minted" flag.
func (s *Server) TokenRecord() mcpauth.Record {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.token
}

// TokenExpiry returns the currently-held record's own expiry instant (zero for a record that has
// not yet been stamped — see mcpauth.LoadOrMintTTL).
func (s *Server) TokenExpiry() time.Time {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.token.ExpiresAt
}

// SetToken replaces this instance's own live auth record — bridge.DbMcpService's Regenerate
// action, safe to call while requests are in flight: the very next request is checked against the
// new record, never a stale in-memory copy. The plaintext itself is not stored here — the
// caller persists it to the helper token mirror file itself, the same file the gate above reads
// back.
func (s *Server) SetToken(rec mcpauth.Record) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.token = rec
}

// Close shuts the HTTP listener down. Idempotent.
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = s.closeHTTP()
		if s.unwatchList != nil {
			s.unwatchList()
		}
		// Read lanes are shared with the external instance; a session server leaves them to the idle timeout.
		if lanes, ok := s.cfg.Query.(laneCloser); ok && s.cfg.Sessions == nil {
			lanes.CloseReadLanes(context.Background())
		}
	})
	return err
}
