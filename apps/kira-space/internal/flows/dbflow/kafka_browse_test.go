package dbflow

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const kafkaBrowseScenario = "studio-kafka-browse"

func kafkaTopicPath(topic string) string {
	return model.EncodePath([]model.PathSegment{{Kind: "topic", Name: topic}})
}

// kafkaTopic creates a topic through the fixture's admin client and returns its node path.
func kafkaTopic(t *testing.T, fx *testsupport.KafkaFixture, name string, partitions int32) string {
	t.Helper()
	resp, err := fx.Admin.CreateTopics(ctx, partitions, 1, nil, name)
	if err != nil {
		t.Fatalf("create topic %s: %v", name, err)
	}
	for _, r := range resp.Sorted() {
		if r.Err != nil {
			t.Fatalf("create topic %s: %v", name, r.Err)
		}
	}
	return kafkaTopicPath(name)
}

// kafkaProduce writes n keyed records through a second client; the keys spread them over the partitions.
func kafkaProduce(t *testing.T, fx *testsupport.KafkaFixture, topic, prefix string, n int) {
	t.Helper()
	recs := make([]*kgo.Record, n)
	for i := range recs {
		recs[i] = &kgo.Record{Topic: topic, Key: []byte(fmt.Sprintf("%s-k%d", prefix, i)), Value: []byte(fmt.Sprintf("%s-v%d", prefix, i))}
	}
	if err := fx.Client.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatalf("produce to %s: %v", topic, err)
	}
}

func kafkaGroupNames(t *testing.T, fx *testsupport.KafkaFixture) []string {
	t.Helper()
	groups, err := fx.Admin.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := groups.Groups()
	slices.Sort(names)
	return names
}

// partitionOffset reads the attrs cell ({"partition":n,"offset":"m"}) of a stream row.
func partitionOffset(t *testing.T, row []any) string {
	t.Helper()
	attrs, _ := row[2].(string)
	if attrs == "" {
		t.Fatalf("row %v has no attrs cell", row)
	}
	return attrs
}

// readAll pages a topic by token until it ends and returns every row; between marks the page after which
// it runs once (a second client writes there).
func readAll(t *testing.T, ds *flowharness.DataStream, connID, path string, pageSize int, afterPage int, between func()) []flowharness.DataPage {
	t.Helper()
	var pages []flowharness.DataPage
	args := flowharness.ReadArgs{ConnectionID: connID, Path: path, PageSize: pageSize}
	for {
		p, _, err := ds.Read(args)
		if err != nil {
			t.Fatalf("read page %d: %v", len(pages)+1, err)
		}
		pages = append(pages, p)
		if len(pages) == afterPage && between != nil {
			between()
		}
		if !p.HasMore {
			return pages
		}
		args.Cursor = model.PageCursor{Mode: "after", Token: p.NextToken}
		if len(pages) > 200 {
			t.Fatal("paging does not end")
		}
	}
}

// J-STR-010 (contract studio-kafka-browse): a topic of 6 partitions pages through a frozen window. A
// second client writes 1,000 records mid-paging: none repeats, none beyond the window shows, and a
// refresh shows them. Browsing creates no consumer group and leaves a group's committed offsets alone.
func TestKafkaBrowsePagesAFrozenWindowWithoutAGroup(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartKafka(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)
	const topic, window = "browse-window", 600
	path := kafkaTopic(t, fx, topic, 6)
	kafkaProduce(t, fx, topic, "a", window)

	// A group "billing" committed offsets before the browse; it must be untouched after.
	offs := kadm.Offsets{}
	offs.Add(kadm.Offset{Topic: topic, Partition: 0, At: 5, LeaderEpoch: -1})
	if _, err := fx.Admin.CommitOffsets(ctx, "billing", offs); err != nil {
		t.Fatalf("commit billing offsets: %v", err)
	}
	groupsBefore := kafkaGroupNames(t, fx)
	billingBefore, err := fx.Admin.FetchOffsets(ctx, "billing")
	if err != nil {
		t.Fatal(err)
	}

	pages := readAll(t, ds, conn.ID, path, 100, 2, func() { kafkaProduce(t, fx, topic, "b", 1000) })
	seen := map[string]bool{}
	rows := 0
	for _, p := range pages {
		for _, r := range p.Rows {
			rows++
			id := partitionOffset(t, r)
			if seen[id] {
				t.Fatalf("record %s shown twice", id)
			}
			seen[id] = true
			if body, _ := r[4].(string); !strings.HasPrefix(body, "a-v") {
				t.Fatalf("record %s body %q is from beyond the window", id, body)
			}
		}
	}
	if rows != window {
		t.Fatalf("browsed %d records, want the window of %d", rows, window)
	}
	last := pages[len(pages)-1]
	if last.HasMore {
		t.Fatal("last page claims more records")
	}
	app.Contract(t, kafkaBrowseScenario, "read:firstRow", pages[0].Rows[0], contractMask, flowharness.Mask("Timestamp"))

	// Until the user refreshes, the first page is the cached one; a refresh opens a new window that
	// includes the appended records.
	if stale, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: path, PageSize: 100}); err != nil || len(stale.Rows) != 100 || stale.Rows[0][2] != pages[0].Rows[0][2] {
		t.Fatalf("re-read before refresh = %d rows, %v; want the cached first page", len(stale.Rows), err)
	}
	if err := ds.Invalidate(conn.ID, path, "pages"); err != nil {
		t.Fatal(err)
	}
	fresh := readAll(t, ds, conn.ID, path, 100, 0, nil)
	total := 0
	for _, p := range fresh {
		total += len(p.Rows)
	}
	if total != window+1000 {
		t.Fatalf("refresh shows %d records, want %d", total, window+1000)
	}

	count, exact, err := ds.Count(conn.ID, path, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if exact || count != window+1000 {
		t.Fatalf("count = %d exact=%v, want %d labelled inexact", count, exact, window+1000)
	}

	if after := kafkaGroupNames(t, fx); !slices.Equal(after, groupsBefore) {
		t.Fatalf("groups after browse = %v, want %v", after, groupsBefore)
	}
	billingAfter, err := fx.Admin.FetchOffsets(ctx, "billing")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fmt.Sprint(billingAfter.Lookup(topic, 0)), fmt.Sprint(billingBefore.Lookup(topic, 0)); got != want {
		t.Fatalf("billing offset moved from %s to %s", want, got)
	}
}

