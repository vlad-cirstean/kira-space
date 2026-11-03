package model

// OpRecord mirrors packages/shared/domain/ops.ts's opRecordSchema.
type OpRecord struct {
	ID           string  `json:"id"`
	ConnectionID *string `json:"connectionId"`
	TabID        *string `json:"tabId"`
	StartedAt    string  `json:"startedAt"`
	DurationMs   *int    `json:"durationMs"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	Rows         *int    `json:"rows"`
	Command      *string `json:"command"`
	Error        *string `json:"error"`
	// CommandTruncated (P23 D1(c)): Command was truncated to maxOpCommandBytes at storage time, so
	// it is no longer the whole script that ran — Re-run must refuse rather than replay a prefix.
	CommandTruncated bool `json:"commandTruncated"`
	// Path (P108 Part 11 F5): the encoded console path (database/schema) this op ran against — nil
	// for every op kind that isn't a console execute() batch, and for a pre-P108-Part-11 row. Set
	// by adapterhost.Dispatcher.Execute (op.SetPath), the one op kind that has a path at all.
	Path *string `json:"path"`
	// DelayMs (P307 Part 2): a preconnect op's configured wait, so the panel can count it down. Live
	// only, never stored in op_log.
	DelayMs *int `json:"delayMs,omitempty"`
	// Source (P328): who started the op, "user" or "mcp".
	Source string `json:"source"`
}

// OpAppend is ops.ts's AppendOpInput.
type OpAppend struct {
	ID           string
	ConnectionID *string
	TabID        *string
	Kind         string
	StartedAt    string
	Source       string
}

// OpFinish is ops.ts's FinishOpPatch.
type OpFinish struct {
	Status     string
	DurationMs int
	Rows       *int
	Command    *string
	Error      *string
	// Path (P108 Part 11 F5): see OpRecord.Path.
	Path *string
}

// opKinds mirrors ops.ts's opKindSchema. Note 'ddl' is deliberately absent (P52 §4.3 / P53 §3.1):
// the legacy 'ddl'->'definition' coercion is dropped, not ported.
var opKinds = map[string]bool{
	"connect": true, "disconnect": true, "children": true, "describe": true, "definition": true,
	"test": true, "read": true, "count": true, "mutate": true, "execute": true, "transfer": true,
	// P2: the op log's first connectionless op kind (internal/httpclient via bridge/http.go).
	"http": true,
	// P440: a Kubernetes cluster call, from the module or MCP; connectionless like http.
	"kubernetes": true,
	// P11 D7: a gRPC unary or server-streaming call (internal/grpcclient via bridge/grpc.go).
	"grpc": true,
	// Router.SchemaColumns/KeyTypes (router.go) were already passing these two
	// kinds to RunOp, but neither was ever a recognized OpKind — oplog.handleOpStart rejected the
	// op:start (never Appended), so the matching op:end fell through handleOpEnd's own
	// no-matching-start fallback and emitted a phantom "test"-kind record with a null
	// connectionId, which state/ops.ts then prepended as a bogus new row on every SQL-completion
	// schema fetch and every Redis browse scroll window.
	"schemaColumns": true, "keyTypes": true,
	// P307: the pre-connect script's startup wait (Router.RunPreconnect).
	"preconnect": true,
}

var opStatuses = map[string]bool{
	"running": true, "ok": true, "error": true, "cancelled": true,
}

var opSources = map[string]bool{"user": true, "mcp": true}

// ValidOpSource mirrors ops.ts's opSourceSchema.
func ValidOpSource(v string) bool { return opSources[v] }

// ValidOpKind mirrors ops.ts's opKindSchema.
func ValidOpKind(v string) bool { return opKinds[v] }

// ValidOpStatus mirrors ops.ts's opStatusSchema.
func ValidOpStatus(v string) bool { return opStatuses[v] }
