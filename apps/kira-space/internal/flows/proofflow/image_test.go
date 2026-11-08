package proofflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

const maxImageBytes = 3<<20 - 1<<10

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

func writeFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func imageStep(desc string, data map[string]any) map[string]any {
	return map[string]any{"description": desc, "view": "image", "source": "playwright", "data": data}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

type imageData struct {
	Width, Height, Bytes int
	Base64               string
}

func TestProofImage(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	shot := jpegBytes(t, 64, 48)
	tall := jpegBytes(t, 64, 2000)
	shotPath := writeFile(t, filepath.Join(dir, "shot.jpg"), shot)
	linkPath := filepath.Join(dir, "link.jpg")
	if err := os.Symlink(shotPath, linkPath); err != nil {
		t.Fatal(err)
	}
	tallPath := writeFile(t, filepath.Join(dir, "tall.jpg"), tall)

	f.save(t, "Login page",
		imageStep("Login form, from a file", map[string]any{"path": shotPath}),
		imageStep("Login form, inline", map[string]any{"base64": b64(shot)}),
		imageStep("Login form, through a symlink", map[string]any{"path": linkPath}),
		imageStep("Long login page", map[string]any{"path": tallPath}),
	)
	// The bytes were copied at save: a later change or deletion of the file never alters the proof.
	if err := os.Remove(shotPath); err != nil {
		t.Fatal(err)
	}

	id := sessionID(t, f.app, "Login page")
	list, err := f.app.W.Proof.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "proof-image", "ProofService.Sessions", list, maskIDs)
	det := session(t, f.app, id)
	f.app.Contract(t, "proof-image", "ProofService.Session", det, maskIDs)
	if len(det.Current.Steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(det.Current.Steps))
	}
	for i, c := range []struct {
		key           string
		file          []byte
		width, height int
	}{{"path", shot, 64, 48}, {"inline", shot, 64, 48}, {"symlink", shot, 64, 48}, {"tall", tall, 64, 2000}} {
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: det.Current.Rev, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		if st.View != "image" {
			t.Fatalf("step %d view = %q, want image", i+1, st.View)
		}
		var p imageData
		if err := json.Unmarshal(st.Data, &p); err != nil {
			t.Fatal(err)
		}
		got, err := base64.StdEncoding.DecodeString(p.Base64)
		if err != nil || !bytes.Equal(got, c.file) {
			t.Fatalf("step %d base64 does not decode to the file bytes (%v)", i+1, err)
		}
		if p.Width != c.width || p.Height != c.height || p.Bytes != len(c.file) {
			t.Fatalf("step %d = %dx%d %d bytes, want %dx%d %d", i+1, p.Width, p.Height, p.Bytes, c.width, c.height, len(c.file))
		}
		if strings.Contains(string(st.Data), dir) {
			t.Fatalf("step %d payload stores the path: %s", i+1, st.Data)
		}
		f.app.Contract(t, "proof-image", "ProofService.Step#"+c.key, st, maskIDs)
	}
}

