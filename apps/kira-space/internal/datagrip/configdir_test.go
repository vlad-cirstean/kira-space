package datagrip

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

func writeSecurityXML(t *testing.T, configDir, provider, keepassDb string) {
	t.Helper()
	options := `<option name="PROVIDER" value="` + provider + `" />`
	if keepassDb != "" {
		options += `<option name="keepassDb" value="` + keepassDb + `" />`
	}
	content := `<application><component name="PasswordSafe">` + options + `</component></application>`
	mustMkdir(t, filepath.Join(configDir, "options"))
	if err := os.WriteFile(filepath.Join(configDir, "options", "security.xml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write security.xml: %v", err)
	}
}

// TestReadSecurityXMLProviderMemoryOnly is case 20: PROVIDER=MEMORY_ONLY maps to
// password-not-saved (via LookupOrder), and the credential store is never opened for it.
func TestReadSecurityXMLProviderMemoryOnly(t *testing.T) {
	dir := t.TempDir()
	writeSecurityXML(t, dir, "MEMORY_ONLY", "")
	cfg := ReadSecurityXML(dir)
	if cfg.Provider != "MEMORY_ONLY" {
		t.Fatalf("Provider = %q, want MEMORY_ONLY", cfg.Provider)
	}
	if order := LookupOrder(cfg.Provider); order != nil {
		t.Errorf("LookupOrder(MEMORY_ONLY) = %v, want nil (no backend tried)", order)
	}
}

// TestReadSecurityXMLMissingOrUnparseable is D5.4: a missing/garbage security.xml means "try the
// keychain", never an error.
func TestReadSecurityXMLMissingOrUnparseable(t *testing.T) {
	dir := t.TempDir()
	cfg := ReadSecurityXML(dir)
	if cfg.Provider != "" {
		t.Errorf("Provider = %q, want empty for a missing file", cfg.Provider)
	}
	if order := LookupOrder(cfg.Provider); len(order) != 1 || order[0] != "keychain" {
		t.Errorf("LookupOrder(\"\") = %v, want [keychain]", order)
	}

	mustMkdir(t, filepath.Join(dir, "options"))
	if err := os.WriteFile(filepath.Join(dir, "options", "security.xml"), []byte("not xml <<<"), 0o644); err != nil {
		t.Fatalf("write garbage security.xml: %v", err)
	}
	cfg2 := ReadSecurityXML(dir)
	if cfg2.Provider != "" {
		t.Errorf("Provider = %q, want empty for an unparseable file", cfg2.Provider)
	}
}

// TestDiscoverConfigDirsRanking is case 21: given DataGrip2025.3, DataGrip2026.1 and
// IntelliJIdea2026.1 candidates, an IU created-in hint ranks IntelliJIdea2026.1 first; a DB hint
// ranks DataGrip2026.1 then DataGrip2025.3.
func TestDiscoverConfigDirsRanking(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "DataGrip2025.3", "c.kdbx"))
	mustTouch(t, filepath.Join(root, "DataGrip2026.1", "c.kdbx"))
	mustTouch(t, filepath.Join(root, "IntelliJIdea2026.1", "c.kdbx"))
	// An unrelated directory with neither marker file must never be picked up.
	mustMkdir(t, filepath.Join(root, "SomeOtherApp"))

	iu, err := DiscoverConfigDirs(root, "IU-261.1234.56")
	if err != nil {
		t.Fatalf("DiscoverConfigDirs: %v", err)
	}
	if len(iu) == 0 || iu[0].Product != "IntelliJIdea2026.1" {
		t.Fatalf("with an IU hint, got %v, want IntelliJIdea2026.1 first", productsOf(iu))
	}

	db, err := DiscoverConfigDirs(root, "DB-261.1234.56")
	if err != nil {
		t.Fatalf("DiscoverConfigDirs: %v", err)
	}
	if len(db) < 2 || db[0].Product != "DataGrip2026.1" || db[1].Product != "DataGrip2025.3" {
		t.Fatalf("with a DB hint, got %v, want [DataGrip2026.1 DataGrip2025.3 ...]", productsOf(db))
	}
}

func productsOf(cs []ConfigCandidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Product
	}
	return out
}

// TestOutlookForKeepassConfiguredIsStoreUnsupported is the regression case: a
// KEEPASS-configured provider must classify as OutlookStoreUnsupported, not OutlookNotSaved —
// DataGrip did save a password here, this app just can't read that store any more now that the
// file-based fallback is gone.
func TestOutlookForKeepassConfiguredIsStoreUnsupported(t *testing.T) {
	ds := DataSource{UUID: "eeeeeeee-0000-0000-0000-000000000001", HasLocal: true, SecretStorage: "master_key"}
	fields := ResolvedFields{Kind: "postgres"}
	if got := outlookFor(ds, fields, SecurityConfig{Provider: "KEEPASS"}); got != OutlookStoreUnsupported {
		t.Errorf("outlookFor(KEEPASS) = %q, want %q", got, OutlookStoreUnsupported)
	}
}

// TestKeychainOtherStubRefusesByName is case 23: keychain_other.go's stub (this is Linux, where
// CI runs — darwin&&cgo is not the active build) returns credential-store-unsupported naming the
// platform, never a silent "not found".
func TestKeychainOtherStubRefusesByName(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("this asserts the non-darwin stub; darwin builds the real keychain_darwin.go instead")
	}
	_, _, err := readKeychainPassword("11111111-0000-0000-0000-000000000001")
	if err == nil {
		t.Fatalf("expected an error")
	}
	re, ok := err.(*RefusalError)
	if !ok {
		t.Fatalf("error is not a *RefusalError: %v", err)
	}
	if re.Code != ReasonCredentialStoreUnsupported {
		t.Errorf("Code = %q, want %q", re.Code, ReasonCredentialStoreUnsupported)
	}
	if !strings.Contains(re.Message, runtime.GOOS) {
		t.Errorf("Message = %q, want it to name the platform %q", re.Message, runtime.GOOS)
	}
	if keychainSupported {
		t.Errorf("keychainSupported = true on %s, want false (non-darwin/no-cgo build)", runtime.GOOS)
	}
}
