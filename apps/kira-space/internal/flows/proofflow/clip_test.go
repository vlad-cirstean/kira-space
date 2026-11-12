package proofflow

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func pngB64(extra string) string {
	return base64.StdEncoding.EncodeToString(clipPNG(extra))
}

func clipPNG(extra string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), extra...)
}

// clipDirs lists the batch dirs under the test's private TMPDIR.
func clipDirs(t *testing.T, tmp string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "kira-proof-clip-") {
			out[filepath.Join(tmp, e.Name())] = true
		}
	}
	return out
}

func TestProofShotCopyAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	f := newFixture(t)
	p := f.app.W.Proof
	clip := f.app.ImageClipboard

	got, err := p.ShotClipboard()
	if err != nil || got.MultiImage {
		t.Fatalf("default capability = %+v, %v; want off", got, err)
	}
	if _, err := p.OpenShotClip(); err == nil {
		t.Fatal("OpenShotClip succeeded without a multi-image clipboard")
	}

	clip.SetMulti(true)
	got, err = p.ShotClipboard()
	if err != nil || !got.MultiImage {
		t.Fatalf("capability = %+v, %v; want on", got, err)
	}
	f.app.Contract(t, "proof", "ProofService.ShotClipboard", got)

	first, err := p.OpenShotClip()
	if err != nil || first.ID == "" {
		t.Fatalf("OpenShotClip = %+v, %v", first, err)
	}
	f.app.Contract(t, "proof", "ProofService.OpenShotClip", first, flowharness.Mask("id"))
	dirs := clipDirs(t, tmp)
	if len(dirs) != 1 {
		t.Fatalf("batch dirs = %v, want 1", dirs)
	}
	var firstDir string
	for d := range dirs {
		firstDir = d
	}

	names := []string{"01-a.png", "02-b.png", "03-c.png"}
	for i, name := range names {
		if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: first.ID, Filename: name, PngBase64: pngB64(string(rune('a' + i)))}); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}
	refused := []bridge.ProofAddShotClipArgs{
		{ClipID: first.ID, Filename: "n.png", PngBase64: jpegB64("n")},
		{ClipID: "shot_unknown", Filename: "n.png", PngBase64: pngB64("n")},
		{ClipID: first.ID, Filename: "01-a.png", PngBase64: pngB64("again")},
		{ClipID: first.ID, Filename: "big.png", PngBase64: strings.Repeat("A", 90<<20)},
	}
	for _, a := range refused {
		if err := p.AddShotClip(a); err == nil {
			t.Fatalf("AddShotClip(%s, %s) succeeded, want refusal", a.ClipID, a.Filename)
		}
	}

	res, err := p.CommitShotClip(bridge.ProofShotClipArgs{ClipID: first.ID})
	if err != nil || res.Count != 3 {
		t.Fatalf("commit = %+v, %v", res, err)
	}
	f.app.Contract(t, "proof", "ProofService.CommitShotClip", res)
	writes := clip.Writes()
	if len(writes) != 1 || len(writes[0]) != 3 {
		t.Fatalf("clipboard writes = %d batches, want 1 of 3", len(writes))
	}
	for i, img := range writes[0] {
		if img.Name != names[i] || !bytes.Equal(img.Data, clipPNG(string(rune('a'+i)))) {
			t.Fatalf("clipboard item %d = %q %q", i, img.Name, img.Data)
		}
	}
	if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: first.ID, Filename: "late.png", PngBase64: pngB64("l")}); err == nil {
		t.Fatal("add after commit succeeded")
	}
	if err := p.CloseShotClip(bridge.ProofShotClipArgs{ClipID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstDir); err != nil {
		t.Fatalf("close after commit removed the batch: %v", err)
	}

	// A later commit removes the earlier batch only after its own write.
	second, err := p.OpenShotClip()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: second.ID, Filename: "01-x.png", PngBase64: pngB64("x")}); err != nil {
		t.Fatal(err)
	}
	if res, err := p.CommitShotClip(bridge.ProofShotClipArgs{ClipID: second.ID}); err != nil || res.Count != 1 {
		t.Fatalf("second commit = %+v, %v", res, err)
	}
	if _, err := os.Stat(firstDir); !os.IsNotExist(err) {
		t.Fatalf("first batch kept after the second commit: %v", err)
	}
	secondDir := onlyDir(t, clipDirs(t, tmp))
	if got := len(clip.Writes()); got != 2 {
		t.Fatalf("clipboard writes = %d, want 2", got)
	}

	// An abandoned batch leaves nothing and writes nothing.
	third, err := p.OpenShotClip()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: third.ID, Filename: "01-y.png", PngBase64: pngB64("y")}); err != nil {
		t.Fatal(err)
	}
	if err := p.CloseShotClip(bridge.ProofShotClipArgs{ClipID: third.ID}); err != nil {
		t.Fatal(err)
	}
	if dirs := clipDirs(t, tmp); len(dirs) != 1 || !dirs[secondDir] {
		t.Fatalf("batch dirs after abandon = %v, want only %s", dirs, secondDir)
	}
	if got := len(clip.Writes()); got != 2 {
		t.Fatalf("abandoned batch wrote: %d batches", got)
	}

	// An empty batch cannot commit; a failed write keeps the previous batch on disk.
	empty, err := p.OpenShotClip()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.CommitShotClip(bridge.ProofShotClipArgs{ClipID: empty.ID}); err == nil {
		t.Fatal("empty commit succeeded")
	}
	if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: empty.ID, Filename: "01-z.png", PngBase64: pngB64("z")}); err != nil {
		t.Fatal(err)
	}
	clip.SetFail(errors.New("pasteboard refused"))
	if _, err := p.CommitShotClip(bridge.ProofShotClipArgs{ClipID: empty.ID}); err == nil {
		t.Fatal("commit succeeded while the clipboard failed")
	}
	if _, err := os.Stat(secondDir); err != nil {
		t.Fatalf("failed commit removed the previous batch: %v", err)
	}
	clip.SetFail(nil)
}

func onlyDir(t *testing.T, dirs map[string]bool) string {
	t.Helper()
	if len(dirs) != 1 {
		t.Fatalf("batch dirs = %v, want 1", dirs)
	}
	for d := range dirs {
		return d
	}
	return ""
}

// Eviction past the batch cap removes an abandoned batch; quitting removes the committed one and any
// open one. Screenshots must not outlive the app in TMPDIR.
func TestProofShotClipLeavesNoFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	f := newFixture(t)
	p := f.app.W.Proof
	f.app.ImageClipboard.SetMulti(true)

	// 9 batches opened and never closed: the first falls out of the 8-batch registry.
	var firstDir string
	for i := 0; i < 9; i++ {
		if _, err := p.OpenShotClip(); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstDir = onlyDir(t, clipDirs(t, tmp))
		}
	}
	if _, err := os.Stat(firstDir); !os.IsNotExist(err) {
		t.Fatalf("evicted batch kept: %v", err)
	}
	if got := len(clipDirs(t, tmp)); got != 8 {
		t.Fatalf("batch dirs = %d, want 8", got)
	}
	committed, err := p.OpenShotClip()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AddShotClip(bridge.ProofAddShotClipArgs{ClipID: committed.ID, Filename: "01-a.png", PngBase64: pngB64("a")}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CommitShotClip(bridge.ProofShotClipArgs{ClipID: committed.ID}); err != nil {
		t.Fatal(err)
	}
	f.app.Quit(t)
	if dirs := clipDirs(t, tmp); len(dirs) != 0 {
		t.Fatalf("batch dirs after quit = %v, want none", dirs)
	}
}
