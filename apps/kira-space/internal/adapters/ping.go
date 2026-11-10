package adapters

import (
	"context"
	"errors"
)

// Pinger is the optional liveness check: one cheap round trip on the connection Execute uses.
// Like StatementClassifier it is not a method on Adapter. It never resolves secrets and never
// dials beyond what the adapter's own pool already does.
type Pinger interface {
	Ping(ctx context.Context) error
}

// PingFailure is the Error a failed Ping returns: err as the adapter's own mapError produced it,
// except a generic query-class failure becomes CodeEngineDown, since an answer-less ping means the
// connection is down, not that a statement was bad.
func PingFailure(err error) error {
	var ae *Error
	if errors.As(err, &ae) && ae.Code == CodeQuery {
		return New(CodeEngineDown, ae.Message, ae.Cause)
	}
	return err
}
