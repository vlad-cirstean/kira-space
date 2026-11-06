package mcpauth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMintVerifyRoundTrip(t *testing.T) {
	plain, rec, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if !Verify(plain, rec) {
		t.Fatal("Verify(plain, rec) = false, want true")
	}
}

func TestVerifyRejectsWrongToken(t *testing.T) {
	_, rec, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	other, _, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if Verify(other, rec) {
		t.Fatal("Verify(other-plaintext, rec) = true, want false")
	}
	if Verify("not-even-base64!!", rec) {
		t.Fatal("Verify(garbage, rec) = true, want false")
	}
	if Verify("", rec) {
		t.Fatal("Verify(\"\", rec) = true, want false")
	}
}

func TestLoadOrMintTTLMintsOnceThenLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")

	plain1, rec1, minted1, err := LoadOrMintTTL(path, TTL)
	if err != nil {
		t.Fatalf("LoadOrMintTTL (first): %v", err)
	}
	if !minted1 || plain1 == "" {
		t.Fatalf("first LoadOrMintTTL: minted=%v plain=%q, want minted=true and a plaintext", minted1, plain1)
	}

	plain2, rec2, minted2, err := LoadOrMintTTL(path, TTL)
	if err != nil {
		t.Fatalf("LoadOrMintTTL (second): %v", err)
	}
	if minted2 || plain2 != "" {
		t.Fatalf("second LoadOrMintTTL: minted=%v plain=%q, want minted=false and no plaintext", minted2, plain2)
	}
	if !bytes.Equal(rec1.Hash, rec2.Hash) || !bytes.Equal(rec1.Salt, rec2.Salt) {
		t.Fatal("second LoadOrMintTTL returned a different record than the first mint persisted")
	}
	if !Verify(plain1, rec2) {
		t.Fatal("the originally-minted plaintext no longer verifies against the loaded record")
	}
}

// TestSaveIsAtomicAndLeavesNoTempFile pins Save's temp-file-plus-rename shape (M7 finding): the
// existing record at path must never be observable as a truncated/partial file, and a successful
// Save must not leave its own scratch file behind for a later Load to trip over.
func TestSaveIsAtomicAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")

	_, rec1, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if err := Save(path, rec1); err != nil {
		t.Fatalf("Save (first): %v", err)
	}

	_, rec2, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if err := Save(path, rec2); err != nil {
		t.Fatalf("Save (second, overwrite): %v", err)
	}

	loaded, ok, err := Load(path)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if !ok {
		t.Fatal("Load after Save: ok = false")
	}
	if !bytes.Equal(loaded.Hash, rec2.Hash) {
		t.Fatal("Load after the second Save returned the first record, not the overwrite")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Fatalf("Save left a stray file behind: %s", e.Name())
		}
	}
}

// TestExpiredBoundaryIsInclusive: exactly-at-ExpiresAt must read as expired (M1 §8 — "exactly-at
// must read as expired"), not one tick shy of it.
func TestExpiredBoundaryIsInclusive(t *testing.T) {
	now := time.Now()
	rec := Record{ExpiresAt: now}
	if !Expired(rec, now) {
		t.Fatal("Expired(rec, rec.ExpiresAt) = false, want true — the boundary instant itself must count as expired")
	}
	if Expired(rec, now.Add(-time.Nanosecond)) {
		t.Fatal("Expired(rec, one tick before ExpiresAt) = true, want false")
	}
	if !Expired(rec, now.Add(time.Nanosecond)) {
		t.Fatal("Expired(rec, one tick after ExpiresAt) = false, want true")
	}
}

// TestZeroExpiresAtNeverExpires: a pre-M1 record (never stamped) must never read as expired.
func TestZeroExpiresAtNeverExpires(t *testing.T) {
	if Expired(Record{}, time.Now().Add(100*365*24*time.Hour)) {
		t.Fatal("Expired(zero-ExpiresAt record, far future) = true, want false")
	}
}

