package proof

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Image limits. A stored image is base64 (4/3 of the file), so the file cap keeps the payload under
// maxPayloadBytes. The side cap is the WebKit decode limit. The pixel cap bounds the integrity decode:
// a small progressive JPEG of uniform content makes image/jpeg allocate about 6 bytes per pixel of
// coefficients plus the decoded image, so 16 MP (a 5K screenshot, 4000x4000) costs roughly 150 MB
// transient where 25 MP cost 300 MB.
const (
	maxImageBytes  = 3<<20 - 1<<10
	maxImageSide   = 16384
	maxImagePixels = 16_000_000
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

// pathRefusal is the one message for every failure of a path source after the path is known absolute.
// Kira reads the file, not the agent's sandbox, so size, type and existence must not tell apart.
func pathRefusal() error {
	return invalid("data.path is not a readable JPEG file within the limits (regular file, at most %d KiB, each side 1-%d pixels, area at most %d megapixels)",
		maxImageBytes>>10, maxImageSide, maxImagePixels/1_000_000)
}

func checkImage(ctx context.Context, data []byte) (any, error) {
	var in imageIn
	if err := decodeStrict(data, &in); err != nil {
		return nil, err
	}
	if (in.Path == nil) == (in.Base64 == nil) {
		return nil, invalid("data must hold exactly one of path or base64")
	}
	if in.Path != nil {
		if !filepath.IsAbs(*in.Path) {
			return nil, invalid("data.path must be an absolute path")
		}
		raw, err := readImageFile(ctx, *in.Path)
		if err == nil {
			var p imagePayload
			if p, err = checkJPEG(raw); err == nil {
				return p, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Detail stays server-side, and never the path: it may name a secret.
		slog.Info("proof: image path refused", "scope", "proof", "reason", err)
		return nil, pathRefusal()
	}
	raw, err := decodeImageBase64(*in.Base64)
	if err != nil {
		return nil, err
	}
	p, err := checkJPEG(raw)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// checkJPEG vets the bytes and builds the stored payload.
func checkJPEG(raw []byte) (imagePayload, error) {
	if len(raw) > maxImageBytes {
		return imagePayload{}, tooBig(len(raw))
	}
	notJPEG := invalid("data is not a JPEG image (baseline or progressive)")
	if !bytes.HasPrefix(raw, jpegPrefix) {
		return imagePayload{}, notJPEG
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return imagePayload{}, notJPEG
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return imagePayload{}, invalid("image is %dx%d; each side must be 1-%d pixels and the area at most %d megapixels", cfg.Width, cfg.Height, maxImageSide, maxImagePixels/1_000_000)
	}
	// Full decode proves integrity; the area check above bounds its memory.
	if _, err := jpeg.Decode(bytes.NewReader(raw)); err != nil {
		return imagePayload{}, notJPEG
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

// readImageFile reads at most the image cap; the caller turns any error into pathRefusal.
func readImageFile(ctx context.Context, p string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// O_NONBLOCK so a FIFO or device cannot hang the save.
	f, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if st.Size() > maxImageBytes {
		return nil, fmt.Errorf("file is %d bytes, over the cap", st.Size())
	}
	return io.ReadAll(io.LimitReader(f, maxImageBytes+1))
}
