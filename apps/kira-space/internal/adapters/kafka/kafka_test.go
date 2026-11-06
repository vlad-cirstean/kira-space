// Ported from packages/db-fixtures/kafka.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. Six cases carry the most weight:
// scenario 17 (a browse joins no consumer group) and 18 (a browse commits no offsets) are
// rewritten to check the broker directly rather than trusting the adapter; scenario 21 (a
// transaction's commit marker still terminates the browse) is the single most important test
// in the suite, since it pins a regression; scenario 6's Configuration half is inverted,
// not ported (the capability recovery); scenario 1 gains a cluster-id assertion; and the
// mid-browse cancellation case (new, alongside 15) is the one to keep correct. This file lands
// failing — no "kafka" adapter is registered yet — and every test below fails at CreateAdapter
// with E_UNSUPPORTED "kafka connections are not supported yet", which is the right reason.
package kafka_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/kafka"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

var deps = adapters.Deps{Log: func(level, message string) {}}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("kafka", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, f *testsupport.KafkaFixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), f.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *KafkaFixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.KafkaFixture]

func topicPath(f *testsupport.KafkaFixture, topic string) model.NodePath {
	return nodePath(f, testsupport.Seg("topic", topic))
}

func groupPath(f *testsupport.KafkaFixture, group string) model.NodePath {
	return nodePath(f, testsupport.Seg("consumerGroup", group))
}

// offsetRead is testsupport.OffsetRead (P107 I2-28).
var offsetRead = testsupport.OffsetRead

// rowSeq reads a seeded/produced message's {"seq": N} body back into an int.
func rowSeq(t *testing.T, sp page.StreamPage, row int) int {
	t.Helper()
	body := testsupport.StreamBodyAt(t, sp, row)
	if body == nil {
		t.Fatalf("row %d: body is nil", row)
	}
	var v struct {
		Seq int `json:"seq"`
	}
	if err := json.Unmarshal([]byte(*body), &v); err != nil {
		t.Fatalf("row %d: body %q: %v", row, *body, err)
	}
	return v.Seq
}

// sameStrings compares two string slices as sets, without mutating either argument.
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// 1. connect / disconnect. Extended per P58e E15: ConnectInfo.Details now carries a real cluster
// id (P32 D13's recovery) alongside the live broker count, not merely a configured-address count.
func TestKafka_ConnectDisconnect(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), f.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if info.ServerVersion != "Kafka" {
		t.Errorf("ServerVersion = %q, want Kafka", info.ServerVersion)
	}
	if info.Details["brokers"] == "" {
		t.Error("Details[brokers] is empty, want a live broker count")
	}
	if info.Details["cluster"] == "" {
		t.Error("Details[cluster] is empty, want a real cluster id (P58e E15)")
	}
	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if _, err := a.Children(context.Background(), nodePath(f), adapters.NewOpCtx("op-1b")); err == nil {
		t.Fatal("Children after disconnect: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect {
		t.Errorf("code = %v, want E_CONNECT", code)
	}
}

// P25 §2.1(4): the missing auth-failure case — a wrong password against a real SASL_PLAINTEXT/
// PLAIN broker is E_AUTH.
func TestKafka_Connect_AuthFailure(t *testing.T) {
	sf := testsupport.StartKafkaSasl(t)
	badCfg := sf.Config
	user, wrong := "kira", "definitely-wrong"
	badCfg.Username, badCfg.Password = &user, &wrong

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-auth"))
	if err == nil {
		t.Fatal("want an error for a wrong password")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeAuth {
		t.Errorf("code = %v, want E_AUTH", code)
	}
}

// The regression test — a non-empty username with an empty password must still fail as a real,
// correctly-coded auth failure against a SASL-requiring broker, not report the broker's
// transport-level refusal as E_QUERY. A half-filled pair deliberately does NOT offer SASL/PLAIN any
// more (client.go dials anonymously, same as an empty pair, since franz-go's own
// plain.Auth.AsMechanism() refuses locally on a half-filled pair before ever contacting the broker)
// — the resulting transport-level refusal is then reclassified as E_AUTH by client.go's own
// ErrFirstReadEOF heuristic instead.
func TestKafka_Connect_UsernameWithoutPasswordIsAuthError(t *testing.T) {
	sf := testsupport.StartKafkaSasl(t)
	cfg := sf.Config
	user, empty := "kira", ""
	cfg.Username, cfg.Password = &user, &empty

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), cfg, adapters.NewOpCtx("op-half"))
	if err == nil {
		t.Fatal("want an error for a username with no password")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeAuth {
		t.Errorf("code = %v, want E_AUTH", code)
	}
}

// 2. cap honesty. Collapses to a plain caps assertion (§5.3) — no container needed.
func TestKafka_Caps(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if c.Tabular {
		t.Error("Tabular = true, want false")
	}
	if !c.Stream {
		t.Error("Stream = false, want true")
	}
	if c.DefaultPageKind != page.PageKindStream {
		t.Errorf("DefaultPageKind = %v, want stream", c.DefaultPageKind)
	}
	if !c.Definition {
		t.Error("Definition = false, want true")
	}
	if c.SQL {
		t.Error("SQL = true, want false")
	}
	if c.ExactCount {
		t.Error("ExactCount = true, want false (offset span over-counts compacted topics)")
	}
	if c.Pagination != adapters.PaginationOffsetWindow {
		t.Errorf("Pagination = %v, want offsetWindow", c.Pagination)
	}
	if !c.CanInsert || c.CanUpdate || c.CanDelete {
		t.Errorf("CanInsert/CanUpdate/CanDelete = %v/%v/%v, want true/false/false", c.CanInsert, c.CanUpdate, c.CanDelete)
	}
	if !c.Writable {
		t.Error("Writable = false, want true")
	}
	if !c.Cancel {
		t.Error("Cancel = false, want true")
	}
	if c.FileTransfer {
		t.Error("FileTransfer = true, want false")
	}
	if c.SchemaColumns {
		t.Error("SchemaColumns = true, want false (a topic/consumer group has no column/PK/FK metadata)")
	}
}

