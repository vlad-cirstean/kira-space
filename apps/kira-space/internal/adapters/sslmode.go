package adapters

// This file hoists the sslmode vocabulary switch all six TLS-capable adapters' own client.go
// independently re-implemented: re-read Options["sslmode"], treat "" and
// "disable" as no TLS, and fail loudly on anything outside that adapter's own accepted vocabulary.
// Each adapter keeps its own tls.Config construction — the mysql-family TLS registry, pg
// verify-ca's hostname-skipping chain check, ClickHouse's scheme choice, and redis's rediss://
// implying TLS — since those differ by driver, not by vocabulary.

// ParseSSLMode reads Options["sslmode"] and normalizes the shared empty/"disable" convention every
// adapter's own switch already agreed on: missing, "", or "disable" means no TLS at all
// (enabled=false, mode=""). Any other value must be one of accepted, or this fails loudly with
// "<engine>: unknown sslmode "<value>"" — a typo here must never silently fall back to a plaintext
// connection while the user believes TLS is configured.
func ParseSSLMode(options map[string]any, engine string, accepted ...string) (mode string, enabled bool, err error) {
	raw, ok := options["sslmode"].(string)
	if !ok || raw == "" || raw == "disable" {
		return "", false, nil
	}
	for _, a := range accepted {
		if raw == a {
			return raw, true, nil
		}
	}
	return "", false, New(CodeConnect, engine+`: unknown sslmode "`+raw+`"`, nil)
}

// SkipsVerification reports whether mode is the "skip certificate verification" escape hatch
// redis, mongo and kafka's own sslmode vocabularies agree on — verify-none/insecure. Postgres,
// mysql-family and ClickHouse have no equivalent value in their own accepted sets.
func SkipsVerification(mode string) bool {
	return mode == "verify-none" || mode == "insecure"
}
