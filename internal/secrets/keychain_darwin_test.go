//go:build darwin && cgo

package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"

	"github.com/keybase/go-keychain"
)

// TestRealKeychainRoundTrip is the recovered Keychain coverage (permanently lost when the UI suite
// moved to the isolated webkit tier) — one of the few things this migration makes strictly better.
// It exercises the real macOS Keychain under a test-only service/account, never the user's real
// key, and cannot be compiled or run outside darwin (see docs/HISTORY.md — this Linux sandbox
// excludes this file by build tag rather than skipping it at runtime).
func TestRealKeychainRoundTrip(t *testing.T) {
	const (
		testService = "Kira Studio Secrets (test)"
		testAccount = "Kira Studio (test)"
	)

	cleanup := func() {
		item := keychain.NewItem()
		item.SetSecClass(keychain.SecClassGenericPassword)
		item.SetService(testService)
		item.SetAccount(testAccount)
		item.SetSynchronizable(keychain.SynchronizableNo)
		_ = keychain.DeleteItem(item) // best-effort; the item may not exist yet
	}
	t.Cleanup(cleanup)
	cleanup() // in case a previous crashed run left the test item behind

	key1, err := loadOrCreateKeyIn(testService, testAccount)
	if err != nil {
		t.Fatalf("loadOrCreateKeyIn (create): %v", err)
	}
	if len(key1) != keyBytes {
		t.Fatalf("len(key1) = %d, want %d", len(key1), keyBytes)
	}

	key2, err := loadOrCreateKeyIn(testService, testAccount)
	if err != nil {
		t.Fatalf("loadOrCreateKeyIn (load): %v", err)
	}
	if !bytes.Equal(key1, key2) {
		t.Errorf("second call returned a different key — create-then-load did not round trip")
	}

	block, err := aes.NewCipher(key1)
	if err != nil {
		t.Fatalf("build cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("build GCM: %v", err)
	}
	c := &Cipher{status: Status{Available: true, Backend: BackendKeychain}, aead: aead}
	enc, err := c.Encrypt(ScopeConnection, "hunter2")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := c.Decrypt(ScopeConnection, enc)
	if err != nil || got != "hunter2" {
		t.Errorf("round trip via the real Keychain key = (%q, %v), want (hunter2, nil)", got, err)
	}

	// §1.1 gotcha 5: log what the OS actually reports for this item's attributes, since the
	// library exposes no kSecUseDataProtectionKeychain and the legacy keychain's enforcement of
	// kSecAttrAccessible for a non-synchronizable item cannot be proven by reading source alone.
	attrQuery := keychain.NewItem()
	attrQuery.SetSecClass(keychain.SecClassGenericPassword)
	attrQuery.SetService(testService)
	attrQuery.SetAccount(testAccount)
	attrQuery.SetSynchronizable(keychain.SynchronizableNo)
	attrQuery.SetMatchLimit(keychain.MatchLimitOne)
	attrQuery.SetReturnAttributes(true)
	results, err := keychain.QueryItem(attrQuery)
	if err != nil {
		t.Fatalf("query attributes: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("query attributes: got %d results, want 1", len(results))
	}
	t.Logf("real Keychain item attributes: service=%q account=%q label=%q",
		results[0].Service, results[0].Account, results[0].Label)

	if err := loadOrCreateKeyDelete(testService, testAccount); err != nil {
		t.Fatalf("delete before regenerate check: %v", err)
	}
	key3, err := loadOrCreateKeyIn(testService, testAccount)
	if err != nil {
		t.Fatalf("loadOrCreateKeyIn (recreate): %v", err)
	}
	if bytes.Equal(key1, key3) {
		t.Errorf("recreated key equals the deleted one — expected a fresh random key")
	}
}

func loadOrCreateKeyDelete(service, account string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetSynchronizable(keychain.SynchronizableNo)
	return keychain.DeleteItem(item)
}
