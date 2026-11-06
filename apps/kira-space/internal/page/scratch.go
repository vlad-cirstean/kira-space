package page

import (
	"unicode/utf8"
)

// columnScratch accumulates one column's rows into growable scratch and only copies into
// exactly-sized buffers at finish() — page.ts's ColumnScratch, field for field. reverse ordering is
// honoured entirely inside finish() by choosing which row order to copy in; every row's byte range
// is already addressable via rowStart, so no extra buffer is needed.
type columnScratch struct {
	buffer        []byte
	used          int
	rowStart      []int
	isNullRow     []bool
	truncatedRows map[int]struct{}
	binaryRows    map[int]struct{}
}

func newColumnScratch() *columnScratch {
	return &columnScratch{buffer: make([]byte, 256), rowStart: []int{0}}
}

func (s *columnScratch) grow(extra int) {
	if s.used+extra <= len(s.buffer) {
		return
	}
	size := len(s.buffer) * 2
	for size < s.used+extra {
		size *= 2
	}
	next := make([]byte, size)
	copy(next, s.buffer[:s.used])
	s.buffer = next
}

// truncateUTF8ToBoundary cuts b at maxBytes on a UTF-8 rune boundary, never mid-sequence — a split
// rune would make the renderer's decoder emit U+FFFD and the cell look corrupted.
//
// F8 (P108 Part 6, wire mirror): this used to back off with utf8.DecodeLastRune, which treats ANY
// trailing byte invalid in isolation as "back off one more" — for a genuine multi-byte UTF-8
// sequence split by the cut that is at most 3 bytes (utf8.UTFMax-1), matching page.ts's own
// truncateUtf8ToBoundary. But a run of bytes that merely *resemble* a UTF-8 lead byte without ever
// completing one (e.g. Latin-1's own 0xE0-0xFF letter range, à-ÿ, each invalid as a standalone
// rune) made DecodeLastRune back off one byte at a time with no bound, walking through the whole
// run and clipping a value far below the intended limit. utf8.RuneStart mirrors page.ts's own
// check (`(bytes[end] & 0xc0) === 0x80`, "is this a continuation byte") instead of asking whether
// the trailing byte decodes on its own — the two now agree, and the loop is bounded the same way
// TS's own back-off effectively is for any real UTF-8 sequence: at most 3 continuation bytes.
func truncateUTF8ToBoundary(b []byte, maxBytes int) []byte {
	if len(b) <= maxBytes {
		return b
	}
	// Mirrors page.ts's own `while (end > 0 && (bytes[end] & 0xc0) === 0x80) end--` exactly: the
	// byte checked at each step is b[end], the first EXCLUDED byte (b[:end] is what gets kept) —
	// a continuation byte there means the cut split whatever rune b[end-1] belongs to, so back off
	// one more and check again. RuneStart(x) is the same "not a continuation byte" test as TS's own
	// bit mask, spelled the other way round.
	end := maxBytes
	for i := 0; i < utf8.UTFMax-1 && end > 0 && !utf8.RuneStart(b[end]); i++ {
		end--
	}
	return b[:end]
}

// markBinary flags row's text as base64 of non-UTF-8 bytes.
func (s *columnScratch) markBinary(row int) {
	if s.binaryRows == nil {
		s.binaryRows = make(map[int]struct{})
	}
	s.binaryRows[row] = struct{}{}
}

// appendValue appends one row's value (nil for NULL) and reports whether it was truncated.
func (s *columnScratch) appendValue(value *string, row int, maxBytes int) bool {
	if value == nil {
		s.isNullRow = append(s.isNullRow, true)
		s.rowStart = append(s.rowStart, s.used)
		return false
	}
	s.isNullRow = append(s.isNullRow, false)
	b := []byte(*value)
	truncated := false
	if len(b) > maxBytes {
		b = truncateUTF8ToBoundary(b, maxBytes)
		if s.truncatedRows == nil {
			s.truncatedRows = make(map[int]struct{})
		}
		s.truncatedRows[row] = struct{}{}
		truncated = true
	}
	s.grow(len(b))
	copy(s.buffer[s.used:], b)
	s.used += len(b)
	s.rowStart = append(s.rowStart, s.used)
	return truncated
}

func (s *columnScratch) finish(rowCount int, reversed bool) Chunk {
	nulls := make([]byte, bitsetBytes(rowCount))
	offsets := make([]uint32, rowCount+1)
	data := make([]byte, s.used)
	// Explicitly non-nil (make always returns non-nil, even at zero length): the FlatBuffers
	// `truncated` vector is (required) and must always be written, even at length zero — an
	// omitted field decodes as `null` on the TypeScript side, not an empty array (P11 schema note).
	truncated := make([]uint32, 0, len(s.truncatedRows))
	var binary []uint32

	cursor := 0
	for newRow := 0; newRow < rowCount; newRow++ {
		oldRow := newRow
		if reversed {
			oldRow = rowCount - 1 - newRow
		}
		start := s.rowStart[oldRow]
		end := s.rowStart[oldRow+1]
		length := end - start
		if length > 0 {
			copy(data[cursor:], s.buffer[start:end])
		}
		cursor += length
		offsets[newRow+1] = uint32(cursor)
		if s.isNullRow[oldRow] {
			nulls[newRow>>3] |= 1 << (newRow & 7)
		}
		if _, ok := s.truncatedRows[oldRow]; ok {
			truncated = append(truncated, uint32(newRow))
		}
		if _, ok := s.binaryRows[oldRow]; ok {
			binary = append(binary, uint32(newRow))
		}
	}
	// truncatedRows iterates the map above in newRow order already (the loop runs newRow
	// ascending), so the result is already sorted — no separate sort needed.
	return Chunk{Data: data, Offsets: offsets, Nulls: nulls, Truncated: truncated, Binary: binary}
}
