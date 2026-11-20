package oplog

import (
	"context"
	"errors"
)

// FinishCall ends op from an MCP tools/call verdict: a canceled ctx is cancelled, a Go error or an
// IsError result (with its text) is error, else ok.
func FinishCall(op *Op, isError bool, text string, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		op.Finish(StatusCancelled, "")
	case err != nil:
		op.Finish(StatusError, err.Error())
	case isError:
		op.Finish(StatusError, text)
	default:
		op.Finish(StatusOK, "")
	}
}
