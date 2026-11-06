package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const connectTimeout = 10 * time.Second // client.ts's CONNECT_TIMEOUT_MS

// connect is client.ts's connectKafka. Resolves host/port/credentials from URI or
// fields mode, then kgo.NewClient (no I/O yet) -> Client.Ping (a bounded reachability probe,
// franz-go's own analogue of client.ts's admin.listTopics({timeout}) probe — "admin.connect()
// alone proves nothing about broker reachability") -> kadm.NewClient. opts is returned alongside
// so read.go can build the ephemeral per-browse client from the same seed/security options
// without duplicating the resolution logic.
// resolveTLSOpt is connect's sslmode handling, split out so the vocabulary can be unit-tested
// without a real broker (connect itself dials via Client.Ping). Ports byte-identically to the
// original inline switch: every non-'disable' mode verifies by default — deliberately
// unlike libpq's own `require` (no verification) — with verify-none/insecure as the explicit
// opt-out (docs/ARCHITECTURE.md's own "sslmode semantics per engine (P21 round 1)" section already
// documented this escape hatch for Kafka; round 1 added it to redis/mongo but missed kafka, leaving
// a broker with a self-signed or internal-CA certificate — the case the escape hatch exists for —
// unable to connect at all).
func resolveTLSOpt(cfg model.ResolvedConnectionConfig) (ssl bool, skipVerify bool, err error) {
	sslmode, sslEnabled, err := adapters.ParseSSLMode(cfg.Options, "kafka", "require", "prefer", "verify-full", "verify-none", "insecure")
	if err != nil || !sslEnabled {
		return false, false, err
	}
	return true, adapters.SkipsVerification(sslmode), nil
}

// closeOnError runs call and closes cl when it fails or ctx ends first. franz-go's request wait
// does not watch ctx while a broker withholds its response, so a cancelled Connect would block
// until the request timeout; closing the client fails the pending request. A cancelled ctx returns
// E_CANCELLED at once, without waiting for call.
func closeOnError(ctx context.Context, cl *kgo.Client, call func() error) error {
	errc := make(chan error, 1)
	go func() { errc <- call() }()
	select {
	case err := <-errc:
		if err != nil {
			cl.Close()
		}
		return err
	case <-ctx.Done():
		go cl.Close()
		return adapters.CheckCancelled(ctx)
	}
}

// brokerEndpoint resolves the seed broker and optional SASL pair from a URI or discrete fields.
func brokerEndpoint(cfg model.ResolvedConnectionConfig) (host string, port int, username, password string, err error) {
	if cfg.Mode == "uri" && cfg.URI != nil && *cfg.URI != "" {
		// url.Parse is permissive where client.ts's `new URL` throws (a malformed string still
		// "parses" with an empty Host) — the emptiness check below, not a parse error, is what
		// carries the "could not parse the connection URI" case (mirrors awscfg.Resolve's own
		// documented asymmetry).
		u, err := url.Parse(*cfg.URI)
		if err != nil || u.Hostname() == "" {
			return "", 0, "", "", mapError(errors.New("could not parse the connection URI"))
		}
		host = u.Hostname()
		port = 9092
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				port = n
			}
		}
		if u.User != nil {
			username = u.User.Username()
			password, _ = u.User.Password()
		}
	} else {
		host = "localhost"
		if cfg.Host != nil && *cfg.Host != "" {
			host = *cfg.Host
		}
		port = 9092
		if cfg.Port != nil {
			port = *cfg.Port
		}
		if cfg.Username != nil {
			username = *cfg.Username
		}
		if cfg.Password != nil {
			password = *cfg.Password
		}
	}
	return host, port, username, password, nil
}

