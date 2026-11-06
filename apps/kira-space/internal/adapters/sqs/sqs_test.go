// Ported from packages/db-fixtures/sqs.spec.ts, case by case where practical — the spec's own
// numbering is kept in each test's name so the two can be diffed. The cases that carry the most
// weight: opening the definition must not receive a message (scenario 6), the headers cell matches
// the hand encoder byte-for-byte (new), a second read and count issue no second GetQueueUrl
// (scenarios 15/16, rewritten around the counting proxy), and repeated small polls eventually see
// every message (scenario 8). Every mutating test creates its own queue — a stricter rule than the
// TypeScript's own, since Go's testing package gives no top-to-bottom single-process ordering
// guarantee.
package sqs_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	_ "github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/sqs"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.StopSqs()
	os.Exit(code)
}

var deps = adapters.Deps{Log: func(level, message string) {}}

func newAdapter(t *testing.T) adapters.Adapter {
	t.Helper()
	a, err := adapters.CreateAdapter("sqs", deps)
	if err != nil {
		t.Fatalf("CreateAdapter: %v", err)
	}
	return a
}

func connectedAdapter(t *testing.T, fixture *testsupport.SqsFixture) adapters.Adapter {
	t.Helper()
	a := newAdapter(t)
	if _, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("connect")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(context.Background()) })
	return a
}

// nodePath is testsupport.FixtureNodePath instantiated for *SqsFixture (P107 I2-28).
var nodePath = testsupport.FixtureNodePath[*testsupport.SqsFixture]

func queuePath(fixture *testsupport.SqsFixture, name string) model.NodePath {
	return nodePath(fixture, testsupport.Seg("queue", name))
}

// offsetRead is testsupport.OffsetRead (P107 I2-28).
var offsetRead = testsupport.OffsetRead

// 1. connect / disconnect
func TestSqs_ConnectDisconnect(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := newAdapter(t)

	info, err := a.Connect(context.Background(), fixture.Config, adapters.NewOpCtx("op-1"))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if info.ServerVersion != "Amazon SQS" {
		t.Errorf("ServerVersion = %q, want Amazon SQS", info.ServerVersion)
	}
	if err := a.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

// 2. an unparseable URI is rejected at connect time. P25 §1.5c/§2.8 row 3: this is a pre-connect
// configuration failure, not an ordinary query-time condition — E_CONNECT, not E_QUERY.
func TestSqs_Connect_UnparseableURI(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	badCfg := fixture.Config
	bad := "not a valid uri at all"
	badCfg.URI = &bad

	a := newAdapter(t)
	_, err := a.Connect(context.Background(), badCfg, adapters.NewOpCtx("op-2"))
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeConnect {
		t.Errorf("code = %v, want E_CONNECT", code)
	}
}

// 3. cap honesty.
func TestSqs_Caps(t *testing.T) {
	a := newAdapter(t)
	c := a.Caps()
	if c.Tabular {
		t.Error("Tabular = true, want false")
	}
	if !c.Stream {
		t.Error("Stream = false, want true")
	}
	if !c.Definition {
		t.Error("Definition = false, want true")
	}
	if c.SQL {
		t.Error("SQL = true, want false")
	}
	if c.ExactCount {
		t.Error("ExactCount = true, want false")
	}
	if c.Pagination != adapters.PaginationBatch {
		t.Errorf("Pagination = %v, want batch", c.Pagination)
	}
	if !c.CanInsert || c.CanUpdate || !c.CanDelete {
		t.Errorf("CanInsert/CanUpdate/CanDelete = %v/%v/%v, want true/false/true", c.CanInsert, c.CanUpdate, c.CanDelete)
	}
	if !c.Cancel {
		t.Error("Cancel = false, want true")
	}
	if c.FileTransfer {
		t.Error("FileTransfer = true, want false")
	}
	if c.SchemaColumns {
		t.Error("SchemaColumns = true, want false (a queue has no column/PK/FK metadata)")
	}
}

// P22c §4.1: the non-SQL kinds report the capability false and the method is not implemented.
func TestSqs_SchemaColumns_Unsupported(t *testing.T) {
	a := newAdapter(t)
	_, err := a.SchemaColumns(context.Background(), model.NodePath{}, adapters.NewOpCtx("op-schema-columns"))
	if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Fatalf("got %v, want E_UNSUPPORTED", err)
	}
}

