package proof

import (
	"bytes"
	"encoding/base64"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Image limits. A stored image is base64 (4/3 of the file), so the file cap keeps the payload under
// maxPayloadBytes. The side cap is the WebKit decode limit; the pixel cap bounds decode memory.
const (
	maxImageBytes  = 3<<20 - 1<<10
	maxImageSide   = 16384
	maxImagePixels = 25_000_000
)

var jpegPrefix = []byte{0xFF, 0xD8, 0xFF}

type imageIn struct {
	Path   *string `json:"path"`
	Base64 *string `json:"base64"`
}

type imagePayload struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
	Base64 string `json:"base64"`
}

func tooBig(n int) error {
	return invalid("data is %d KiB; the limit is %d KiB per image", n>>10, maxImageBytes>>10)
}

func checkImage(data []byte) (any, error) {
	var in imageIn
	if err := decodeStrict(data, &in); err != nil {
		return nil, err
	}
	if (in.Path == nil) == (in.Base64 == nil) {
		return nil, invalid("data must hold exactly one of path or base64")
	}
	var raw []byte
	var err error
	if in.Path != nil {
		raw, err = readImageFile(*in.Path)
	} else {
		raw, err = decodeImageBase64(*in.Base64)
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > maxImageBytes {
		return nil, tooBig(len(raw))
	}
	notJPEG := invalid("data is not a JPEG image (baseline or progressive)")
	if !bytes.HasPrefix(raw, jpegPrefix) {
		return nil, notJPEG
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, notJPEG
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return nil, invalid("image is %dx%d; each side must be 1-%d pixels and the area at most 25 megapixels", cfg.Width, cfg.Height, maxImageSide)
	}
	// Full decode proves integrity; the pixel cap above bounds its memory.
	if _, err := jpeg.Decode(bytes.NewReader(raw)); err != nil {
		return nil, notJPEG
	}
	return imagePayload{Width: cfg.Width, Height: cfg.Height, Bytes: len(raw), Base64: base64.StdEncoding.EncodeToString(raw)}, nil
}

func decodeImageBase64(s string) ([]byte, error) {
	notStd := invalid("data.base64 is not standard base64")
	if strings.HasPrefix(s, "data:") || strings.ContainsAny(s, "-_") {
		return nil, notStd
	}
	// Refuse before allocating: base64 of an over-cap image is longer than this.
	if base64.StdEncoding.DecodedLen(len(s)) > maxImageBytes+3 {
		return nil, tooBig(base64.StdEncoding.DecodedLen(len(s)))
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, notStd
	}
	return raw, nil
}

func readImageFile(p string) ([]byte, error) {
	if !filepath.IsAbs(p) {
		return nil, invalid("data.path must be an absolute path")
	}
	// One message for every failure, and never the path: it may name a secret.
	unreadable := invalid("data.path is not a readable regular file")
	// O_NONBLOCK so a FIFO or device cannot hang the save.
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, unreadable
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil, unreadable
	}
	if st.Size() > maxImageBytes {
		return nil, tooBig(int(min(st.Size(), 1<<40)))
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil {
		return nil, unreadable
	}
	return raw, nil
}
