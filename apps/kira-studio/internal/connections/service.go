// Package connections is the Go analogue of src/main/connections.ts: the full connection
// lifecycle service — CRUD, the in-memory connection-state map, connect/disconnect wired to
// internal/preconnect and internal/adapterhost, and the in-flight-connect dedupe.
package connections

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kirathecat/kira-studio/internal/kiratime"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/localauth"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/preconnect"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/notify"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Backend is the slice of adapter lifecycle operations this service calls through instead of
// straight into internal/adapterhost (the per-consumer-interface discipline — the same shape as
// tree.Backend and bridge.Canceller). *adapterhost.Router satisfies this structurally; a two-line
// fake can too, which is what keeps this package's own tests simple. Three methods: Test and
// Connect each need their own method (they resolve config differently and return different
// shapes), and Disconnect covers all three fire-and-forget call sites (onPreconnectExit, Remove,
// Disconnect).
type Backend interface {
	// Connect resolves cfg.Kind to a live adapter and returns what the caller needs to build the
	// connected state.
	Connect(ctx context.Context, cfg model.ResolvedConnectionConfig) (ConnectResult, error)
	// Test probes cfg without ever registering a live adapter — control.ts's handleTest:
	// connect, read the server version, unconditionally disconnect. A real Go error, not a
	// never-throws contract; Test (below) is what wraps it into TestResult.
	Test(ctx context.Context, cfg model.ResolvedConnectionConfig) (serverVersion string, err error)
	// Disconnect is fire-and-forget at every call site today, so it stays that shape here.
	Disconnect(ctx context.Context, connectionID string) error
	// SetThrottle installs (perSec > 0) or clears (perSec == 0) connectionID's command rate limit.
	// Called just before Connect (attemptConnect) and, when the connection is
	// currently connected, after a successful Update — so tuning the limit while hitting it
	// applies live, with no disconnect/reconnect round trip.
	SetThrottle(connectionID string, perSec float64)
	// RunPreconnect runs the pre-connect startup wait (run) as a `preconnect` op so the
	// Operations panel shows the script and a timer. command is the script text; connectionID is
	// nil for a dialog Test. delay is the connection's configured wait, shown as a countdown.
	RunPreconnect(ctx context.Context, connectionID *string, command string, delay time.Duration, run func(context.Context) error) error
}

// ConnectResult is engine-ops.ts's adapter:connect success payload, the part attemptConnect
// actually reads (serverVersion, caps) — Caps stays untyped because both a native adapter's
// adapters.Caps and a Node-forwarded connection's raw decoded JSON land here.
type ConnectResult struct {
	ServerVersion string
	Caps          any
}

// TestResult mirrors connections.ts's ConnectionTestResult — the shape Service.Test builds from
// Backend.Test's (serverVersion, error) return, matching the JSON the renderer already expects.
type TestResult struct {
	OK            bool    `json:"ok"`
	ServerVersion *string `json:"serverVersion,omitempty"`
	Error         *string `json:"error,omitempty"`
}

// RevealResult mirrors reveal()'s never-throws contract (P25 D9). Outcome is P14 D6's own
// discriminant — revealed | cancelled | confirmation-required | error — the renderer switches on
// it rather than inferring the case from which of Password/Error is set.
type RevealResult struct {
	Password *string `json:"password"`
	// URI is the decrypted connection URI of a URI-mode connection (P181); Password is nil then.
	URI     *string `json:"uri"`
	Error   *string `json:"error"`
	Outcome string  `json:"outcome"`
}

const (
	revealOutcomeRevealed             = "revealed"
	revealOutcomeCancelled            = "cancelled"
	revealOutcomeConfirmationRequired = "confirmation-required"
	revealOutcomeError                = "error"
)

// revealReason is LAContext.evaluatePolicy's localizedReason (P14 D11) — macOS prefixes it with
// "Kira Studio is trying to …" inside its own sheet, so this reads as a sentence fragment, not a
// standalone label.
const (
	revealReason    = "reveal a saved connection password."
	revealURIReason = "reveal a saved connection URI."
)

// Authorizer is P14's reveal gate (internal/localauth.Authorizer satisfies this) — an interface
// here, not the concrete type, so tests can inject a fake outcome sequence without a real clock or
// OS-auth probe wired through.
type Authorizer interface {
	Authorize(reason string, confirmed bool) (localauth.Outcome, error)
}

// Deps is everything the service needs from the rest of the app.
type Deps struct {
	Conns      *repos.ConnectionsRepo
	Secrets    *repos.SecretsRepo
	Metadata   *repos.MetadataCacheRepo
	Cipher     *secrets.Cipher
	Auth       Authorizer
	Backend    Backend
	Preconnect *preconnect.Supervisor
	// MaskRules is the rule store — Duplicate's own use: a duplicated connection
	// must carry over the source's mask rules, not just its MCP exposure, or it silently exposes
	// PII the original was protecting. Nil is tolerated (a test harness with nothing to duplicate
	// against masking need not set it) — Duplicate then skips copying rather than panicking.
	MaskRules *repos.MaskRulesRepo
}

// attempt is one in-flight Connect(id) call, shared by every caller that asks for the same id
// while it is running (at most one in-flight connect per connection). cancel: Disconnect/Remove
// call it to abort an attempt racing ahead of them, rather than letting it finish unobserved and
// resurrect what they just asked to go away.
type attempt struct {
	done   chan struct{}
	state  model.ConnectionState
	err    error
	cancel context.CancelFunc
}

