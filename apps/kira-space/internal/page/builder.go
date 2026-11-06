package page

import (
	"fmt"
	"time"
)

// TypeClass mirrors page.ts's TypeClass.
type TypeClass string

const (
	TypeClassNumber   TypeClass = "number"
	TypeClassText     TypeClass = "text"
	TypeClassBoolean  TypeClass = "boolean"
	TypeClassTemporal TypeClass = "temporal"
	TypeClassBinary   TypeClass = "binary"
	TypeClassJSON     TypeClass = "json"
	TypeClassOther    TypeClass = "other"
)

// ColumnDescriptor mirrors page.ts's ColumnDescriptor.
type ColumnDescriptor struct {
	Name         string    `json:"name"`
	DataType     string    `json:"dataType"`
	TypeClass    TypeClass `json:"typeClass"`
	Nullable     bool      `json:"nullable"`
	IsPrimaryKey bool      `json:"isPrimaryKey"`
	Generated    bool      `json:"generated"`
}

// PagePosition mirrors page.ts's PagePosition.
type PagePosition struct {
	Offset    *int    `json:"offset"`
	PageSize  int     `json:"pageSize"`
	HasMore   bool    `json:"hasMore"`
	NextToken *string `json:"nextToken"`
	PrevToken *string `json:"prevToken"`
	Strategy  string  `json:"strategy"` // "keyset" | "offset" | "cursor" | "offsetWindow" | "batch"
	// Truncated: a console result stopped at the ResultCap with at least one more row available.
	Truncated bool `json:"truncated,omitempty"`
}

// ResultCap bounds one console result. Zero fields mean "no limit on that axis".
type ResultCap struct {
	Rows  int
	Bytes int
}

// Reached reports whether a result already holding rows rows and bytes cell bytes is at the cap.
func (c ResultCap) Reached(rows, bytes int) bool {
	return (c.Rows > 0 && rows >= c.Rows) || (c.Bytes > 0 && bytes >= c.Bytes)
}

// CappedPosition is UnpagedPosition plus the console cap's truncation flag.
func CappedPosition(rowCount int, truncated bool) PagePosition {
	p := UnpagedPosition(rowCount)
	p.Truncated = truncated
	return p
}

// UnpagedPosition is page.ts's unpagedPosition — a page that is the whole result.
func UnpagedPosition(rowCount int) PagePosition {
	offset := 0
	return PagePosition{Offset: &offset, PageSize: rowCount, HasMore: false, Strategy: "offset"}
}

// PageKind mirrors shared/caps.ts's PageKind (page.ts re-exports it for convenience; the Go
// package lives here instead, since adapters.Caps must import page anyway for the Page type, and
// page must not import adapters back).
type PageKind string

const (
	PageKindTabular  PageKind = "tabular"
	PageKindDocument PageKind = "document"
	PageKindKeyValue PageKind = "keyvalue"
	PageKindStream   PageKind = "stream"
)

// Page is any of TabularPage, DocumentPage, KeyValuePage, StreamPage — page.ts's Page union.
// Size and Rows, not ByteSize/RowCount, because a field of each of those names already exists on
// every concrete type and Go does not allow a method and a field to share a name.
// internal/enginecache needs the byte cost through the interface (Size); adapterhost's data
// dispatcher needs the row count the same way, to call OpCtx.SetRows the way every control.ts
// handler calls ctx.setRows(result.*.length) — JS's duck typing has no Go equivalent here.
type Page interface {
	PageKind() PageKind
	Size() int
	Rows() int
}

func nowEpochMs() int64 {
	return time.Now().UnixMilli()
}

// sumChunkBytes totals ChunkByteSize across chunks — DocumentPageBuilder/KeyValuePageBuilder/
// StreamPageBuilder.Finish (I2-45) each summed their own two-to-five chunks' byte sizes this way.
func sumChunkBytes(chunks ...Chunk) int {
	total := 0
	for _, c := range chunks {
		total += ChunkByteSize(c)
	}
	return total
}

// TabularPage mirrors page.ts's TabularPage.
type TabularPage struct {
	Kind           PageKind
	Columns        []ColumnDescriptor
	RowCount       int
	Chunks         []Chunk // index-aligned with Columns
	Position       PagePosition
	TruncatedCells int
	ByteSize       int // measured, not estimated — what L2 budgets against
	FetchedAt      int64
}

