package taskfiles

import (
	"context"
	"errors"
	"fmt"
)

// Parsed is a handler's parsed form of a file; the core never looks inside.
type Parsed any

// ParseError is a validation failure with the place it was found; Line is 1-based, 0 when unknown.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Handler is the content semantics of one kind of file. The core owns walking, reading, hashing,
// deciding and the index; a handler owns what the bytes mean and the SQLite cache of them. Every
// method runs inside the transaction of its file.
type Handler interface {
	// Parse validates the bytes. A *ParseError names the line; any other error is a failure of Kira.
	Parse(rel string, class Class, raw []byte) (Parsed, error)
	// Diff compares the accepted cache of the file with the parsed content. A nil parsed means the
	// file is gone.
	Diff(ctx context.Context, tx Tx, taskID, rel string, class Class, parsed Parsed) ([]Change, error)
	// Apply writes the accepted changes to the cache; the others stay out.
	Apply(ctx context.Context, tx Tx, taskID, rel string, class Class, parsed Parsed, accepted []Change) error
	// Adopt makes the cache equal the whole parsed file: the file is truth.
	Adopt(ctx context.Context, tx Tx, taskID, rel string, class Class, parsed Parsed) error
	// Remove drops the cache of a file that is gone.
	Remove(ctx context.Context, tx Tx, taskID, rel string, class Class) error
	// Render rebuilds the accepted file from the cache; false when no accepted copy exists.
	Render(ctx context.Context, tx Tx, taskID, rel string, class Class) ([]byte, bool, error)
}

// PendingItem is one file waiting on the user.
type PendingItem struct {
	Path      string
	Kind      Kind
	Cause     Cause
	Changes   []Change
	Collision *Collision
}

// PendingSubmit is the approval one submit raises: every pending file of the call, one reason.
type PendingSubmit struct {
	TaskID  string
	Session Session
	Reason  string
	Items   []PendingItem
}

// Events receives what a submit decided beyond the reply.
type Events interface {
	// Pending raises the approval of the pending files; a later submit of the same file replaces its item.
	Pending(ctx context.Context, p PendingSubmit) error
	// Accepted reports an accepted file that carries a notification.
	Accepted(ctx context.Context, taskID, rel string, n Notify) error
}

// ErrNoHandler marks a kind with no registered handler.
var ErrNoHandler = errors.New("taskfiles: no handler for the kind")
