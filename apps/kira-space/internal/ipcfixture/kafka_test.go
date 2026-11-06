package ipcfixture

import (
	"context"
	"sort"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapterhost"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kafka"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/tree"
)

// kafkaTombstoneKey is this test's own extra message: a record
// with a nil Value, Kafka's tombstone convention. Produced here rather than in testsupport's
// shared seed so it stays scoped to this one read assertion instead of shifting
// testsupport.KafkaOrdersMessageCount for every other Kafka test.
const kafkaTombstoneKey = "key-6"

// sortStreamByKey is kafka.backend.spec.ts's own sortStreamByKey, ported: the Kafka client's own
// read fans across both partitions and interleaves them by arrival, not by any key/offset order —
// confirmed empirically to differ between separate, identically-seeded captures (the same reasoning
// as redis's own HSCAN reordering finding). Sorting by key before a fixture ever sees the page makes
// the capture deterministic without weakening what this scenario actually tests.
func sortStreamByKey(p LogicalStreamPage) LogicalStreamPage {
	order := make([]int, len(p.Keys))
	for i := range order {
		order[i] = i
	}
	key := func(i int) string {
		if p.Keys[i] == nil {
			return ""
		}
		return *p.Keys[i]
	}
	sort.SliceStable(order, func(a, b int) bool { return key(order[a]) < key(order[b]) })
	reorder := func(in []*string) []*string {
		out := make([]*string, len(in))
		for i, idx := range order {
			out[i] = in[idx]
		}
		return out
	}
	p.Keys = reorder(p.Keys)
	p.Headers = reorder(p.Headers)
	p.Attrs = reorder(p.Attrs)
	p.Timestamps = reorder(p.Timestamps)
	p.Bodies = reorder(p.Bodies)
	return p
}

func sectionByTitle(sections []model.DefinitionSection, title string) *model.DefinitionSection {
	for i := range sections {
		if sections[i].Title == title {
			return &sections[i]
		}
	}
	return nil
}