// teardown counts the Disconnect/Remove calls tearing one id down; done closes when the last ends.
type teardown struct {
	n    int
	done chan struct{}
}

// Service is the Go analogue of connections.ts's ConnectionsService.
type Service struct {
	deps Deps

	mu       sync.Mutex
	states   map[string]model.ConnectionState
	inFlight map[string]*attempt
	// tearing holds one entry per id while Disconnect/Remove is aborting and tearing it down.
	tearing map[string]*teardown
	// tests holds the dialog Test runs in progress; Shutdown cancels and waits them out because
	// their pre-connect script is not tracked by Preconnect.StopAll until it has started.
	tests map[*attempt]struct{}
	// closed is set once by Shutdown (F13, P108 Part 7): every Connect arriving after refuses
	// outright rather than racing a StopAll that has already taken its own snapshot of what to kill.
	closed bool
	// emitMu makes store-then-emit one step, so two racing emits for an id reach subscribers in
	// the order states[id] was written. Subscribers never re-enter emitState.
	emitMu sync.Mutex

	stateChanged        notify.Emitter[model.ConnectionState]
	metadataInvalidated notify.Emitter[string]
	listChanged         notify.Emitter[[]model.ConnectionSummary]
}

func New(d Deps) *Service {
	return &Service{
		deps:     d,
		states:   make(map[string]model.ConnectionState),
		inFlight: make(map[string]*attempt),
		tearing:  make(map[string]*teardown),
		tests:    make(map[*attempt]struct{}),
	}
}

// Start wires the preconnect exit handler (D14: split from New so main.go controls wiring order
// and every test can attach its own listener before the first event).
func (s *Service) Start() {
	s.deps.Preconnect.OnExit(s.onPreconnectExit)
}

// Shutdown refuses every Connect from here on, cancels and waits out every attempt already
// in-flight, then kills every live pre-connect process. Called from main's before-quit (F13,
// P108 Part 7, ordered after DB MCP is already stopped there).
//
// Preconnect.StopAll only kills what is already tracked in its own entries map — an attempt whose
// Preconnect.Start call is still inside the settle window is not tracked yet at all (supervisor.go's
// own doc comment on entry), so a StopAll that ran while such an attempt was still settling would
// leave its sidecar to start being tracked moments later, orphaned, with nothing left running that
// will ever kill it. Cancelling every in-flight attempt's ctx first and waiting for each one's
// doConnect to actually return closes that gap: a cancelled Start either aborts within the settle
// window (supervisor's own ctx.Done() branch, never reaching s.entries) or, if it already lost that
// race and got tracked, is torn down by finalizeAbortedAttempt's own Preconnect.Stop(id) once
// attemptConnect notices ctx.Err() — either way, by the time every attempt's done channel closes,
// nothing this service itself started is still outside StopAll's final snapshot.
func (s *Service) Shutdown() {
	s.mu.Lock()
	s.closed = true
	attempts := make([]*attempt, 0, len(s.inFlight)+len(s.tests))
	for _, a := range s.inFlight {
		attempts = append(attempts, a)
	}
	for a := range s.tests {
		attempts = append(attempts, a)
	}
	s.mu.Unlock()

	for _, a := range attempts {
		a.cancel()
	}
	for _, a := range attempts {
		<-a.done
	}

	s.deps.Preconnect.StopAll()
}

// onPreconnectExit is preconnect.ts:154-168's port: any exit while armed means the connection can
// no longer reach its target — best-effort disconnect the adapter and surface why.
func (s *Service) onPreconnectExit(exit preconnect.Exit) {
	// D6: the best-effort adapter:disconnect runs on its own goroutine, exactly as
	// connections.ts:155's `void … .catch(() => {})` does — this handler must not block the
	// shared preconnect exit-emitter goroutine for however long that disconnect takes.
	go func() {
		_ = s.deps.Backend.Disconnect(context.Background(), exit.ConnectionID)
	}()

	detail := "(exit unknown)"
	if exit.Code != nil {
		detail = fmt.Sprintf("(exit %d)", *exit.Code)
	}
	if exit.Signal != "" {
		detail = fmt.Sprintf("(signal %s)", exit.Signal)
	}
	tail := ""
	if exit.LastStderr != nil && *exit.LastStderr != "" {
		tail = ": " + *exit.LastStderr
	}
	msg := i18n.T("backend.studio.connection.preconnectExit.error", map[string]string{"detail": detail, "tail": tail})
	s.emitState(model.ConnectionState{ConnectionID: exit.ConnectionID, Status: "error", Error: &msg, Since: nowMillis()})
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// errorMessage extracts the human message a *ipcerr.Error (or any other error) carries, for the
// state/result string fields that surface a failure's text rather than propagate a Go error —
// the Go analogue of `err instanceof Error ? err.message : String(err)`.
func errorMessage(err error) string {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie.Message
	}
	return err.Error()
}

// wrapErr satisfies P55 §2 D5: every error crossing out of this package is an *ipcerr.Error. An
// error that already is one (or wraps one, e.g. repos/secrets' fmt.Errorf-wrapped E_SECRET_STORE)
// passes through with its original code and message; anything else becomes E_INTERNAL.

func (s *Service) emitState(state model.ConnectionState) {
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	s.states[state.ConnectionID] = state
	s.mu.Unlock()
	s.stateChanged.Emit(state)
}

