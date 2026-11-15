package proofflow

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

const svgShot = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="30"><rect width="40" height="30" fill="#36c"/></svg>`

// jpegBytes encodes a w x h image: a solid fill with a contrasting block in the top-left corner.
func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{R: 30, G: 90, B: 200, A: 255}
			if x < w/2 && y < min(h, 48)/2 {
				c = color.RGBA{R: 240, G: 220, B: 40, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func imageStep(desc string, raw []byte, ext string) map[string]any {
	return map[string]any{"description": desc, "view": "image", "source": "playwright", "image": raw, "ext": ext}
}

// An image is a file beside proof.md: PNG, JPEG and SVG all open through the proof window's step read.
func TestProofImage(t *testing.T) {
	f := newFixture(t)
	shot := jpegBytes(t, 64, 48)
	tall := jpegBytes(t, 64, 2000)
	f.save(t, "Login page",
		imageStep("Login form, JPEG", shot, "jpg"),
		imageStep("Login form, PNG", pngBytes(t), "png"),
		imageStep("Login logo, SVG", []byte(svgShot), "svg"),
		imageStep("Long login page", tall, "jpg"),
	)
	// The file in the folder is the proof: the bytes are in the blob store, so deleting it changes nothing here.
	id := f.idOf(t, "Login page")
	f.app.Contract(t, "proof-image", "AdeTaskService.TaskProofs", f.proofs(t), maskIDs)
	det := session(t, f.app, id)
	f.app.Contract(t, "proof-image", "ProofService.Session", det, maskIDs)
	if len(det.Current.Steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(det.Current.Steps))
	}
	for i, c := range []struct {
		key, format   string
		file          []byte
		width, height int
	}{{"jpeg", "jpeg", shot, 64, 48}, {"png", "png", pngBytes(t), 8, 8}, {"svg", "svg", []byte(svgShot), 40, 30}, {"tall", "jpeg", tall, 64, 2000}} {
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: det.Current.Rev, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		if st.View != "image" {
			t.Fatalf("step %d view = %q, want image", i+1, st.View)
		}
		var p struct {
			Format, File, Base64 string
			Width, Height, Bytes int
		}
		if err := json.Unmarshal(st.Data, &p); err != nil {
			t.Fatal(err)
		}
		if p.Format != c.format || p.Width != c.width || p.Height != c.height || p.Bytes != len(c.file) {
			t.Fatalf("step %d = %+v, want %s %dx%d %d bytes", i+1, p, c.format, c.width, c.height, len(c.file))
		}
		if strings.Contains(string(st.Data), f.folder) {
			t.Fatalf("step %d payload stores the path: %s", i+1, st.Data)
		}
		f.app.Contract(t, "proof-image", "ProofService.Step#"+c.key, st, maskIDs)
	}
	// The file on disk can go; the accepted copy stays readable and the step still opens.
	if err := os.Remove(filepath.Join(f.folder, "proofs", "login-page", "s1.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: det.Current.Rev, Ordinal: 1}); err != nil {
		t.Fatal(err)
	}
}

// Every refusal names the proof file, the step and the rule, and leaves no proof behind.
func TestProofImageRefused(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name string
		step map[string]any
		want string
	}{
		{"gif", imageStep("bad", []byte("GIF89a"), "gif"), "s1.gif"},
		{"png named jpg", imageStep("bad", pngBytes(t), "jpg"), "extension mismatch"},
		{"garbage", imageStep("bad", []byte("not an image"), "png"), "not an allowed image"},
		{"svg script", imageStep("bad", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><script>x</script></svg>`), "svg"), "<script> is not allowed"},
	}
	for _, c := range cases {
		msg := f.saveFail(t, "Bad image", c.step)
		if !strings.Contains(msg, c.want) || !strings.Contains(msg, "proofs/bad-image/") {
			t.Errorf("%s: reply = %q, want %q", c.name, msg, c.want)
		}
		if strings.Contains(msg, f.folder) {
			t.Errorf("%s: reply echoes the folder path: %q", c.name, msg)
		}
	}
	if list := f.proofs(t); len(list) != 0 {
		t.Fatalf("refused proofs left rows behind: %+v", list)
	}
}
