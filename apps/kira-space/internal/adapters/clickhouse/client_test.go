// client_test.go is a pure unit test (no server) for resolveTarget's sslmode handling.
package clickhouse

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// `prefer` is a valid, documented value for every other engine
// (postgres/mysqlfamily/redis/mongo/kafka) — ClickHouse's own switch rejected it,
// and since Options only ever comes from a connection string's own query parameters (there is no
// fields-mode sslmode UI), a user carrying one connection-string style across engines hit this
// immediately.
func TestResolveTarget_Sslmode(t *testing.T) {
	host := "ch.example.com"
	cfg := func(sslmode string) model.ResolvedConnectionConfig {
		return model.ResolvedConnectionConfig{
			Host:    &host,
			Options: map[string]any{"sslmode": sslmode},
		}
	}

	t.Run("disable stays http", func(t *testing.T) {
		target, err := resolveTarget(cfg("disable"), nil)
		if err != nil {
			t.Fatalf("resolveTarget: %v", err)
		}
		if target.scheme != "http" {
			t.Fatalf("scheme = %q, want http", target.scheme)
		}
	})

	for _, mode := range []string{"require", "prefer", "verify-full"} {
		t.Run(mode+" speaks https", func(t *testing.T) {
			target, err := resolveTarget(cfg(mode), nil)
			if err != nil {
				t.Fatalf("resolveTarget(%s): %v", mode, err)
			}
			if target.scheme != "https" {
				t.Fatalf("scheme = %q for sslmode=%s, want https", target.scheme, mode)
			}
		})
	}

	t.Run("an unrecognized sslmode fails loudly rather than falling back to plaintext", func(t *testing.T) {
		if _, err := resolveTarget(cfg("bogus"), nil); err == nil {
			t.Fatal("resolveTarget(bogus) returned nil error, want a connect error")
		}
	})
}