// emitListChanged broadcasts the authoritative list after any mutation — the renderer store
// otherwise only ever sees a connection created/changed through its own dialog wrappers, never
// one created via a direct call. Run synchronously (unlike connections.ts:96's `void
// emitListChanged()` fire-and-forget): a local list query never blocks long enough to need its
// own goroutine, and a synchronous broadcast is strictly easier to reason about, not a behaviour
// change a caller could observe. Failure is swallowed — a broadcast is best-effort and must not
// turn a successful mutation into a reported error.
func (s *Service) emitListChanged() {
	list, err := s.deps.Conns.List()
	if err != nil {
		return
	}
	s.listChanged.Emit(list)
}

func (s *Service) List() ([]model.ConnectionSummary, error) {
	list, err := s.deps.Conns.List()
	if err != nil {
		return nil, ipcerr.Wrap(err)
	}
	return list, nil
}

func (s *Service) Create(in Input) (model.ConnectionSummary, error) {
	in = in.normalized()
	if err := in.Validate(); err != nil {
		return model.ConnectionSummary{}, err
	}
	if in.Mode == "uri" && in.URI == nil {
		return model.ConnectionSummary{}, errURIRequired()
	}

	// P25 D6: encrypt the secret before writing anything — a failure here (cipher unavailable)
	// leaves no row behind at all. P21 round 3 finding 4: the row and its secrets are written by
	// the same INSERT statement (InsertWithSecret), never as separate writes.
	var storedSecret, storedURI *string
	var err error
	if in.Mode == "uri" {
		storedURI, err = s.sealURI(*in.URI, in.Password)
	} else if in.Password != nil {
		storedSecret, err = s.sealString(secrets.ScopeConnection, *in.Password)
	}
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}

	fields := in.ConnectionFields
	fields.Name = strings.TrimSpace(fields.Name)
	id := uuid.NewString()
	created, err := s.deps.Conns.InsertWithSecret(id, fields, kiratime.NowISO(), storedSecret, storedURI)
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}
	s.emitListChanged()
	return created, nil
}

func errURIRequired() *ipcerr.Error {
	return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connection.uriRequired.body", nil)
}

func errConnectionNotFound() *ipcerr.Error {
	return ipcerr.T(ipcerr.CodeNotFound, "backend.studio.connection.notFound.body", nil)
}

// sealString encrypts plain under scope into a column value.
func (s *Service) sealString(scope secrets.Scope, plain string) (*string, error) {
	enc, err := s.deps.Cipher.Encrypt(scope, plain)
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

// sealURI folds password into uri (foldPassword) and encrypts the result — the only form in which
// a URI is ever stored (P181).
func (s *Service) sealURI(uri string, password *string) (*string, error) {
	return s.sealString(secrets.ScopeConnectionURI, foldPassword(uri, password))
}

// secretWrites computes Update's two ciphertext writes (P181). Fields mode: password three-state
// (nil = unchanged, "" = clear, else replace), uri cleared. URI mode with a URI: sealed fold, the
// password column cleared. URI mode without one means "unchanged" and is only valid on a URI row.
func (s *Service) secretWrites(id string, existing *model.ConnectionSummary, in Input) (password, uri repos.SecretWrite, err error) {
	if in.Mode != "uri" {
		if in.Password != nil && *in.Password != "" {
			enc, err := s.sealString(secrets.ScopeConnection, *in.Password)
			if err != nil {
				return password, uri, err
			}
			password = repos.SecretWrite{Set: true, Value: enc}
		} else if in.Password != nil {
			password = repos.SecretWrite{Set: true}
		}
		return password, repos.SecretWrite{Set: true}, nil
	}
	if in.URI == nil {
		if existing.Mode != "uri" {
			return password, uri, errURIRequired()
		}
		if in.Password != nil {
			return password, uri, ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connection.passwordInUri.body", nil)
		}
		return password, uri, nil
	}
	// Fold the stored password only when nothing else supplies one: a fields-mode connection
	// flipped to URI without revealing its password must not lose it silently.
	effective := in.Password
	if pw := userinfoPassword(*in.URI); effective == nil && (pw == nil || *pw == "") {
		if effective, err = s.deps.Secrets.Get(id); err != nil {
			return password, uri, err
		}
	}
	enc, err := s.sealURI(*in.URI, effective)
	if err != nil {
		return password, uri, err
	}
	return repos.SecretWrite{Set: true}, repos.SecretWrite{Set: true, Value: enc}, nil
}

func (s *Service) Update(id string, in Input) (model.ConnectionSummary, error) {
	in = in.normalized()
	if err := in.Validate(); err != nil {
		return model.ConnectionSummary{}, err
	}

	// Read the stored row before it's overwritten, so a live connection's destination and ReadOnly
	// flag can be compared against the draft below. A connected adapter and both cache layers are
	// stamped with the config resolved at Connect time and never revisit it — editing them here
	// would otherwise silently do nothing (ReadOnly) or serve the old destination's cached data
	// under the new one's name.
	existing, err := s.deps.Conns.Get(id)
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}
	if existing == nil {
		return model.ConnectionSummary{}, errConnectionNotFound()
	}

	// Encrypt before writing anything — a failure here (cipher unavailable) means Update
	// never runs, leaving every other field exactly as it was rather than a half-applied edit.
	// The row and its secrets are written by the same UPDATE statement
	// (UpdateWithSecret), so a failed write cannot leave a new secret against the old destination.
	password, uri, err := s.secretWrites(id, existing, in)
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}
	// Read before the write below replaces the stored URI.
	unchanged := s.destinationUnchangedFor(id, in, existing)

	fields := in.ConnectionFields
	fields.Name = strings.TrimSpace(fields.Name)
	updated, err := s.deps.Conns.UpdateWithSecret(id, fields, kiratime.NowISO(), password, uri)
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}

	// destinationUnchanged is the same denylist Test
	// already trusts to decide whether the old destination's password may be injected — it is a
	// deliberate denylist (not host/port/database alone) so a future new field defaults to gated.
	// ReadOnly is the one field it excludes on purpose (safe to change without re-gating Test's
	// password injection) that still needs to force a reconnect here: it is captured once into
	// the adapter at Connect time (postgres/mysqlfamily/sqlite/redis/sqs/kafka each read their own
	// captured copy), so toggling it on a live connection was otherwise silently inert.
	// An in-flight attempt resolved its config before this edit, so it counts as
	// live — Disconnect aborts and waits it out, then Connect starts fresh on the new config.
	if (s.StateOf(id).Status == "connected" || s.connecting(id)) &&
		(!unchanged || in.ReadOnly != existing.ReadOnly) {
		// Disconnect (which already drops the enginecache's pages/counts for this connection,
		// adapterhost/router.go's own DropConnection) then reconnect — attemptConnect moves the
		// connection's metadata_cache epoch forward and emits the invalidation on its own,
		// exactly the sequence setConnectionReadOnly's frontend precedent runs for the read-only-
		// only case. A failed reconnect lands the connection in its normal "error" state, which the
		// UI already renders — never a silent no-op.
		if _, err := s.Disconnect(id); err != nil {
			slog.Warn("disconnect before reconnect failed", "scope", "connections", "id", id, "err", err)
		}
		if _, err := s.Connect(id); err != nil {
			slog.Warn("reconnect after update failed", "scope", "connections", "id", id, "err", err)
		}
	} else if s.StateOf(id).Status == "connected" {
		// Applies live, but only while connected — an edit to a disconnected
		// connection has nothing running to pace yet, and attemptConnect installs the right value
		// on the next connect regardless. Skipped above when a reconnect already happened, since
		// attemptConnect installs the current throttle itself.
		s.deps.Backend.SetThrottle(id, fields.ThrottlePerSec)
	}
	s.emitListChanged()
	return updated, nil
}