// TestLoadOrMintTTLStampsZeroExpiryOnceNotRepeatedly covers the one-shot side effect:
// a zero-ExpiresAt record gets stamped with now+ttl on its first load, reported as a load rather
// than a mint, and the stamped ExpiresAt must not move on a later load — otherwise
// the clock resets on every restart and rotation never bites.
func TestLoadOrMintTTLStampsZeroExpiryOnceNotRepeatedly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	plain, rec, err := MintTTL(0) // ExpiresAt = now+0 = now, but we overwrite it below to simulate a pre-M1 file
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	rec.ExpiresAt = time.Time{} // simulate a pre-M1 record with no ExpiresAt recorded yet
	if err := Save(path, rec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stampedPlain, stamped, minted, err := LoadOrMintTTL(path, TTL)
	if err != nil {
		t.Fatalf("LoadOrMintTTL (stamp): %v", err)
	}
	if minted || stampedPlain != "" {
		t.Fatalf("stamping load: minted=%v plain=%q, want minted=false and no plaintext — a stamp is a load, not a mint", minted, stampedPlain)
	}
	if stamped.ExpiresAt.IsZero() {
		t.Fatal("LoadOrMintTTL did not stamp a zero ExpiresAt")
	}
	if !Verify(plain, stamped) {
		t.Fatal("stamping changed the record's own Hash/Salt — it must only add ExpiresAt")
	}

	firstStamp := stamped.ExpiresAt
	time.Sleep(2 * time.Millisecond)
	_, second, minted2, err := LoadOrMintTTL(path, TTL)
	if err != nil {
		t.Fatalf("LoadOrMintTTL (second load): %v", err)
	}
	if minted2 {
		t.Fatal("second LoadOrMintTTL minted, want a plain load of the already-stamped record")
	}
	if !second.ExpiresAt.Equal(firstStamp) {
		t.Fatalf("ExpiresAt moved on a second load: %v -> %v, want stable (stamping is one-shot)", firstStamp, second.ExpiresAt)
	}
}

// TestLoadOrMintTTLReMintsOnLapsedRecord: a record that has genuinely lapsed (non-zero, past
// ExpiresAt) must be reminted on load, not silently reused past its own expiry (M1 §2.3's "mints
// when the file is missing or the stored record has lapsed").
func TestLoadOrMintTTLReMintsOnLapsedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	oldPlain, oldRec, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	oldRec.ExpiresAt = time.Now().Add(-time.Hour) // force it lapsed
	if err := Save(path, oldRec); err != nil {
		t.Fatalf("Save: %v", err)
	}

	newPlain, newRec, minted, err := LoadOrMintTTL(path, TTL)
	if err != nil {
		t.Fatalf("LoadOrMintTTL: %v", err)
	}
	if !minted || newPlain == "" {
		t.Fatalf("LoadOrMintTTL on a lapsed record: minted=%v plain=%q, want minted=true and a fresh plaintext", minted, newPlain)
	}
	if Verify(oldPlain, newRec) {
		t.Fatal("the old, lapsed plaintext still verifies against the reminted record")
	}
	if Expired(newRec, time.Now()) {
		t.Fatal("the freshly reminted record already reads as expired")
	}
}

// TestCheckThreeWayOutcomes: exactly the three cases §2.5/§8 call for, including the one that must
// not leak — a wrong token against a lapsed record must read as OutcomeInvalid, not
// OutcomeExpired, so the expiry instant is never disclosed to a caller who doesn't hold the token.
func TestCheckThreeWayOutcomes(t *testing.T) {
	now := time.Now()
	plain, rec, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}

	if got := Check(plain, rec, now); got != OutcomeValid {
		t.Fatalf("Check(valid token, live record) = %v, want OutcomeValid", got)
	}

	lapsed := rec
	lapsed.ExpiresAt = now.Add(-time.Second)
	if got := Check(plain, lapsed, now); got != OutcomeExpired {
		t.Fatalf("Check(valid token, lapsed record) = %v, want OutcomeExpired", got)
	}

	wrongPlain, _, err := MintTTL(TTL)
	if err != nil {
		t.Fatalf("MintTTL: %v", err)
	}
	if got := Check(wrongPlain, rec, now); got != OutcomeInvalid {
		t.Fatalf("Check(wrong token, live record) = %v, want OutcomeInvalid", got)
	}
	if got := Check(wrongPlain, lapsed, now); got != OutcomeInvalid {
		t.Fatalf("Check(wrong token, lapsed record) = %v, want OutcomeInvalid — a wrong token must never learn the record has expired", got)
	}
}
