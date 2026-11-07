//go:build darwin && cgo

package secrets

import (
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/keybase/go-keychain"
)

// One-time move of the v0.3.1 Keychain item ("Kira Studio") to the Kira Space names. The item holds the
// key that seals every stored secret, so the move carries the key over; a plain rename would mint a
// new key and orphan every sealed value. The next chapter deletes this file, its test and the call in
// loadOrCreateKey.
const (
	legacyKeychainService = "Kira Studio Secrets"
	legacyKeychainAccount = "Kira Studio"
)

// moveLegacyKey moves the v0.3.1 item to the current names.
func moveLegacyKey() error {
	return moveKeyIn(legacyKeychainService, legacyKeychainAccount, keychainService, keychainAccount)
}

// moveKeyIn carries the key of the old item to the new one and deletes the old item once the new one
// reads back equal. An absent or malformed old item is nothing to move. Two different keys is an error
// that deletes nothing. A crash between add and delete leaves two equal items; the next call deletes
// the old one. Errors name the step and the item service, never key bytes.
func moveKeyIn(oldService, oldAccount, newService, newAccount string) error {
	old, ok, err := readKey(oldService, oldAccount)
	if err != nil || !ok || len(old) != keyBytes {
		return err
	}
	cur, ok, err := readKey(newService, newAccount)
	if err != nil {
		return err
	}
	if ok && len(cur) != keyBytes {
		if err := deleteKey(newService, newAccount); err != nil {
			return fmt.Errorf("secrets: delete malformed keychain item %q: %w", newService, err)
		}
		ok = false
	}
	if !ok {
		// A duplicate means another process added it first: compare below like any existing item.
		if err := addKey(newService, newAccount, old); err != nil && !errors.Is(err, keychain.ErrorDuplicateItem) {
			return fmt.Errorf("secrets: add keychain item %q: %w", newService, err)
		}
		if cur, ok, err = readKey(newService, newAccount); err != nil {
			return err
		}
		if !ok || len(cur) != keyBytes {
			return fmt.Errorf("secrets: keychain item %q did not read back after the move; %q kept", newService, oldService)
		}
	}
	if subtle.ConstantTimeCompare(old, cur) != 1 {
		return fmt.Errorf("secrets: keychain items %q and %q hold different keys; neither was changed", oldService, newService)
	}
	if err := deleteKey(oldService, oldAccount); err != nil {
		return fmt.Errorf("secrets: delete old keychain item %q: %w", oldService, err)
	}
	return nil
}

// readKey returns the item's data; ok is false when the item does not exist.
func readKey(service, account string) (data []byte, ok bool, err error) {
	query := keychain.NewItem()
	query.SetSecClass(keychain.SecClassGenericPassword)
	query.SetService(service)
	query.SetAccount(account)
	query.SetSynchronizable(keychain.SynchronizableNo)
	query.SetMatchLimit(keychain.MatchLimitOne)
	query.SetReturnData(true)
	results, err := keychain.QueryItem(query)
	if err != nil {
		return nil, false, fmt.Errorf("secrets: query keychain item %q: %w", service, err)
	}
	if len(results) != 1 {
		return nil, false, nil
	}
	return results[0].Data, true, nil
}

func addKey(service, account string, key []byte) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetLabel(keychainLabel)
	item.SetData(key)
	item.SetSynchronizable(keychain.SynchronizableNo)
	item.SetAccessible(keychain.AccessibleWhenUnlockedThisDeviceOnly)
	return keychain.AddItem(item)
}

func deleteKey(service, account string) error {
	item := keychain.NewItem()
	item.SetSecClass(keychain.SecClassGenericPassword)
	item.SetService(service)
	item.SetAccount(account)
	item.SetSynchronizable(keychain.SynchronizableNo)
	return keychain.DeleteItem(item)
}