func (s *Service) Duplicate(id string) (model.ConnectionSummary, error) {
	existing, err := s.deps.Conns.Get(id)
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}
	if existing == nil {
		return model.ConnectionSummary{}, errConnectionNotFound()
	}
	newID := uuid.NewString()
	fields := existing.ConnectionFields
	fields.Name = duplicateName(fields.Name)
	// (a raw column copy, not decrypt-then-re-encrypt — the plaintext is never used, so
	// there is no reason for this path to need the OS key at all) plus the atomicity fix: the
	// new row and its copied password column are now written by the very same INSERT statement
	// (InsertDuplicateWithSecret), never as two separate writes — a crash between them can no
	// longer leave a passwordless duplicate behind.
	// mcp_enabled is always inserted off (that method's own doc comment) — flipped on below, once
	// mask rules are copied.
	created, err := s.deps.Conns.InsertDuplicateWithSecret(id, newID, fields, kiratime.NowISO())
	if err != nil {
		return model.ConnectionSummary{}, ipcerr.Wrap(err)
	}
	// Finding #6, M6: ConnectionFields carries the source's MCP exposure (McpEnabled/read/write/DDL
	// modes), but mask rules live in a separate table keyed by connection_id and are never copied by
	// InsertDuplicateWithSecret's own INSERT. Without this, a duplicated connection that was
	// masked-and-MCP-exposed would become MCP-exposed with zero mask rules, silently exposing
	// whatever PII the original was protecting.
	if s.deps.MaskRules != nil {
		if err := s.copyMaskRules(id, newID); err != nil {
			// The new row is already committed above (InsertDuplicateWithSecret)
			// — keep the UI in sync with what storage holds (a real, if not fully set up,
			// duplicate) rather than returning an error the caller sees while emitListChanged never
			// ran, leaving the new row invisible until the next unrelated list refresh.
			s.emitListChanged()
			return model.ConnectionSummary{}, ipcerr.Wrap(err)
		}
	}
	// Only now — after the mask rule copy above has succeeded — is MCP exposure switched on to
	// match the source. Before this, InsertDuplicateWithSecret copied McpEnabled verbatim in the
	// same INSERT that committed the row, so an error from copyMaskRules above still returned to
	// the caller, but the already-committed duplicate was left live and MCP-exposed with zero mask
	// rules. A crash between this write and the one above leaves the duplicate merely
	// not-yet-MCP-enabled (safe), never exposed-without-rules (unsafe).
	if fields.McpEnabled {
		if err := s.deps.Conns.SetMcpEnabled(newID, true, kiratime.NowISO()); err != nil {
			s.emitListChanged() // F8: same reasoning as the mask-rule-copy failure above.
			return model.ConnectionSummary{}, ipcerr.Wrap(err)
		}
		created.McpEnabled = true
	}
	s.emitListChanged()
	return created, nil
}

// copyMaskRules copies every mask rule on fromID onto toID — Duplicate's own step.
// uuid.NewString is passed through as the repo's own id-mint function: the list and every insert
// it drives now run inside CopyForConnection's own single transaction, rather than this method
// listing once (to count rows and mint that many ids) before a second, separate list call inside
// the repo — closing the "N ids for M rules" mismatch a concurrent rule add/remove on fromID could
// otherwise produce between those two, previously-separate list calls.
func (s *Service) copyMaskRules(fromID, toID string) error {
	return s.deps.MaskRules.CopyForConnection(fromID, toID, uuid.NewString, kiratime.NowISO())
}