// P22c §4.1: the non-SQL kinds report the capability false and the method is not implemented.
func TestKafka_SchemaColumns_Unsupported(t *testing.T) {
	a := newAdapter(t)
	_, err := a.SchemaColumns(context.Background(), model.NodePath{}, adapters.NewOpCtx("op-schema-columns"))
	var ae *adapters.Error
	if !errors.As(err, &ae) || ae.Code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED", err)
	}
}

// 3. tree enumeration: root is topics + consumer groups, siblings.
func TestKafka_Children_RootIsTopicsAndGroups(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	children, err := a.Children(context.Background(), nodePath(f), adapters.NewOpCtx("op-3"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	var topics, groups []string
	for _, n := range children.Nodes {
		switch n.Kind {
		case "topic":
			topics = append(topics, n.Name)
		case "consumerGroup":
			groups = append(groups, n.Name)
			if n.HasChildren {
				t.Errorf("group %s: HasChildren = true, want false", n.Name)
			}
		}
	}
	if !sameStrings(topics, []string{testsupport.KafkaEmptyTopic, testsupport.KafkaOrdersTopic}) {
		t.Errorf("topics = %v, want [%s %s]", topics, testsupport.KafkaEmptyTopic, testsupport.KafkaOrdersTopic)
	}
	if !sameStrings(groups, []string{testsupport.KafkaConsumerGroup}) {
		t.Errorf("groups = %v, want [%s]", groups, testsupport.KafkaConsumerGroup)
	}
}

// 4. tree enumeration: a topic node has hasChildren:false (P23 D3), children() still works — the
// tree no longer expands a topic, but StreamView.vue's partition filter is a second, live caller
// of exactly this call (index.ts:69-77).
func TestKafka_Children_TopicHasNoChildrenButPartitionsEnumerate(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	root, err := a.Children(context.Background(), nodePath(f), adapters.NewOpCtx("op-4a"))
	if err != nil {
		t.Fatalf("Children root: %v", err)
	}
	var ordersNode *model.TreeNode
	for i := range root.Nodes {
		if root.Nodes[i].Kind == "topic" && root.Nodes[i].Name == testsupport.KafkaOrdersTopic {
			ordersNode = &root.Nodes[i]
		}
	}
	if ordersNode == nil {
		t.Fatal("orders topic node not found")
	}
	if ordersNode.HasChildren {
		t.Error("orders node: HasChildren = true, want false (P23 D3)")
	}

	partitions, err := a.Children(context.Background(), topicPath(f, testsupport.KafkaOrdersTopic), adapters.NewOpCtx("op-4b"))
	if err != nil {
		t.Fatalf("Children partitions: %v", err)
	}
	if len(partitions.Nodes) != testsupport.KafkaOrdersPartitionCount {
		t.Errorf("partitions = %d, want %d", len(partitions.Nodes), testsupport.KafkaOrdersPartitionCount)
	}
	var names []string
	for _, n := range partitions.Nodes {
		if n.Kind != "partition" || n.HasChildren {
			t.Errorf("node %+v: want kind=partition, hasChildren=false", n)
		}
		names = append(names, n.Name)
	}
	if !sameStrings(names, []string{"0", "1"}) {
		t.Errorf("partition names = %v, want [0 1]", names)
	}
}

// 5. children of a leaf (partition / consumer group).
func TestKafka_Children_LeavesAreEmpty(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	partitionLeaf := nodePath(f, testsupport.Seg("topic", testsupport.KafkaOrdersTopic), testsupport.Seg("partition", "0"))
	partitionChildren, err := a.Children(context.Background(), partitionLeaf, adapters.NewOpCtx("op-5a"))
	if err != nil {
		t.Fatalf("Children partition leaf: %v", err)
	}
	if len(partitionChildren.Nodes) != 0 {
		t.Errorf("Nodes = %v, want empty", partitionChildren.Nodes)
	}

	groupChildren, err := a.Children(context.Background(), groupPath(f, testsupport.KafkaConsumerGroup), adapters.NewOpCtx("op-5b"))
	if err != nil {
		t.Fatalf("Children group leaf: %v", err)
	}
	if len(groupChildren.Nodes) != 0 {
		t.Errorf("Nodes = %v, want empty", groupChildren.Nodes)
	}
}

// 6. describe stays unsupported; definition shows a topic and a consumer group. The Configuration
// half is INVERTED, not ported: kafka.spec.ts 6 asserts an empty Configuration section plus a
// "DescribeConfigs" note, because the TypeScript client has no DescribeConfigs call at all.
// kadm.DescribeTopicConfigs recovers that capability (confirmed against a real broker and
// inventoried: 32 real config rows, cleanup.policy and retention.ms both present and stable), so
// this test asserts the opposite of its TypeScript ancestor.
func TestKafka_DescribeUnsupported_DefinitionConfigurationIsPopulated(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	if _, err := a.Describe(ctx, topicPath(f, testsupport.KafkaOrdersTopic), adapters.NewOpCtx("op-6a")); err == nil {
		t.Fatal("Describe: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Describe code = %v, want E_UNSUPPORTED", code)
	}

	topicDef, err := a.Definition(ctx, topicPath(f, testsupport.KafkaOrdersTopic), adapters.NewOpCtx("op-6b"))
	if err != nil {
		t.Fatalf("Definition(topic): %v", err)
	}
	if topicDef.Kind != "topic" {
		t.Errorf("Kind = %q, want topic", topicDef.Kind)
	}
	var partitions, config *model.DefinitionSection
	var titles []string
	for i := range topicDef.Sections {
		titles = append(titles, topicDef.Sections[i].Title)
		switch topicDef.Sections[i].Title {
		case "Partitions":
			partitions = &topicDef.Sections[i]
		case "Configuration":
			config = &topicDef.Sections[i]
		}
	}
	if !sameStrings(titles, []string{"Partitions", "Configuration"}) {
		t.Errorf("section titles = %v, want [Partitions Configuration]", titles)
	}
	if partitions == nil || len(partitions.Rows) != testsupport.KafkaOrdersPartitionCount {
		t.Fatalf("Partitions section = %+v, want %d rows", partitions, testsupport.KafkaOrdersPartitionCount)
	}
	leaderPattern := regexp.MustCompile(`^leader \d+$`)
	for _, r := range partitions.Rows {
		if !leaderPattern.MatchString(r.Value) {
			t.Errorf("partition row %+v: value doesn't match /^leader \\d+$/", r)
		}
	}
	if config == nil || len(config.Rows) == 0 {
		t.Fatal("Configuration section is empty, want real rows (P58e E11)")
	}
	foundKnownKey := false
	for _, r := range config.Rows {
		if r.Name == "cleanup.policy" || r.Name == "retention.ms" {
			foundKnownKey = true
		}
	}
	if !foundKnownKey {
		t.Errorf("Configuration rows = %+v, want to contain cleanup.policy or retention.ms", config.Rows)
	}
	for _, n := range topicDef.Notes {
		if strings.Contains(n, "DescribeConfigs") {
			t.Errorf("notes = %v, want nothing mentioning DescribeConfigs (the capability came back)", topicDef.Notes)
		}
	}

	// The seed consumer group has real committed offsets but no active members — the "empty
	// section, not an empty tab" case.
	groupDef, err := a.Definition(ctx, groupPath(f, testsupport.KafkaConsumerGroup), adapters.NewOpCtx("op-6c"))
	if err != nil {
		t.Fatalf("Definition(group): %v", err)
	}
	if groupDef.Kind != "consumerGroup" {
		t.Errorf("Kind = %q, want consumerGroup", groupDef.Kind)
	}
	var groupTitles []string
	var groupSection, offsets *model.DefinitionSection
	for i := range groupDef.Sections {
		groupTitles = append(groupTitles, groupDef.Sections[i].Title)
		switch groupDef.Sections[i].Title {
		case "Group":
			groupSection = &groupDef.Sections[i]
		case "Committed offsets":
			offsets = &groupDef.Sections[i]
		}
	}
	// P58e E13: kadm.DescribedGroup has no Type field, so the group definition loses its `type`
	// row and definition.ts's numeric-enum apparatus (reverseLookup) is deleted, not ported —
	// State is already a string on this client.
	if !sameStrings(groupTitles, []string{"Group", "Members", "Committed offsets"}) {
		t.Errorf("group section titles = %v, want [Group Members Committed offsets]", groupTitles)
	}
	if groupSection == nil {
		t.Fatal("Group section missing")
	}
	var stateValue string
	for _, r := range groupSection.Rows {
		if r.Name == "state" {
			stateValue = r.Value
		}
	}
	if matched, _ := regexp.MatchString(`^[A-Za-z ]+$`, stateValue); !matched {
		t.Errorf("state = %q, want a name, not a bare digit", stateValue)
	}
	if offsets == nil || len(offsets.Rows) != testsupport.KafkaOrdersPartitionCount {
		t.Fatalf("Committed offsets section = %+v, want %d rows", offsets, testsupport.KafkaOrdersPartitionCount)
	}
	var offsetNames []string
	for _, r := range offsets.Rows {
		offsetNames = append(offsetNames, r.Name)
	}
	want := []string{fmt.Sprintf("%s[0]", testsupport.KafkaOrdersTopic), fmt.Sprintf("%s[1]", testsupport.KafkaOrdersTopic)}
	if !sameStrings(offsetNames, want) {
		t.Errorf("offset row names = %v, want %v", offsetNames, want)
	}
}

// 7. read: browses a topic across partitions, offsetWindow pagination.
func TestKafka_Read_BrowsesAcrossPartitions(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	p, err := a.Read(context.Background(), offsetRead(topicPath(f, testsupport.KafkaOrdersTopic), testsupport.KafkaOrdersMessageCount), adapters.NewOpCtx("op-7"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp, ok := p.(page.StreamPage)
	if !ok {
		t.Fatalf("Read returned %T, want page.StreamPage", p)
	}
	if sp.Position.Strategy != "offsetWindow" {
		t.Errorf("Strategy = %q, want offsetWindow", sp.Position.Strategy)
	}
	if sp.RowCount != testsupport.KafkaOrdersMessageCount {
		t.Errorf("RowCount = %d, want %d", sp.RowCount, testsupport.KafkaOrdersMessageCount)
	}
	if sp.Position.HasMore {
		t.Error("HasMore = true, want false")
	}
	if sp.VisibilityTimeoutSeconds != nil {
		t.Errorf("VisibilityTimeoutSeconds = %v, want nil", sp.VisibilityTimeoutSeconds)
	}

	keyPattern := regexp.MustCompile(`^key-\d$`)
	seqs := make([]int, sp.RowCount)
	for r := 0; r < sp.RowCount; r++ {
		key := testsupport.StreamKeyAt(t, sp, r)
		if key == nil || !keyPattern.MatchString(*key) {
			t.Errorf("row %d: key = %v, want /^key-\\d$/", r, key)
		}
		headers := testsupport.StreamHeadersAt(t, sp, r)
		if headers == nil {
			t.Fatalf("row %d: headers is nil", r)
		}
		var h map[string]any
		if err := json.Unmarshal([]byte(*headers), &h); err != nil {
			t.Fatalf("row %d: headers %q: %v", r, *headers, err)
		}
		if h["source"] != "seed" {
			t.Errorf("row %d: headers.source = %v, want seed", r, h["source"])
		}
		attrs := testsupport.StreamAttrsAt(t, sp, r)
		if attrs == nil {
			t.Fatalf("row %d: attrs is nil", r)
		}
		var rawAttrs map[string]json.RawMessage
		if err := json.Unmarshal([]byte(*attrs), &rawAttrs); err != nil {
			t.Fatalf("row %d: attrs %q: %v", r, *attrs, err)
		}
		// P58e E8: the number/string asymmetry ports verbatim — partition is a JSON number,
		// offset a JSON string.
		var partitionNum float64
		if err := json.Unmarshal(rawAttrs["partition"], &partitionNum); err != nil {
			t.Errorf("row %d: attrs.partition is not a JSON number: %v", r, err)
		}
		var offsetStr string
		if err := json.Unmarshal(rawAttrs["offset"], &offsetStr); err != nil {
			t.Errorf("row %d: attrs.offset is not a JSON string: %v", r, err)
		}
		if ts := testsupport.StreamTimestampAt(t, sp, r); ts == nil {
			t.Errorf("row %d: timestamp is nil, want a value", r)
		}
		seqs[r] = rowSeq(t, sp, r)
	}
	sort.Ints(seqs)
	for i, s := range seqs {
		if s != i {
			t.Errorf("seqs = %v, want 0..%d", seqs, testsupport.KafkaOrdersMessageCount-1)
			break
		}
	}
}

// 8. read: a smaller page size pages forward with a token.
func TestKafka_Read_PagesForwardWithToken(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	seen := make(map[int]bool)
	cursor := model.PageCursor{Mode: "offset", Offset: 0}
	for guard := 0; guard < testsupport.KafkaOrdersMessageCount+2; guard++ {
		req := adapters.ReadRequest{Path: topicPath(f, testsupport.KafkaOrdersTopic), PageSize: 2, Cursor: cursor}
		p, err := a.Read(context.Background(), req, adapters.NewOpCtx(fmt.Sprintf("op-8-%d", guard)))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		sp := p.(page.StreamPage)
		for r := 0; r < sp.RowCount; r++ {
			seen[rowSeq(t, sp, r)] = true
		}
		if !sp.Position.HasMore {
			break
		}
		if sp.Position.NextToken == nil {
			t.Fatal("expected a nextToken on a truncated page")
		}
		cursor = model.PageCursor{Mode: "after", Token: *sp.Position.NextToken}
	}
	if len(seen) != testsupport.KafkaOrdersMessageCount {
		t.Errorf("seen = %d distinct messages, want %d", len(seen), testsupport.KafkaOrdersMessageCount)
	}
}

// 9. read: an empty topic returns a terminal empty page.
func TestKafka_Read_EmptyTopicIsTerminalEmptyPage(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	p, err := a.Read(context.Background(), offsetRead(topicPath(f, testsupport.KafkaEmptyTopic), 10), adapters.NewOpCtx("op-9"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := p.(page.StreamPage)
	if sp.RowCount != 0 || sp.Position.HasMore {
		t.Errorf("RowCount/HasMore = %d/%v, want 0/false", sp.RowCount, sp.Position.HasMore)
	}
}

// 10. read: forward-only — a "before" cursor is E_UNSUPPORTED.
func TestKafka_Read_BeforeCursorIsUnsupported(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	req := adapters.ReadRequest{Path: topicPath(f, testsupport.KafkaOrdersTopic), PageSize: 10, Cursor: model.PageCursor{Mode: "before", Token: "anything"}}
	_, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-10"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("code = %v, want E_UNSUPPORTED", code)
	}
}

// 11. read: a nonexistent topic is E_QUERY, not E_NOT_FOUND. A missing topic surfaces inside
// ListedOffsets' own per-partition Err, not as a returned error from
// ListStartOffsets/ListEndOffsets — a literal try/catch port would silently return an empty
// window set and a blank page. This was measured at ~4ms against a real broker, nowhere near
// kafka.spec.ts 11's 20s allowance for kafkajs's own metadata-retry budget, so no extended timeout
// is needed here.
func TestKafka_Read_NonexistentTopicIsQueryError(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	_, err := a.Read(context.Background(), offsetRead(topicPath(f, "this-topic-was-never-created"), 10), adapters.NewOpCtx("op-11"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// 12. count: high/low watermark subtraction, reported as an estimate (F13).
func TestKafka_Count_EstimateViaWatermarkSubtraction(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	got, err := a.Count(ctx, adapters.CountRequest{Path: topicPath(f, testsupport.KafkaOrdersTopic)}, adapters.NewOpCtx("op-12a"))
	if err != nil {
		t.Fatalf("Count(orders): %v", err)
	}
	if got.Value != testsupport.KafkaOrdersMessageCount || got.Exact {
		t.Errorf("Count(orders) = %+v, want {%d false}", got, testsupport.KafkaOrdersMessageCount)
	}
	gotEmpty, err := a.Count(ctx, adapters.CountRequest{Path: topicPath(f, testsupport.KafkaEmptyTopic)}, adapters.NewOpCtx("op-12b"))
	if err != nil {
		t.Fatalf("Count(empty): %v", err)
	}
	if gotEmpty.Value != 0 || gotEmpty.Exact {
		t.Errorf("Count(empty) = %+v, want {0 false}", gotEmpty)
	}
}

// 12b. count: P21 round 2 functional finding 7 — a Count request's own Filter must scope the
// watermark subtraction exactly the way a Read under the same filter scopes its browse, not sum
// every partition regardless. Before the fix, Count ignored its Filter entirely (countTopic took
// no filter argument at all and adapter.go never passed req.Filter through), so a per-partition
// filtered count here would equal the *unfiltered* total for every partition, and the two
// partitions' filtered counts would sum to roughly double the real total rather than exactly it.
// Message keys are distinct per record (testsupport's own seed), so which of the two partitions
// each message actually landed in is not known ahead of time — this asserts the *structural*
// property that must hold regardless of that distribution: the two partitions' own filtered counts
// partition the unfiltered total exactly, and filtering to both explicitly reproduces it.
func TestKafka_Count_ScopedToPartitionFilter(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()
	path := topicPath(f, testsupport.KafkaOrdersTopic)

	total, err := a.Count(ctx, adapters.CountRequest{Path: path}, adapters.NewOpCtx("op-12c"))
	if err != nil {
		t.Fatalf("Count(unfiltered): %v", err)
	}
	if total.Value != testsupport.KafkaOrdersMessageCount {
		t.Fatalf("Count(unfiltered) = %d, want %d", total.Value, testsupport.KafkaOrdersMessageCount)
	}

	countForPartitions := func(t *testing.T, partitions string) int64 {
		t.Helper()
		filter := `{"offset":null,"partitions":` + partitions + `,"timestampMs":null}`
		got, err := a.Count(ctx, adapters.CountRequest{Path: path, Filter: &filter}, adapters.NewOpCtx("op-12d-"+partitions))
		if err != nil {
			t.Fatalf("Count(filter=%s): %v", partitions, err)
		}
		if got.Exact {
			t.Errorf("Count(filter=%s).Exact = true, want false", partitions)
		}
		return got.Value
	}

	part0 := countForPartitions(t, "[0]")
	part1 := countForPartitions(t, "[1]")
	if part0+part1 != total.Value {
		t.Errorf("Count(partition 0) + Count(partition 1) = %d + %d = %d, want the unfiltered total %d",
			part0, part1, part0+part1, total.Value)
	}
	// A partition filter naming every partition must reproduce the unfiltered total exactly — the
	// same "any of these partitions is a union, not an intersection" rule freshWindows itself
	// documents.
	if both := countForPartitions(t, "[0,1]"); both != total.Value {
		t.Errorf("Count(partitions=[0,1]) = %d, want the unfiltered total %d", both, total.Value)
	}

	// An offset filter set past every real offset in a partition clamps to that partition's own
	// end (freshWindows' own clamp), so a partition scoped to it alone must count to exactly zero
	// — proving the offset half of the filter is honoured too, not just partition selection.
	pastEndFilter := `{"offset":"999999999","partitions":[0],"timestampMs":null}`
	pastEnd, err := a.Count(ctx, adapters.CountRequest{Path: path, Filter: &pastEndFilter}, adapters.NewOpCtx("op-12e"))
	if err != nil {
		t.Fatalf("Count(offset filter past end): %v", err)
	}
	if pastEnd.Value != 0 {
		t.Errorf("Count(offset filter past end) = %d, want 0", pastEnd.Value)
	}
}

// 13. preview/mutate: update/delete/execute stay unsupported (D13, canUpdate/canDelete false).
// Only insert (produce) is supported — see test 16 for that path working end to end.
func TestKafka_UpdateDeleteExecuteStayUnsupported(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	previewPlan := model.MutationPlan{Path: topicPath(f, testsupport.KafkaOrdersTopic), Ops: []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{}}}}
	if _, err := a.Preview(previewPlan); err == nil {
		t.Fatal("Preview: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Preview code = %v, want E_UNSUPPORTED", code)
	}

	updatePlan := model.MutationPlan{Path: topicPath(f, testsupport.KafkaOrdersTopic), Ops: []model.MutationRowOp{{Kind: "update", Key: model.RowValues{}, Changes: model.RowValues{}}}}
	if _, err := a.Mutate(context.Background(), updatePlan, adapters.NewOpCtx("op-13a")); err == nil {
		t.Fatal("Mutate update: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Mutate code = %v, want E_UNSUPPORTED", code)
	}

	req := model.ConsoleRequest{Path: topicPath(f, testsupport.KafkaOrdersTopic), Statements: []string{"x"}}
	if _, err := a.Execute(context.Background(), req, adapters.NewOpCtx("op-13b")); err == nil {
		t.Fatal("Execute: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Execute code = %v, want E_UNSUPPORTED", code)
	}
}

// 14. cancel is a permanent no-op (D6/D14).
func TestKafka_Cancel_PermanentNoOp(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	ok, err := a.Cancel(context.Background(), "some-op-id")
	if err != nil || ok {
		t.Errorf("Cancel = %v/%v, want false/nil", ok, err)
	}
}

// 15. read: an already-cancelled context aborts the browse before anything runs.
func TestKafka_Read_AlreadyCancelledContext(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Read(ctx, offsetRead(topicPath(f, testsupport.KafkaOrdersTopic), 10), adapters.NewOpCtx("op-15"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
		t.Errorf("code = %v, want E_CANCELLED", code)
	}
}

// P174: a key, value and header that are not valid UTF-8 arrive as base64 with the binary row marked,
// never as replacement characters.
func TestKafka_Read_BinaryPayloadsAreMarkedBase64(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	const topic = "binary-payload-topic"
	testsupport.CreateTopic(t, f, topic)
	raw := []byte{0xff, 0xfe, 0x00, 0x80}
	rec := &kgo.Record{Topic: topic, Key: raw, Value: raw, Headers: []kgo.RecordHeader{{Key: "h", Value: raw}}}
	if err := f.Client.ProduceSync(context.Background(), rec).FirstErr(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	p, err := a.Read(context.Background(), offsetRead(topicPath(f, topic), 10), adapters.NewOpCtx("op-binary"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := p.(page.StreamPage)
	if sp.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", sp.RowCount)
	}
	const want = "//4AgA==" // base64 of ff fe 00 80
	if k := testsupport.StreamKeyAt(t, sp, 0); k == nil || *k != want || !page.IsBinary(sp.Keys, 0) {
		t.Errorf("key = %v binary = %v, want %s binary", k, page.IsBinary(sp.Keys, 0), want)
	}
	if b := testsupport.StreamBodyAt(t, sp, 0); b == nil || *b != want || !page.IsBinary(sp.Bodies, 0) {
		t.Errorf("body = %v binary = %v, want %s binary", b, page.IsBinary(sp.Bodies, 0), want)
	}
	if h := testsupport.StreamHeadersAt(t, sp, 0); h == nil || *h != `{"h":{"base64":"`+want+`"}}` {
		t.Errorf("headers = %v, want h as {base64: %s}", h, want)
	}
}

// 15b. A mid-browse cancellation — the case scenario 15 alone cannot reach, since an
// already-cancelled context never reaches PollRecords. kgo.Client.PollRecords returns a
// Fetches, not an error, and injects a fake fetch carrying ctx.Err() on cancellation — a literal
// port that inspects only returned errors would keep polling against a dead context.
//
// A record count alone is a speed race: a faster broker or runner finishes before the cancel. A
// broker-side consumer quota on the adapter's client id makes the browse last seconds, so the
// cancel always lands mid-flight.
// adapterClientID mirrors kgo.ClientID in client.go; a mismatch stops the quota applying and the
// cancellation test fails with "want an error".
const adapterClientID = "kira-studio"

func throttleAdapterFetches(t *testing.T, f *testsupport.KafkaFixture, bytesPerSec float64) {
	t.Helper()
	name := adapterClientID
	entity := kadm.ClientQuotaEntity{{Type: "client-id", Name: &name}}
	alter := func(op kadm.AlterClientQuotaOp) {
		res, err := f.Admin.AlterClientQuotas(context.Background(), []kadm.AlterClientQuotaEntry{{Entity: entity, Ops: []kadm.AlterClientQuotaOp{op}}})
		if err != nil {
			t.Fatalf("AlterClientQuotas: %v", err)
		}
		for _, r := range res {
			if r.Err != nil {
				t.Fatalf("AlterClientQuotas %v: %v (%s)", r.Entity, r.Err, r.ErrMessage)
			}
		}
	}
	waitQuota := func(want bool) {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			got, err := f.Admin.DescribeClientQuotas(context.Background(), true, []kadm.DescribeClientQuotaComponent{{Type: "client-id", MatchName: &name}})
			if err == nil && (len(got) > 0) == want {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("client-id quota for %s did not reach present=%v within 10s", name, want)
	}
	alter(kadm.AlterClientQuotaOp{Key: "consumer_byte_rate", Value: bytesPerSec})
	t.Cleanup(func() {
		alter(kadm.AlterClientQuotaOp{Key: "consumer_byte_rate", Remove: true})
		waitQuota(false)
	})
	waitQuota(true)
}

func TestKafka_Read_MidBrowseCancellation(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	const topic = "browse-cancel-topic"
	testsupport.CreateTopic(t, f, topic)
	const n = 20000
	// Random pad: the producer compresses batches, so a repetitive pad would fit one fetch.
	rng := rand.New(rand.NewSource(1))
	pad := make([]byte, 150)
	records := make([]*kgo.Record, n)
	for i := 0; i < n; i++ {
		rng.Read(pad)
		records[i] = &kgo.Record{Topic: topic, Value: []byte(fmt.Sprintf(`{"seq":%d,"pad":"%x"}`, i, pad))}
	}
	if err := f.Client.ProduceSync(context.Background(), records...).FirstErr(); err != nil {
		t.Fatalf("seed %s: %v", topic, err)
	}

	throttleAdapterFetches(t, f, 64*1024)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := a.Read(ctx, offsetRead(topicPath(f, topic), n), adapters.NewOpCtx("op-15b"))
		errCh <- err
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("want an error")
		}
		if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
			t.Errorf("code = %v, want E_CANCELLED", code)
		}
		if elapsed > 5*time.Second {
			t.Errorf("Read returned after %s, want well under FetchMaxWait", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Read did not return within 10s of cancellation")
	}
}

// 16. mutate: producing a message actually appears in a fresh browse (canInsert). P58e E27: its
// own topic, never empty-topic — orders and empty-topic stay read-only fixtures.
func TestKafka_Mutate_ProduceThenBrowse(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	const topic = "test-16-produce"
	testsupport.CreateTopic(t, f, topic)

	plan := model.MutationPlan{
		Path: topicPath(f, topic),
		Ops: []model.MutationRowOp{{
			Kind: "insert",
			Values: model.RowValues{
				{Name: "$key", Value: testsupport.Strp("produced-key")},
				{Name: "$body", Value: testsupport.Strp(`{"seq":999}`)},
				{Name: "$headers", Value: nil},
			},
		}},
	}
	result, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-16a"))
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", result.AffectedRows)
	}

	p, err := a.Read(ctx, offsetRead(topicPath(f, topic), 10), adapters.NewOpCtx("op-16b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := p.(page.StreamPage)
	if sp.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", sp.RowCount)
	}
	key := testsupport.StreamKeyAt(t, sp, 0)
	if key == nil || *key != "produced-key" {
		t.Errorf("key = %v, want produced-key", key)
	}
	body := testsupport.StreamBodyAt(t, sp, 0)
	if body == nil || *body != `{"seq":999}` {
		t.Errorf("body = %v, want {\"seq\":999}", body)
	}
}

// A browse joins no consumer group. A browse never creates group state. Snapshots adm.ListGroups
// before and after paging the whole of ORDERS_TOPIC at a small page size (several ephemeral
// consumers under the hood) and asserts the two sets are equal — stronger than "the seeded group is
// still present", since it also catches a browse that deletes or mutates group state.
func TestKafka_Read_BrowseJoinsNoConsumerGroup(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	before, err := f.Admin.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups before: %v", err)
	}

	cursor := model.PageCursor{Mode: "offset", Offset: 0}
	for guard := 0; guard < testsupport.KafkaOrdersMessageCount+2; guard++ {
		req := adapters.ReadRequest{Path: topicPath(f, testsupport.KafkaOrdersTopic), PageSize: 2, Cursor: cursor}
		p, err := a.Read(ctx, req, adapters.NewOpCtx(fmt.Sprintf("op-17-%d", guard)))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		sp := p.(page.StreamPage)
		if !sp.Position.HasMore {
			break
		}
		if sp.Position.NextToken == nil {
			t.Fatal("expected a nextToken on a truncated page")
		}
		cursor = model.PageCursor{Mode: "after", Token: *sp.Position.NextToken}
	}

	after, err := f.Admin.ListGroups(ctx)
	if err != nil {
		t.Fatalf("ListGroups after: %v", err)
	}
	if !sameStrings(before.Groups(), after.Groups()) {
		t.Errorf("groups before = %v, after = %v, want equal (P10 D6)", before.Groups(), after.Groups())
	}
}

// 18. a browse commits no offsets (D19/D30). Rewritten (§5.3) to ask the broker directly rather
// than through the adapter's own definition() — going below the adapter matters, since a bug in
// buildGroupDefinition that hid rows would still let an adapter-level assertion pass.
func TestKafka_Read_BrowseCommitsNoOffsets(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	req := offsetRead(topicPath(f, testsupport.KafkaOrdersTopic), testsupport.KafkaOrdersMessageCount)
	if _, err := a.Read(ctx, req, adapters.NewOpCtx("op-18")); err != nil {
		t.Fatalf("Read: %v", err)
	}

	offsets, err := f.Admin.FetchOffsets(ctx, "kira-studio-browse")
	if err != nil {
		return // the browse group was never created — exactly as promised
	}
	if got := len(offsets[testsupport.KafkaOrdersTopic]); got != 0 {
		t.Errorf("committed offsets for %s under kira-studio-browse = %d rows, want 0", testsupport.KafkaOrdersTopic, got)
	}
}

// A timestamp filter still seeks. Learns each seeded message's real timestamp rather
// than assuming a hand-picked value falls between two of them, and seeks via a hand-built
// KafkaStreamFilter JSON string (packages/shared/domain/streamFilter.ts's own wire shape:
// {"offset":null,"partitions":[],"timestampMs":N}) — the same field ReadRequest.Filter already
// carries for every adapter.
func TestKafka_Read_TimestampFilterSeeks(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()

	full, err := a.Read(ctx, offsetRead(topicPath(f, testsupport.KafkaOrdersTopic), testsupport.KafkaOrdersMessageCount), adapters.NewOpCtx("op-19a"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := full.(page.StreamPage)
	timestamps := make([]int64, sp.RowCount)
	for r := 0; r < sp.RowCount; r++ {
		ts := testsupport.StreamTimestampAt(t, sp, r)
		if ts == nil {
			t.Fatalf("row %d: timestamp is nil", r)
		}
		parsed, err := time.Parse(time.RFC3339Nano, *ts)
		if err != nil {
			t.Fatalf("row %d: timestamp %q: %v", r, *ts, err)
		}
		timestamps[r] = parsed.UnixMilli()
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
	boundary := timestamps[len(timestamps)/2]
	expectedCount := 0
	for _, ts := range timestamps {
		if ts >= boundary {
			expectedCount++
		}
	}

	filterJSON := fmt.Sprintf(`{"offset":null,"partitions":[],"timestampMs":%d}`, boundary)
	req := adapters.ReadRequest{
		Path: topicPath(f, testsupport.KafkaOrdersTopic), PageSize: testsupport.KafkaOrdersMessageCount,
		Filter: &filterJSON, Cursor: model.PageCursor{Mode: "offset", Offset: 0},
	}
	filtered, err := a.Read(ctx, req, adapters.NewOpCtx("op-19b"))
	if err != nil {
		t.Fatalf("Read filtered: %v", err)
	}
	fp := filtered.(page.StreamPage)
	if fp.RowCount != expectedCount {
		t.Errorf("RowCount = %d, want %d", fp.RowCount, expectedCount)
	}
	for r := 0; r < fp.RowCount; r++ {
		ts := testsupport.StreamTimestampAt(t, fp, r)
		if ts == nil {
			t.Fatalf("row %d: timestamp is nil", r)
		}
		parsed, err := time.Parse(time.RFC3339Nano, *ts)
		if err != nil {
			t.Fatalf("row %d: timestamp %q: %v", r, *ts, err)
		}
		if parsed.UnixMilli() < boundary {
			t.Errorf("row %d: timestamp %s (%d ms) < boundary %d ms", r, *ts, parsed.UnixMilli(), boundary)
		}
	}
}

// 20. an oversized start offset is refused, not truncated.
// Number.MAX_SAFE_INTEGER+1 is an ordinary int64 in Go (toNativeOffset's guard is deleted, not
// ported — Go's own int64 removes the hazard it existed for), so this hand-crafts a page token
// whose window offset exceeds int64's own range (2^63) instead. NOTE: this constructs the
// adapter's own internal per-partition window JSON by hand (field names "partition"/"next"/"end",
// matching read.ts's own naming) — read.go is the actual source of truth for this shape and
// this test may need adjusting once it exists.
func TestKafka_Read_OversizedOffsetTokenIsRefused(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)

	const oversized = "9223372036854775808" // int64 max is 9223372036854775807
	windowsJSON := fmt.Sprintf(`[{"partition":0,"next":%s,"end":%s}]`, oversized, oversized)
	pageSize := 10
	fingerprint := adapters.RequestFingerprint(struct {
		Topic    string  `json:"topic"`
		PageSize int     `json:"pageSize"`
		Filter   *string `json:"filter"`
	}{Topic: testsupport.KafkaOrdersTopic, PageSize: pageSize, Filter: nil})
	token := adapters.EncodePageToken([]string{windowsJSON}, fingerprint)
	req := adapters.ReadRequest{
		Path: topicPath(f, testsupport.KafkaOrdersTopic), PageSize: pageSize,
		Cursor: model.PageCursor{Mode: "after", Token: token},
	}

	_, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-20"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeContract {
		t.Errorf("code = %v, want E_CONTRACT", code)
	}
	if ae, ok := err.(*adapters.Error); ok && !strings.Contains(ae.Message, "malformed page token") {
		t.Errorf("message = %q, want to mention %q", ae.Message, "malformed page token")
	}
}

// 21. read: a partition ending in a transaction's commit marker still terminates. Re-baselined
// against franz-go: a partition's high watermark counts a transaction's own commit marker as an
// offset the consumer never receives as a delivered message, so `next` can land one behind `end`
// forever unless the clamp fires on the provably-drained watermark comparison. This is the single
// most important test in the suite — the assertion that fails against a naive port is the third
// one below, not the first. Drives the driver under test's own franz-go client directly
// (kgo.TransactionalID + BeginTransaction/ProduceSync/EndTransaction), the same way produce.ts
// drove node-rdkafka's classic Producer, so gap-topic is dedicated and transaction-only.
func TestKafka_Read_TransactionCommitMarkerGapStillTerminates(t *testing.T) {
	f := testsupport.StartKafka(t)
	ctx := context.Background()

	const gapTopic = "gap-topic"
	testsupport.CreateTopic(t, f, gapTopic)

	txClient, err := kgo.NewClient(
		kgo.SeedBrokers(fmt.Sprintf("%s:%d", *f.Config.Host, *f.Config.Port)),
		kgo.TransactionalID("kira-test-gap-txn"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer txClient.Close()

	if err := txClient.BeginTransaction(); err != nil {
		t.Fatalf("BeginTransaction: %v", err)
	}
	rec := &kgo.Record{Topic: gapTopic, Key: []byte("gap-key"), Value: []byte(`{"seq":0}`)}
	if err := txClient.ProduceSync(ctx, rec).FirstErr(); err != nil {
		t.Fatalf("ProduceSync: %v", err)
	}
	if err := txClient.EndTransaction(ctx, kgo.TryCommit); err != nil {
		t.Fatalf("EndTransaction: %v", err)
	}

	a := connectedAdapter(t, f)
	var lastPage page.StreamPage
	cursor := model.PageCursor{Mode: "offset", Offset: 0}
	// Read to exhaustion, mirroring test 8's own paging loop — the transactional message is the
	// only row this topic will ever produce, so the very first page is already terminal.
	for guard := 0; guard < 4; guard++ {
		req := adapters.ReadRequest{Path: topicPath(f, gapTopic), PageSize: 10, Cursor: cursor}
		p, err := a.Read(ctx, req, adapters.NewOpCtx(fmt.Sprintf("op-21-%d", guard)))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		lastPage = p.(page.StreamPage)
		if !lastPage.Position.HasMore {
			break
		}
		if lastPage.Position.NextToken == nil {
			t.Fatal("expected a nextToken on a truncated page")
		}
		cursor = model.PageCursor{Mode: "after", Token: *lastPage.Position.NextToken}
	}
	if lastPage.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", lastPage.RowCount)
	}
	if got := rowSeq(t, lastPage, 0); got != 0 {
		t.Errorf("seq = %d, want 0", got)
	}
	// The assertion that fails against the pre-fix tree: `hasMore` stayed true and `nextToken`
	// non-null forever, because `next` (stuck at the commit marker's offset) never caught up to
	// `end` (the watermark including it) through any number of empty re-browses.
	if lastPage.Position.HasMore {
		t.Error("HasMore = true, want false (P43 iter2 F19/D26 clamp did not fire)")
	}
	if lastPage.Position.NextToken != nil {
		t.Error("NextToken != nil, want nil")
	}
}

// TestKafka_Read_ExactlyPageCappedGapEventuallyTerminates: the preceding test's page size (10) is
// larger than the topic's one real record, so the very first
// round already delivers with room to spare (collected < req.PageSize) — advanceWindows' own clamp
// fires immediately in that same, uncapped round. This test instead sets PageSize to exactly the
// real record count, so the delivering round is *exactly* capped (collected == req.PageSize) and
// advanceWindows deliberately skips clamping it (a capped round proves nothing on its own — see
// advanceWindows' own doc comment). The next page's request must then poll the transaction commit
// marker's now-unreachable offset with nothing left to deliver, exhausting maxEmptyPolls with no
// round ever reporting a fresh watermark — exactly the gap advanceWindows' per-round clamp cannot
// close by itself, which readTopic's own exhaustedByEmptyPolls fallback exists to close.
// Pre-fix, `next` would advance to the following poll's request only until the two topic offsets
// were consumed once, and `hasMore`/`nextToken` would latch true forever afterward: every further
// Next click return a 0-row page after a ~2s stall (maxEmptyPolls rounds at pollTimeout each).
func TestKafka_Read_ExactlyPageCappedGapEventuallyTerminates(t *testing.T) {
	f := testsupport.StartKafka(t)
	ctx := context.Background()

	const gapTopic = "exactly-capped-gap-topic"
	testsupport.CreateTopic(t, f, gapTopic)

	txClient, err := kgo.NewClient(
		kgo.SeedBrokers(fmt.Sprintf("%s:%d", *f.Config.Host, *f.Config.Port)),
		kgo.TransactionalID("kira-test-exactly-capped-gap-txn"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer txClient.Close()

	if err := txClient.BeginTransaction(); err != nil {
		t.Fatalf("BeginTransaction: %v", err)
	}
	rec := &kgo.Record{Topic: gapTopic, Key: []byte("gap-key"), Value: []byte(`{"seq":0}`)}
	if err := txClient.ProduceSync(ctx, rec).FirstErr(); err != nil {
		t.Fatalf("ProduceSync: %v", err)
	}
	if err := txClient.EndTransaction(ctx, kgo.TryCommit); err != nil {
		t.Fatalf("EndTransaction: %v", err)
	}

	a := connectedAdapter(t, f)
	var lastPage page.StreamPage
	cursor := model.PageCursor{Mode: "offset", Offset: 0}
	// PageSize 1 == the topic's one real record, so the first delivering round is exactly capped.
	// Bounded at 4 rounds (comfortably above the 1-2 actually needed) so a regression that never
	// terminates fails this test instead of hanging the whole suite.
	for guard := 0; guard < 4; guard++ {
		req := adapters.ReadRequest{Path: topicPath(f, gapTopic), PageSize: 1, Cursor: cursor}
		p, err := a.Read(ctx, req, adapters.NewOpCtx(fmt.Sprintf("op-cap-gap-%d", guard)))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		lastPage = p.(page.StreamPage)
		if !lastPage.Position.HasMore {
			break
		}
		if lastPage.Position.NextToken == nil {
			t.Fatal("expected a nextToken on a truncated page")
		}
		cursor = model.PageCursor{Mode: "after", Token: *lastPage.Position.NextToken}
	}
	if lastPage.Position.HasMore {
		t.Error("HasMore = true, want false — the exhausted gap must eventually clamp even though no round ever reported a fresh watermark for it")
	}
	if lastPage.Position.NextToken != nil {
		t.Error("NextToken != nil, want nil")
	}
}

// F14: when some messages of a batch land and one is refused, the error says which, so a retry
// does not duplicate the ones that landed.
func TestKafka_Mutate_PartialBatchFailureNamesWhatLanded(t *testing.T) {
	f := testsupport.StartKafka(t)
	a := connectedAdapter(t, f)
	ctx := context.Background()
	const topic = "test-14-partial"
	testsupport.CreateTopic(t, f, topic)

	insert := func(body string) model.MutationRowOp {
		return model.MutationRowOp{Kind: "insert", Values: model.RowValues{
			{Name: "$body", Value: testsupport.Strp(body)}, {Name: "$headers", Value: testsupport.Strp(`{"b":"1","a":"2","b":"3"}`)},
		}}
	}
	tooLarge := strings.Repeat("x", 3<<20)
	plan := model.MutationPlan{Path: topicPath(f, topic), Ops: []model.MutationRowOp{insert("one"), insert(tooLarge), insert("three")}}
	_, err := a.Mutate(ctx, plan, adapters.NewOpCtx("op-14a"))
	if err == nil {
		t.Fatal("want an error for the oversized message")
	}
	if !strings.Contains(err.Error(), "2 of 3 messages were produced") || !strings.Contains(err.Error(), "Messages 2 failed") {
		t.Errorf("error = %q, want it to name 2 of 3 produced and message 2 failed", err)
	}
	p, err := a.Read(ctx, offsetRead(topicPath(f, topic), 10), adapters.NewOpCtx("op-14b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if rc := p.(page.StreamPage).RowCount; rc != 2 {
		t.Errorf("RowCount = %d, want the 2 acknowledged messages", rc)
	}
}

func TestKafka_Connect_CancelledCtxReturns(t *testing.T) {
	f := testsupport.StartKafka(t)
	proxy := testsupport.StartPausableProxy(t, f.Config)
	testsupport.ConnectCancelScenario(t, newAdapter(t), proxy.Config(), proxy)
}