// 4. tree enumeration: root is a flat queue list.
func TestSqs_Children_RootIsFlatQueueList(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), nodePath(fixture), adapters.NewOpCtx("op-4"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	names := testsupport.ChildNames(t, children)
	for _, want := range []string{testsupport.SQSOrdersQueue, testsupport.SQSDrainQueue, testsupport.SQSEmptyQueue} {
		if !testsupport.ContainsName(names, want) {
			t.Errorf("names = %v, want to contain %q", names, want)
		}
	}
	for _, n := range children.Nodes {
		if n.Kind != "queue" || n.HasChildren {
			t.Errorf("node %+v: want kind=queue, hasChildren=false", n)
		}
	}
}

// 5. children of a leaf (queue).
func TestSqs_Children_QueueIsLeaf(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	children, err := a.Children(context.Background(), queuePath(fixture, testsupport.SQSOrdersQueue), adapters.NewOpCtx("op-5"))
	if err != nil {
		t.Fatalf("Children: %v", err)
	}
	if len(children.Nodes) != 0 {
		t.Errorf("Nodes = %v, want empty", children.Nodes)
	}
}

// 6. describe stays unsupported; definition shows the queue attributes and issues no ReceiveMessage.
func TestSqs_DescribeUnsupported_DefinitionShowsAttributesWithNoMessageReceived(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	if _, err := a.Describe(ctx, queuePath(fixture, testsupport.SQSOrdersQueue), adapters.NewOpCtx("op-6a")); err == nil {
		t.Fatal("Describe: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Describe code = %v, want E_UNSUPPORTED", code)
	}

	countBefore, err := a.Count(ctx, adapters.CountRequest{Path: queuePath(fixture, testsupport.SQSOrdersQueue)}, adapters.NewOpCtx("op-6b"))
	if err != nil {
		t.Fatalf("Count before: %v", err)
	}

	def, err := a.Definition(ctx, queuePath(fixture, testsupport.SQSOrdersQueue), adapters.NewOpCtx("op-6c"))
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if def.Kind != "queue" {
		t.Errorf("Kind = %q, want queue", def.Kind)
	}
	if len(def.Sections) != 1 || def.Sections[0].Title != "Attributes" {
		t.Errorf("Sections = %+v, want one Attributes section", def.Sections)
	}
	var names []string
	for _, r := range def.Sections[0].Rows {
		names = append(names, r.Name)
	}
	wantSubset := []string{"VisibilityTimeout", "ApproximateNumberOfMessages", "QueueArn"}
	for _, w := range wantSubset {
		found := false
		for _, n := range names {
			if n == w {
				found = true
			}
		}
		if !found {
			t.Errorf("names = %v, want to contain %q", names, w)
		}
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("names = %v, not sorted", names)
			break
		}
	}

	// P23 D9: opening the definition must not receive or hide a single message.
	countAfter, err := a.Count(ctx, adapters.CountRequest{Path: queuePath(fixture, testsupport.SQSOrdersQueue)}, adapters.NewOpCtx("op-6d"))
	if err != nil {
		t.Fatalf("Count after: %v", err)
	}
	if countAfter.Value != countBefore.Value {
		t.Errorf("count changed from %d to %d after opening the definition", countBefore.Value, countAfter.Value)
	}
}

