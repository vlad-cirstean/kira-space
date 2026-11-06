// client_test.go is a pure unit test (no broker) for resolveTLSOpt, the sslmode vocabulary
// connect's own dial can't be exercised against without a real container — mirrors
// redis/client_test.go's TestResolveFields_TLSModes for the same vocabulary.
package kafka

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// docs/ARCHITECTURE.md's own "sslmode semantics per
// engine (P21 round 1)" section documents verify-none/insecure as Kafka's escape hatch alongside
// redis and mongo — round 1 added it to the other two and missed kafka, whose default: branch
// rejected it outright, leaving a broker with a self-signed or internal-CA certificate unable to
// connect at all. This pins the fix and the rest of the vocabulary around it.
func TestResolveTLSOpt(t *testing.T) {
	cfg := func(sslmode string) model.ResolvedConnectionConfig {
		return model.ResolvedConnectionConfig{Options: map[string]any{"sslmode": sslmode}}
	}

	t.Run("no sslmode option leaves TLS off", func(t *testing.T) {
		ssl, skipVerify, err := resolveTLSOpt(model.ResolvedConnectionConfig{Options: map[string]any{}})
		if err != nil || ssl || skipVerify {
			t.Fatalf("resolveTLSOpt(no option) = (%v, %v, %v), want (false, false, nil)", ssl, skipVerify, err)
		}
	})

	t.Run("disable leaves TLS off", func(t *testing.T) {
		ssl, skipVerify, err := resolveTLSOpt(cfg("disable"))
		if err != nil || ssl || skipVerify {
			t.Fatalf("resolveTLSOpt(disable) = (%v, %v, %v), want (false, false, nil)", ssl, skipVerify, err)
		}
	})

	for _, mode := range []string{"require", "prefer", "verify-full"} {
		t.Run(mode+" enables TLS with verification", func(t *testing.T) {
			ssl, skipVerify, err := resolveTLSOpt(cfg(mode))
			if err != nil || !ssl || skipVerify {
				t.Fatalf("resolveTLSOpt(%s) = (%v, %v, %v), want (true, false, nil)", mode, ssl, skipVerify, err)
			}
		})
	}

	for _, mode := range []string{"verify-none", "insecure"} {
		t.Run(mode+" is the explicit opt-out", func(t *testing.T) {
			ssl, skipVerify, err := resolveTLSOpt(cfg(mode))
			if err != nil || !ssl || !skipVerify {
				t.Fatalf("resolveTLSOpt(%s) = (%v, %v, %v), want (true, true, nil)", mode, ssl, skipVerify, err)
			}
		})
	}

	t.Run("an unrecognized sslmode fails loudly rather than falling back to plaintext", func(t *testing.T) {
		ssl, skipVerify, err := resolveTLSOpt(cfg("bogus"))
		if err == nil {
			t.Fatal("resolveTLSOpt(bogus) returned nil error, want a connect error")
		}
		if ssl || skipVerify {
			t.Fatalf("resolveTLSOpt(bogus) = (%v, %v, %v), want ssl/skipVerify both false on error", ssl, skipVerify, err)
		}
	})
}
