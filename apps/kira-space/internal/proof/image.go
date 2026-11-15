package proof

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path"
	"strings"

	"golang.org/x/image/webp"
)

// Image limits. Files live beside proof.md, so the caps bound the integrity decode, not a payload.
// The side cap is the WebKit decode limit. The pixel cap bounds the decode: a small progressive JPEG
// of uniform content makes image/jpeg allocate about 6 bytes per pixel of coefficients plus the
// decoded image, so 16 MP (a 5K screenshot) costs roughly 150 MB transient.
const (
	// MaxRasterBytes and MaxSVGBytes cap one image file.
	MaxRasterBytes = 10 << 20
	MaxSVGBytes    = 2 << 20
	maxImageSide   = 16384
	maxImagePixels = 16_000_000
)

// Image formats a step can hold.
const (
	FormatPNG  = "png"
	FormatJPEG = "jpeg"
	FormatWebP = "webp"
	FormatSVG  = "svg"
)

// ImageInfo describes a vetted image file.
type ImageInfo struct {
	Format        string
	Width, Height int
	Bytes         int
}

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
)

// FormatOfName maps a file extension to an image format; "" when the extension is not an allowed one.
func FormatOfName(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".png":
		return FormatPNG
	case ".jpg", ".jpeg":
		return FormatJPEG
	case ".webp":
		return FormatWebP
	case ".svg":
		return FormatSVG
	}
	return ""
}

// ImageExt is the file extension of a format.
func ImageExt(format string) string {
	if format == FormatJPEG {
		return "jpg"
	}
	return format
}

// MaxImageBytes is the file cap of a format.
func MaxImageBytes(format string) int {
	if format == FormatSVG {
		return MaxSVGBytes
	}
	return MaxRasterBytes
}

// CheckImage vets one image file: the extension of name picks the format and must agree with the
// content; raster files must decode within the limits, SVG must pass the safe-SVG rules. Messages are
// agent-facing and carry no file name; the caller prefixes it.
func CheckImage(name string, raw []byte) (ImageInfo, error) {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".svgz", ".gif", ".bmp", ".tif", ".tiff", ".heic", ".avif":
		return ImageInfo{}, invalid("not an allowed image: use PNG, JPEG, WebP or SVG (not %s)", strings.TrimPrefix(ext, "."))
	}
	want := FormatOfName(name)
	if want == "" {
		return ImageInfo{}, invalid("not an allowed image: the name must end in .png, .jpg, .jpeg, .webp or .svg")
	}
	if len(raw) > MaxImageBytes(want) {
		return ImageInfo{}, invalid("too large: %d KiB, the limit is %d KiB for a %s file", len(raw)>>10, MaxImageBytes(want)>>10, want)
	}
	if want == FormatSVG {
		w, h, err := CheckSVG(raw)
		if err != nil {
			return ImageInfo{}, err
		}
		return ImageInfo{Format: FormatSVG, Width: w, Height: h, Bytes: len(raw)}, nil
	}
	got := sniffRaster(raw)
	switch {
	case got == "":
		return ImageInfo{}, invalid("not an allowed image: the content is not PNG, JPEG or WebP")
	case got != want:
		return ImageInfo{}, invalid("extension mismatch: the file is %s but its name says %s", got, want)
	}
	if err := checkNotAnimated(got, raw); err != nil {
		return ImageInfo{}, err
	}
	cfg, err := decodeConfig(got, raw)
	if err != nil {
		return ImageInfo{}, invalid("damaged data: the %s header does not decode", got)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return ImageInfo{}, invalid("side or area out of range: image is %dx%d; each side must be 1-%d pixels and the area at most %d megapixels",
			cfg.Width, cfg.Height, maxImageSide, maxImagePixels/1_000_000)
	}
	// Full decode proves integrity; the area check above bounds its memory.
	if err := decodeFull(got, raw); err != nil {
		return ImageInfo{}, invalid("damaged data: the %s data does not decode", got)
	}
	return ImageInfo{Format: got, Width: cfg.Width, Height: cfg.Height, Bytes: len(raw)}, nil
}

func sniffRaster(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, pngMagic):
		return FormatPNG
	case bytes.HasPrefix(raw, jpegMagic):
		return FormatJPEG
	case len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return FormatWebP
	}
	return ""
}

func decodeConfig(format string, raw []byte) (image.Config, error) {
	r := bytes.NewReader(raw)
	switch format {
	case FormatPNG:
		return png.DecodeConfig(r)
	case FormatJPEG:
		return jpeg.DecodeConfig(r)
	}
	return webp.DecodeConfig(r)
}

func decodeFull(format string, raw []byte) error {
	r := bytes.NewReader(raw)
	var err error
	switch format {
	case FormatPNG:
		_, err = png.Decode(r)
	case FormatJPEG:
		_, err = jpeg.Decode(r)
	default:
		_, err = webp.Decode(r)
	}
	return err
}

// checkNotAnimated refuses an APNG (an acTL chunk before the first IDAT) and an animated WebP (the
// VP8X animation flag or an ANIM chunk).
func checkNotAnimated(format string, raw []byte) error {
	switch format {
	case FormatPNG:
		for off := 8; off+8 <= len(raw); {
			n := int(binary.BigEndian.Uint32(raw[off:]))
			typ := string(raw[off+4 : off+8])
			if typ == "acTL" {
				return invalid("animated: APNG is not allowed, use a still PNG")
			}
			if typ == "IDAT" {
				return nil
			}
			off += 12 + n
		}
	case FormatWebP:
		for off := 12; off+8 <= len(raw); {
			typ := string(raw[off : off+4])
			n := int(binary.LittleEndian.Uint32(raw[off+4:]))
			if typ == "ANIM" || typ == "ANMF" || typ == "VP8X" && off+9 <= len(raw) && raw[off+8]&0x02 != 0 {
				return invalid("animated: animated WebP is not allowed, use a still image")
			}
			off += 8 + n + n&1
		}
	}
	return nil
}

// ImageMeta is the stored payload of an image step: metadata only, the bytes stay in the file.
type ImageMeta struct {
	Format string `json:"format"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Bytes  int    `json:"bytes"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
}

// String names an image for a message.
func (i ImageInfo) String() string { return fmt.Sprintf("%s %dx%d", i.Format, i.Width, i.Height) }

// SniffFormat names the raster format of raw by its magic bytes; "" when it is none of PNG, JPEG, WebP.
func SniffFormat(raw []byte) string { return sniffRaster(raw) }
