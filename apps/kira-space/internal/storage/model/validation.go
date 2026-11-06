package model

import "fmt"

// ValidationError marks a caller-input failure (as opposed to a storage failure), so a bridge can
// answer E_BAD_REQUEST for it and E_INTERNAL for everything else.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Invalid builds a ValidationError from a format string.
func Invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}
