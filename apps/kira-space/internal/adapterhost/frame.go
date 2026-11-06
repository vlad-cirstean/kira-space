package adapterhost

import (
	"fmt"

	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/enginecache"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page/wire"
)

// This file is dataframe.go's envelope: every response, error and event frame it sends is built
// here, through the generated FlatBuffers code, and finished with the "KIF1" file identifier
// (P11). Renderer -> Go frames are unaffected — they stay JSON text (P11 D3).

// encodePayload writes payload into b and returns its offset and union tag. payload is always one
// of the nine wire.Payload member types handleDataOp's own nine r.respond call sites produce
// (ReadResponse/CountResponse/PreviewResponse/MutateResponse/ExecuteResponse/
// ObjectDownloadResponse), plus pingPayload, enginecache.CacheStats, and struct{}{} for the two ops
// that answer empty. An unhandled payload type is a programming error and is reported as
// E_INTERNAL by the caller, never encoded.
func encodePayload(b *flatbuffers.Builder, payload any) (flatbuffers.UOffsetT, wire.Payload, error) {
	switch v := payload.(type) {
	case ReadResponse:
		pageOff := page.EncodePage(b, v.Page)
		wire.ReadResponseStart(b)
		wire.ReadResponseAddPage(b, pageOff)
		wire.ReadResponseAddSource(b, encodeSource(v.Source))
		return wire.ReadResponseEnd(b), wire.PayloadReadResponse, nil

	case CountResponse:
		wire.CountResponseStart(b)
		wire.CountResponseAddValue(b, float64(v.Value))
		wire.CountResponseAddExact(b, v.Exact)
		wire.CountResponseAddAt(b, float64(v.At))
		wire.CountResponseAddStale(b, v.Stale)
		wire.CountResponseAddSource(b, encodeSource(v.Source))
		return wire.CountResponseEnd(b), wire.PayloadCountResponse, nil

	case PreviewResponse:
		statementsOff := createStringVector(b, v.Statements)
		wire.PreviewResponseStart(b)
		wire.PreviewResponseAddStatements(b, statementsOff)
		return wire.PreviewResponseEnd(b), wire.PayloadPreviewResponse, nil

	case MutateResponse:
		wire.MutateResponseStart(b)
		wire.MutateResponseAddAffectedRows(b, int32(v.AffectedRows))
		return wire.MutateResponseEnd(b), wire.PayloadMutateResponse, nil

	case ExecuteResponse:
		pageOffs := make([]flatbuffers.UOffsetT, len(v.Pages))
		for i, p := range v.Pages {
			pageOffs[i] = page.EncodePage(b, p)
		}
		pagesOff := b.CreateVectorOfTables(pageOffs)
		wire.ExecuteResponseStart(b)
		wire.ExecuteResponseAddPages(b, pagesOff)
		return wire.ExecuteResponseEnd(b), wire.PayloadExecuteResponse, nil

	case ObjectDownloadResponse:
		wire.ObjectDownloadResponseStart(b)
		wire.ObjectDownloadResponseAddBytes(b, float64(v.Bytes))
		return wire.ObjectDownloadResponseEnd(b), wire.PayloadObjectDownloadResponse, nil

	case pingPayload:
		wire.PingPayloadStart(b)
		wire.PingPayloadAddPong(b, v.Pong)
		wire.PingPayloadAddEnginePid(b, int32(v.EnginePid))
		wire.PingPayloadAddAt(b, float64(v.At))
		return wire.PingPayloadEnd(b), wire.PayloadPingPayload, nil

	case enginecache.CacheStats:
		wire.CacheStatsStart(b)
		wire.CacheStatsAddL2Bytes(b, float64(v.L2Bytes))
		wire.CacheStatsAddL2BudgetBytes(b, float64(v.L2BudgetBytes))
		wire.CacheStatsAddL2Entries(b, int32(v.L2Entries))
		wire.CacheStatsAddL2Hits(b, int32(v.L2Hits))
		wire.CacheStatsAddL2Misses(b, int32(v.L2Misses))
		wire.CacheStatsAddL3Entries(b, int32(v.L3Entries))
		return wire.CacheStatsEnd(b), wire.PayloadCacheStats, nil

	case struct{}:
		wire.EmptyResponseStart(b)
		return wire.EmptyResponseEnd(b), wire.PayloadEmptyResponse, nil

	default:
		return 0, wire.PayloadNONE, fmt.Errorf("adapterhost: encodePayload: unhandled payload type %T", payload)
	}
}

