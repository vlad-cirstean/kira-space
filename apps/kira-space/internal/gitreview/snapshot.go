package gitreview

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"sync"
)

// ContentKind is one review_file row's own discriminant for what (if anything) its content column
// holds (D9).
type ContentKind string

const (
	// ContentText is the only kind that actually stores bytes.
	ContentText ContentKind = "text"
	// ContentBinary — looksBinary at mark time; change detection is exact through tier 0 (blob-oid
	// equality) regardless, so storing the bytes buys nothing.
	ContentBinary ContentKind = "binary"
	// ContentTooLarge — over MaxSnapshotBytes, or catfile.ErrTooLarge from the session's own gate.
	ContentTooLarge ContentKind = "tooLarge"
	// ContentAbsent — the path did not exist at reviewed_at_sha (reviewed as deleted).
	ContentAbsent ContentKind = "absent"
)

// MaxSnapshotBytes caps how large a file's UNCOMPRESSED content this package will ever store —
// gitsession.MaxPatchBytes's own sibling (D9/F15): a file whose content exceeds the size at which
// this chapter already refuses to render a patch is a file whose snapshot buys nothing.
const MaxSnapshotBytes = 1 << 20

// Compress flate-encodes raw at BestCompression — chosen over gzip (D9) because git's own blob oid
// (stored alongside every snapshot regardless) is already a stronger integrity check than gzip's
// CRC32 would add.
func Compress(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := getFlateWriter(&buf)
	if err != nil {
		return nil, err
	}
	defer flateWriters.Put(w)
	if _, err := w.Write(raw); err != nil {
		return nil, fmt.Errorf("gitreview: flate write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("gitreview: flate close: %w", err)
	}
	return buf.Bytes(), nil
}

// flateWriters reuses BestCompression writers: allocating one costs ~0.3 ms and dominated a
// many-file review.markMany (P739). Reset yields the same output as a fresh writer.
var flateWriters sync.Pool

func getFlateWriter(dst io.Writer) (*flate.Writer, error) {
	if w, ok := flateWriters.Get().(*flate.Writer); ok {
		w.Reset(dst)
		return w, nil
	}
	w, err := flate.NewWriter(dst, flate.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("gitreview: flate writer: %w", err)
	}
	return w, nil
}

// Decompress flate-decodes compressed and checks the result's length against want (the row's own
// content_bytes column) — a corrupt or truncated BLOB is detected here (length mismatch) rather
// than silently producing a shorter file (D9).
func Decompress(compressed []byte, want int) ([]byte, error) {
	if want < 0 || want > MaxSnapshotBytes {
		return nil, fmt.Errorf("gitreview: stored length %d outside 0..%d", want, MaxSnapshotBytes)
	}
	r := flate.NewReader(bytes.NewReader(compressed))
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, int64(want)+1))
	if err != nil {
		return nil, fmt.Errorf("gitreview: flate decode: %w", err)
	}
	if len(raw) != want {
		return nil, fmt.Errorf("gitreview: decompressed length %d does not match stored length %d", len(raw), want)
	}
	return raw, nil
}
