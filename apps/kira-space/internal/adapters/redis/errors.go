package redis

import (
	"context"
	"errors"
	"regexp"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

var authPrefixRE = regexp.MustCompile(`(?i)^(NOAUTH|WRONGPASS|NOPERM)\b|invalid password`)

// mapError mirrors errors.ts's mapError — a single place that turns a driver-thrown error
// into the closed AdapterError code set, preserving the server's own message verbatim. ioredis's
// two message-text tests ("connection is closed"/"stream isn't writeable") are deleted, not
// ported: go-redis's own *net.OpError/*net.DNSError and context errors already cover that ground
// through Go's own error types, not string matching.
func mapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	message := err.Error()

	if errors.Is(err, context.Canceled) {
		return adapters.New(adapters.CodeCancelled, message, err)
	}

	var redisErr goredis.Error
	if errors.As(err, &redisErr) {
		if authPrefixRE.MatchString(message) {
			return adapters.New(adapters.CodeAuth, message, err)
		}
		return adapters.New(adapters.CodeQuery, message, err)
	}

	if ne := adapters.ClassifyNetError(err); ne.IsNetError {
		if ne.NetTimeout {
			return adapters.New(adapters.CodeTimeout, message, err)
		}
		return adapters.New(adapters.CodeConnect, message, err)
	}
	if errors.Is(err, goredis.ErrClosed) {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}
