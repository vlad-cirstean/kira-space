package grpcclient

import "github.com/kirathecat/kira-studio/internal/ipcerr"

// The four codes this package owns (P11 D16) — deliberately not adapters.ErrorCode
// (views/shared/viewOp.ts's DISCONNECTED_CODES would misread a connect failure as "the database
// connection is gone" and pop a Reconnect gate over a tab that has no connection to reconnect),
// mirroring httpclient's own closed vocabulary (P2 D8).
//
// The crucial distinction (D16): a non-OK gRPC status is NOT one of these four. codes.Unavailable
// (a refused connection, a TLS failure, a plaintext/TLS mismatch — F10) and a context-caused
// codes.Canceled/codes.DeadlineExceeded are the two exceptions that DO become one of these —
// everything else a server actually answered with (PermissionDenied, NotFound, ResourceExhausted,
// …) comes back as a CallResult with Code/CodeName/StatusMessage populated and err == nil.
const (
	CodeBadRequest = string(ipcerr.CodeBadRequest)
	CodeSchema     = string(ipcerr.CodeBadRequest)
	CodeTransport  = string(ipcerr.CodeTransport)
	CodeCancelled  = string(ipcerr.CodeCancelled)
	// CodeTimeout: a server that accepted TCP and then never answered.
	CodeTimeout = string(ipcerr.CodeTimeout)
)

// Error is the one error type every function in this package returns on failure — mirrors
// httpclient.Error's own shape (Code + Message), widened by one field. Partial is non-nil only for
// a ServerStream that delivered some messages before failing or being cancelled: the terminal
// counts/header/trailer it got as far as, mirroring the Details channel so
// bridge/grpc.go can still record a completed history entry and push a terminal event carrying the
// true partial state.
type Error struct {
	Code    string
	Message string
	Partial *CallResult
}

func (e *Error) Error() string { return e.Message }

func newError(code, message string) *Error { return &Error{Code: code, Message: message} }

// BadRequest — a malformed target, an unknown method, a request JSON that fails protojson, or an
// illegal metadata key.
func BadRequest(message string) *Error { return newError(CodeBadRequest, message) }

// SchemaError — reflection unavailable, a .proto that will not compile, or a method not present in
// the resolved schema.
func SchemaError(message string) *Error { return newError(CodeSchema, message) }

// Transport — dial, TLS, or codes.Unavailable.
func Transport(message string) *Error { return newError(CodeTransport, message) }

// Timeout — a deadline of this package's own (dial, reflection, unary) expired.
func Timeout(message string) *Error { return newError(CodeTimeout, message) }

// Cancelled — the call's context ended (the Stop button, or a caller-supplied deadline).
func Cancelled(message string) *Error { return newError(CodeCancelled, message) }