// estimateFrameSize returns a starting buffer size for payload, close enough to the final wire
// size that flatbuffers.NewBuilder rarely has to grow-and-copy its backing buffer before the frame
// is done. docs/PERF.md traced a meaningful share of the Go-side encode regression to
// NewBuilder(0) starting from empty and doubling repeatedly up to the final size, paying a full
// copy of the buffer on every doubling. Page payloads dominate frame size, so only those get a
// size-aware estimate (page.Page.Size() is the same measured ByteSize the byteSize-preservation
// invariant checks); every other payload is small and fixed enough that a modest constant skips
// the first few doublings just as well.
func estimateFrameSize(payload any) int {
	switch v := payload.(type) {
	case ReadResponse:
		return pageSizeEstimate(v.Page.Size())
	case ExecuteResponse:
		total := 0
		for _, p := range v.Pages {
			total += p.Size()
		}
		return pageSizeEstimate(total)
	default:
		return 256
	}
}

// pageSizeEstimate pads a page's own measured byte size with room for the FlatBuffers table/
// vtable overhead around it — a few hundred fixed bytes plus a small fraction of the payload,
// converging toward zero at real page sizes (docs/PERF.md's wire-overhead numbers).
// Deliberately approximate: an undershoot costs one grow-and-copy pass, same as before this
// existed; this only needs to land close enough to skip most of them, not be exact.
func pageSizeEstimate(rawBytes int) int {
	return rawBytes + rawBytes/16 + 512
}

// encodeResponse builds a `res` frame answering id with payload.
func encodeResponse(id int, payload any) ([]byte, error) {
	b := flatbuffers.NewBuilder(estimateFrameSize(payload))
	payloadOff, payloadType, err := encodePayload(b, payload)
	if err != nil {
		return nil, err
	}

	wire.FrameStart(b)
	wire.FrameAddKind(b, wire.FrameKindres)
	wire.FrameAddId(b, int32(id))
	wire.FrameAddOk(b, true)
	wire.FrameAddPayloadType(b, payloadType)
	wire.FrameAddPayload(b, payloadOff)
	frameOff := wire.FrameEnd(b)
	wire.FinishFrameBuffer(b, frameOff)
	return b.FinishedBytes(), nil
}

// encodeError builds a `res`, !ok frame answering id with message/code. code may be empty.
func encodeError(id int, message, code string) []byte {
	b := flatbuffers.NewBuilder(0)
	messageOff := b.CreateString(message)
	var codeOff flatbuffers.UOffsetT
	if code != "" {
		codeOff = b.CreateString(code)
	}
	wire.ErrorStart(b)
	wire.ErrorAddMessage(b, messageOff)
	if code != "" {
		wire.ErrorAddCode(b, codeOff)
	}
	errOff := wire.ErrorEnd(b)

	wire.FrameStart(b)
	wire.FrameAddKind(b, wire.FrameKindres)
	wire.FrameAddId(b, int32(id))
	wire.FrameAddOk(b, false)
	wire.FrameAddError(b, errOff)
	frameOff := wire.FrameEnd(b)
	wire.FinishFrameBuffer(b, frameOff)
	return b.FinishedBytes()
}

// encodeEvent builds an `evt` frame — pushCacheStats's only caller.
func encodeEvent(topic string, payload any) ([]byte, error) {
	b := flatbuffers.NewBuilder(estimateFrameSize(payload))
	payloadOff, payloadType, err := encodePayload(b, payload)
	if err != nil {
		return nil, err
	}
	topicOff := b.CreateString(topic)

	wire.FrameStart(b)
	wire.FrameAddKind(b, wire.FrameKindevt)
	wire.FrameAddTopic(b, topicOff)
	wire.FrameAddPayloadType(b, payloadType)
	wire.FrameAddPayload(b, payloadOff)
	frameOff := wire.FrameEnd(b)
	wire.FinishFrameBuffer(b, frameOff)
	return b.FinishedBytes(), nil
}

// createStringVector writes strs as a FlatBuffers vector of strings.
func createStringVector(b *flatbuffers.Builder, strs []string) flatbuffers.UOffsetT {
	offs := make([]flatbuffers.UOffsetT, len(strs))
	for i, s := range strs {
		offs[i] = b.CreateString(s)
	}
	return b.CreateVectorOfTables(offs)
}

func encodeSource(s string) wire.Source {
	switch s {
	case "cache":
		return wire.Sourcecache
	case "server":
		return wire.Sourceserver
	default:
		panic(fmt.Sprintf("adapterhost: encodeSource: unknown Source %q", s))
	}
}