// J-STR-010: a tombstone reads as a null body, headers keep their order and repeats, and a page
// token whose position the broker trimmed away is refused instead of silently skipping or repeating.
func TestKafkaBrowseTombstoneHeadersAndLostPosition(t *testing.T) {
	flowharness.RequireDocker(t)
	fx := testsupport.StartKafka(t)
	app := flowharness.New(t)
	conn := createConn(t, app, inputFromConfig(fx.Config))
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	ds := app.DataStream(t)

	path := kafkaTopic(t, fx, "browse-shapes", 1)
	recs := []*kgo.Record{
		{Topic: "browse-shapes", Key: []byte("gone"), Value: nil},
		{Topic: "browse-shapes", Key: []byte("h"), Value: []byte("with headers"), Headers: []kgo.RecordHeader{
			{Key: "trace", Value: []byte("a")}, {Key: "trace", Value: []byte("b")}, {Key: "type", Value: []byte("x")},
		}},
		{Topic: "browse-shapes", Value: []byte("")},
	}
	if err := fx.Client.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
	p, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: path})
	if err != nil || len(p.Rows) != 3 {
		t.Fatalf("read = %d rows, %v", len(p.Rows), err)
	}
	if p.Rows[0][4] != nil {
		t.Fatalf("tombstone body = %v, want a null cell", p.Rows[0][4])
	}
	if p.Rows[2][4] != "" || p.Rows[2][0] != nil {
		t.Fatalf("empty-body record = key %v body %v, want a null key and an empty body", p.Rows[2][0], p.Rows[2][4])
	}
	if got := p.Rows[1][1]; got != `{"trace":["a","b"],"type":"x"}` {
		t.Fatalf("headers = %v, want order and repeats kept", got)
	}
	app.Contract(t, kafkaBrowseScenario, "read:shapes", p.Rows, contractMask, flowharness.Mask("Timestamp"))

	// Page one of a 300-record topic, then the broker trims the log past the token's position.
	lost := kafkaTopic(t, fx, "browse-lost", 1)
	kafkaProduce(t, fx, "browse-lost", "l", 300)
	first, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: lost, PageSize: 100})
	if err != nil || len(first.Rows) != 100 || !first.HasMore {
		t.Fatalf("first page = %d rows more=%v, %v", len(first.Rows), first.HasMore, err)
	}
	del, err := fx.Admin.DeleteRecords(ctx, kadm.Offsets{"browse-lost": {0: kadm.Offset{Topic: "browse-lost", Partition: 0, At: 200, LeaderEpoch: -1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := del.Error(); err != nil {
		t.Fatal(err)
	}
	next, _, err := ds.Read(flowharness.ReadArgs{ConnectionID: conn.ID, Path: lost, PageSize: 100, Cursor: model.PageCursor{Mode: "after", Token: first.NextToken}})
	var derr *flowharness.DataError
	if err == nil {
		t.Fatalf("next page after the trim returned %d rows, want a position-lost refusal", len(next.Rows))
	}
	if !errors.As(err, &derr) || !strings.Contains(strings.ToLower(derr.Message), "position") {
		t.Fatalf("next page after the trim = %v, want a position-lost refusal", err)
	}
	app.Contract(t, kafkaBrowseScenario, "read:positionLost", derr, contractMask)
}
