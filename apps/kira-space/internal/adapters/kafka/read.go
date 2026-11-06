package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

const (
	pollTimeout   = 1 * time.Second // read.ts's POLL_TIMEOUT_MS, ported as a per-round bound (see readTopic)
	maxEmptyPolls = 2               // read.ts's MAX_EMPTY_POLLS
)

// partitionWindow is read.ts's PartitionWindow, with int64 offsets rather than decimal strings
// (Go's int64 has no JS-number-precision problem, so toNativeOffset's safe-integer guard has no
// equivalent here). Field names/tags match read.ts's own naming (partition/next/end) per the
// diffability rule — this struct's own shape is the page token's payload.
type partitionWindow struct {
	Partition int32 `json:"partition"`
	Next      int64 `json:"next"`
	End       int64 `json:"end"` // frozen high watermark for this browse — never re-fetched mid-browse (P10 D6/D7)
}

// kafkaStreamFilter mirrors packages/shared/domain/streamFilter.ts's KafkaStreamFilter wire shape.
type kafkaStreamFilter struct {
	Offset      *string `json:"offset"`
	Partitions  []int32 `json:"partitions"`
	TimestampMs *int64  `json:"timestampMs"`
}

func parseStreamFilter(raw *string) (kafkaStreamFilter, error) {
	if raw == nil {
		return kafkaStreamFilter{}, nil
	}
	var f kafkaStreamFilter
	if err := json.Unmarshal([]byte(*raw), &f); err != nil {
		return kafkaStreamFilter{}, err
	}
	return f, nil
}

// readFingerprintParts is requestFingerprint({topic, pageSize, filter})'s Go shape (read.ts:205) —
// the fingerprint covers only these three, deliberately. Field names/tags/order must
// match kafka_test.go scenario 20's own reconstruction exactly, since RequestFingerprint hashes
// the marshalled JSON bytes.
type readFingerprintParts struct {
	Topic    string  `json:"topic"`
	PageSize int     `json:"pageSize"`
	Filter   *string `json:"filter"`
}

// headersToPlain is read.ts's headersToPlain, folded onto franz-go's flat, ordered
// []RecordHeader ({Key string; Value []byte}) — closer to librdkafka's own [{k:v}] array than to
// kafkajs's Record, so the repeated-key promotion (a second occurrence of a header name becomes a
// []string) ports unchanged in intent (P58e E8). Each value passes through page.BytesCell:
// raw broker bytes carry no encoding guarantee, unlike Node's Buffer.toString('utf8'), which
// replaces invalid sequences invisibly. A non-UTF-8 value becomes {"base64": "..."} (page.BytesCell's
// encoding) instead of a replacement-character string.
func headersToPlain(headers []kgo.RecordHeader) map[string]any {
	out := map[string]any{}
	for _, h := range headers {
		// A nil header value stays JSON null, distinct from an empty one.
		var value any
		if h.Value != nil {
			text, binary := page.BytesCell(h.Value)
			if binary {
				value = map[string]string{"base64": text}
			} else {
				value = text
			}
		}
		if existing, seen := out[h.Key]; !seen {
			out[h.Key] = value
		} else if list, ok := existing.([]any); ok {
			out[h.Key] = append(list, value)
		} else {
			out[h.Key] = []any{existing, value}
		}
	}
	return out
}

// buildStreamRow is read.ts:281-291's row construction, with P58e E8 applied.
func buildStreamRow(rec *kgo.Record) (page.StreamRow, error) {
	var key *string
	var keyBinary bool
	if rec.Key != nil {
		k, binary := page.BytesCell(rec.Key)
		key, keyBinary = &k, binary
	}
	headersJSON, err := json.Marshal(headersToPlain(rec.Headers))
	if err != nil {
		return page.StreamRow{}, err
	}
	// The number/string asymmetry ports verbatim (read.ts:288): partition is a JSON number,
	// offset a JSON string — kafka.spec.ts 7's own assertion, and a Go port that "tidies" offset
	// into a number would break a cell users read.
	attrsJSON, err := json.Marshal(map[string]any{
		"partition": rec.Partition,
		"offset":    strconv.FormatInt(rec.Offset, 10),
	})
	if err != nil {
		return page.StreamRow{}, err
	}
	var timestamp *string
	if !rec.Timestamp.IsZero() {
		// P58d D11's exact-three-fractional-digits format — never time.RFC3339Nano, which drops
		// trailing zeros.
		t := rec.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		timestamp = &t
	}
	var body *string
	var bodyBinary bool
	if rec.Value != nil {
		v, binary := page.BytesCell(rec.Value)
		body, bodyBinary = &v, binary
	}
	return page.StreamRow{
		Key: key, Headers: string(headersJSON), Attrs: string(attrsJSON), Timestamp: timestamp, Body: body,
		KeyBinary: keyBinary, BodyBinary: bodyBinary,
	}, nil
}

