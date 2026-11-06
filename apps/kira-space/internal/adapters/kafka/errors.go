package kafka

import (
	"context"
	"errors"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
)

// mapError is errors.ts's mapError, ported from librdkafka's numeric error codes to *kerr.Error's
// protocol codes. Dispatch order ports errors.ts's own verbatim: cancellation first,
// then connect, then timeout, then auth, then the unknown-topic special case, then E_QUERY as the
// default — the default is load-bearing, because a topic/partition gone at read time (deleted
// concurrently) is an ordinary query-time condition, not a connection failure.
func mapError(err error) *adapters.Error {
	if err == nil {
		return nil
	}
	var ae *adapters.Error
	if errors.As(err, &ae) {
		return ae
	}
	message := err.Error()

	// KF-2(c): a cancelled context surfaces from kadm wrapped in a *fmt.wrapError ("operation was
	// canceled"), not always a bare context.Canceled — errors.Is, never a type assertion.
	if errors.Is(err, context.Canceled) {
		return adapters.New(adapters.CodeCancelled, message, err)
	}

	// The ERR__-prefixed librdkafka client-local codes (ERR__TRANSPORT, ERR__RESOLVE,
	// ERR__ALL_BROKERS_DOWN, ERR__STATE, ERR__AUTHENTICATION) have no protocol-level equivalent in
	// franz-go's *kerr.Error — re-derived from Go's own network error types instead, the same
	// pattern postgres/errors.go, mysqlfamily/errors.go, redis/errors.go and awscfg/errors.go
	// already use. Ping against an unreachable host returns a *fmt.wrapError around a
	// plain *net.OpError, reachable via errors.As, in ~280µs.
	if ne := adapters.ClassifyNetError(err); ne.DNS || ne.OpError != nil {
		return adapters.New(adapters.CodeConnect, message, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return adapters.New(adapters.CodeTimeout, message, err)
	}
	if errors.Is(err, kgo.ErrClientClosed) {
		return adapters.New(adapters.CodeConnect, message, err)
	}

	var ke *kerr.Error
	if errors.As(err, &ke) {
		switch ke.Code {
		case kerr.RequestTimedOut.Code:
			return adapters.New(adapters.CodeTimeout, message, err)
		case kerr.SaslAuthenticationFailed.Code, kerr.TopicAuthorizationFailed.Code,
			kerr.GroupAuthorizationFailed.Code, kerr.ClusterAuthorizationFailed.Code:
			return adapters.New(adapters.CodeAuth, message, err)
		case kerr.UnknownTopicOrPartition.Code, kerr.UnknownTopicID.Code:
			return adapters.New(adapters.CodeQuery, message, err)
		}
		return adapters.New(adapters.CodeQuery, message, err)
	}

	return adapters.New(adapters.CodeQuery, message, err)
}
