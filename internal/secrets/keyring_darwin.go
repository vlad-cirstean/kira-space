//go:build darwin && cgo

package secrets

import (
	"crypto/rand"
	"fmt"

	"github.com/keybase/go-keychain"
)

// This is the only file in the repo importing github.com/keybase/go-keychain (P52 §6.3's chosen
// library, confirmed against its real source for P55 — see docs/HISTORY.md (v1)
// P55-go-application-services.md §1.1 for the six gotchas handled below).
const (
	// "Safe Storage" is Chromium/Electron's own naming convention for this kind of Keychain item
	// (what safeStorage itself would have called it) — this app has no Electron/Chromium in it
	// (P57), so naming this item after a mechanism it doesn't use would be misleading.
	keychainService = "Kira Studio Secrets"
	keychainAccount = "Kira Studio"
	keychainLabel   = "Kira Studio Secrets"
	keyBytes        = 32
)

// loadOrCreateKey returns the app's single AES-256-GCM key, creating it on first run. This round
// trip is also the darwin availability probe (P55 §2 D2): a canary item would be strictly weaker
// and would leave litter in the user's Keychain.
func loadOrCreateKey() ([]byte, error) {
	return loadOrCreateKeyIn(keychainService, keychainAccount)
}

// loadOrCreateKeyIn takes the item's identity as parameters so keychain_darwin_test.go can
// exercise the real Keychain under a test-only service/account without ever touching the user's
// real key.
func loadOrCreateKeyIn(service, account string) ([]byte, error) {
	// Gotcha 1: GetGenericPassword's query does not set kSecAttrSynchronizable, and our item is
	// added with SynchronizableNo — so the query must build the Item by hand and set it too, or
	// matching depends on the OS's default query-time synchronizable semantics.
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetSynchronizable(keychain.SynchronizableNo)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)

	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, fmt.Errorf("secrets: query keychain item: %w", err)
	}
	// Gotcha 2: not-found is (nil, nil), not an error.
	if len(results) == 1 && len(results[0].Data) == keyBytes {
		return results[0].Data, nil
	}
	if len(results) == 1 {
		// A foreign or corrupt item of the wrong length: delete it and fall through to create,
		// rather than failing forever.
		staleQuery := keychain.NewItem()
		staleQuery.SetSecClass(keychain.SecClassGenericPassword)
		staleQuery.SetService(service)
		staleQuery.SetAccount(account)
		staleQuery.SetSynchronizable(keychain.SynchronizableNo)
		if err := keychain.DeleteItem(staleQuery); err != nil {
			return nil, fmt.Errorf("secrets: delete malformed keychain item: %w", err)
		}
	}

	key := make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("secrets: generate key: %w", err)
	}

	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetLabel(keychainLabel)
	item.SetData(key)
	// The two attributes P52 §6.2 requires: the key must never be restorable from a backup onto
	// another machine, and must never reach iCloud Keychain.
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlockedThisDeviceOnly)

	if err := keychain.AddItem(item); err != nil {
		// Gotcha 3: AddItem does not upsert — a concurrent creator (or a race with the delete
		// above) surfaces as ErrorDuplicateItem, not a failure to add.
		if err == keychain.ErrorDuplicateItem {
			results, reErr := keychain.QueryItem(query)
			if reErr != nil {
				return nil, fmt.Errorf("secrets: re-query after duplicate item: %w", reErr)
			}
			// The main path above (line 52) applies this same keyBytes check —
			// without it here too, a wrong-length item (foreign, corrupt, or from an old app
			// version) reaches secrets.New()'s aes.NewCipher(key) and panics the app at startup
			// instead of reporting secret storage as unavailable, the way every other malformed-key
			// path in this file already does.
			if len(results) == 1 && len(results[0].Data) == keyBytes {
				return results[0].Data, nil
			}
			if len(results) == 1 {
				return nil, fmt.Errorf("secrets: duplicate item has an unexpected length (%d bytes, want %d)", len(results[0].Data), keyBytes)
			}
			return nil, fmt.Errorf("secrets: duplicate item reported but not found on re-query")
		}
		return nil, fmt.Errorf("secrets: add keychain item: %w", err)
	}
	return key, nil
}