func position(windows []partitionWindow, hasMore bool, fingerprint string, pageSize int) page.PagePosition {
	var nextToken *string
	if hasMore {
		raw, err := json.Marshal(windows)
		if err != nil {
			panic(err) // partitionWindow is a plain struct of ints; cannot fail
		}
		token := adapters.EncodePageToken([]string{string(raw)}, fingerprint)
		nextToken = &token
	}
	return page.PagePosition{Offset: nil, PageSize: pageSize, HasMore: hasMore, NextToken: nextToken, PrevToken: nil, Strategy: "offsetWindow"}
}

// filterPartitions is freshWindows' own partition-filter block: "any of these partitions" — a
// union, not an intersection.
func filterPartitions(partitions []int32, filter kafkaStreamFilter, topic string) ([]int32, error) {
	if len(filter.Partitions) == 0 {
		return partitions, nil
	}
	wanted := make(map[int32]bool, len(filter.Partitions))
	for _, p := range filter.Partitions {
		wanted[p] = true
	}
	selected := make([]int32, 0, len(partitions))
	for _, p := range partitions {
		if wanted[p] {
			selected = append(selected, p)
		}
	}
	if len(selected) == 0 {
		names := make([]string, len(filter.Partitions))
		for i, p := range filter.Partitions {
			names[i] = strconv.FormatInt(int64(p), 10)
		}
		return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.kafkaPartitionsMissing.body", map[string]string{"topic": topic, "partitions": strings.Join(names, ", ")}, nil)
	}
	return selected, nil
}

// resolveStartOffsets is freshWindows' own switch on filter.TimestampMs/filter.Offset: each
// partition's starting offset defaults to startsByPartition's own, adjusted per the filter when
// one applies.
func resolveStartOffsets(ctx context.Context, adm *kadm.Client, topic string, partitions []int32, startsByPartition, endsByPartition map[int32]kadm.ListedOffset, filter kafkaStreamFilter) (map[int32]int64, error) {
	start := make(map[int32]int64, len(partitions))
	for _, p := range partitions {
		start[p] = startsByPartition[p].Offset
	}

	switch {
	case filter.TimestampMs != nil:
		// kadm's own doc: "if a partition has no offsets after the requested millisecond, the
		// offset will be the current end offset" — a different answer from librdkafka's -1
		// sentinel (KF-4(c)), re-baselined in scenario 19 rather than guessed.
		byTs, err := adm.ListOffsetsAfterMilli(ctx, *filter.TimestampMs, topic)
		if err != nil {
			return nil, mapError(err)
		}
		if err := byTs.Error(); err != nil {
			return nil, mapError(err)
		}
		if err := adapters.CheckCancelled(ctx); err != nil {
			return nil, err
		}
		for p, lo := range byTs[topic] {
			if _, ok := start[p]; ok {
				start[p] = lo.Offset
			}
		}
	case filter.Offset != nil:
		requested, err := strconv.ParseInt(*filter.Offset, 10, 64)
		if err != nil {
			return nil, adapters.T(adapters.CodeQuery, "backend.studio.adapter.kafkaOffsetFilterMalformed.body", map[string]string{"filter": *filter.Offset}, nil)
		}
		for _, p := range partitions {
			lo := startsByPartition[p].Offset
			hi := endsByPartition[p].Offset
			clamped := requested
			if requested < lo {
				clamped = lo
			} else if requested > hi {
				clamped = hi
			}
			start[p] = clamped
		}
	}
	return start, nil
}