func (s *Service) Remove(id string) error {
	// F4 (P108 Part 3): abort a racing in-flight Connect before it can finish and re-register a
	// live adapter/states entry/sidecar for an id this call is about to delete outright.
	defer s.beginTeardown(id)()
	s.abortInFlight(id)
	current := s.StateOf(id)
	if current.Status == "connected" || current.Status == "connecting" {
		_ = s.deps.Backend.Disconnect(context.Background(), id)
	}
	s.deps.Preconnect.Stop(id)
	if err := s.deps.Conns.Delete(id); err != nil { // cascades filters, metadata cache, saved queries
		return ipcerr.Wrap(err)
	}
	if err := s.deps.Secrets.Delete(id); err != nil {
		return ipcerr.Wrap(err)
	}
	s.mu.Lock()
	delete(s.states, id)
	s.mu.Unlock()
	s.emitListChanged()
	return nil
}

// Reorder persists the sidebar's drag order. An id deleted in another window fails the call; the
// renderer resyncs from List.
func (s *Service) Reorder(ids []string) ([]model.ConnectionSummary, error) {
	if len(ids) == 0 {
		return nil, ipcerr.Required("ids")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return nil, ipcerr.Contract("ids must be unique")
		}
		seen[id] = true
	}
	if err := s.deps.Conns.Reorder(ids); err != nil {
		return nil, ipcerr.Wrap(err)
	}
	s.emitListChanged()
	return s.List()
}

// Reveal never errors (P25 D9): the renderer's edit dialog has no error handling around this
// call, so an undecryptable stored secret — or a failed/declined authentication — must not become
// an unhandled failure, only a RevealResult whose Outcome names what happened.
//
// P14 D6: confirmed is honoured only when the gate itself reports OS authentication unavailable —
// Authorizer.Authorize enforces that (it ignores confirmed whenever it can actually evaluate), so
// this method never has to re-check which branch produced a grant.
func (s *Service) Reveal(id string, confirmed bool) RevealResult {
	existing, err := s.deps.Conns.Get(id)
	if err == nil && existing == nil {
		err = fmt.Errorf("connection %s not found", id)
	}
	if err != nil {
		msg := errorMessage(err)
		return RevealResult{Outcome: revealOutcomeError, Error: &msg}
	}
	uriMode := existing.Mode == "uri"
	reason, fetch := revealReason, func() (*string, error) { return s.deps.Secrets.Get(id) }
	if uriMode {
		reason, fetch = revealURIReason, func() (*string, error) { return s.deps.Secrets.GetURI(id) }
	}
	outcome, value, err := localauth.Gated(s.deps.Auth.Authorize, reason, confirmed, fetch)
	switch outcome {
	case localauth.GateAuthError:
		msg := errorMessage(err)
		slog.Warn("local authentication errored before reveal", "scope", "connections", "id", id, "err", msg)
		return RevealResult{Outcome: revealOutcomeError, Error: &msg}
	case localauth.GateCancelled:
		// D11: the user cancelled on purpose — no message, that would be nagging.
		return RevealResult{Outcome: revealOutcomeCancelled}
	case localauth.GateConfirmationRequired:
		return RevealResult{Outcome: revealOutcomeConfirmationRequired}
	case localauth.GateFetchError:
		msg := errorMessage(err)
		slog.Warn("secret reveal failed", "scope", "connections", "id", id, "err", msg)
		return RevealResult{Outcome: revealOutcomeError, Error: &msg}
	}
	slog.Info("secret revealed", "scope", "connections", "id", id)
	if uriMode {
		return RevealResult{Outcome: revealOutcomeRevealed, URI: value}
	}
	return RevealResult{Outcome: revealOutcomeRevealed, Password: value}
}

// destinationUnchanged reports whether the draft still points at the same place, over the same
// route, as the stored row. Test only injects the stored secret when this holds (see the comment
// on Test below): a draft edited to reach a different destination must never get the old
// destination's password handed to it.
//
// A real denylist, not one shaped like an allowlist that merely happened to cover every
// currently-present field — zeroExemptFields clears every member this function does not gate on,
// then reflect.DeepEqual compares what is left. Every field zeroExemptFields does not name is
// therefore gated by construction: a future new ConnectionFields member defaults to gated (safe)
// instead of silently falling through an allowlist the way the original fix
// (host/port/database/URI only, missing Preconnect/PreconnectSidecar/Options) once did.
func destinationUnchanged(in Input, stored model.ConnectionFields) bool {
	a, b := in.ConnectionFields, stored
	zeroExemptFields(&a)
	zeroExemptFields(&b)
	return reflect.DeepEqual(a, b)
}

// destinationUnchangedFor is destinationUnchanged against the stored row, with the stored URI
// decrypted into the comparison copy when the draft carries one (P181: the repo never returns it).
// A draft with no URI compares nil to nil; a decrypt failure counts as changed.
func (s *Service) destinationUnchangedFor(id string, in Input, stored *model.ConnectionSummary) bool {
	fields := stored.ConnectionFields
	if in.URI != nil && stored.Mode == "uri" {
		uri, err := s.deps.Secrets.GetURI(id)
		if err != nil || uri == nil {
			return false
		}
		fields.URI = uri
	}
	return destinationUnchanged(in, fields)
}

