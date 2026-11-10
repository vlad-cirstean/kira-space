package proofflow

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
)

func jpegB64(extra string) string {
	return base64.StdEncoding.EncodeToString(append([]byte("\xff\xd8\xff"), extra...))
}

func TestProofShotSaveAll(t *testing.T) {
	f := newFixture(t)
	p := f.app.W.Proof
	home := f.app.Home

	loc, err := p.ShotLocation()
	if err != nil || loc != (bridge.ProofShotLocation{Path: home, Exists: true}) {
		t.Fatalf("no Desktop: location = %+v, %v; want home", loc, err)
	}
	desktop := filepath.Join(home, "Desktop")
	if err := os.Mkdir(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	loc, err = p.ShotLocation()
	if err != nil || loc != (bridge.ProofShotLocation{Path: desktop, Exists: true}) {
		t.Fatalf("location = %+v, %v; want the Desktop", loc, err)
	}
	f.app.Contract(t, "proof", "ProofService.ShotLocation", loc)

	first, err := p.OpenShotFolder(bridge.ProofOpenShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || first.Path != filepath.Join(desktop, "Checkout fix") {
		t.Fatalf("first = %+v, %v", first, err)
	}
	if len(f.app.Dialogs.Directory) != 0 {
		t.Fatalf("picker opened: %+v", f.app.Dialogs.Directory)
	}
	f.app.Contract(t, "proof", "ProofService.OpenShotFolder", first, flowharness.Mask("id"))

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

	if err := p.CloseShotFolder(bridge.ProofShotFolderArgs{FolderID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveJpegTo(bridge.ProofSaveJpegToArgs{FolderID: first.ID, Filename: "late.jpg", JpegBase64: jpegB64("l")}); err == nil {
		t.Fatal("save after close succeeded")
	}
	if err := p.RevealShotFolder(bridge.ProofShotFolderArgs{FolderID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if got := f.app.FileOpener.RevealedFiles(); len(got) != 1 || got[0] != first.Path {
		t.Fatalf("revealed = %v, want %s", got, first.Path)
	}

	second, err := p.OpenShotFolder(bridge.ProofOpenShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || second.Path != filepath.Join(desktop, "Checkout fix-2") {
		t.Fatalf("second = %+v, %v", second, err)
	}
	if err := p.CloseShotFolder(bridge.ProofShotFolderArgs{FolderID: second.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(second.Path); !os.IsNotExist(err) {
		t.Fatalf("empty folder kept: %v", err)
	}
	if err := p.RevealShotFolder(bridge.ProofShotFolderArgs{FolderID: second.ID}); err == nil {
		t.Fatal("reveal of a removed folder succeeded")
	}

	tmp := t.TempDir()
	canceled, err := p.ChooseShotLocation()
	if err != nil || !canceled.Canceled || canceled.Location != loc {
		t.Fatalf("cancel = %+v, %v", canceled, err)
	}
	f.app.Dialogs.AnswerDirectory(tmp)
	chosen, err := p.ChooseShotLocation()
	want := bridge.ProofShotLocation{Path: tmp, Custom: true, Exists: true}
	if err != nil || chosen.Canceled || chosen.Location != want {
		t.Fatalf("choose = %+v, %v", chosen, err)
	}
	f.app.Contract(t, "proof", "ProofService.ChooseShotLocation", chosen, flowharness.Replace(tmp, "<pick>"))
	if got, err := p.ShotLocation(); err != nil || got != want {
		t.Fatalf("persisted location = %+v, %v", got, err)
	}
	if got := f.app.Dialogs.Directory; len(got) != 2 || got[1].Title == "" {
		t.Fatalf("picker requests = %+v", got)
	}
	inTmp, err := p.OpenShotFolder(bridge.ProofOpenShotFolderArgs{SessionName: "Checkout fix"})
	if err != nil || inTmp.Path != filepath.Join(tmp, "Checkout fix") {
		t.Fatalf("custom open = %+v, %v", inTmp, err)
	}

	file := filepath.Join(tmp, "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.app.Dialogs.AnswerDirectory(file)
	if _, err := p.ChooseShotLocation(); err == nil || !strings.Contains(err.Error(), "is not a folder Kira Space can save to") {
		t.Fatalf("file pick: err = %v", err)
	}
	if got, _ := p.ShotLocation(); got != want {
		t.Fatalf("location after refused pick = %+v", got)
	}

	// Root bypasses mode bits, so the unwritable case only runs as a regular user.
	if os.Geteuid() != 0 {
		ro := filepath.Join(tmp, "ro")
		if err := os.Mkdir(ro, 0o755); err != nil {
			t.Fatal(err)
		}
		f.app.Dialogs.AnswerDirectory(ro)
		if _, err := p.ChooseShotLocation(); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
		if _, err := p.OpenShotFolder(bridge.ProofOpenShotFolderArgs{SessionName: "x"}); err == nil || !strings.Contains(err.Error(), "can’t write to") {
			t.Fatalf("read-only open: err = %v", err)
		}
	}

	if err := os.RemoveAll(tmp); err != nil {
		t.Fatal(err)
	}
	if got, err := p.ShotLocation(); err != nil || got.Exists || !got.Custom {
		t.Fatalf("removed location = %+v, %v", got, err)
	}
	if _, err := p.OpenShotFolder(bridge.ProofOpenShotFolderArgs{SessionName: "x"}); err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("missing location: err = %v", err)
	}

	reset, err := p.ResetShotLocation()
	if err != nil || reset != loc {
		t.Fatalf("reset = %+v, %v; want %+v", reset, err, loc)
	}
}