func (TabularPage) PageKind() PageKind { return PageKindTabular }
func (p TabularPage) Size() int        { return p.ByteSize }
func (p TabularPage) Rows() int        { return p.RowCount }

// columnEnvelopeBytesFor mirrors page.ts's pageByteSize UTF-16-length estimate: JS .length counts
// UTF-16 code units, which equals len([]rune(s)) for the entire BMP and differs only for
// astral-plane identifiers — an accepted, documented approximation (P58a's own decision).
func columnEnvelopeBytesFor(col ColumnDescriptor) int {
	return (len([]rune(col.Name))+len([]rune(col.DataType)))*2 + columnEnvelopeBytes
}

// PageByteSize ports page.ts's pageByteSize formula exactly, including the UTF-16 estimate — the
// L2 budget is only as honest as this number, and changing it would silently change eviction
// behaviour relative to what tests/unit/engine-cache.spec.ts's Go successor was written against.
func PageByteSize(p TabularPage) int {
	total := 0
	for i, chunk := range p.Chunks {
		total += ChunkByteSize(chunk) + columnEnvelopeBytesFor(p.Columns[i])
	}
	return total
}

// TabularPageBuilder mirrors page.ts's TabularPageBuilder.
type TabularPageBuilder struct {
	columns   []ColumnDescriptor
	scratches []*columnScratch
	rowCount  int
	reversed  bool
	truncated int
}

// NewTabularPageBuilder mirrors page.ts's createTabularPageBuilder.
func NewTabularPageBuilder(columns []ColumnDescriptor) *TabularPageBuilder {
	scratches := make([]*columnScratch, len(columns))
	for i := range scratches {
		scratches[i] = newColumnScratch()
	}
	return &TabularPageBuilder{columns: columns, scratches: scratches}
}

// AppendRow appends one row: one *string (nil for NULL) per column, in Columns order.
func (b *TabularPageBuilder) AppendRow(values []*string) error {
	if len(values) != len(b.columns) {
		return fmt.Errorf("page: row has %d values, expected %d columns", len(values), len(b.columns))
	}
	row := b.rowCount
	for i := range b.columns {
		if b.scratches[i].appendValue(values[i], row, MaxCellBytes) {
			b.truncated++
		}
	}
	b.rowCount++
	return nil
}

// RowCount is the number of rows appended so far.
func (b *TabularPageBuilder) RowCount() int { return b.rowCount }

// Bytes is the cell text bytes held so far, after per-cell truncation.
func (b *TabularPageBuilder) Bytes() int {
	total := 0
	for _, s := range b.scratches {
		total += s.used
	}
	return total
}

// SetColumns swaps the column descriptors, for a caller that streams rows before the final
// descriptors are known. Length must match the columns the builder was created with.
func (b *TabularPageBuilder) SetColumns(columns []ColumnDescriptor) error {
	if len(columns) != len(b.columns) {
		return fmt.Errorf("page: SetColumns got %d columns, builder has %d", len(columns), len(b.columns))
	}
	b.columns = columns
	return nil
}

// Reverse reverses the accumulated rows before Finish — used by a keyset 'before' page.
func (b *TabularPageBuilder) Reverse() { b.reversed = true }

// Finish builds the TabularPage; ByteSize is measured against the descriptors current at Finish.
func (b *TabularPageBuilder) Finish(position PagePosition) TabularPage {
	chunks := make([]Chunk, len(b.scratches))
	for i, s := range b.scratches {
		chunks[i] = s.finish(b.rowCount, b.reversed)
	}
	p := TabularPage{
		Kind:           PageKindTabular,
		Columns:        b.columns,
		RowCount:       b.rowCount,
		Chunks:         chunks,
		Position:       position,
		TruncatedCells: b.truncated,
		FetchedAt:      nowEpochMs(),
	}
	p.ByteSize = PageByteSize(p)
	return p
}

// DocumentPage mirrors page.ts's DocumentPage.
type DocumentPage struct {
	Kind      PageKind
	Position  PagePosition
	IDs       Chunk
	Bodies    Chunk
	RowCount  int
	ByteSize  int
	FetchedAt int64
}

func (DocumentPage) PageKind() PageKind { return PageKindDocument }
func (p DocumentPage) Size() int        { return p.ByteSize }
func (p DocumentPage) Rows() int        { return p.RowCount }

// DocumentPageBuilder mirrors page.ts's DocumentPageBuilder.
type DocumentPageBuilder struct {
	maxBytes int
	ids      *columnScratch
	bodies   *columnScratch
	rowCount int
}

