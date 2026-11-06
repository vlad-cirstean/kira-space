package mongo

import "testing"

// require/prefer used to map to InsecureSkipVerify:true, with no native MongoDB convention to
// justify it. require/prefer/verify-full now all verify, matching the Kafka adapter;
// verify-none/insecure is the new explicit opt-out.
func TestTLSConfigForSslmode(t *testing.T) {
	for _, mode := range []string{"require", "prefer", "verify-full"} {
		t.Run(mode+" verifies", func(t *testing.T) {
			cfg, err := tlsConfigForSslmode(mode)
			if err != nil {
				t.Fatalf("tlsConfigForSslmode(%q): %v", mode, err)
			}
			if cfg.InsecureSkipVerify {
				t.Fatalf("InsecureSkipVerify = true for sslmode=%s — certificate verification is disabled", mode)
			}
		})
	}

	for _, mode := range []string{"verify-none", "insecure"} {
		t.Run(mode+" is the explicit opt-out", func(t *testing.T) {
			cfg, err := tlsConfigForSslmode(mode)
			if err != nil {
				t.Fatalf("tlsConfigForSslmode(%q): %v", mode, err)
			}
			if !cfg.InsecureSkipVerify {
				t.Fatalf("InsecureSkipVerify = false for sslmode=%s, want true", mode)
			}
		})
	}

	t.Run("unknown mode fails loudly", func(t *testing.T) {
		if _, err := tlsConfigForSslmode("bogus"); err == nil {
			t.Fatal("expected an error for an unrecognized sslmode")
		}
	})
}
