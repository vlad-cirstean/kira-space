package proof

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

const (
	webpLossy    = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"
	webpLossless = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="
)

func b64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// chunk builds one PNG chunk; the CRC is not checked by the animation scan.
func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)
	return append(out, 0, 0, 0, 0)
}

func apng(t *testing.T) []byte {
	t.Helper()
	raw := pngOf(t, 4, 4)
	idat := bytes.Index(raw, []byte("IDAT")) - 4
	out := append([]byte{}, raw[:idat]...)
	out = append(out, pngChunk("acTL", make([]byte, 8))...)
	return append(out, raw[idat:]...)
}

func animatedWebP(flagOnly bool) []byte {
	vp8x := make([]byte, 10)
	vp8x[0] = 0x02 // animation flag
	body := []byte("WEBP")
	body = append(body, "VP8X"...)
	body = binary.LittleEndian.AppendUint32(body, 10)
	body = append(body, vp8x...)
	if !flagOnly {
		vp8x[0] = 0
		body = []byte("WEBPVP8X")
		body = binary.LittleEndian.AppendUint32(body, 10)
		body = append(body, vp8x...)
		body = append(body, "ANIM"...)
		body = binary.LittleEndian.AppendUint32(body, 6)
		body = append(body, make([]byte, 6)...)
	}
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	return append(out, body...)
}

// One row per rule: the extension picks the format, the content must agree, caps and animation are refused.
func TestCheckImage(t *testing.T) {
	cases := []struct {
		name, file string
		raw        []byte
		format     string
		w, h       int
		want       string // "" accepts
	}{
		{"png", "a.png", pngOf(t, 5, 3), FormatPNG, 5, 3, ""},
		{"jpeg", "a.jpg", jpegOf(t, 6, 4), FormatJPEG, 6, 4, ""},
		{"jpeg long ext", "a.jpeg", jpegOf(t, 6, 4), FormatJPEG, 6, 4, ""},
		{"webp lossy", "a.webp", b64(t, webpLossy), FormatWebP, 1, 1, ""},
		{"webp lossless", "a.webp", b64(t, webpLossless), FormatWebP, 1, 1, ""},
		{"svg", "a.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="10"/>`), FormatSVG, 20, 10, ""},
		{"extension mismatch", "a.jpg", pngOf(t, 2, 2), "", 0, 0, "extension mismatch: the file is png but its name says jpeg"},
		{"gif", "a.gif", []byte("GIF89a"), "", 0, 0, "not an allowed image"},
		{"svgz", "a.svgz", []byte("x"), "", 0, 0, "not an allowed image"},
		{"no extension", "a", pngOf(t, 2, 2), "", 0, 0, "the name must end in"},
		{"garbage", "a.png", []byte("hello"), "", 0, 0, "the content is not PNG, JPEG or WebP"},
		{"apng", "a.png", apng(t), "", 0, 0, "animated: APNG"},
		{"animated webp flag", "a.webp", animatedWebP(true), "", 0, 0, "animated: animated WebP"},
		{"animated webp chunk", "a.webp", animatedWebP(false), "", 0, 0, "animated: animated WebP"},
		{"truncated png", "a.png", pngOf(t, 40, 40)[:60], "", 0, 0, "damaged data"},
		{"too wide", "a.png", pngOf(t, 16385, 1), "", 0, 0, "side or area out of range"},
		{"over the area", "a.jpg", jpegOf(t, 4001, 4001), "", 0, 0, "area at most 16 megapixels"},
		{"raster over the byte cap", "a.png", append(pngOf(t, 2, 2), make([]byte, MaxRasterBytes)...), "", 0, 0, "too large"},
		{"svg over the byte cap", "a.svg", bytes.Repeat([]byte(" "), MaxSVGBytes+1), "", 0, 0, "too large"},
	}
	for _, c := range cases {
		info, err := CheckImage(c.file, c.raw)
		if c.want == "" {
			if err != nil || info.Format != c.format || info.Width != c.w || info.Height != c.h || info.Bytes != len(c.raw) {
				t.Errorf("%s: got %+v, %v; want %s %dx%d", c.name, info, err, c.format, c.w, c.h)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
}