// NewDocumentPageBuilder mirrors page.ts's createDocumentPageBuilder.
func NewDocumentPageBuilder(singleRow bool) *DocumentPageBuilder {
	maxBytes := MaxCellBytes
	if singleRow {
		maxBytes = SingleRowMaxBytes
	}
	return &DocumentPageBuilder{maxBytes: maxBytes, ids: newColumnScratch(), bodies: newColumnScratch()}
}

// Push appends one document: id and body are each pre-serialized EJSON text.
func (b *DocumentPageBuilder) Push(id, body string) {
	row := b.rowCount
	b.ids.appendValue(&id, row, b.maxBytes)
	b.bodies.appendValue(&body, row, b.maxBytes)
	b.rowCount++
}

// RowCount is the number of documents pushed so far.
func (b *DocumentPageBuilder) RowCount() int { return b.rowCount }

// Bytes is the cell text bytes held so far.
func (b *DocumentPageBuilder) Bytes() int { return b.ids.used + b.bodies.used }

func (b *DocumentPageBuilder) Finish(position PagePosition) DocumentPage {
	ids := b.ids.finish(b.rowCount, false)
	bodies := b.bodies.finish(b.rowCount, false)
	return DocumentPage{
		Kind: PageKindDocument, Position: position, IDs: ids, Bodies: bodies, RowCount: b.rowCount,
		ByteSize: sumChunkBytes(ids, bodies), FetchedAt: nowEpochMs(),
	}
}

// KeyValuePage mirrors page.ts's KeyValuePage.
type KeyValuePage struct {
	Kind        PageKind
	Position    PagePosition
	RedisType   string // "string" | "hash" | "list" | "set" | "zset" | "stream" | "object"
	TTLMs       *int64
	MemoryBytes *int64
	Fields      Chunk
	Values      Chunk
	RowCount    int
	ByteSize    int
	FetchedAt   int64
	// FieldsAreColumns reports whether Fields carries genuine per-value identifiers a mask rule's
	// column_name could legitimately match (a hash field, a string key's fixed "value" name, an S3
	// metadata key) — true by default. False for a shape with no real per-entry field concept (a
	// Redis list/set's synthetic display index, a stream entry id, a generic console reply with no
	// per-command shape): dbmcp's own renderKeyValuePage refuses to apply mask
	// rules to such a page rather than silently pass a real value through under a Field no rule
	// could ever legitimately match by construction.
	FieldsAreColumns bool
}

func (KeyValuePage) PageKind() PageKind { return PageKindKeyValue }
func (p KeyValuePage) Size() int        { return p.ByteSize }
func (p KeyValuePage) Rows() int        { return p.RowCount }

// KeyValuePageBuilder mirrors page.ts's KeyValuePageBuilder.
type KeyValuePageBuilder struct {
	redisType        string
	ttlMs            *int64
	memoryBytes      *int64
	valueMaxBytes    int
	fields           *columnScratch
	values           *columnScratch
	rowCount         int
	fieldsAreColumns bool
}

// NewKeyValuePageBuilder mirrors page.ts's createKeyValuePageBuilder.
func NewKeyValuePageBuilder(redisType string, ttlMs, memoryBytes *int64, singleRow bool) *KeyValuePageBuilder {
	valueMaxBytes := MaxCellBytes
	if singleRow {
		valueMaxBytes = SingleRowMaxBytes
	}
	return &KeyValuePageBuilder{
		redisType: redisType, ttlMs: ttlMs, memoryBytes: memoryBytes, valueMaxBytes: valueMaxBytes,
		fields: newColumnScratch(), values: newColumnScratch(),
		fieldsAreColumns: true, // default: Field values are genuine, mask-rule-matchable identifiers
	}
}

// SetFieldsAreColumns overrides the default (see KeyValuePage.FieldsAreColumns's own doc comment)
// — call with false when this page's Field values are synthetic display artifacts rather than a
// genuine per-value identifier.
func (b *KeyValuePageBuilder) SetFieldsAreColumns(v bool) *KeyValuePageBuilder {
	b.fieldsAreColumns = v
	return b
}

// RowCount is the number of entries pushed so far.
func (b *KeyValuePageBuilder) RowCount() int { return b.rowCount }

// Bytes is the cell text bytes held so far.
func (b *KeyValuePageBuilder) Bytes() int { return b.fields.used + b.values.used }