// freshWindows is read.ts's freshWindows (:76-143), only ever consulted for a fresh browse — a
// token-continued page's windows were already resolved once, and re-applying the filter there
// would just be wrong once the user has paged partway through.
func freshWindows(ctx context.Context, adm *kadm.Client, topic string, rawFilter *string) ([]partitionWindow, error) {
	// P58e E12: a nonexistent topic surfaces inside ListedOffsets' own per-partition Err, not as a
	// returned error — checking .Error() after each call is what turns it into E_QUERY instead of
	// a silently empty window set.
	starts, err := adm.ListStartOffsets(ctx, topic)
	if err != nil {
		return nil, mapError(err)
	}
	if err := starts.Error(); err != nil {
		return nil, mapError(err)
	}
	ends, err := adm.ListEndOffsets(ctx, topic)
	if err != nil {
		return nil, mapError(err)
	}
	if err := ends.Error(); err != nil {
		return nil, mapError(err)
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return nil, err
	}

	filter, err := parseStreamFilter(rawFilter)
	if err != nil {
		return nil, adapters.Contract("malformed stream filter", err)
	}

	endsByPartition := ends[topic]
	startsByPartition := starts[topic]
	partitions := make([]int32, 0, len(endsByPartition))
	for p := range endsByPartition {
		partitions = append(partitions, p)
	}
	sort.Slice(partitions, func(i, j int) bool { return partitions[i] < partitions[j] })

	partitions, err = filterPartitions(partitions, filter, topic)
	if err != nil {
		return nil, err
	}

	start, err := resolveStartOffsets(ctx, adm, topic, partitions, startsByPartition, endsByPartition, filter)
	if err != nil {
		return nil, err
	}

	windows := make([]partitionWindow, len(partitions))
	for i, p := range partitions {
		windows[i] = partitionWindow{Partition: p, Next: start[p], End: endsByPartition[p].Offset}
	}
	return windows, nil
}

// advanceWindows applies the end-of-log clamp for one poll round (unit tested in read_test.go).
// touched carries the HighWatermark this SAME round reported for every partition the fetch
// mentioned; recordsThisRound carries how many records this SAME round's own fetches.Records() held
// for each partition, counted before any page-budget cutoff — a partition is provably drained —
// its remaining [Next, End) gap can only ever be non-data offsets (a transaction's commit marker,
// a compacted offset, or one aged out by retention) that will never arrive as a record — only when
// THIS round delivered/skipped zero records for it (not merely "the round as a whole wasn't
// page-capped": franz-go caps each partition's own fetch at 1 MiB per round (config.go's
// maxPartBytes) independent of the overall page budget, so a partition can have real, un-fetched
// data left in [Next, End) even in a round that, as a whole, came in under the page budget — the
// previous "round wasn't page-capped" test alone treated "the broker had less of *this* partition
// to send this specific round" as "this partition is drained", silently truncating the browse),
// and the round's own reported watermark for that partition has reached or passed its frozen End.
// This is evaluated after every delivering round, never after a follow-up "peek" poll: once a
// partition is genuinely exhausted, a subsequent poll blocks for the caller's entire remaining
// context with no partition metadata to peek at, so the clamp signal has to come from the fetch
// that delivered data.
func advanceWindows(windows []partitionWindow, touched map[int32]int64, recordsThisRound map[int32]int) {
	for i := range windows {
		w := &windows[i]
		if w.Next >= w.End {
			continue
		}
		if recordsThisRound[w.Partition] > 0 {
			continue
		}
		if hw, ok := touched[w.Partition]; ok && hw >= w.End {
			w.Next = w.End
		}
	}
}

// prepareWindows resolves readTopic's starting windows — decoded from an "after" cursor's page
// token, or freshly computed via freshWindows for a fresh browse — and remaining, the subset of
// those windows that still have data left ([Next, End) non-empty).
func prepareWindows(ctx context.Context, adm *kadm.Client, topic string, req adapters.ReadRequest, fingerprint string) (windows, remaining []partitionWindow, err error) {
	if req.Cursor.Mode == "after" {
		keys, err := adapters.DecodePageToken(req.Cursor.Token, fingerprint)
		if err != nil {
			return nil, nil, err
		}
		if len(keys) != 1 {
			return nil, nil, adapters.Contract("malformed page token", nil)
		}
		if err := json.Unmarshal([]byte(keys[0]), &windows); err != nil {
			return nil, nil, adapters.Contract("malformed page token", err)
		}
	} else {
		w, err := freshWindows(ctx, adm, topic, req.Filter)
		if err != nil {
			return nil, nil, err
		}
		windows = w
	}

	for _, w := range windows {
		if w.Next < w.End {
			remaining = append(remaining, w)
		}
	}
	return windows, remaining, nil
}