func connect(ctx context.Context, cfg model.ResolvedConnectionConfig, log func(level, message string)) (*kgo.Client, *kadm.Client, []kgo.Opt, error) {
	host, port, username, password, err := brokerEndpoint(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	ssl, skipVerify, err := resolveTLSOpt(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(net.JoinHostPort(host, strconv.Itoa(port))),
		kgo.ClientID("kira-studio"),
		kgo.DialTimeout(connectTimeout),
		// P58e E14: franz-go's idempotent producing defaults on where librdkafka's defaults off.
		// Idempotency needs an InitProducerId round trip, which hangs rather than failing on a
		// single-broker cluster whose transaction.state.log.replication.factor is Kafka's default
		// of 3 (packages/db-fixtures/support/kafka.ts's own scar). Matching librdkafka's default is the
		// conservative, behaviour-preserving choice — kept regardless of what a test container
		// (which sets the replication factor to 1) would show (KF-4(e)).
		kgo.DisableIdempotentWrite(),
	}
	if ssl {
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{InsecureSkipVerify: skipVerify})) //nolint:gosec // explicit opt-out via verify-none/insecure, not the default
	}
	// SASL/PLAIN was configured only when username *and* password were both non-empty —
	// with only one of the pair present, the whole mechanism was silently dropped and the client
	// connected anonymously; against a SASL-requiring broker that surfaced as a transport-level
	// refusal (E_QUERY, "is SASL missing?"), blaming the broker for a credential this app chose not
	// to send.
	//
	// franz-go's plain.Auth.AsMechanism() (pkg/sasl/plain/plain.go) refuses locally — before ever
	// contacting the broker — the instant either User or Pass is empty ("PLAIN user and pass must
	// be non-empty"), an error kafka/errors.go's mapError has no reason to classify as auth.
	// Reproduced against a real SASL_PLAINTEXT broker, "username with no password" still surfaced
	// as E_QUERY through that client-side refusal, not E_AUTH. Both halves of a half-filled pair
	// are equally unusable by this mechanism, so both are treated symmetrically.
	halfFilledCreds := (username == "") != (password == "")
	if username != "" && password != "" {
		opts = append(opts, kgo.SASL(plain.Auth{User: username, Pass: password}.AsMechanism()))
	}
	// A half-filled pair no longer fails up front: ConnectionDialog.vue pairs
	// username/password on one row, so a PLAINTEXT broker with a stray, half-typed credential must
	// still connect anonymously, ignoring it — dial without SASL
	// (the branch above skips it for a half-filled pair, same as an empty pair) and let the dial
	// itself decide.
	if halfFilledCreds {
		// round-2 finding 6: against a PLAINTEXT broker this silently drops the half-typed
		// credential with no trace — log it, matching the redis adapter's own INFO-refused
		// precedent (internal/adapters/redis/adapter.go).
		log("warn", "kafka: half-filled username/password ignored (both are required for SASL/PLAIN)")
	}

	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, nil, nil, mapError(err)
	}
	if err := closeOnError(ctx, cl, func() error { return cl.Ping(ctx) }); err != nil {
		// Only when the anonymous dial fails the specific way a SASL-requiring broker's own
		// missing-credential refusal does — franz-go's own *kgo.ErrFirstReadEOF, its "is SASL
		// missing?" diagnosis (confirmed against a real SASL_PLAINTEXT broker: an anonymous dial
		// here fails with exactly this) — is the half-filled pair likely the reason,
		// so surface the clear E_AUTH instead of the raw, confusing broker-side text.
		// A fully-omitted pair does not get this treatment: nothing was offered there, so blaming
		// a credential the caller never attempted would be wrong; here, one half of a pair was
		// typed in.
		//
		// ErrFirstReadEOF is constructed at three different call sites inside franz-go with no way
		// to tell them apart from here, so it can't distinguish a SASL-requiring broker from a TLS
		// misconfiguration or a connection severed for some other reason — the message below names
		// multiple possibilities rather than asserting SASL specifically, even though SASL is the
		// scenario this heuristic is scoped to detect.
		if halfFilledCreds {
			var firstReadEOF *kgo.ErrFirstReadEOF
			if errors.As(err, &firstReadEOF) {
				return nil, nil, nil, adapters.New(adapters.CodeAuth,
					"kafka: connection closed immediately after a half-filled username/password — "+
						"the broker may require SASL/PLAIN authentication (needs both a username and "+
						"a password), TLS, or the connection was otherwise refused", nil)
			}
		}
		return nil, nil, nil, mapError(err)
	}
	return cl, kadm.NewClient(cl), opts, nil
}
