package mongo

import (
	"context"
	"errors"

	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// mapError mirrors errors.ts's mapError. Ordered: a genuine context cancellation first (this only
// ever fires for Connect's own real ctx — every data-plane call runs on RunWithAbortRace's
// detached context, which never surfaces context.Canceled here); then a network/server-selection
// failure; then the two auth command codes (18 = AuthenticationFailed, 13 = Unauthorized); else
// E_QUERY, same as errors.ts's own fallback.
func mapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	message := err.Error()

	if errors.Is(err, context.Canceled) {
		return adapters.New(adapters.CodeCancelled, message, err)
	}
	if mongodriver.IsNetworkError(err) || mongodriver.IsTimeout(err) {
		return adapters.New(adapters.CodeConnect, message, err)
	}
	var cmdErr mongodriver.CommandError
	if errors.As(err, &cmdErr) && (cmdErr.Code == 18 || cmdErr.Code == 13) {
		return adapters.New(adapters.CodeAuth, message, err)
	}
	return adapters.New(adapters.CodeQuery, message, err)
}
