package secrets

// Scope is the kind of secret a ciphertext was sealed for. It is AES-GCM's additional
// authenticated data, which is what makes a ciphertext refuse to decrypt anywhere but the
// kind of column it was written for.
//
// These strings are a storage format, not a label: changing one orphans every secret already
// stored under it, exactly the way changing envelopePrefix does. cipher_test.go freezes them.
type Scope string

const (
	ScopeConnection      Scope = "connection"
	ScopeVariable        Scope = "variable"
	ScopeVariableHistory Scope = "variable-history"
	// ScopeMaskKey is M5's own per-connection correlation key (plan §2.5) — connections.
	// mask_correlation_key, read and written only by repos.MaskKeysRepo.
	ScopeMaskKey Scope = "mask-key"
	// ScopeConnectionURI seals the whole connection URI (P181) — connections.uri, read and written
	// only by repos.SecretsRepo.
	ScopeConnectionURI Scope = "connection-uri"
	// ScopeScriptParam seals a Runs secret param's saved value (P447) — secret_values rows owned by
	// a custom script, read and written only through Store.
	ScopeScriptParam Scope = "script-param"
)

func (s Scope) valid() bool {
	switch s {
	case ScopeConnection, ScopeVariable, ScopeVariableHistory, ScopeMaskKey, ScopeConnectionURI, ScopeScriptParam:
		return true
	}
	return false
}

// aad is the scope qualified by the envelope prefix — "kira:v3:connection", etc. Including the
// prefix costs nothing and buys domain separation across format versions: a v3 ciphertext will not
// authenticate under a future kira:v4: even if both use the same scope string.
func (s Scope) aad() []byte { return []byte(envelopePrefix + string(s)) }
