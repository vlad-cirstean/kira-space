package bridge

import (
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/mcpauth"
)

// TestDbMcpTokenProviderForRemintsOnMissingHelperFile guards: a record that loads fine but has
// no helper token mirror on disk at all (never written) must not be
// served as-is — no client could ever present a plaintext that verifies against it. The provider
// must remint fresh, both files together.
func TestDbMcpTokenProviderForRemintsOnMissingHelperFile(t *testing.T) {
	home := t.TempDir()
	path := mcpauth.PathNamed(home, dbMcpTokenName)
	_, rec, err := mcpauth.MintTTL(mcpauth.TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if err := mcpauth.Save(path, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Deliberately no SaveHelperToken call — the helper mirror never existed.

	got, plain, minted, err := dbMcpTokenProviderFor(home, false)()
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if !minted {
		t.Fatal("minted = false, want true — a record with no matching helper file must be reminted")
	}
	if plain == "" {
		t.Fatal("plain is empty, want the freshly minted plaintext")
	}
	if got.Hash == nil {
		t.Fatal("returned record has no hash")
	}
	if !helperTokenMatchesRecord(mcpauth.HelperTokenPathNamed(home, dbMcpTokenName), got) {
		t.Fatal("the freshly written helper file does not verify against the freshly written record")
	}
}

// TestDbMcpTokenProviderForRemintsOnMismatchedHelperFile guards the other half: a helper file
// that exists but holds a plaintext that does NOT verify against the loaded record (Save
// succeeded, SaveHelperToken failed or wrote something else on a previous run) is just as broken
// as a missing one — every client would 401 forever with nothing here able to recover the real
// plaintext from a hash.
func TestDbMcpTokenProviderForRemintsOnMismatchedHelperFile(t *testing.T) {
	home := t.TempDir()
	path := mcpauth.PathNamed(home, dbMcpTokenName)
	helperPath := mcpauth.HelperTokenPathNamed(home, dbMcpTokenName)
	_, rec, err := mcpauth.MintTTL(mcpauth.TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if err := mcpauth.Save(path, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mcpauth.SaveHelperToken(helperPath, "stale-plaintext-from-a-different-mint"); err != nil {
		t.Fatalf("SaveHelperToken: %v", err)
	}

	got, plain, minted, err := dbMcpTokenProviderFor(home, false)()
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if !minted {
		t.Fatal("minted = false, want true — a mismatched helper file must be reminted")
	}
	if plain == "" {
		t.Fatal("plain is empty, want the freshly minted plaintext")
	}
	if !helperTokenMatchesRecord(helperPath, got) {
		t.Fatal("the rewritten helper file does not verify against the rewritten record")
	}
}

// TestDbMcpTokenProviderForKeepsMatchingHelperFile is the control: a record and a helper file that
// already agree must be served as-is, not reminted on every ordinary restart.
func TestDbMcpTokenProviderForKeepsMatchingHelperFile(t *testing.T) {
	home := t.TempDir()
	path := mcpauth.PathNamed(home, dbMcpTokenName)
	helperPath := mcpauth.HelperTokenPathNamed(home, dbMcpTokenName)
	plainWant, rec, err := mcpauth.MintTTL(mcpauth.TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if err := mcpauth.Save(path, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := mcpauth.SaveHelperToken(helperPath, plainWant); err != nil {
		t.Fatalf("SaveHelperToken: %v", err)
	}

	got, plain, minted, err := dbMcpTokenProviderFor(home, false)()
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if minted {
		t.Fatal("minted = true, want false — a matching helper file must not be reminted")
	}
	if plain != "" {
		t.Fatalf("plain = %q, want empty on a load (LoadOrMintTTL's own contract)", plain)
	}
	if !helperTokenMatchesRecord(helperPath, got) {
		t.Fatal("the untouched helper file no longer verifies against the loaded record")
	}
}
