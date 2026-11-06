// Package page is the Go analogue of packages/shared/protocol/page.ts: the columnar TextColumnChunk
// codec, the four page-kind builders, and the byte-size accounting the L2 cache budgets against.
//
// Bulk data crosses the wire as a FlatBuffers Frame, not JSON+base64:
// EncodePage (encode.go) writes a Chunk's four buffers into a FlatBuffers table with zero copying
// on the decode side — offsets/truncated are [uint32] vectors specifically so the generated
// TypeScript can hand back a zero-copy Uint32Array view.
package page

import (
	"encoding/base64"
	"sort"
	"unicode/utf8"
)

// Constants, mirroring page.ts.
const (
	MaxCellBytes = 64 << 10
	MaxPageSize  = 10_000
	// SingleRowMaxBytes is the per-value budget for a page that is one explicitly requested row,
	// and the ceiling on an object body fetched and rendered at all.
	SingleRowMaxBytes    = MaxCellBytes * 64
	ObjectBodyEditBytes  = MaxCellBytes * 16
	ObjectUploadMaxBytes = 5 << 30

	// columnEnvelopeBytes is the per-column object overhead estimate pageByteSize adds — a
	// measurement, not a guess (page.ts's own comment); changing it changes L2 eviction behaviour.
	columnEnvelopeBytes = 64
)

// Chunk is the Go analogue of page.ts's TextColumnChunk. Three exactly-sized buffers and no
// per-row object:
//
//	text of row i = utf8.decode(Data[Offsets[i]:Offsets[i+1]])
//	row i is NULL  = (Nulls[i>>3] & (1 << (i&7))) != 0
//
// A NULL row has Offsets[i] == Offsets[i+1]; an empty string does too, which is why the bitset is
// the only thing that distinguishes them — every value reaching a builder is a *string all the way
// from the driver's row scan, never a bare "" standing in for NULL.
//
// Offsets and Truncated are []uint32, not []byte — EncodePage writes each as a
// FlatBuffers [uint] vector, which is 4-byte aligned by construction, letting the generated
// TypeScript decoder hand back a zero-copy Uint32Array view over the received frame instead of a
// manual byte-by-byte reinterpretation.
type Chunk struct {
	Data      []byte
	Offsets   []uint32
	Nulls     []byte
	Truncated []uint32
	// Binary lists sorted row indices whose text is standard base64 of raw bytes that were not
	// valid UTF-8. Nil when none.
	Binary []uint32
}

// BytesCell returns b as cell text: as-is when valid UTF-8, else standard base64 with binary set.
func BytesCell(b []byte) (text string, binary bool) {
	if utf8.Valid(b) {
		return string(b), false
	}
	return base64.StdEncoding.EncodeToString(b), true
}

func bitsetBytes(rowCount int) int {
	return (rowCount + 7) / 8
}

// IsNull reports whether row is NULL in chunk.
func IsNull(chunk Chunk, row int) bool {
	return chunk.Nulls[row>>3]&(1<<(row&7)) != 0
}

// CellText decodes row's text out of chunk.
func CellText(chunk Chunk, row int) string {
	return string(chunk.Data[chunk.Offsets[row]:chunk.Offsets[row+1]])
}

// IsTruncated reports whether row's cell was clipped at MaxCellBytes. Truncated is the sorted row
// index list the builder writes, not a bitset — protocol/page.ts's isTruncated binary-searches the
// same list.
func IsTruncated(chunk Chunk, row int) bool {
	i := sort.Search(len(chunk.Truncated), func(i int) bool { return chunk.Truncated[i] >= uint32(row) })
	return i < len(chunk.Truncated) && chunk.Truncated[i] == uint32(row)
}

// IsBinary reports whether row's cell is base64 of non-UTF-8 bytes.
func IsBinary(chunk Chunk, row int) bool {
	i := sort.Search(len(chunk.Binary), func(i int) bool { return chunk.Binary[i] >= uint32(row) })
	return i < len(chunk.Binary) && chunk.Binary[i] == uint32(row)
}

// ChunkByteSize is the real, measured byte cost of chunk — what L2 budgets against. Unchanged by
// P11's []byte->[]uint32 switch: four uint32s of pre-encoded little-endian bytes and one []uint32
// of the same four bytes each cost exactly the same number of bytes.
func ChunkByteSize(chunk Chunk) int {
	return len(chunk.Data) + len(chunk.Offsets)*4 + len(chunk.Nulls) + len(chunk.Truncated)*4 + len(chunk.Binary)*4
}