// 7. read: polls messages off a queue, batch pagination; the headers cell matches P58d D8 exactly.
func TestSqs_Read_PollsMessages_HeadersCellMatchesHandEncoder(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(queuePath(fixture, testsupport.SQSOrdersQueue), 10), adapters.NewOpCtx("op-7"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp, ok := p.(page.StreamPage)
	if !ok {
		t.Fatalf("Read returned %T, want page.StreamPage", p)
	}
	if sp.Position.Strategy != "batch" || sp.Position.HasMore || sp.Position.NextToken != nil || sp.Position.PrevToken != nil {
		t.Errorf("Position = %+v, want batch/hasMore=false/nil tokens", sp.Position)
	}
	if sp.RowCount == 0 || sp.RowCount > testsupport.SQSOrdersMessageCount {
		t.Errorf("RowCount = %d, want in (0, %d]", sp.RowCount, testsupport.SQSOrdersMessageCount)
	}
	if sp.VisibilityTimeoutSeconds == nil {
		t.Error("VisibilityTimeoutSeconds is nil, want a number")
	}

	key := testsupport.StreamKeyAt(t, sp, 0)
	if key == nil || *key == "" {
		t.Error("key is nil/empty, want the MessageId")
	}
	timestamp := testsupport.StreamTimestampAt(t, sp, 0)
	if timestamp == nil {
		t.Error("timestamp is nil, want a value")
	}
	headers := testsupport.StreamHeadersAt(t, sp, 0)
	if headers == nil {
		t.Fatal("headers is nil")
	}
	// P58d D8: exactly the fields JSON.stringify(message.MessageAttributes ?? {}) would produce —
	// no BinaryValue/StringListValues/BinaryListValues nulls, unlike a naive json.Marshal of the
	// SDK struct (M8.0's AWS-1(c) confirmed this is the actual divergence).
	if want := `{"source":{"DataType":"String","StringValue":"seed"}}`; *headers != want {
		t.Errorf("headers = %s, want %s", *headers, want)
	}
	attrs := testsupport.StreamAttrsAt(t, sp, 0)
	if attrs == nil {
		t.Fatal("attrs is nil")
	}
}

// 8. read: repeated small polls eventually see every message.
func TestSqs_Read_RepeatedSmallPollsSeeEveryMessage(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	total := 0
	for guard := 0; guard < testsupport.SQSDrainMessageCount+3 && total < testsupport.SQSDrainMessageCount; guard++ {
		p, err := a.Read(context.Background(), offsetRead(queuePath(fixture, testsupport.SQSDrainQueue), 2), adapters.NewOpCtx("op-8"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		sp := p.(page.StreamPage)
		total += sp.RowCount
		if sp.RowCount == 0 {
			break
		}
	}
	if total != testsupport.SQSDrainMessageCount {
		t.Errorf("total = %d, want %d", total, testsupport.SQSDrainMessageCount)
	}
}

// 9. read: an empty queue returns an empty page, not an error.
func TestSqs_Read_EmptyQueueIsEmptyPage(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	p, err := a.Read(context.Background(), offsetRead(queuePath(fixture, testsupport.SQSEmptyQueue), 10), adapters.NewOpCtx("op-9"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := p.(page.StreamPage)
	if sp.RowCount != 0 || sp.Position.HasMore {
		t.Errorf("RowCount/HasMore = %d/%v, want 0/false", sp.RowCount, sp.Position.HasMore)
	}
}

// 10. read: a nonexistent queue is E_QUERY, not E_NOT_FOUND.
func TestSqs_Read_NonexistentQueueIsQueryError(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	_, err := a.Read(context.Background(), offsetRead(queuePath(fixture, "this-queue-was-never-created"), 10), adapters.NewOpCtx("op-10"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

// 11. count: approximate, never exact.
func TestSqs_Count_ApproximateNeverExact(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	result, err := a.Count(context.Background(), adapters.CountRequest{Path: queuePath(fixture, testsupport.SQSEmptyQueue)}, adapters.NewOpCtx("op-11"))
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if result.Value != 0 || result.Exact {
		t.Errorf("Count = %+v, want {0 false}", result)
	}
}

// 12. preview/mutate: update stays unsupported; execute has no console.
func TestSqs_UpdateUnsupported_NoConsole(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	plan := model.MutationPlan{Path: queuePath(fixture, testsupport.SQSOrdersQueue), Ops: []model.MutationRowOp{{Kind: "update"}}}
	if _, err := a.Preview(plan); err == nil {
		t.Fatal("Preview: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Preview code = %v, want E_UNSUPPORTED", code)
	}

	if _, err := a.Mutate(context.Background(), plan, adapters.NewOpCtx("op-12")); err == nil {
		t.Fatal("Mutate: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Mutate code = %v, want E_UNSUPPORTED", code)
	}

	req := model.ConsoleRequest{Path: queuePath(fixture, testsupport.SQSOrdersQueue), Statements: []string{"x"}}
	if _, err := a.Execute(context.Background(), req, adapters.NewOpCtx("op-12b")); err == nil {
		t.Fatal("Execute: want an error")
	} else if code, _ := adapters.CodeOf(err); code != adapters.CodeUnsupported {
		t.Errorf("Execute code = %v, want E_UNSUPPORTED", code)
	}
}

// 13. cancel is a permanent no-op.
func TestSqs_Cancel_PermanentNoOp(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	ok, err := a.Cancel(context.Background(), "some-op-id")
	if err != nil || ok {
		t.Errorf("Cancel = %v/%v, want false/nil", ok, err)
	}
}

// 14. read: an already-cancelled context rejects before running anything.
func TestSqs_Read_AlreadyCancelledContext(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := a.Read(ctx, offsetRead(queuePath(fixture, testsupport.SQSEmptyQueue), 10), adapters.NewOpCtx("op-14"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeCancelled {
		t.Errorf("code = %v, want E_CANCELLED", code)
	}
}

// 15. a second read/count on the same queue issues no second GetQueueUrl (rewritten around
// P58d D10's counting proxy in place of Node's spyOn).
func TestSqs_QueueURLCache_NoRepeatGetQueueUrl(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)
	fixture.Proxy.Reset()

	if _, err := a.Read(context.Background(), offsetRead(queuePath(fixture, testsupport.SQSOrdersQueue), 10), adapters.NewOpCtx("op-15a")); err != nil {
		t.Fatalf("Read 1: %v", err)
	}
	if got := fixture.Proxy.Count(); got != 1 {
		t.Errorf("proxy count after first read = %d, want 1 (cache miss)", got)
	}

	if _, err := a.Read(context.Background(), offsetRead(queuePath(fixture, testsupport.SQSOrdersQueue), 10), adapters.NewOpCtx("op-15b")); err != nil {
		t.Fatalf("Read 2: %v", err)
	}
	if got := fixture.Proxy.Count(); got != 1 {
		t.Errorf("proxy count after second read = %d, want still 1 (cache hit)", got)
	}

	if _, err := a.Count(context.Background(), adapters.CountRequest{Path: queuePath(fixture, testsupport.SQSOrdersQueue)}, adapters.NewOpCtx("op-15c")); err != nil {
		t.Fatalf("Count: %v", err)
	}
	if got := fixture.Proxy.Count(); got != 1 {
		t.Errorf("proxy count after count() = %d, want still 1 (shared cache)", got)
	}
}

// 16. a disconnect/connect cycle re-resolves the queue URL.
func TestSqs_Disconnect_ClearsQueueURLCache(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := newAdapter(t)
	ctx := context.Background()

	if _, err := a.Connect(ctx, fixture.Config, adapters.NewOpCtx("connect-1")); err != nil {
		t.Fatalf("Connect 1: %v", err)
	}
	fixture.Proxy.Reset()
	if _, err := a.Read(ctx, offsetRead(queuePath(fixture, testsupport.SQSOrdersQueue), 10), adapters.NewOpCtx("op-16a")); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := a.Disconnect(ctx); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}

	if _, err := a.Connect(ctx, fixture.Config, adapters.NewOpCtx("connect-2")); err != nil {
		t.Fatalf("Connect 2: %v", err)
	}
	defer a.Disconnect(ctx)
	fixture.Proxy.Reset()
	if _, err := a.Read(ctx, offsetRead(queuePath(fixture, testsupport.SQSOrdersQueue), 10), adapters.NewOpCtx("op-16b")); err != nil {
		t.Fatalf("Read after reconnect: %v", err)
	}
	if got := fixture.Proxy.Count(); got != 1 {
		t.Errorf("proxy count after reconnect+read = %d, want 1 (cache was cleared)", got)
	}
}

// P174: a read-only poll hides messages only briefly and keeps no receipt handles, so a second poll
// still sees every message; the page carries the redrive policy's receive limit.
func TestSqs_Read_ReadOnlyPollKeepsMessagesVisible(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	ctx := context.Background()

	dlq, err := fixture.Client.CreateQueue(ctx, &awssqs.CreateQueueInput{QueueName: aws.String("test-p174-dlq")})
	if err != nil {
		t.Fatalf("CreateQueue(dlq): %v", err)
	}
	dlqAttrs, err := fixture.Client.GetQueueAttributes(ctx, &awssqs.GetQueueAttributesInput{
		QueueUrl: dlq.QueueUrl, AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		t.Fatalf("GetQueueAttributes(dlq): %v", err)
	}
	queueName := "test-p174-browse"
	redrive := `{"deadLetterTargetArn":"` + dlqAttrs.Attributes["QueueArn"] + `","maxReceiveCount":3}`
	q, err := fixture.Client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName:  aws.String(queueName),
		Attributes: map[string]string{"RedrivePolicy": redrive, "VisibilityTimeout": "60"},
	})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := fixture.Client.SendMessage(ctx, &awssqs.SendMessageInput{QueueUrl: q.QueueUrl, MessageBody: aws.String("m")}); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
	}

	cfg := fixture.Config
	cfg.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(ctx, cfg, adapters.NewOpCtx("connect-ro")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(ctx) })

	poll := func() page.StreamPage {
		p, err := a.Read(ctx, offsetRead(queuePath(fixture, queueName), 10), adapters.NewOpCtx("op-p174"))
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		return p.(page.StreamPage)
	}
	first := poll()
	if first.RowCount != 3 {
		t.Fatalf("first poll RowCount = %d, want 3", first.RowCount)
	}
	if first.MaxReceiveCount == nil || *first.MaxReceiveCount != 3 {
		t.Errorf("MaxReceiveCount = %v, want 3", first.MaxReceiveCount)
	}
	time.Sleep(2 * time.Second) // past the 1 s browse hide, far short of the queue's own 60 s
	if second := poll(); second.RowCount != 3 {
		t.Errorf("second poll RowCount = %d, want 3 (messages stayed visible)", second.RowCount)
	}
}

// P184: a bad filter is refused before any connection use, so no Docker is needed. An
// out-of-range value is a query error; a filter of the wrong shape is a Kira bug (E_CONTRACT).
func TestSqs_Read_RejectsInvalidVisibilityTimeout(t *testing.T) {
	a := newAdapter(t)
	for _, filter := range []string{
		`0`, `43201`, `1.5`, `"30"`, `{"visibilityTimeoutSeconds":0}`, `{"visibilityTimeoutSeconds":43201}`,
		`{"visibilityTimeoutSeconds":1.5}`, `{"visibilityTimeoutSeconds":"30"}`,
		`{"visibilityTimeoutSeconds":30,"x":1}`, `{`,
	} {
		req := testsupport.OffsetRead(model.NodePath{}, 10)
		req.Filter = &filter
		_, err := a.Read(context.Background(), req, adapters.NewOpCtx("op-p184-bad"))
		if err == nil {
			t.Errorf("filter %s: want an error", filter)
			continue
		}
		want := adapters.CodeContract
		if strings.HasPrefix(filter, `{"visibilityTimeoutSeconds":`) && (strings.HasSuffix(filter, `:0}`) || strings.HasSuffix(filter, `:43201}`)) {
			want = adapters.CodeQuery
		}
		if code, _ := adapters.CodeOf(err); code != want {
			t.Errorf("filter %s: code = %v, want %v", filter, code, want)
		}
	}
}

// P184: a read-only poll hides messages for the filter's timeout, not the queue's own 60 s.
func TestSqs_Read_ReadOnlyPollUsesChosenVisibilityTimeout(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	ctx := context.Background()

	queueName := "test-p184-hide"
	q, err := fixture.Client.CreateQueue(ctx, &awssqs.CreateQueueInput{
		QueueName:  aws.String(queueName),
		Attributes: map[string]string{"VisibilityTimeout": "60"},
	})
	if err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := fixture.Client.SendMessage(ctx, &awssqs.SendMessageInput{QueueUrl: q.QueueUrl, MessageBody: aws.String("m")}); err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
	}

	cfg := fixture.Config
	cfg.ReadOnly = true
	a := newAdapter(t)
	if _, err := a.Connect(ctx, cfg, adapters.NewOpCtx("connect-ro")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = a.Disconnect(ctx) })

	filter := `{"visibilityTimeoutSeconds":4}`
	poll := func() (page.StreamPage, string) {
		req := offsetRead(queuePath(fixture, queueName), 10)
		req.Filter = &filter
		op := adapters.NewOpCtx("op-p184")
		p, err := a.Read(ctx, req, op)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		return p.(page.StreamPage), op.Command()
	}
	first, command := poll()
	if first.RowCount != 3 {
		t.Fatalf("first poll RowCount = %d, want 3", first.RowCount)
	}
	if !strings.Contains(command, "VisibilityTimeout=4") {
		t.Errorf("command = %q, want VisibilityTimeout=4", command)
	}
	if second, _ := poll(); second.RowCount != 0 {
		t.Errorf("second poll RowCount = %d, want 0 (still hidden)", second.RowCount)
	}
	time.Sleep(5 * time.Second)
	if third, _ := poll(); third.RowCount != 3 {
		t.Errorf("third poll RowCount = %d, want 3 (released before the queue's 60 s)", third.RowCount)
	}
}

// 17. mutate: sending then deleting a message round-trips through the queue. P58d D23: its own
// fresh queue, not one of the three shared fixtures.
func TestSqs_Mutate_SendThenDeleteRoundTrips(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)
	ctx := context.Background()

	queueName := "test-17-send-delete"
	if _, err := fixture.Client.CreateQueue(ctx, &awssqs.CreateQueueInput{QueueName: aws.String(queueName)}); err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}

	sendPlan := model.MutationPlan{
		Path: queuePath(fixture, queueName),
		Ops:  []model.MutationRowOp{{Kind: "insert", Values: model.RowValues{{Name: "$body", Value: testsupport.Strp("hello")}}}},
	}
	sendResult, err := a.Mutate(ctx, sendPlan, adapters.NewOpCtx("op-17a"))
	if err != nil {
		t.Fatalf("Mutate insert: %v", err)
	}
	if sendResult.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", sendResult.AffectedRows)
	}

	p, err := a.Read(ctx, offsetRead(queuePath(fixture, queueName), 10), adapters.NewOpCtx("op-17b"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sp := p.(page.StreamPage)
	if sp.RowCount != 1 {
		t.Fatalf("RowCount = %d, want 1", sp.RowCount)
	}
	body := testsupport.StreamBodyAt(t, sp, 0)
	if body == nil || *body != "hello" {
		t.Errorf("body = %v, want hello", body)
	}
	key := testsupport.StreamKeyAt(t, sp, 0)
	if key == nil || *key == "" {
		t.Fatal("key is nil/empty, want the MessageId")
	}

	deletePlan := model.MutationPlan{
		Path: queuePath(fixture, queueName),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "messageId", Value: key}}}},
	}
	deleteResult, err := a.Mutate(ctx, deletePlan, adapters.NewOpCtx("op-17c"))
	if err != nil {
		t.Fatalf("Mutate delete: %v", err)
	}
	if deleteResult.AffectedRows != 1 {
		t.Errorf("AffectedRows = %d, want 1", deleteResult.AffectedRows)
	}
}

