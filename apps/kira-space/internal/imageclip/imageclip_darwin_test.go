//go:build darwin && cgo

package imageclip

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePNG(t *testing.T, dir, name string, shade uint8) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	img.SetGray(1, 1, color.Gray{Y: shade})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Only check of the native write, on real AppKit. Uses a private pasteboard, never the general one.
func TestWriteImagesPrivatePasteboard(t *testing.T) {
	c := New(func(f func()) { f() })
	name := fmt.Sprintf("kira.test.%d.%d", os.Getpid(), time.Now().UnixNano())
	t.Cleanup(func() { releasePasteboard(name) })

	dir := t.TempDir()
	paths := []string{
		writePNG(t, dir, "01-a.png", 10),
		writePNG(t, dir, "02-b.png", 100),
		writePNG(t, dir, "03-c.png", 200),
	}
	if err := c.writeImagesTo(name, paths); err != nil {
		t.Fatal(err)
	}
	items := readPasteboard(name)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for i, p := range paths {
		want, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(items[i].png, want) {
			t.Errorf("item %d: public.png differs from file", i)
		}
		wantURL := (&url.URL{Scheme: "file", Path: p}).String()
		if items[i].url != wantURL {
			t.Errorf("item %d: file-url = %q, want %q", i, items[i].url, wantURL)
		}
	}

	err := c.writeImagesTo(name, []string{paths[0], filepath.Join(dir, "missing.png")})
	if !errors.Is(err, errUnreadable) {
		t.Fatalf("missing file err = %v, want errUnreadable", err)
	}
	if got := readPasteboard(name); len(got) != 3 {
		t.Errorf("failed write changed the pasteboard: %d items", len(got))
	}
}
