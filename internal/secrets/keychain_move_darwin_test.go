//go:build darwin && cgo

package secrets

import (
	"bytes"
	"testing"
)

// Real Keychain, test-only item names; the user's own items are never touched.
const (
	moveOldService = "Kira Space Secrets (test old)"
	moveOldAccount = "Kira Space (test old)"
	moveNewService = "Kira Space Secrets (test new)"
	moveNewAccount = "Kira Space (test new)"
)

func resetMoveItems(t *testing.T) {
	t.Helper()
	clean := func() {
		_ = deleteKey(moveOldService, moveOldAccount)
		_ = deleteKey(moveNewService, moveNewAccount)
	}
	clean()
	t.Cleanup(clean)
}

func put(t *testing.T, service, account string, key []byte) {
	t.Helper()
	if err := addKey(service, account, key); err != nil {
		t.Fatalf("seed keychain item: %v", err)
	}
}

func holds(t *testing.T, service, account string) ([]byte, bool) {
	t.Helper()
	data, ok, err := readKey(service, account)
	if err != nil {
		t.Fatal(err)
	}
	return data, ok
}

func TestKeychainMoveCarriesTheKey(t *testing.T) {
	resetMoveItems(t)
	key := bytes.Repeat([]byte{7}, keyBytes)
	put(t, moveOldService, moveOldAccount, key)
	if err := moveKeyIn(moveOldService, moveOldAccount, moveNewService, moveNewAccount); err != nil {
		t.Fatal(err)
	}
	if got, ok := holds(t, moveNewService, moveNewAccount); !ok || !bytes.Equal(got, key) {
		t.Fatal("new item does not hold the old key")
	}
	if _, ok := holds(t, moveOldService, moveOldAccount); ok {
		t.Fatal("old item survived the move")
	}
}

func TestKeychainMoveWithEqualItemsDeletesOld(t *testing.T) {
	resetMoveItems(t)
	key := bytes.Repeat([]byte{9}, keyBytes)
	put(t, moveOldService, moveOldAccount, key)
	put(t, moveNewService, moveNewAccount, key)
	if err := moveKeyIn(moveOldService, moveOldAccount, moveNewService, moveNewAccount); err != nil {
		t.Fatal(err)
	}
	if _, ok := holds(t, moveOldService, moveOldAccount); ok {
		t.Fatal("old item survived")
	}
	if got, ok := holds(t, moveNewService, moveNewAccount); !ok || !bytes.Equal(got, key) {
		t.Fatal("new item changed")
	}
}

func TestKeychainMoveWithDifferentKeysChangesNothing(t *testing.T) {
	resetMoveItems(t)
	a, b := bytes.Repeat([]byte{1}, keyBytes), bytes.Repeat([]byte{2}, keyBytes)
	put(t, moveOldService, moveOldAccount, a)
	put(t, moveNewService, moveNewAccount, b)
	if err := moveKeyIn(moveOldService, moveOldAccount, moveNewService, moveNewAccount); err == nil {
		t.Fatal("different keys moved without an error")
	}
	if got, ok := holds(t, moveOldService, moveOldAccount); !ok || !bytes.Equal(got, a) {
		t.Fatal("old item changed")
	}
	if got, ok := holds(t, moveNewService, moveNewAccount); !ok || !bytes.Equal(got, b) {
		t.Fatal("new item changed")
	}
}

func TestKeychainMoveWithNoItemsIsANoOp(t *testing.T) {
	resetMoveItems(t)
	if err := moveKeyIn(moveOldService, moveOldAccount, moveNewService, moveNewAccount); err != nil {
		t.Fatal(err)
	}
	if _, ok := holds(t, moveNewService, moveNewAccount); ok {
		t.Fatal("a new item appeared from nothing")
	}
}