// zeroExemptFields clears every ConnectionFields member that is genuinely safe to change without
// re-gating Test's stored-password injection — every field zeroExemptFields leaves untouched is
// exactly what destinationUnchanged compares.
//
// Name/Color are purely cosmetic. ReadOnly/AutoExplain gate this app's own console, never what a
// connection connects to. ThrottlePerSec paces commands against whatever destination is
// already resolved, so it never changes what a destination-unchanged edit connects to —
// "edit the throttle → Test connection" must still inject the stored password. McpEnabled
// gates whether this connection is exposed to the DB MCP server, never what it connects to.
// McpDescription is free-text metadata, never interpreted; McpReadMode/McpWriteMode/McpDdlMode
// gate DB-MCP-path permissions, not the destination. McpAutoExplain gates DB-MCP-path plan-first
// behaviour, the same non-destination footing as AutoExplain. McpReviewResults gates DB-MCP-path
// result review, never the destination. PreconnectDelaySec is timing only,
// never the destination.
func zeroExemptFields(f *model.ConnectionFields) {
	f.Name = ""
	f.Color = ""
	f.ReadOnly = false
	f.AutoExplain = false
	f.ThrottlePerSec = 0
	f.PreconnectDelaySec = 0
	f.McpEnabled = false
	f.McpDescription = ""
	f.McpReadMode = ""
	f.McpWriteMode = ""
	f.McpDdlMode = ""
	f.McpAutoExplain = false
	f.McpReviewResults = ""
}

// Test never errors: a test run is never armed and never leaves a process behind, however it
// ended (the deferred Stop mirrors connections.ts:348-351's `finally`). It still runs the same
// Validate() Create/Update do (this was the one Input-accepting entry point that skipped
// it) — without that, an out-of-range Port reaches postgres/client.go's `uint16(*cfg.Port)`
// unchecked and silently wraps around to a different, in-range port instead of being rejected.
//
// existingID is the dialog's editingId, optional. Since the draft's password is no longer
// pre-filled on open, testing an existing connection the user hasn't retyped credentials for would
// otherwise probe with no password at all — resolving the stored secret here, server-side, fixes
// that properly for both fields and URI mode (URI mode already had this exact gap, for the same
// reason: an unchanged draft carries `password: null`). Not gated: Test uses the secret without
// displaying it, the same footing as Connect. A missing or undecryptable secret here is not
// fatal — resolveFromInput/Backend.Test below still run and report failure the normal way,
// exactly as a wrong password would.
//
// The stored secret is only injected when destinationUnchanged holds — otherwise
// "Edit connection → change Host (or Preconnect, or
// Options["sslmode"]/["endpoint"]) → Test" would decrypt the old destination's password and send
// it down whatever route the draft currently points at, with no auth gate on this path at all
// (Test is deliberately not auth-gated, see above). An edited destination instead tests with no
// password, same as a brand-new draft.
func (s *Service) Test(in Input, existingID string) TestResult {
	in = in.normalized()
	if existingID != "" {
		var err error
		if in, err = s.fillFromStored(in, existingID); err != nil {
			msg := errorMessage(err)
			return TestResult{OK: false, Error: &msg}
		}
	}
	err := in.Validate()
	if err == nil && in.Mode == "uri" && in.URI == nil {
		err = errURIRequired()
	}
	if err != nil {
		msg := errorMessage(err)
		return TestResult{OK: false, Error: &msg}
	}
	r := resolveFromInput(in)

	ctx, cancel := context.WithCancel(context.Background())
	run := &attempt{done: make(chan struct{}), cancel: cancel}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		msg := errorMessage(ipcerr.T(ipcerr.CodeUnavailable, "backend.studio.connection.shuttingDown.body", nil))
		return TestResult{OK: false, Error: &msg}
	}
	s.tests[run] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.deps.Preconnect.Stop(r.config.ID)
		cancel()
		s.mu.Lock()
		delete(s.tests, run)
		s.mu.Unlock()
		close(run.done)
	}()

	if r.preconnect != nil {
		err := s.deps.Backend.RunPreconnect(ctx, nil, *r.preconnect, r.preconnectDelay, func(opCtx context.Context) error {
			_, err := s.deps.Preconnect.Start(opCtx, r.config.ID, *r.preconnect, readyCheck(r.readyAddr), r.preconnectDelay)
			return err
		})
		if err != nil {
			msg := errorMessage(err)
			return TestResult{OK: false, Error: &msg}
		}
	}
	serverVersion, err := s.deps.Backend.Test(ctx, r.config)
	if err != nil {
		msg := errorMessage(err)
		return TestResult{OK: false, Error: &msg}
	}
	return TestResult{OK: true, ServerVersion: &serverVersion}
}

// fillFromStored supplies what a draft leaves to the stored row: a fields-mode
// draft with no password gets the stored one, a URI-mode draft with no URI (masked, never
// revealed) gets the stored decrypted URI — both only while destinationUnchanged holds. A URI
// decrypt failure is returned: there is nothing else to test.
func (s *Service) fillFromStored(in Input, existingID string) (Input, error) {
	existing, err := s.deps.Conns.Get(existingID)
	if err != nil || existing == nil {
		return in, nil
	}
	switch {
	case in.Mode == "uri" && in.URI == nil:
		if existing.Mode != "uri" || !s.destinationUnchangedFor(existingID, in, existing) {
			return in, nil
		}
		uri, err := s.deps.Secrets.GetURI(existingID)
		if err != nil {
			return in, err
		}
		in.URI = uri
	case in.Mode != "uri" && in.Password == nil && destinationUnchanged(in, existing.ConnectionFields):
		if pw, err := s.deps.Secrets.Get(existingID); err == nil {
			in.Password = pw
		}
	}
	return in, nil
}

