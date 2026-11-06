package testsupport

import (
	"context"
	"os"
	"testing"

	"github.com/twmb/franz-go/pkg/kadm"
)

// TestMain is this package's own — none of the other testsupport fixtures need one, since
// localstack_test.go terminates its container inline rather than through the fixture[T] memo.
// KafkaFixture is the first testsupport fixture whose own package test exercises the memo
// directly, so this is where StopKafka's call site lands (after m.Run(), never t.Cleanup).
func TestMain(m *testing.M) {
	code := m.Run()
	StopKafka()
	os.Exit(code)
}

// TestKafkaFixtureSeed is M9.1's own acceptance check (§4.6): confirms the seed matches its
// checklist using only the raw franz-go/kadm client — no Kafka adapter is involved, so this test
// passes before the adapter package (M9.2) exists at all.
func TestKafkaFixtureSeed(t *testing.T) {
	f := StartKafka(t)
	ctx := context.Background()

	md, err := f.Admin.Metadata(ctx, KafkaOrdersTopic, KafkaEmptyTopic)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	orders, ok := md.Topics[KafkaOrdersTopic]
	if !ok || orders.Err != nil {
		t.Fatalf("topic %s: present=%v err=%v", KafkaOrdersTopic, ok, orders.Err)
	}
	if got := len(orders.Partitions); got != KafkaOrdersPartitionCount {
		t.Errorf("%s partitions = %d, want %d", KafkaOrdersTopic, got, KafkaOrdersPartitionCount)
	}
	empty, ok := md.Topics[KafkaEmptyTopic]
	if !ok || empty.Err != nil {
		t.Fatalf("topic %s: present=%v err=%v", KafkaEmptyTopic, ok, empty.Err)
	}
	if got := len(empty.Partitions); got != 1 {
		t.Errorf("%s partitions = %d, want 1", KafkaEmptyTopic, got)
	}

	starts, err := f.Admin.ListStartOffsets(ctx, KafkaOrdersTopic)
	if err != nil {
		t.Fatalf("ListStartOffsets: %v", err)
	}
	if err := starts.Error(); err != nil {
		t.Fatalf("ListStartOffsets: %v", err)
	}
	ends, err := f.Admin.ListEndOffsets(ctx, KafkaOrdersTopic)
	if err != nil {
		t.Fatalf("ListEndOffsets: %v", err)
	}
	if err := ends.Error(); err != nil {
		t.Fatalf("ListEndOffsets: %v", err)
	}
	var total int64
	starts.Each(func(lo kadm.ListedOffset) {
		hi, found := ends.Lookup(lo.Topic, lo.Partition)
		if !found {
			t.Fatalf("no end offset for partition %d", lo.Partition)
		}
		total += hi.Offset - lo.Offset
	})
	if total != KafkaOrdersMessageCount {
		t.Errorf("total messages = %d, want %d", total, KafkaOrdersMessageCount)
	}

	emptyEnds, err := f.Admin.ListEndOffsets(ctx, KafkaEmptyTopic)
	if err != nil {
		t.Fatalf("ListEndOffsets(%s): %v", KafkaEmptyTopic, err)
	}
	emptyStarts, err := f.Admin.ListStartOffsets(ctx, KafkaEmptyTopic)
	if err != nil {
		t.Fatalf("ListStartOffsets(%s): %v", KafkaEmptyTopic, err)
	}
	if hi, ok := emptyEnds.Lookup(KafkaEmptyTopic, 0); ok {
		if lo, ok := emptyStarts.Lookup(KafkaEmptyTopic, 0); ok && hi.Offset != lo.Offset {
			t.Errorf("%s: end offset %d != start offset %d, want equal (never written)", KafkaEmptyTopic, hi.Offset, lo.Offset)
		}
	}

	groups, err := f.Admin.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups: %v", err)
	}
	names := groups.Groups() // already sorted
	found := false
	for _, n := range names {
		if n == KafkaConsumerGroup {
			found = true
		}
	}
	if !found {
		t.Fatalf("groups = %v, want to contain %q", names, KafkaConsumerGroup)
	}

	described, err := f.Admin.DescribeGroups(ctx, KafkaConsumerGroup)
	if err != nil {
		t.Fatalf("DescribeGroups: %v", err)
	}
	dg, ok := described[KafkaConsumerGroup]
	if !ok || dg.Err != nil {
		t.Fatalf("describe %s: present=%v err=%v", KafkaConsumerGroup, ok, dg.Err)
	}
	if len(dg.Members) != 0 {
		t.Errorf("%s members = %d, want 0 (no group join)", KafkaConsumerGroup, len(dg.Members))
	}

	offsets, err := f.Admin.FetchOffsets(ctx, KafkaConsumerGroup)
	if err != nil {
		t.Fatalf("FetchOffsets: %v", err)
	}
	if err := offsets.Error(); err != nil {
		t.Fatalf("FetchOffsets: %v", err)
	}
	if got := len(offsets[KafkaOrdersTopic]); got != KafkaOrdersPartitionCount {
		t.Errorf("committed offset partitions = %d, want %d", got, KafkaOrdersPartitionCount)
	}
}
