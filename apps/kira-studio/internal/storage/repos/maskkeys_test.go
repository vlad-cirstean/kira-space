package repos_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// TestEnsureKeyConcurrentFirstMintConverges: two callers resolving a connection's key for the
// first time at once (the MCP render path's MaskSetFor and the grid preview's
// CorrelationKeyHex, per maskrules.Service) must never both persist — the loser overwriting the
// winner's already-in-use key would leave whichever caller cached the winner's key tagging with
// bytes the database no longer agrees are the key. Every concurrent caller must converge on the
// exact same key, and it must be the one stored.
func TestEnsureKeyConcurrentFirstMintConverges(t *testing.T) {
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	connID := uuid.NewString()
	seedConnection(t, db, connID)
	keys := repos.NewMaskKeys(db, secrets.New())

	const n = 8
	results := make([][]byte, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = keys.EnsureKey(connID)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureKey[%d]: %v", i, err)
		}
	}
	for i, k := range results {
		if len(k) != 32 {
			t.Fatalf("EnsureKey[%d] returned a %d-byte key, want 32", i, len(k))
		}
		if !bytes.Equal(k, results[0]) {
			t.Fatalf("EnsureKey[%d] returned a different key than EnsureKey[0] — every concurrent first-mint caller must converge on one key", i)
		}
	}

	stored, err := keys.Get(connID)
	if err != nil {
		t.Fatalf("Get after concurrent EnsureKey: %v", err)
	}
	if !bytes.Equal(stored, results[0]) {
		t.Fatal("the persisted key does not match what every EnsureKey caller was handed back")
	}
}

// TestEnsureKeyNoSuchConnectionReturnsNilNotAnUnpersistedKey pins the fail-closed direction of the
// RowsAffected check: EnsureKey against an id with no connections row must never hand back a key
// it generated but could not actually store.
func TestEnsureKeyNoSuchConnectionReturnsNilNotAnUnpersistedKey(t *testing.T) {
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	keys := repos.NewMaskKeys(db, secrets.New())

	key, err := keys.EnsureKey(uuid.NewString())
	if err != nil {
		t.Fatalf("EnsureKey(missing connection): %v", err)
	}
	if key != nil {
		t.Fatalf("EnsureKey(missing connection) = %x, want nil (never hand back an unpersisted key)", key)
	}
}

// TestRegenerateNoSuchConnectionErrors pins setKey's own RowsAffected guard: Regenerate against a
// missing connection must report failure rather than silently no-op'ing (M7 finding).
func TestRegenerateNoSuchConnectionErrors(t *testing.T) {
	t.Setenv("KIRA_INSECURE_SECRETS", "1")
	db := newRepos(t).DB
	keys := repos.NewMaskKeys(db, secrets.New())

	if err := keys.Regenerate(uuid.NewString()); err == nil {
		t.Fatal("Regenerate(missing connection) = nil error, want an error")
	}
}
