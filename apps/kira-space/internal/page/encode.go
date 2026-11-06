package page

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page/wire"
)

// hostIsLittleEndian is evaluated once at init. Every platform this app ships or is built on
// (macOS/Linux on amd64 or arm64) is little-endian; createUint32Vector's fast path only applies
// when this holds, so a hypothetical big-endian host stays correct via the slow path rather than
// silently miswriting the vector.
var hostIsLittleEndian = func() bool {
	var x uint16 = 1
	return *(*byte)(unsafe.Pointer(&x)) == 1
}()

// EncodePage writes p into b and returns the offset of the wire.Page table wrapping it. Callers
// must not have a table open on b — building is inner-to-outer, so every Chunk, string,
// ColumnDescriptor and PagePosition is finished before the page table starts, and the page table
// before the wire.Page wrapper.
func EncodePage(b *flatbuffers.Builder, p Page) flatbuffers.UOffsetT {
	var bodyType wire.PageBody
	var bodyOff flatbuffers.UOffsetT
	switch v := p.(type) {
	case TabularPage:
		bodyType, bodyOff = wire.PageBodyTabularPage, encodeTabularPage(b, v)
	case DocumentPage:
		bodyType, bodyOff = wire.PageBodyDocumentPage, encodeDocumentPage(b, v)
	case KeyValuePage:
		bodyType, bodyOff = wire.PageBodyKeyValuePage, encodeKeyValuePage(b, v)
	case StreamPage:
		bodyType, bodyOff = wire.PageBodyStreamPage, encodeStreamPage(b, v)
	default:
		panic(fmt.Sprintf("page: EncodePage: unhandled Page implementation %T", p))
	}
	wire.PageStart(b)
	wire.PageAddBodyType(b, bodyType)
	wire.PageAddBody(b, bodyOff)
	return wire.PageEnd(b)
}

func encodeTabularPage(b *flatbuffers.Builder, p TabularPage) flatbuffers.UOffsetT {
	columnOffs := make([]flatbuffers.UOffsetT, len(p.Columns))
	for i, c := range p.Columns {
		columnOffs[i] = encodeColumnDescriptor(b, c)
	}
	columnsOff := b.CreateVectorOfTables(columnOffs)

	chunkOffs := make([]flatbuffers.UOffsetT, len(p.Chunks))
	for i, c := range p.Chunks {
		chunkOffs[i] = encodeChunk(b, c)
	}
	chunksOff := b.CreateVectorOfTables(chunkOffs)

	positionOff := encodePagePosition(b, p.Position)

	wire.TabularPageStart(b)
	wire.TabularPageAddColumns(b, columnsOff)
	wire.TabularPageAddRowCount(b, int32(p.RowCount))
	wire.TabularPageAddChunks(b, chunksOff)
	wire.TabularPageAddPosition(b, positionOff)
	wire.TabularPageAddTruncatedCells(b, int32(p.TruncatedCells))
	wire.TabularPageAddByteSize(b, float64(p.ByteSize))
	wire.TabularPageAddFetchedAt(b, float64(p.FetchedAt))
	return wire.TabularPageEnd(b)
}

func encodeDocumentPage(b *flatbuffers.Builder, p DocumentPage) flatbuffers.UOffsetT {
	idsOff := encodeChunk(b, p.IDs)
	bodiesOff := encodeChunk(b, p.Bodies)
	positionOff := encodePagePosition(b, p.Position)

	wire.DocumentPageStart(b)
	wire.DocumentPageAddPosition(b, positionOff)
	wire.DocumentPageAddIds(b, idsOff)
	wire.DocumentPageAddBodies(b, bodiesOff)
	wire.DocumentPageAddRowCount(b, int32(p.RowCount))
	wire.DocumentPageAddByteSize(b, float64(p.ByteSize))
	wire.DocumentPageAddFetchedAt(b, float64(p.FetchedAt))
	return wire.DocumentPageEnd(b)
}