// openBrowseClient builds readTopic's own ephemeral browse client: a
// kgo.ConsumePartitions client anchored at each remaining window's own Next offset — never
// kgo.ConsumeTopics or kgo.ConsumerGroup, which is what makes the no-group promise
// structural rather than disciplinary.
func openBrowseClient(baseOpts []kgo.Opt, topic string, remaining []partitionWindow) (*kgo.Client, error) {
	partitionOffsets := make(map[int32]kgo.Offset, len(remaining))
	for _, w := range remaining {
		partitionOffsets[w.Partition] = kgo.NewOffset().At(w.Next)
	}
	browseOpts := append(append([]kgo.Opt{}, baseOpts...),
		kgo.ConsumePartitions(map[string]map[int32]kgo.Offset{topic: partitionOffsets}),
		kgo.FetchMaxWait(pollTimeout))
	browse, err := kgo.NewClient(browseOpts...)
	if err != nil {
		return nil, mapError(err)
	}
	return browse, nil
}

// pollRoundResult is pollRound's own verdict on what readTopic's poll loop should do next: keep
// polling, or stop — and if it stopped because MAX_EMPTY_POLLS was reached, readTopic still has to
// set exhaustedByEmptyPolls itself (pollRoundExhausted only reports the fact; the latch it drives
// stays in readTopic, see readTopic's own comment).
type pollRoundResult int

const (
	pollRoundContinue pollRoundResult = iota
	pollRoundStop
	pollRoundExhausted
)

// pollRound runs one PollRecords round of readTopic's browse loop: polls under a per-round bound
// (see readTopic's own comment on why), classifies the outcome, and — for a successful poll —
// pushes whatever new rows it delivered into builder and applies the advanceWindows
// clamp. emptyPolls is the running MAX_EMPTY_POLLS counter, carried across rounds by the caller.
// collected is passed and returned by value rather than by pointer so the caller's own loop
// condition (`collected < req.PageSize`) always reads a value it assigned itself.
func pollRound(ctx context.Context, browse *kgo.Client, windows []partitionWindow, cursor map[int32]*partitionWindow, builder *page.StreamPageBuilder, req adapters.ReadRequest, collected int, emptyPolls *int) (int, pollRoundResult, error) {
	// A per-round bound, not the raw op ctx — see readTopic's own comment. Mirrors read.ts's own
	// consumer.setDefaultConsumeTimeout(POLL_TIMEOUT_MS).
	roundCtx, cancel := context.WithTimeout(ctx, pollTimeout)
	fetches := browse.PollRecords(roundCtx, req.PageSize-collected)
	cancel()

	if err := fetches.Err(); err != nil {
		if ctx.Err() != nil {
			// The op's own context is what ended this, not the round's bound (P58e E3).
			return collected, pollRoundStop, adapters.T(adapters.CodeCancelled, "backend.studio.adapter.cancelled.body", nil, ctx.Err())
		}
		if errors.Is(err, kgo.ErrClientClosed) {
			return collected, pollRoundStop, nil // the browse client's own teardown racing this poll
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			// Just this round's own bound expiring with genuinely nothing to report (KF-3) —
			// indistinguishable from a slow broker, so it counts toward MAX_EMPTY_POLLS rather
			// than failing the op.
			*emptyPolls++
			if *emptyPolls >= maxEmptyPolls {
				return collected, pollRoundExhausted, nil
			}
			return collected, pollRoundContinue, nil
		}
		return collected, pollRoundStop, mapError(err)
	}

	touched := make(map[int32]int64)
	fetches.EachPartition(func(fp kgo.FetchTopicPartition) {
		touched[fp.Partition] = fp.HighWatermark
	})

	recordsThisRound := make(map[int32]int)

	if fetches.NumRecords() == 0 {
		*emptyPolls++
	} else {
		*emptyPolls = 0
		// Counted over every record this round delivered, before the page-budget
		// break below stops processing them — advanceWindows' own "this partition delivered zero
		// records this round" signal must reflect what the broker sent, not how much of
		// that this round's own page cap left time to consume.
		records := fetches.Records()
		for _, rec := range records {
			recordsThisRound[rec.Partition]++
		}
		for _, rec := range records {
			if collected >= req.PageSize {
				break
			}
			w, ok := cursor[rec.Partition]
			if !ok || rec.Offset < w.Next || rec.Offset >= w.End {
				continue
			}
			row, err := buildStreamRow(rec)
			if err != nil {
				return collected, pollRoundStop, mapError(err)
			}
			builder.Push(row)
			collected++
			w.Next = rec.Offset + 1
		}
	}

	advanceWindows(windows, touched, recordsThisRound)

	if fetches.NumRecords() == 0 && *emptyPolls >= maxEmptyPolls {
		return collected, pollRoundExhausted, nil
	}
	return collected, pollRoundContinue, nil
}

