package model

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
	// P532: the table allow list's catalog probe (Router.StatementTables).
	"tables": true,
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
