package dbflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const kafkaProduceScenario = "studio-kafka-produce"

// kafkaConsume reads a topic from its start with a second client (no group) until n records arrive.
func kafkaConsume(t *testing.T, fx *testsupport.KafkaFixture, topic string, n int) []*kgo.Record {
	t.Helper()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(fmt.Sprintf("%s:%d", *fx.Config.Host, *fx.Config.Port)),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	pollCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var out []*kgo.Record
	for len(out) < n {
		fetches := client.PollFetches(pollCtx)
		if err := pollCtx.Err(); err != nil {
			break
		}
		fetches.EachRecord(func(r *kgo.Record) { out = append(out, r) })
	}
	return out
}

// kafkaEnd is the topic's total record count by the broker's own watermarks.
func kafkaEnd(t *testing.T, fx *testsupport.KafkaFixture, topic string) int64 {
	t.Helper()
	ends, err := fx.Admin.ListEndOffsets(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	ends.Each(func(o kadm.ListedOffset) { total += o.Offset })
	return total
}

func produceOp(key *string, body *string, headers string) model.MutationRowOp {
	// The composer always sends $key, null when the field is empty.
	v := model.RowValues{{Name: "$key", Value: key}}
	if body != nil {
		v = append(v, model.RowValue{Name: "$body", Value: body})
	}
	if headers != "" {
		v = append(v, model.RowValue{Name: "$headers", Value: &headers})
	}
	return ins(v)
}

// J-STR-011 (contract studio-kafka-produce): the composed message reaches the broker as written,
// with headers in order and repeats, a null key, an empty body. Bad input is refused before the broker.
func TestKafkaProduceKeepsHeadersKeysAndBodies(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartKafka(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const topic = "produce-shapes"
	path := kafkaTopic(t, fx, topic, 1)

	body, empty, key := "hello", "", "k1"
	headers := `{"trace":"a","trace":"b","type":"x"}`
	ops := []model.MutationRowOp{
		produceOp(&key, &body, headers),
		produceOp(nil, &body, ""),
		produceOp(&key, &empty, ""),
	}
	app.Contract(t, kafkaProduceScenario, "mutate:ops", flowharness.MutateWire(conn.ID, path, ops), contractMask)
	if n, err := ds.Mutate(conn.ID, path, ops); err != nil || n != 3 {
		t.Fatalf("produce = %d, %v; want 3 landed", n, err)
	}
	got := kafkaConsume(t, fx, topic, 3)
	if len(got) != 3 {
		t.Fatalf("second client read %d records, want 3", len(got))
	}
	var pairs []string
	for _, h := range got[0].Headers {
		pairs = append(pairs, h.Key+"="+string(h.Value))
	}
	if strings.Join(pairs, ",") != "trace=a,trace=b,type=x" {
		t.Fatalf("headers = %v, want trace=a, trace=b, type=x in order", pairs)
	}
	if string(got[0].Key) != "k1" || string(got[0].Value) != "hello" {
		t.Fatalf("record 1 = %q/%q", got[0].Key, got[0].Value)
	}
	if got[1].Key != nil {
		t.Fatalf("record 2 key = %q, want a null key", got[1].Key)
	}
	if got[2].Value == nil || len(got[2].Value) != 0 {
		t.Fatalf("record 3 value = %v, want stored empty (not a tombstone)", got[2].Value)
	}

	// Refused before the broker: an absent body, a malformed or non-string header map.
	end := kafkaEnd(t, fx, topic)
	for name, bad := range map[string]model.MutationRowOp{
		"absent body":       produceOp(&key, nil, ""),
		"malformed headers": produceOp(&key, &body, `{"a":`),
		"headers not map":   produceOp(&key, &body, `["a"]`),
		"header not string": produceOp(&key, &body, `{"a":1}`),
	} {
		_, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{bad})
		var derr *flowharness.DataError
		if !errors.As(err, &derr) {
			t.Fatalf("%s: err = %v, want a refusal", name, err)
		}
		switch name {
		case "absent body":
			app.Contract(t, kafkaProduceScenario, "mutate:absentBodyError", derr, contractMask)
		case "malformed headers":
			app.Contract(t, kafkaProduceScenario, "mutate:malformedHeadersError", derr, contractMask)
		}
	}
	if after := kafkaEnd(t, fx, topic); after != end {
		t.Fatalf("topic grew from %d to %d records after refused messages", end, after)
	}
}

// J-STR-011: a 5-message batch whose 3rd message exceeds the broker limit lands 4 and names position 3;
// resending only that message (now small enough) leaves exactly 5 records, none twice.
func TestKafkaProducePartialBatchAndResend(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartKafka(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const topic = "produce-batch"
	path := kafkaTopic(t, fx, topic, 1)

	big := strings.Repeat("x", 1<<20+1024)
	bodies := []string{"m1", "m2", big, "m4", "m5"}
	ops := make([]model.MutationRowOp, len(bodies))
	for i := range bodies {
		k := fmt.Sprintf("m%d", i+1)
		ops[i] = produceOp(&k, &bodies[i], "")
	}
	n, err := ds.Mutate(conn.ID, path, ops)
	var derr *flowharness.DataError
	if !errors.As(err, &derr) {
		t.Fatalf("batch with an oversized message = %d, %v; want a partial-produce error", n, err)
	}
	if !strings.Contains(derr.Message, "4 of 5") || !strings.Contains(derr.Message, "3 failed") {
		t.Fatalf("partial-produce message = %q, want 4 of 5 landed and position 3 failed", derr.Message)
	}
	app.Contract(t, kafkaProduceScenario, "mutate:partialError", derr, contractMask)
	if end := kafkaEnd(t, fx, topic); end != 4 {
		t.Fatalf("topic holds %d records after the partial batch, want 4", end)
	}

	small, key := "m3", "m3"
	if n, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{produceOp(&key, &small, "")}); err != nil || n != 1 {
		t.Fatalf("resend = %d, %v", n, err)
	}
	got := kafkaConsume(t, fx, topic, 5)
	seen := map[string]int{}
	for _, r := range got {
		seen[string(r.Key)]++
	}
	if len(got) != 5 || len(seen) != 5 {
		t.Fatalf("topic holds %d records (%v), want exactly 5, none twice", len(got), seen)
	}
}

// J-STR-011: a read-only connection refuses a message and the broker sees no request.
func TestKafkaProduceRefusedOnReadOnlyConnection(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartKafka(t)
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	in.ReadOnly = true
	conn := createConn(t, app, in)
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const topic = "produce-readonly"
	path := kafkaTopic(t, fx, topic, 1)
	body := "nope"
	_, err := ds.Mutate(conn.ID, path, []model.MutationRowOp{produceOp(nil, &body, "")})
	var derr *flowharness.DataError
	if !errors.As(err, &derr) {
		t.Fatalf("produce on a read-only connection = %v, want a refusal", err)
	}
	app.Contract(t, kafkaProduceScenario, "mutate:readOnlyError", derr, contractMask)
	if end := kafkaEnd(t, fx, topic); end != 0 {
		t.Fatalf("read-only produce reached the broker: %d records", end)
	}
}
