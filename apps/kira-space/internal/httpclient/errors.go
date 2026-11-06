package httpclient

import (
	"errors"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// ErrorCode is httpclient's own closed error vocabulary — deliberately not adapters.ErrorCode:
// that set's own comment forbids additions without a matching renderer change, and
// reusing e.g. E_CONNECT for a refused TCP connection would make views/shared/viewOp.ts's
// DISCONNECTED_CODES classify an HTTP transport failure as "the database connection is gone" and
// pop a Reconnect gate over a tab that has no connection to reconnect. bridge/http.go maps these
// straight through to ipcerr.New(code, message) instead, joining the ipcerr family.
type ErrorCode string

const (
	// E_BAD_REQUEST — unparseable URL, non-http(s) scheme, missing host, unknown method: refused
	// before anything is sent.
	CodeBadRequest = ErrorCode(ipcerr.CodeBadRequest)
	// E_CANCELLED — ctx.Err() == context.Canceled: the Stop button, or the window closing.
	CodeCancelled = ErrorCode(ipcerr.CodeCancelled)
	// E_TIMEOUT — context.DeadlineExceeded.
	CodeTimeout = ErrorCode(ipcerr.CodeTimeout)
	// E_TRANSPORT — everything else: DNS failure, refused connection, TLS handshake
	// failure, a truncated response.
	CodeTransport = ErrorCode(ipcerr.CodeTransport)
	// E_CONTRACT — the UI always sets the value, so a violation is a Kira bug.
	CodeContract = ErrorCode(ipcerr.CodeContract)
)

// Error is httpclient's own error type, mirroring adapters.Error's shape (Code/Message/Cause,
// errors.As classification) in the package that owns this domain.
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
	// Timeline: what was measured before the send failed — non-nil only
	// for a failure classifySendErr produces (a transport failure or a body-read failure), since
	// only those run after newTimeline has installed a trace. Every other Error (a bad URL, an
	// unsupported method) fails before a send is ever attempted, and leaves this nil.
	Timeline *Timeline
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func newError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// newErrorT builds an Error whose message is catalog text for key.
func newErrorT(code ErrorCode, key string, params map[string]string, cause error) *Error {
	return newError(code, i18n.T(key, params), cause)
}

// CodeOf reports the ErrorCode of err via errors.As, for a caller to branch on without importing
// this package's concrete type everywhere.
func CodeOf(err error) (ErrorCode, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e.Code, true
	}
	return "", false
}