// TestFixture_Kafka runs against tests/ipc/kafka/kafka.fixture.ts's own committed scenario:
// connect, a tree with topics and
// consumer groups both at root, a partition-filter popover's own children() call, an offsetWindow
// read of the orders topic (its own arrival order and wall-clock timestamps frozen), an empty
// topic's zero-row read, the orders topic's own definition (Partitions + Configuration), and the
// consumer group's own definition (Group/Members/Committed offsets).
//
// The consumer group's own Group section carries two fewer rows than the committed fixture
// (type, partitionAssignor) — kadm.DescribedGroup exposes neither field, a permanent capability
// difference already documented at the adapter level (kafka/definition.go's own comment),
// reconciled at comparison time by frozen.go's own MaskContinuationTokens rather than re-created
// here or silently regenerated over.
func TestFixture_Kafka(t *testing.T) {
	fixture := testsupport.StartKafka(t)
	app := NewApp(t)
	cfg := fixture.Config

	app.SeedConnection(t, cfg.ID, fieldsOf(cfg), cfg.Password)
	rec := NewRecorder(app)

	// --- connect -------------------------------------------------------------------------------
	list := rec.ConnectionsList(t)
	if len(list) != 1 || list[0].ID != cfg.ID {
		t.Fatalf("connections list = %+v, want exactly one row for %s", list, cfg.ID)
	}
	if states := rec.ConnectionsStates(t); len(states) != 0 {
		t.Fatalf("connections states = %+v, want none", states)
	}
	state := rec.ConnectionsConnect(t, cfg.ID)
	if state.ServerVersion == nil || *state.ServerVersion != "Kafka" {
		t.Fatalf("serverVersion = %v, want Kafka", state.ServerVersion)
	}

	// --- tree: topics and consumer groups both at root, ungrouped by the backend ----------------
	root := rec.TreeChildren(t, cfg.ID, "", false)
	ordersTopicNode := nodeByName(root.Nodes, testsupport.KafkaOrdersTopic)
	emptyTopicNode := nodeByName(root.Nodes, testsupport.KafkaEmptyTopic)
	groupNode := nodeByName(root.Nodes, testsupport.KafkaConsumerGroup)
	if ordersTopicNode == nil || emptyTopicNode == nil || groupNode == nil {
		t.Fatalf("expected %s/%s topic nodes and a %s group node in %+v", testsupport.KafkaOrdersTopic, testsupport.KafkaEmptyTopic, testsupport.KafkaConsumerGroup, root.Nodes)
	}

	// --- the partition-filter popover's own live children() call against the topic path ---------
	partitions := rec.TreeChildren(t, cfg.ID, ordersTopicNode.Path, false)
	if len(partitions.Nodes) != testsupport.KafkaOrdersPartitionCount {
		t.Fatalf("orders topic partitions = %d, want %d", len(partitions.Nodes), testsupport.KafkaOrdersPartitionCount)
	}

	// --- a tombstone: a Value of nil, Kafka's own null-value convention, distinct from "" ---------
	tombstone := &kgo.Record{
		Topic:   testsupport.KafkaOrdersTopic,
		Key:     []byte(kafkaTombstoneKey),
		Value:   nil,
		Headers: []kgo.RecordHeader{{Key: "source", Value: []byte("seed")}},
	}
	if err := fixture.Client.ProduceSync(context.Background(), tombstone).FirstErr(); err != nil {
		t.Fatalf("produce tombstone: %v", err)
	}
	wantOrdersMessageCount := testsupport.KafkaOrdersMessageCount + 1

	// --- open the orders topic: offsetWindow auto-loads on mount --------------------------------
	readReq := adapterhost.ReadRequestWire{
		OpID: "be-read-orders", ConnectionID: cfg.ID, Path: ordersTopicNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	readResp, err := app.Dispatcher.Read(context.Background(), WireRead(readReq))
	if err != nil {
		t.Fatalf("read orders topic: %v", err)
	}
	readLogical, err := DecodePage(readResp.Page)
	if err != nil {
		t.Fatalf("decode orders page: %v", err)
	}
	readStream, ok := readLogical.(LogicalStreamPage)
	if !ok || len(readStream.Keys) != wantOrdersMessageCount || readStream.Position.HasMore {
		t.Fatalf("orders page = %+v, want %d keys, hasMore=false", readLogical, wantOrdersMessageCount)
	}
	// The seed publishes these messages at container-start time (real wall-clock), so their
	// timestamps differ run to run — frozen for the fixture; then reordered deterministically
	// (see sortStreamByKey's own comment).
	for i := range readStream.Timestamps {
		ts := "2024-01-01T00:00:00.000Z"
		readStream.Timestamps[i] = &ts
	}
	recordDataRead(rec, readReq, sortStreamByKey(readStream), readResp.Source)

	// --- an empty topic's read comes back with zero rows, no error -------------------------------
	emptyReq := adapterhost.ReadRequestWire{
		OpID: "be-read-empty", ConnectionID: cfg.ID, Path: emptyTopicNode.Path,
		PageSize: 100, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	emptyResp := rec.DataRead(t, emptyReq, nil)
	emptyLogical, err := DecodePage(emptyResp.Page)
	if err != nil {
		t.Fatalf("decode empty page: %v", err)
	}
	emptyStream, ok := emptyLogical.(LogicalStreamPage)
	if !ok || len(emptyStream.Keys) != 0 {
		t.Fatalf("expected a zero-row stream page, got %+v", emptyLogical)
	}

	// --- the orders topic's definition — Partitions + Configuration (no describeConfigs) ---------
	topicDefinitionResult, err := app.TreeSvc.Definition(bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: ordersTopicNode.Path, Refresh: false, TabID: nil})
	if err != nil {
		t.Fatalf("orders topic definition: %v", err)
	}
	partitionsSection := sectionByTitle(topicDefinitionResult.Definition.Sections, "Partitions")
	if partitionsSection == nil || len(partitionsSection.Rows) != testsupport.KafkaOrdersPartitionCount {
		t.Fatalf("Partitions section = %+v, want %d rows", partitionsSection, testsupport.KafkaOrdersPartitionCount)
	}
	// The Configuration section is compared row for row against the committed fixture.
	if sectionByTitle(topicDefinitionResult.Definition.Sections, "Configuration") == nil {
		t.Fatalf("expected a Configuration section, got %+v", topicDefinitionResult.Definition.Sections)
	}
	rec.recordControl(channelTreeDefinition, bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: ordersTopicNode.Path, Refresh: false, TabID: nil},
		tree.DefinitionResult{Definition: FreezeDefinition(topicDefinitionResult.Definition), Source: topicDefinitionResult.Source})

	// --- the consumer group's definition — Group/Members/Committed offsets -----------------------
	groupDefinitionResult, err := app.TreeSvc.Definition(bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: groupNode.Path, Refresh: false, TabID: nil})
	if err != nil {
		t.Fatalf("consumer group definition: %v", err)
	}
	offsetsSection := sectionByTitle(groupDefinitionResult.Definition.Sections, "Committed offsets")
	if offsetsSection == nil || len(offsetsSection.Rows) != testsupport.KafkaOrdersPartitionCount {
		t.Fatalf("Committed offsets section = %+v, want %d rows", offsetsSection, testsupport.KafkaOrdersPartitionCount)
	}
	groupSection := sectionByTitle(groupDefinitionResult.Definition.Sections, "Group")
	if groupSection == nil {
		t.Fatalf("expected a Group section, got %+v", groupDefinitionResult.Definition.Sections)
	}
	var hasCoordinatorRow bool
	for _, row := range groupSection.Rows {
		hasCoordinatorRow = hasCoordinatorRow || row.Name == "coordinator"
	}
	if !hasCoordinatorRow {
		t.Fatalf("expected a coordinator row in %+v", groupSection.Rows)
	}
	rec.recordControl(channelTreeDefinition, bridge.TreeDescribeArgs{ConnectionID: cfg.ID, Path: groupNode.Path, Refresh: false, TabID: nil},
		tree.DefinitionResult{Definition: FreezeCoordinator(FreezeDefinition(groupDefinitionResult.Definition)), Source: groupDefinitionResult.Source})

	if maybeWriteFixture(t, rec, "kafka") {
		return
	}
	assertMatchesCommittedJSONFixture(t, rec, "testdata/kafka.fixture.json")
}