func encodeKeyValuePage(b *flatbuffers.Builder, p KeyValuePage) flatbuffers.UOffsetT {
	fieldsOff := encodeChunk(b, p.Fields)
	valuesOff := encodeChunk(b, p.Values)
	positionOff := encodePagePosition(b, p.Position)

	wire.KeyValuePageStart(b)
	wire.KeyValuePageAddPosition(b, positionOff)
	wire.KeyValuePageAddRedisType(b, encodeRedisType(p.RedisType))
	if p.TTLMs != nil {
		wire.KeyValuePageAddTtlMs(b, float64(*p.TTLMs))
	}
	if p.MemoryBytes != nil {
		wire.KeyValuePageAddMemoryBytes(b, float64(*p.MemoryBytes))
	}
	wire.KeyValuePageAddFields(b, fieldsOff)
	wire.KeyValuePageAddValues(b, valuesOff)
	wire.KeyValuePageAddRowCount(b, int32(p.RowCount))
	wire.KeyValuePageAddByteSize(b, float64(p.ByteSize))
	wire.KeyValuePageAddFetchedAt(b, float64(p.FetchedAt))
	return wire.KeyValuePageEnd(b)
}

func encodeStreamPage(b *flatbuffers.Builder, p StreamPage) flatbuffers.UOffsetT {
	keysOff := encodeChunk(b, p.Keys)
	headersOff := encodeChunk(b, p.Headers)
	attrsOff := encodeChunk(b, p.Attrs)
	timestampsOff := encodeChunk(b, p.Timestamps)
	bodiesOff := encodeChunk(b, p.Bodies)
	positionOff := encodePagePosition(b, p.Position)

	wire.StreamPageStart(b)
	wire.StreamPageAddPosition(b, positionOff)
	wire.StreamPageAddKeys(b, keysOff)
	wire.StreamPageAddHeaders(b, headersOff)
	wire.StreamPageAddAttrs(b, attrsOff)
	wire.StreamPageAddTimestamps(b, timestampsOff)
	wire.StreamPageAddBodies(b, bodiesOff)
	wire.StreamPageAddRowCount(b, int32(p.RowCount))
	wire.StreamPageAddByteSize(b, float64(p.ByteSize))
	wire.StreamPageAddFetchedAt(b, float64(p.FetchedAt))
	if p.VisibilityTimeoutSeconds != nil {
		wire.StreamPageAddVisibilityTimeoutSeconds(b, int32(*p.VisibilityTimeoutSeconds))
	}
	if p.MaxReceiveCount != nil {
		wire.StreamPageAddMaxReceiveCount(b, int32(*p.MaxReceiveCount))
	}
	return wire.StreamPageEnd(b)
}

// encodeChunk always writes all four vectors, even at length zero: offsets/truncated are
// `(required)` in the schema, and an omitted vector decodes as `null` on the TypeScript side, not
// an empty array (P11 schema note).
func encodeChunk(b *flatbuffers.Builder, c Chunk) flatbuffers.UOffsetT {
	dataOff := b.CreateByteVector(c.Data)
	offsetsOff := createUint32Vector(b, c.Offsets)
	nullsOff := b.CreateByteVector(c.Nulls)
	truncatedOff := createUint32Vector(b, c.Truncated)
	var binaryOff flatbuffers.UOffsetT
	if len(c.Binary) > 0 {
		binaryOff = createUint32Vector(b, c.Binary)
	}

	wire.ChunkStart(b)
	wire.ChunkAddData(b, dataOff)
	wire.ChunkAddOffsets(b, offsetsOff)
	wire.ChunkAddNulls(b, nullsOff)
	wire.ChunkAddTruncated(b, truncatedOff)
	if len(c.Binary) > 0 {
		wire.ChunkAddBinary(b, binaryOff)
	}
	return wire.ChunkEnd(b)
}

func encodeColumnDescriptor(b *flatbuffers.Builder, c ColumnDescriptor) flatbuffers.UOffsetT {
	nameOff := b.CreateString(c.Name)
	dataTypeOff := b.CreateString(c.DataType)

	wire.ColumnDescriptorStart(b)
	wire.ColumnDescriptorAddName(b, nameOff)
	wire.ColumnDescriptorAddDataType(b, dataTypeOff)
	wire.ColumnDescriptorAddTypeClass(b, encodeTypeClass(c.TypeClass))
	wire.ColumnDescriptorAddNullable(b, c.Nullable)
	wire.ColumnDescriptorAddIsPrimaryKey(b, c.IsPrimaryKey)
	wire.ColumnDescriptorAddGenerated(b, c.Generated)
	return wire.ColumnDescriptorEnd(b)
}