func TestProofImageRefusals(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	shot := jpegBytes(t, 64, 48)
	good := writeFile(t, filepath.Join(dir, "ok.jpg"), shot)

	fifo := filepath.Join(dir, "pipe.jpg")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(dir, "huge.jpg")
	hf, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hf.Write(shot[:3]); err != nil {
		t.Fatal(err)
	}
	if err := hf.Truncate(maxImageBytes + 1); err != nil { // sparse: refused on size, never read
		t.Fatal(err)
	}
	_ = hf.Close()

	wide := writeFile(t, filepath.Join(dir, "wide.jpg"), jpegBytes(t, 16385, 1))
	pngPath := writeFile(t, filepath.Join(dir, "x.jpg"), pngBytes(t))
	corrupt := writeFile(t, filepath.Join(dir, "corrupt.jpg"), append(append([]byte{}, shot[:len(shot)/2]...), 0xFF))
	const notJPEG = "data is not a JPEG image (baseline or progressive)"
	const unreadable = "data.path is not a readable regular file"

	cases := []struct {
		name string
		data map[string]any
		want string
	}{
		{"relative path", map[string]any{"path": "shot.jpg"}, "data.path must be an absolute path"},
		{"tilde path", map[string]any{"path": "~/x.jpg"}, "data.path must be an absolute path"},
		{"missing file", map[string]any{"path": filepath.Join(dir, "nope.jpg")}, unreadable},
		{"directory", map[string]any{"path": dir}, unreadable},
		{"png by path", map[string]any{"path": pngPath}, notJPEG},
		{"png inline", map[string]any{"base64": b64(pngBytes(t))}, notJPEG},
		{"truncated jpeg", map[string]any{"path": corrupt}, notJPEG},
		{"sparse over the cap", map[string]any{"path": huge}, "the limit is 3071 KiB per image"},
		{"inline over the cap", map[string]any{"base64": b64(append(append([]byte{}, shot...), make([]byte, maxImageBytes)...))}, "the limit is 3071 KiB per image"},
		{"both", map[string]any{"path": good, "base64": b64(shot)}, "data must hold exactly one of path or base64"},
		{"neither", map[string]any{}, "data must hold exactly one of path or base64"},
		{"url-safe base64", map[string]any{"base64": strings.NewReplacer("+", "-", "/", "_").Replace(b64(append([]byte{0xFF, 0xD8, 0xFF, 0xFB, 0xFF}, shot[3:]...)))}, "data.base64 is not standard base64"},
		{"data url", map[string]any{"base64": "data:image/jpeg;base64," + b64(shot)}, "data.base64 is not standard base64"},
		{"too wide", map[string]any{"path": wide}, "image is 16385x1; each side must be 1-16384 pixels"},
		{"unknown field", map[string]any{"path": good, "mime": "image/jpeg"}, "mime"},
	}
	for _, c := range cases {
		msg := f.saveFail(t, "Bad image", imageStep("bad", c.data))
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s: message = %q, want %q", c.name, msg, c.want)
		}
		if strings.Contains(msg, dir) {
			t.Errorf("%s: message echoes the path: %q", c.name, msg)
		}
	}

	// A FIFO must be refused, not block the save.
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := f.sess.CallTool(cctx, &mcp.CallToolParams{Name: "proof_save", Arguments: saveArgs("Bad image", []map[string]any{imageStep("bad", map[string]any{"path": fifo})})})
	if err != nil || !res.IsError || !strings.Contains(text(res), unreadable) {
		t.Fatalf("fifo: err = %v, result = %+v", err, res)
	}

	if list, err := f.app.W.Proof.Sessions(); err != nil || len(list) != 0 {
		t.Fatalf("refused saves left sessions behind: %+v, %v", list, err)
	}
}

func TestProofImageSessionCap(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	// Pad a small JPEG with trailing bytes (ignored after EOI) up to the file cap.
	big := append(jpegBytes(t, 64, 48), make([]byte, maxImageBytes)...)[:maxImageBytes]
	path := writeFile(t, filepath.Join(dir, "big.jpg"), big)
	steps := make([]map[string]any, 9)
	for i := range steps {
		steps[i] = imageStep("max image", map[string]any{"path": path})
	}
	// Each stores about 4 MiB (4/3 of the file), so the ninth passes the 32 MiB session cap.
	msg := f.saveFail(t, "Too many images", steps...)
	if !strings.Contains(msg, "steps[8]") || !strings.Contains(msg, "32 MiB data limit") {
		t.Fatalf("message = %q, want steps[8] and the 32 MiB limit", msg)
	}
	if list, err := f.app.W.Proof.Sessions(); err != nil || len(list) != 0 {
		t.Fatalf("session left behind: %+v, %v", list, err)
	}
	if c := f.save(t, "Eight images", steps[:8]...); c.Steps != 8 {
		t.Fatalf("eight images = %+v", c)
	}
}