// Connect deduplicates concurrent calls for the same id: every caller that arrives while an
// attempt is already running gets that same attempt's result instead of starting a second one.
// The attempt's own ctx is cancelled by Disconnect/Remove (abortInFlight) if either races ahead
// of it — threaded through to attemptConnect below. Refuses outright once Shutdown has run — see
// Shutdown's own doc comment for why.
func (s *Service) Connect(id string) (model.ConnectionState, error) {
	return s.ConnectContext(context.Background(), id)
}

// ConnectContext is Connect with the caller's context values (such as adapterhost.WithOpSource)
// carried into the attempt. Its cancellation is not: the attempt is shared with other waiters and
// ends only through abortInFlight.
func (s *Service) ConnectContext(parent context.Context, id string) (model.ConnectionState, error) {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return model.ConnectionState{}, ipcerr.T(ipcerr.CodeUnavailable, "backend.studio.connection.shuttingDown.body", nil)
		}
		// A Connect arriving mid-teardown waits it out, then starts fresh: starting now would race
		// the teardown's own Backend.Disconnect / row delete.
		if t, ok := s.tearing[id]; ok {
			s.mu.Unlock()
			<-t.done
			continue
		}
		prev, ok := s.inFlight[id]
		if !ok {
			break
		}
		s.mu.Unlock()
		<-prev.done
		// An aborted attempt's waiters get its result, never a fresh attempt: the aborting
		// teardown is still running and may delete the connection.
		return prev.state, prev.err
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	a := &attempt{done: make(chan struct{}), cancel: cancel}
	s.inFlight[id] = a
	s.mu.Unlock()

	a.state, a.err = s.doConnect(id, ctx)
	cancel() // releases ctx's own resources regardless of outcome; a no-op if already cancelled

	// Dropped before done closes, so a waiter that wakes on done never finds this attempt still
	// registered.
	s.mu.Lock()
	if s.inFlight[id] == a {
		delete(s.inFlight, id)
	}
	s.mu.Unlock()
	close(a.done)

	return a.state, a.err
}

// beginTeardown marks id as being torn down until the returned func runs; Connect holds off new
// attempts meanwhile.
func (s *Service) beginTeardown(id string) (end func()) {
	s.mu.Lock()
	t, ok := s.tearing[id]
	if !ok {
		t = &teardown{done: make(chan struct{})}
		s.tearing[id] = t
	}
	t.n++
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if t.n--; t.n == 0 {
			delete(s.tearing, id)
			close(t.done)
		}
	}
}

// abortInFlight cancels id's in-flight Connect attempt, if any, and waits for it to unwind —
// Disconnect, Remove and Update call it first. Cancelling only
// *asks* the attempt to stop, promptly inside a cancellation-aware wait (Backend.Connect,
// Preconnect.Start's settle window) and otherwise at its next check; finalizeAbortedAttempt
// undoes whatever it had registered. Waiting guarantees the attempt's last state emit lands before
// the caller's own teardown and emit, never after.
func (s *Service) abortInFlight(id string) {
	s.mu.Lock()
	a, ok := s.inFlight[id]
	s.mu.Unlock()
	if ok {
		a.cancel()
		<-a.done
	}
}

func (s *Service) connecting(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.inFlight[id]
	return ok
}

// doConnect is connections.ts:170-231's port. Only the pre-checks below (the row not existing, or
// a real read failure) return a Go error; everything attemptConnect can fail on becomes this
// connection's error *state* instead, exactly as the TS's catch block does.
func (s *Service) doConnect(id string, ctx context.Context) (model.ConnectionState, error) {
	summary, err := s.deps.Conns.Get(id)
	if err != nil {
		return model.ConnectionState{}, ipcerr.Wrap(err)
	}
	if summary == nil {
		return model.ConnectionState{}, errConnectionNotFound()
	}

	s.emitState(model.ConnectionState{ConnectionID: id, Status: "connecting", Since: nowMillis()})

	state, connErr := s.attemptConnect(id, ctx)
	if connErr == nil {
		return state, nil
	}

	msg := errorMessage(connErr)
	errState := model.ConnectionState{ConnectionID: id, Status: "error", Error: &msg, Since: nowMillis()}
	s.emitState(errState)
	return errState, nil
}