// 18. mutate delete: a message never received in this session has no receipt handle.
func TestSqs_Mutate_DeleteWithoutReceiptHandleIsQueryError(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	a := connectedAdapter(t, fixture)

	deletePlan := model.MutationPlan{
		Path: queuePath(fixture, testsupport.SQSOrdersQueue),
		Ops:  []model.MutationRowOp{{Kind: "delete", Key: model.RowValues{{Name: "messageId", Value: testsupport.Strp("never-received")}}}},
	}
	_, err := a.Mutate(context.Background(), deletePlan, adapters.NewOpCtx("op-18"))
	if err == nil {
		t.Fatal("want an error")
	}
	if code, _ := adapters.CodeOf(err); code != adapters.CodeQuery {
		t.Errorf("code = %v, want E_QUERY", code)
	}
}

func TestSqs_Connect_CancelledCtxReturns(t *testing.T) {
	fixture := testsupport.StartSqs(t)
	proxy := testsupport.StartPausableProxyTo(t, strings.TrimPrefix(fixture.Proxy.Endpoint, "http://"), fixture.Config)
	cfg := fixture.Config
	cfg.Options = map[string]any{"endpoint": "http://" + proxy.Addr()}
	testsupport.ConnectCancelScenario(t, newAdapter(t), cfg, proxy)
}