// buildPartitionCursor indexes windows by partition for readTopic's poll loop to update in place
// as each partition's Next offset advances.
func buildPartitionCursor(windows []partitionWindow) map[int32]*partitionWindow {
	cursor := make(map[int32]*partitionWindow, len(windows))
	for i := range windows {
		cursor[windows[i].Partition] = &windows[i]
	}
	return cursor
}

// windowsHaveMore reports whether any window still has data left ([Next, End) non-empty).
func windowsHaveMore(windows []partitionWindow) bool {
	for _, w := range windows {
		if w.Next < w.End {
			return true
		}
	}
	return false
}

// clampExhaustedWindows sets every window still short of its frozen End to End — the mechanical
// half of P21 round 3 finding 3's fix; readTopic itself keeps the exhaustedByEmptyPolls decision of
// *when* to call this.
func clampExhaustedWindows(windows []partitionWindow) {
	for i := range windows {
		if windows[i].Next < windows[i].End {
			windows[i].Next = windows[i].End
		}
	}
}

// readTopic is read.ts's readTopic (:193-319). The browse client is a fresh, ephemeral
// kgo.Client per call, built from the adapter's own baseOpts plus kgo.ConsumePartitions at exact
// offsets — never kgo.ConsumeTopics or kgo.ConsumerGroup, which is what makes the no-group promise
// structural rather than disciplinary. ctx goes straight to every kadm/kgo call;
// each poll additionally runs on its own bounded sub-context (see the loop below) because
// PollRecords, given nothing new to deliver, does not return on its own — it keeps retrying
// internally against FetchMaxWait until data arrives or its context ends, so an unbounded
// per-round context would hang the whole op on a genuinely-exhausted-with-a-gap topic.
func readTopic(ctx context.Context, adm *kadm.Client, baseOpts []kgo.Opt, topic string, req adapters.ReadRequest, op *adapters.OpCtx) (page.StreamPage, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return page.StreamPage{}, err
	}
	if req.Cursor.Mode == "before" {
		return page.StreamPage{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.kafkaForwardOnly.body", nil, nil)
	}
	fingerprint := adapters.RequestFingerprint(readFingerprintParts{Topic: topic, PageSize: req.PageSize, Filter: req.Filter})

	windows, remaining, err := prepareWindows(ctx, adm, topic, req, fingerprint)
	if err != nil {
		return page.StreamPage{}, err
	}
	if len(remaining) == 0 {
		// No client is ever constructed (read.ts:212-215).
		builder := page.NewStreamPageBuilder(nil)
		return builder.Finish(position(windows, false, fingerprint, req.PageSize)), nil
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return page.StreamPage{}, err
	}

	op.SetCommand(fmt.Sprintf("browse %s (%d partition(s) of %d)", topic, len(remaining), len(windows)))

	browse, err := openBrowseClient(baseOpts, topic, remaining)
	if err != nil {
		return page.StreamPage{}, err
	}
	defer browse.Close()

	cursor := buildPartitionCursor(windows)

	builder := page.NewStreamPageBuilder(nil)
	collected := 0
	emptyPolls := 0
	// P21 round 3 functional finding 3: set when the loop below ends because maxEmptyPolls
	// consecutive rounds against the *remaining* windows' own offsets came back with nothing —
	// never because the page filled or every window's own touched watermark already cleared it.
	// advanceWindows' own clamp only fires on a round whose fetches.EachPartition actually
	// reported a watermark, which a round that failed with context.DeadlineExceeded (the "genuinely
	// nothing to report" case just below) never does — fetches carries no per-partition metadata
	// at all when PollRecords returns with only a context error, so `touched` stays empty for that
	// round and advanceWindows(windows, touched, false) clamps nothing. A partition whose last data
	// offset is a transaction commit marker, a compacted offset, or one aged out by retention hits
	// exactly this: every subsequent poll times out with nothing to report, `hasMore` latches true
	// forever, and the caller sees Next stay enabled with each click returning a 0-row page after a
	// ~2s stall. Once polling has genuinely been retried maxEmptyPolls times against these exact
	// offsets with nothing to show for it, the remaining [Next, End) gap is exhausted regardless of
	// whether any round ever reported a fresh watermark.
	exhaustedByEmptyPolls := false

pollLoop:
	for collected < req.PageSize && windowsHaveMore(windows) {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return page.StreamPage{}, err
		}

		newCollected, result, roundErr := pollRound(ctx, browse, windows, cursor, builder, req, collected, &emptyPolls)
		collected = newCollected
		if roundErr != nil {
			return page.StreamPage{}, roundErr
		}
		switch result {
		case pollRoundStop:
			break pollLoop // the browse client's own teardown racing this poll — loop end, not an error
		case pollRoundExhausted:
			exhaustedByEmptyPolls = true
			break pollLoop
		}
	}

	if err := adapters.CheckCancelled(ctx); err != nil {
		return page.StreamPage{}, err
	}

	// advanceWindows' own per-round clamp never fires for a
	// window whose remaining gap is entirely non-data offsets *and* every round against it failed
	// outright (DeadlineExceeded, no watermark reported) rather than succeeding empty — see the
	// exhaustedByEmptyPolls comment above. Having now genuinely retried maxEmptyPolls times against
	// these exact offsets with nothing delivered, every window still short of its frozen End is
	// exhausted; clamping here (rather than leaving hasMore latched true) is what stops Next from
	// returning an empty page forever.
	if exhaustedByEmptyPolls {
		clampExhaustedWindows(windows)
	}

	hasMore := windowsHaveMore(windows)
	return builder.Finish(position(windows, hasMore, fingerprint, req.PageSize)), nil
}