// attemptConnect is doConnect's try block: resolve, optionally start the pre-connect script, call
// the engine, optionally arm the sidecar, and on success push a metadata invalidation (P24 D6: the
// connection's own metadata_cache rows are no longer deleted here — the new epoch this state's
// Since carries is what internal/tree.Service's freshness rule treats every existing row as stale
// against, so each path re-fetches lazily, once, the first time a user actually opens it, rather
// than every row being deleted upfront and re-read whether or not anything ever asks for it again).
//
// F4 (P108 Part 3): ctx is cancelled when Disconnect/Remove races ahead of this attempt
// (abortInFlight). Checked after every step that could have raced against one of them —
// Preconnect.Start, Backend.Connect, and right before finalizing "connected" — routing to
// finalizeAbortedAttempt instead of the normal error/success path whenever it fires, so a
// Disconnect/Remove that arrived mid-connect is never silently undone once this attempt finishes.
func (s *Service) attemptConnect(id string, ctx context.Context) (model.ConnectionState, error) {
	r, err := resolve(s.deps.Conns, s.deps.Secrets, id)
	if err != nil {
		return model.ConnectionState{}, err
	}

	started := false
	if r.preconnect != nil {
		err := s.deps.Backend.RunPreconnect(ctx, &id, *r.preconnect, r.preconnectDelay, func(opCtx context.Context) error {
			_, err := s.deps.Preconnect.Start(opCtx, id, *r.preconnect, readyCheck(r.readyAddr), r.preconnectDelay)
			return err
		})
		if err != nil {
			// A cancelled script with ctx alive is the Operations panel's Cancel: a user choice, not
			// a failure, so it ends disconnected like a Disconnect-aborted wait.
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return s.finalizeAbortedAttempt(id)
			}
			return model.ConnectionState{}, err
		}
		started = true
	}

	// P28 §5.5: installed just before Connect, from the summary already read above.
	s.deps.Backend.SetThrottle(id, r.throttlePerSec)
	result, err := s.deps.Backend.Connect(ctx, r.config)
	if err != nil {
		if started {
			s.deps.Preconnect.Stop(id)
		}
		if ctx.Err() != nil {
			return s.finalizeAbortedAttempt(id)
		}
		return model.ConnectionState{}, err
	}

	if r.preconnectSidecar {
		// D7 (§4.4): overrides the settle-window auto-detection — always arm() here. A no-op if
		// the script already exited; may synchronously flip this connection to 'error' via
		// onPreconnectExit if it died between Start and here.
		s.deps.Preconnect.Arm(id)
	}
	if afterArm := s.StateOf(id); afterArm.Status == "error" {
		return afterArm, nil
	}
	if ctx.Err() != nil {
		return s.finalizeAbortedAttempt(id)
	}

	state := model.ConnectionState{
		ConnectionID: id, Status: "connected", ServerVersion: &result.ServerVersion,
		Since: nowMillis(), Caps: result.Caps,
	}
	s.emitState(state)
	// D11 (Step 6a numbering): the renderer's own in-memory copies (the tree's expanded set,
	// schemaColumnsState) drop on this push regardless — only the persisted L1 rows survive now
	// (P24 D6), so a path the user re-opens this session reads through to a fresh server answer.
	s.metadataInvalidated.Emit(id)
	return state, nil
}

// finalizeAbortedAttempt is attemptConnect's own unwind: ctx was cancelled by
// a Disconnect/Remove that raced ahead of this attempt — undo whatever it had already registered
// (a live adapter, an armed sidecar) rather than letting it finalize as "connected" and resurrect
// a connection the caller already asked to go away. Backend.Disconnect/Preconnect.Stop are each
// idempotent no-ops when there is nothing to undo (Router.Disconnect's own "no live adapter"
// no-op; the already-dead-entry guard in killEntry), so both run unconditionally rather than
// tracking exactly how far this attempt got before it was cancelled.
func (s *Service) finalizeAbortedAttempt(id string) (model.ConnectionState, error) {
	_ = s.deps.Backend.Disconnect(context.Background(), id)
	s.deps.Preconnect.Stop(id)
	state := model.ConnectionState{ConnectionID: id, Status: "disconnected", Since: nowMillis()}
	if existing, err := s.deps.Conns.Get(id); err == nil && existing != nil {
		// Disconnect raced us, not Remove — the row is still there, so keep states[id] in sync
		// (idempotent if Disconnect() itself already emitted this same state).
		s.emitState(state)
	} else {
		// Remove raced us — the row (and, by the time this runs, likely its states entry too) is
		// already gone. Delete states[id] rather than resurrecting an entry for an id that no
		// longer exists, regardless of whether Remove's own delete already ran.
		s.mu.Lock()
		delete(s.states, id)
		s.mu.Unlock()
	}
	return state, nil
}

func (s *Service) Disconnect(id string) (model.ConnectionState, error) {
	// F4 (P108 Part 3): abort a racing in-flight Connect before it can finish and re-register a
	// live adapter/"connected" state moments after this call reports "disconnected".
	defer s.beginTeardown(id)()
	s.abortInFlight(id)
	s.deps.Preconnect.Stop(id)
	_ = s.deps.Backend.Disconnect(context.Background(), id)
	// Cached metadata stays — "metadata stays, it is on disk".
	state := model.ConnectionState{ConnectionID: id, Status: "disconnected", Since: nowMillis()}
	s.emitState(state)
	return state, nil
}

// States returns every known state sorted by connection id (D7): Go map iteration is randomised,
// so a literal port of `[...states.values()]` would hand the renderer a different order on every
// call.
func (s *Service) States() []model.ConnectionState {
	s.mu.Lock()
	out := make([]model.ConnectionState, 0, len(s.states))
	for _, st := range s.states {
		out = append(out, st)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ConnectionID < out[j].ConnectionID })
	return out
}

func (s *Service) StateOf(id string) model.ConnectionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.states[id]; ok {
		return st
	}
	return model.ConnectionState{ConnectionID: id, Status: "disconnected", Since: nowMillis()}
}

func (s *Service) SecretsStatus() secrets.Status { return s.deps.Cipher.Status() }

func (s *Service) OnStateChange(fn func(model.ConnectionState)) (unsubscribe func()) {
	return s.stateChanged.Subscribe(fn)
}

func (s *Service) OnMetadataInvalidated(fn func(connectionID string)) (unsubscribe func()) {
	return s.metadataInvalidated.Subscribe(fn)
}

func (s *Service) OnListChanged(fn func([]model.ConnectionSummary)) (unsubscribe func()) {
	return s.listChanged.Subscribe(fn)
}
