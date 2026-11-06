package kafka

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Sentinel keys (mirrors mongo/mutate.ts's $document, sqs/mutate.go's $body/$headers): a new
// message is expressed through the existing relational-shaped MutationRowOp's values rather than
// widening the shared mutation schema. '$' can never start a real Kafka header name worth
// round-tripping, so it can't collide with genuine data.
const (
	keyField     = "$key"
	bodyField    = "$body"
	headersField = "$headers"
)

// renderOpText renders the produce preview against the real kgo.ProduceSync call this adapter
// makes (P58f D6) — produce.ts's own renderOpText named node-rdkafka's producer.produce(...), an
// API with no Go analogue, which stopped being a faithful preview the moment this adapter shipped.
func renderOpText(op model.MutationRowOp, topic string) (string, error) {
	if op.Kind != "insert" {
		// A topic's log is immutable — no per-message update or delete (caps.go's own comment).
		return "", adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.kafkaInsertOnly.body", nil, nil)
	}
	keyText := "<none>"
	if key, _ := op.Values.Get(keyField); key != nil {
		keyText = *key
	}
	return "ProduceSync " + topic + " key=" + keyText, nil
}

// preview is produce.ts's preview — synchronous (Adapter rule 3): no network, no catalog lookup.
func preview(plan model.MutationPlan, topic string) ([]string, error) {
	return adapters.PreviewProduce(plan, topic, renderOpText)
}

func toRecordHeaders(headers []adapters.HeaderPair) []kgo.RecordHeader {
	if headers == nil {
		return nil
	}
	out := make([]kgo.RecordHeader, 0, len(headers))
	for _, h := range headers {
		out = append(out, kgo.RecordHeader{Key: h.Key, Value: []byte(h.Value)})
	}
	return out
}

// produce is produce.ts's produce: kgo.ProduceSync on the adapter's own long-lived
// client, which already carries kgo.DisableIdempotentWrite() (client.go) — matching librdkafka's
// own default and sidestepping the InitProducerId hang a single-broker cluster's default
// transaction-log replication factor would otherwise cause. No separate producer client, unlike
// produce.ts's fresh Producer per mutate: that existed only to dodge a NAN/Electron-sandbox
// delivery-report crash with no Go analogue whatsoever.
func produce(ctx context.Context, client *kgo.Client, topic string, readOnly bool, plan model.MutationPlan, op *adapters.OpCtx) (model.MutationResult, error) {
	// §8.12's standard: enforced here, not only greyed out in the UI (mirrors mongo/sqs).
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}

	statements, err := preview(plan, topic)
	if err != nil {
		return model.MutationResult{}, err
	}
	command := ""
	for i, s := range statements {
		if i > 0 {
			command += ";\n"
		}
		command += s
	}
	op.SetCommand(command)

	records := make([]*kgo.Record, 0, len(plan.Ops))
	for _, rowOp := range plan.Ops {
		if rowOp.Kind != "insert" {
			return model.MutationResult{}, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.kafkaInsertOnly.body", nil, nil)
		}
		body, ok := rowOp.Values.Get(bodyField)
		if !ok || body == nil {
			return model.MutationResult{}, adapters.New(adapters.CodeQuery, "a new message requires a "+bodyField, nil)
		}
		headersRaw, _ := rowOp.Values.Get(headersField)
		headers, err := adapters.ParseHeaderPairs(headersRaw)
		if err != nil {
			return model.MutationResult{}, err
		}
		rec := &kgo.Record{Topic: topic, Value: []byte(*body), Headers: toRecordHeaders(headers)}
		if key, _ := rowOp.Values.Get(keyField); key != nil {
			rec.Key = []byte(*key)
		}
		records = append(records, rec)
	}

	// A capability gain over produce.ts, whose own comment concedes it can only report "queued
	// into librdkafka", not "the broker acknowledged this specific message" — ProduceSync reports
	// exactly that (P58e E14).
	results := client.ProduceSync(ctx, records...)
	// Results arrive in completion order, so a record's position comes from its identity.
	position := make(map[*kgo.Record]int, len(records))
	for i, rec := range records {
		position[rec] = i + 1
	}
	landed := 0
	var failedAt []int
	var firstErr error
	for _, r := range results {
		if r.Err == nil {
			landed++
			continue
		}
		if firstErr == nil {
			firstErr = r.Err
		}
		failedAt = append(failedAt, position[r.Record])
	}
	sort.Ints(failedAt)
	failed := make([]string, len(failedAt))
	for i, n := range failedAt {
		failed[i] = strconv.Itoa(n)
	}
	if firstErr != nil {
		mapped := mapError(firstErr)
		if landed == 0 {
			return model.MutationResult{}, mapped
		}
		// The host drops a result that travels with an error; the message must say what landed so a
		// retry does not duplicate it.
		return model.MutationResult{AffectedRows: landed}, adapters.T(mapped.Code, "backend.studio.adapter.kafkaPartialProduce.error", map[string]string{"landed": fmt.Sprint(landed), "total": fmt.Sprint(len(records)), "failed": strings.Join(failed, ", "), "detail": mapped.Message}, mapped.Cause)
	}

	return model.MutationResult{AffectedRows: len(records)}, nil
}