func encodePagePosition(b *flatbuffers.Builder, p PagePosition) flatbuffers.UOffsetT {
	var nextTokenOff, prevTokenOff flatbuffers.UOffsetT
	if p.NextToken != nil {
		nextTokenOff = b.CreateString(*p.NextToken)
	}
	if p.PrevToken != nil {
		prevTokenOff = b.CreateString(*p.PrevToken)
	}

	wire.PagePositionStart(b)
	if p.Offset != nil {
		wire.PagePositionAddOffset(b, int32(*p.Offset))
	}
	wire.PagePositionAddPageSize(b, int32(p.PageSize))
	wire.PagePositionAddHasMore(b, p.HasMore)
	if p.NextToken != nil {
		wire.PagePositionAddNextToken(b, nextTokenOff)
	}
	if p.PrevToken != nil {
		wire.PagePositionAddPrevToken(b, prevTokenOff)
	}
	wire.PagePositionAddStrategy(b, encodeStrategy(p.Strategy))
	wire.PagePositionAddTruncated(b, p.Truncated)
	return wire.PagePositionEnd(b)
}

// createUint32Vector writes v as a FlatBuffers [uint] vector — 4-byte aligned by construction,
// which is what lets the generated TypeScript hand back a zero-copy Uint32Array view.
//
// docs/PERF.md traced part of the Go-side encode regression to this vector: the generated
// Go runtime has a bulk CreateByteVector for []byte but nothing equivalent for []uint32, so the
// original implementation went through one PrependUint32 call per element (~400,000 calls at the
// widest fixture). On a little-endian host, v's own memory layout is already byte-identical
// to the wire format's little-endian uint32 vector (confirmed against encode.go's WriteUint32,
// which always writes least-significant-byte first regardless of host), so the fast path
// reinterprets v as a []byte with unsafe.Slice and reuses CreateByteVector's own single-copy
// Prep+copy — then patches CreateByteVector's length prefix, which counts bytes, into an element
// count, since the wire format's vector length is "how many uint32s" not "how many bytes".
func createUint32Vector(b *flatbuffers.Builder, v []uint32) flatbuffers.UOffsetT {
	if !hostIsLittleEndian || len(v) == 0 {
		return createUint32VectorSlow(b, v)
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&v[0])), len(v)*4)
	off := b.CreateByteVector(raw)
	pos := len(b.Bytes) - int(off)
	binary.LittleEndian.PutUint32(b.Bytes[pos:], uint32(len(v)))
	return off
}

func createUint32VectorSlow(b *flatbuffers.Builder, v []uint32) flatbuffers.UOffsetT {
	b.StartVector(4, len(v), 4)
	for i := len(v) - 1; i >= 0; i-- {
		b.PrependUint32(v[i])
	}
	return b.EndVector(len(v))
}

func encodeTypeClass(t TypeClass) wire.TypeClass {
	switch t {
	case TypeClassNumber:
		return wire.TypeClassnumber
	case TypeClassText:
		return wire.TypeClasstext
	case TypeClassBoolean:
		return wire.TypeClassboolean
	case TypeClassTemporal:
		return wire.TypeClasstemporal
	case TypeClassBinary:
		return wire.TypeClassbinary
	case TypeClassJSON:
		return wire.TypeClassjson
	case TypeClassOther:
		return wire.TypeClassother
	default:
		panic(fmt.Sprintf("page: encodeTypeClass: unknown TypeClass %q", t))
	}
}

func encodeStrategy(s string) wire.Strategy {
	switch s {
	case "keyset":
		return wire.Strategykeyset
	case "offset":
		return wire.Strategyoffset
	case "cursor":
		return wire.Strategycursor
	case "offsetWindow":
		return wire.StrategyoffsetWindow
	case "batch":
		return wire.Strategybatch
	default:
		panic(fmt.Sprintf("page: encodeStrategy: unknown Strategy %q", s))
	}
}

// encodeRedisType maps this package's plain "string"/"set" literals onto the wire enum's
// string_/set_ members — the schema suffixes those two (wire.fbs) to dodge the reserved-word
// collision that the plain names cause in generated TypeScript; the wire values these decode to
// are unaffected.
func encodeRedisType(s string) wire.RedisType {
	switch s {
	case "string":
		return wire.RedisTypestring_
	case "hash":
		return wire.RedisTypehash
	case "list":
		return wire.RedisTypelist
	case "set":
		return wire.RedisTypeset_
	case "zset":
		return wire.RedisTypezset
	case "stream":
		return wire.RedisTypestream
	case "object":
		return wire.RedisTypeobject
	default:
		panic(fmt.Sprintf("page: encodeRedisType: unknown RedisType %q", s))
	}
}