func (b *KeyValuePageBuilder) Push(field, value string) {
	row := b.rowCount
	b.fields.appendValue(&field, row, MaxCellBytes)
	b.values.appendValue(&value, row, b.valueMaxBytes)
	b.rowCount++
}

func (b *KeyValuePageBuilder) Finish(position PagePosition) KeyValuePage {
	fields := b.fields.finish(b.rowCount, false)
	values := b.values.finish(b.rowCount, false)
	return KeyValuePage{
		Kind: PageKindKeyValue, Position: position, RedisType: b.redisType, TTLMs: b.ttlMs, MemoryBytes: b.memoryBytes,
		Fields: fields, Values: values, RowCount: b.rowCount,
		ByteSize: sumChunkBytes(fields, values), FetchedAt: nowEpochMs(),
		FieldsAreColumns: b.fieldsAreColumns,
	}
}

// StreamPage mirrors page.ts's StreamPage.
type StreamPage struct {
	Kind                     PageKind
	Position                 PagePosition
	Keys                     Chunk
	Headers                  Chunk
	Attrs                    Chunk
	Timestamps               Chunk
	Bodies                   Chunk
	RowCount                 int
	ByteSize                 int
	FetchedAt                int64
	VisibilityTimeoutSeconds *int
	// MaxReceiveCount is the SQS redrive policy limit; nil when the queue has none (or Kafka).
	MaxReceiveCount *int
}

func (StreamPage) PageKind() PageKind { return PageKindStream }
func (p StreamPage) Size() int        { return p.ByteSize }
func (p StreamPage) Rows() int        { return p.RowCount }

// StreamRow is one row pushed into a StreamPageBuilder.
type StreamRow struct {
	Key       *string
	Headers   string
	Attrs     string
	Timestamp *string
	// Body nil is a null cell (a Kafka tombstone), distinct from "".
	Body *string
	// KeyBinary/BodyBinary: the cell text is base64 of non-UTF-8 bytes (see BytesCell).
	KeyBinary, BodyBinary bool
}

// StreamPageBuilder mirrors page.ts's StreamPageBuilder.
type StreamPageBuilder struct {
	visibilityTimeoutSeconds                 *int
	maxReceiveCount                          *int
	keys, headers, attrs, timestamps, bodies *columnScratch
	rowCount                                 int
}

// NewStreamPageBuilder mirrors page.ts's createStreamPageBuilder.
func NewStreamPageBuilder(visibilityTimeoutSeconds *int) *StreamPageBuilder {
	return &StreamPageBuilder{
		visibilityTimeoutSeconds: visibilityTimeoutSeconds,
		keys:                     newColumnScratch(), headers: newColumnScratch(), attrs: newColumnScratch(),
		timestamps: newColumnScratch(), bodies: newColumnScratch(),
	}
}

// SetMaxReceiveCount records the SQS redrive limit on the finished page.
func (b *StreamPageBuilder) SetMaxReceiveCount(n *int) { b.maxReceiveCount = n }

func (b *StreamPageBuilder) Push(row StreamRow) {
	i := b.rowCount
	b.keys.appendValue(row.Key, i, MaxCellBytes)
	if row.KeyBinary {
		b.keys.markBinary(i)
	}
	headers := row.Headers
	b.headers.appendValue(&headers, i, MaxCellBytes)
	attrs := row.Attrs
	b.attrs.appendValue(&attrs, i, MaxCellBytes)
	b.timestamps.appendValue(row.Timestamp, i, MaxCellBytes)
	b.bodies.appendValue(row.Body, i, MaxCellBytes)
	if row.BodyBinary {
		b.bodies.markBinary(i)
	}
	b.rowCount++
}

func (b *StreamPageBuilder) Finish(position PagePosition) StreamPage {
	keys := b.keys.finish(b.rowCount, false)
	headers := b.headers.finish(b.rowCount, false)
	attrs := b.attrs.finish(b.rowCount, false)
	timestamps := b.timestamps.finish(b.rowCount, false)
	bodies := b.bodies.finish(b.rowCount, false)
	return StreamPage{
		Kind: PageKindStream, Position: position, Keys: keys, Headers: headers, Attrs: attrs, Timestamps: timestamps,
		Bodies: bodies, RowCount: b.rowCount,
		ByteSize:  sumChunkBytes(keys, headers, attrs, timestamps, bodies),
		FetchedAt: nowEpochMs(), VisibilityTimeoutSeconds: b.visibilityTimeoutSeconds, MaxReceiveCount: b.maxReceiveCount,
	}
}
