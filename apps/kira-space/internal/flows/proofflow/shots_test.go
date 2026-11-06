package proofflow

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func pngB64(extra string) string {
	return base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), extra...))
}

func TestProofShotFolder(t *testing.T) {
	f := newFixture(t)
	p := f.app.W.Proof
	tmp := t.TempDir()

	canceled, err := p.ChooseShotFolder(bridge.ProofChooseShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || !canceled.Canceled || canceled.ID != "" {
		t.Fatalf("cancel = %+v, %v", canceled, err)
	}
	if ents, _ := os.ReadDir(tmp); len(ents) != 0 {
		t.Fatalf("cancel created %v", ents)
	}

	f.app.Dialogs.AnswerDirectory(tmp)
	first, err := p.ChooseShotFolder(bridge.ProofChooseShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || first.Canceled || first.Path != filepath.Join(tmp, "Checkout fix") {
		t.Fatalf("first = %+v, %v", first, err)
	}
	f.app.StudioContract(t, "proof", "ProofService.ChooseShotFolder", first,
		flowharness.Mask("id", "path"))
	f.app.Dialogs.AnswerDirectory(tmp)
	second, err := p.ChooseShotFolder(bridge.ProofChooseShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || second.Path != filepath.Join(tmp, "Checkout fix-2") {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if got := f.app.Dialogs.Directory; len(got) != 3 || got[0].Title == "" {
		t.Fatalf("picker requests = %+v", got)
	}

	for i, name := range []string{"01-a.png", "02-b.png", "03-c.png"} {
		body := string(rune('a' + i))
		res, err := p.SavePngTo(bridge.ProofSavePngToArgs{FolderID: first.ID, Filename: name, PngBase64: pngB64(body)})
		if err != nil || res.Path != filepath.Join(first.Path, name) {
			t.Fatalf("save %s = %+v, %v", name, res, err)
		}
		got, err := os.ReadFile(res.Path)
		if err != nil || !bytes.Equal(got, append([]byte("\x89PNG\r\n\x1a\n"), body...)) {
			t.Fatalf("read back %s = %q, %v", name, got, err)
		}
	}
	res, err := p.SavePngTo(bridge.ProofSavePngToArgs{FolderID: first.ID, Filename: "../x.png", PngBase64: pngB64("x")})
	if err != nil || res.Path != filepath.Join(first.Path, "x.png") {
		t.Fatalf("traversal = %+v, %v", res, err)
	}

	refused := []bridge.ProofSavePngToArgs{
		{FolderID: first.ID, Filename: "01-a.png", PngBase64: pngB64("again")},
		{FolderID: first.ID, Filename: "n.png", PngBase64: base64.StdEncoding.EncodeToString([]byte("not a png"))},
		{FolderID: "shot_unknown", Filename: "n.png", PngBase64: pngB64("n")},
	}
	for _, a := range refused {
		if _, err := p.SavePngTo(a); err == nil {
			t.Fatalf("SavePngTo(%+v) succeeded, want refusal", a)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(first.Path, "01-a.png")); !bytes.HasSuffix(got, []byte("a")) {
		t.Fatalf("existing file overwritten: %q", got)
	}

	if err := p.ForgetShotFolder(bridge.ProofForgetShotFolderArgs{FolderID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SavePngTo(bridge.ProofSavePngToArgs{FolderID: first.ID, Filename: "late.png", PngBase64: pngB64("l")}); err == nil {
		t.Fatal("save after forget succeeded")
	}
}
