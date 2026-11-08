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

func jpegB64(extra string) string {
	return base64.StdEncoding.EncodeToString(append([]byte("\xff\xd8\xff"), extra...))
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
	f.app.Contract(t, "proof", "ProofService.ChooseShotFolder", first,
		flowharness.Mask("id", "path"))
	f.app.Dialogs.AnswerDirectory(tmp)
	second, err := p.ChooseShotFolder(bridge.ProofChooseShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || second.Path != filepath.Join(tmp, "Checkout fix-2") {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if got := f.app.Dialogs.Directory; len(got) != 3 || got[0].Title == "" {
		t.Fatalf("picker requests = %+v", got)
	}

	for i, name := range []string{"01-a.jpg", "02-b.jpg", "03-c.jpg"} {
		body := string(rune('a' + i))
		res, err := p.SaveJpegTo(bridge.ProofSaveJpegToArgs{FolderID: first.ID, Filename: name, JpegBase64: jpegB64(body)})
		if err != nil || res.Path != filepath.Join(first.Path, name) {
			t.Fatalf("save %s = %+v, %v", name, res, err)
		}
		got, err := os.ReadFile(res.Path)
		if err != nil || !bytes.Equal(got, append([]byte("\xff\xd8\xff"), body...)) {
			t.Fatalf("read back %s = %q, %v", name, got, err)
		}
	}
	res, err := p.SaveJpegTo(bridge.ProofSaveJpegToArgs{FolderID: first.ID, Filename: "../x.jpg", JpegBase64: jpegB64("x")})
	if err != nil || res.Path != filepath.Join(first.Path, "x.jpg") {
		t.Fatalf("traversal = %+v, %v", res, err)
	}

	refused := []bridge.ProofSaveJpegToArgs{
		{FolderID: first.ID, Filename: "01-a.jpg", JpegBase64: jpegB64("again")},
		{FolderID: first.ID, Filename: "n.jpg", JpegBase64: base64.StdEncoding.EncodeToString([]byte("not a jpeg"))},
		{FolderID: "shot_unknown", Filename: "n.jpg", JpegBase64: jpegB64("n")},
	}
	for _, a := range refused {
		if _, err := p.SaveJpegTo(a); err == nil {
			t.Fatalf("SaveJpegTo(%+v) succeeded, want refusal", a)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(first.Path, "01-a.jpg")); !bytes.HasSuffix(got, []byte("a")) {
		t.Fatalf("existing file overwritten: %q", got)
	}

	if err := p.ForgetShotFolder(bridge.ProofForgetShotFolderArgs{FolderID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveJpegTo(bridge.ProofSaveJpegToArgs{FolderID: first.ID, Filename: "late.jpg", JpegBase64: jpegB64("l")}); err == nil {
		t.Fatal("save after forget succeeded")
	}
}
