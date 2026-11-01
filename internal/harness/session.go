package harness

import (
	"context"
	"time"
)

// SessionLocator is a provider whose interactive session id is assigned by the agent after launch.
// A provider without it takes the id Kira proposes in InteractiveSpec.
type SessionLocator interface {
	// FindSession returns the id of the newest session in q.Cwd started at or after q.Since, ""
	// when none exists yet.
	FindSession(ctx context.Context, q SessionQuery) (string, error)
}

// SessionQuery narrows FindSession to one launch.
type SessionQuery struct {
	Cwd   string
	Since time.Time
}

// ValidSessionID reports whether id is safe to put in a later shell command: 1 to 128 characters
// of letters, digits, '_', '.' and '-'.
func ValidSessionID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '-':
		default:
			return false
		}
	}
	return true
}