// countTopic returns an upper bound from high/low watermarks, summed across every partition;
// compaction and transaction markers leave offsets with no record.
//
// rawFilter used to be dropped on the floor entirely (the caller always passed nil), so "N total"
// answered the high/low watermark across *every* partition even when the browse itself (read.ts's
// own freshWindows, called from readTopic) was scoped to a partition/offset/timestamp filter —
// the toolbar's own "<n> rows on this page" and "<N> total" then answered two different questions
// side by side. This now calls the exact same freshWindows the read path uses to open a browse
// under this filter, and sums each window's own [Next, End) gap — a partition filter narrows which
// windows exist at all, and an offset/timestamp filter moves Next away from the partition's true
// start offset, both of which now scope the total identically to what a browse under this filter
// would return.
func countTopic(ctx context.Context, adm *kadm.Client, topic string, rawFilter *string) (adapters.CountResult, error) {
	if err := adapters.CheckNotStarted(ctx); err != nil {
		return adapters.CountResult{}, err
	}
	windows, err := freshWindows(ctx, adm, topic, rawFilter)
	if err != nil {
		return adapters.CountResult{}, err
	}
	if err := adapters.CheckCancelled(ctx); err != nil {
		return adapters.CountResult{}, err
	}

	var total int64
	for _, w := range windows {
		total += w.End - w.Next
	}
	// An offset span is an upper bound: compaction and transaction markers leave offsets with no
	// record behind them.
	return adapters.CountResult{Value: total, Exact: false}, nil
}
