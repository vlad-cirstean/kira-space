package dbflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const sqsScenario = "studio-sqs"

type sqsRig struct {
	fx   *testsupport.SqsFixture
	app  *flowharness.App
	conn model.ConnectionSummary
	ds   *flowharness.DataStream
}

func newSqsRig(t *testing.T) *sqsRig {
	t.Helper()
	flowharness.RequireDocker(t)
	fx := testsupport.StartSqs(t)
	app := flowharness.New(t)
	in := inputFromConfig(fx.Config)
	// URI mode derives options from the URI query, so the LocalStack endpoint rides there.
	in.URI = aws.String(*fx.Config.URI + "?endpoint=" + url.QueryEscape(fx.Config.Options["endpoint"].(string)))
	conn := createConn(t, app, in)
	if st := connect(t, app, conn.ID); st.Error != nil {
		t.Fatalf("connect: %s", *st.Error)
	}
	return &sqsRig{fx: fx, app: app, conn: conn, ds: app.DataStream(t)}
}

func (r *sqsRig) queue(t *testing.T, name string, attrs map[string]string) (queueURL, path string) {
	t.Helper()
	out, err := r.fx.Client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(name), Attributes: attrs})
	if err != nil {
		t.Fatalf("create queue %s: %v", name, err)
	}
	return aws.ToString(out.QueueUrl), model.EncodePath([]model.PathSegment{{Kind: "queue", Name: name}})
}

func (r *sqsRig) send(t *testing.T, url, body string) string {
	t.Helper()
	out, err := r.fx.Client.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: aws.String(url), MessageBody: aws.String(body)})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	return aws.ToString(out.MessageId)
}

// depth is the queue's visible plus in-flight message count by the service's own attributes.
func (r *sqsRig) depth(t *testing.T, url string) int {
	t.Helper()
	out, err := r.fx.Client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(url),
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages, sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var total int
	for _, v := range out.Attributes {
		var n int
		fmt.Sscan(v, &n)
		total += n
	}
	return total
}

func (r *sqsRig) poll(t *testing.T, path string) flowharness.DataPage {
	t.Helper()
	// The tab's Poll invalidates the cached page first, so each poll reaches the queue.
	if err := r.ds.Invalidate(r.conn.ID, path, "pages"); err != nil {
		t.Fatal(err)
	}
	p, _, err := r.ds.Read(flowharness.ReadArgs{ConnectionID: r.conn.ID, Path: path})
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	return p
}

// receiveCounts maps message id to the ApproximateReceiveCount cell of a polled page.
func receiveCounts(t *testing.T, p flowharness.DataPage) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, row := range p.Rows {
		var attrs map[string]string
		if err := json.Unmarshal([]byte(row[2].(string)), &attrs); err != nil {
			t.Fatal(err)
		}
		out[row[0].(string)] = attrs["ApproximateReceiveCount"]
	}
	return out
}

func asDataError(t *testing.T, err error, what string) *flowharness.DataError {
	t.Helper()
	var derr *flowharness.DataError
	if !errors.As(err, &derr) {
		t.Fatalf("%s: err = %v, want a refusal", what, err)
	}
	return derr
}

// J-STR-012 (contract studio-sqs): every explicit poll raises ApproximateReceiveCount once and nothing else
// does; deleting with a lapsed receipt names the expiry and leaves the message; deleting a message this
// session never received names the missing receipt.
func TestSqsPollCountsReceivesAndRefusesStaleDeletes(t *testing.T) {
	r := newSqsRig(t)
	dlqURL, _ := r.queue(t, "rc-dlq", nil)
	dlqArn := "arn:aws:sqs:us-east-1:000000000000:rc-dlq"
	policy, _ := json.Marshal(map[string]any{"deadLetterTargetArn": dlqArn, "maxReceiveCount": 5})
	// Visibility 0: a polled message is receivable again at once and its receipt is lapsed at once, so no test waits on a clock.
	url, path := r.queue(t, "rc-q", map[string]string{"VisibilityTimeout": "0", "RedrivePolicy": string(policy)})
	r.send(t, url, "m1")
	r.send(t, url, "m2")

	// Opening the tab reads the definition and the count; neither receives a message.
	if _, _, err := r.ds.Count(r.conn.ID, path, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := r.app.W.Tree.Definition(bridge.TreeDescribeArgs{ConnectionID: r.conn.ID, Path: path}); err != nil {
		t.Fatal(err)
	}
	first := receiveCounts(t, r.poll(t, path))
	if len(first) != 2 {
		t.Fatalf("first poll holds %d messages, want 2", len(first))
	}
	for id, n := range first {
		if n != "1" {
			t.Fatalf("message %s receive count after the first poll = %s, want 1 (something received it earlier)", id, n)
		}
	}
	// Reads that are not polls leave the count alone; the next poll raises it by exactly one.
	if _, _, err := r.ds.Count(r.conn.ID, path, nil, true); err != nil {
		t.Fatal(err)
	}
	second := r.poll(t, path)
	for id, n := range receiveCounts(t, second) {
		if n != "2" {
			t.Fatalf("message %s receive count after the second poll = %s, want 2", id, n)
		}
	}
	if depth := r.depth(t, dlqURL); depth != 0 {
		t.Fatalf("dead-letter queue holds %d messages after two polls, want 0", depth)
	}

	// The receipt from the second poll lapsed at once (visibility 0): the delete is refused, the message stays.
	id := second.Rows[0][0].(string)
	_, err := r.ds.Mutate(r.conn.ID, path, []model.MutationRowOp{del(vals("messageId", id))})
	expired := asDataError(t, err, "delete with a lapsed receipt")
	if !strings.Contains(expired.Message, "expired") {
		t.Fatalf("lapsed-receipt delete = %q, want the receipt-expired message", expired.Message)
	}
	r.app.Contract(t, sqsScenario, "mutate:receiptExpiredError", expired, contractMask)
	if depth := r.depth(t, url); depth != 2 {
		t.Fatalf("queue holds %d messages after the refused delete, want 2", depth)
	}

	// A message nobody in this session received has no receipt at all.
	goneURL, gonePath := r.queue(t, "gone-q", map[string]string{"VisibilityTimeout": "60"})
	foreign := r.send(t, goneURL, "never polled")
	_, err = r.ds.Mutate(r.conn.ID, gonePath, []model.MutationRowOp{del(vals("messageId", foreign))})
	gone := asDataError(t, err, "delete of a never-polled message")
	if !strings.Contains(gone.Message, "receipt handle is gone") {
		t.Fatalf("never-polled delete = %q, want the receipt-gone message", gone.Message)
	}
	if depth := r.depth(t, goneURL); depth != 1 {
		t.Fatalf("queue holds %d messages after the refused delete, want 1", depth)
	}
	r.app.Contract(t, sqsScenario, "mutate:receiptGoneError", gone, contractMask)
}

// J-STR-012: a fresh receipt deletes the message for good.
func TestSqsDeleteWithAFreshReceipt(t *testing.T) {
	r := newSqsRig(t)
	url, path := r.queue(t, "del-q", map[string]string{"VisibilityTimeout": "60"})
	r.send(t, url, "only")
	p := r.poll(t, path)
	if len(p.Rows) != 1 {
		t.Fatalf("poll = %d rows", len(p.Rows))
	}
	if n, err := r.ds.Mutate(r.conn.ID, path, []model.MutationRowOp{del(vals("messageId", p.Rows[0][0].(string)))}); err != nil || n != 1 {
		t.Fatalf("delete = %d, %v", n, err)
	}
	if depth := r.depth(t, url); depth != 0 {
		t.Fatalf("queue holds %d messages after the delete, want 0", depth)
	}
}

// J-STR-012: FIFO queues need a group id, and a dedup id unless the queue deduplicates by content.
func TestSqsFifoSendRules(t *testing.T) {
	r := newSqsRig(t)
	plainURL, plainPath := r.queue(t, "rules.fifo", map[string]string{"FifoQueue": "true"})
	contentURL, contentPath := r.queue(t, "content.fifo", map[string]string{"FifoQueue": "true", "ContentBasedDeduplication": "true"})
	send := func(path string, kv ...any) error {
		body := "b"
		_, err := r.ds.Mutate(r.conn.ID, path, []model.MutationRowOp{ins(vals(append([]any{"$body", body}, kv...)...))})
		return err
	}

	noGroup := asDataError(t, send(plainPath), "FIFO send without a group id")
	r.app.Contract(t, sqsScenario, "mutate:fifoNoGroupError", noGroup, contractMask)
	noDedup := asDataError(t, send(plainPath, "$messageGroupId", "g"), "FIFO send without a dedup id")
	r.app.Contract(t, sqsScenario, "mutate:fifoNoDedupError", noDedup, contractMask)
	if depth := r.depth(t, plainURL); depth != 0 {
		t.Fatalf("refused sends left %d messages", depth)
	}
	if err := send(plainPath, "$messageGroupId", "g", "$messageDeduplicationId", "d1"); err != nil {
		t.Fatalf("FIFO send with group and dedup id: %v", err)
	}
	if err := send(contentPath, "$messageGroupId", "g"); err != nil {
		t.Fatalf("FIFO send on a content-deduplicating queue: %v", err)
	}
	if depth := r.depth(t, plainURL); depth != 1 {
		t.Fatalf("plain FIFO queue holds %d, want 1", depth)
	}
	if depth := r.depth(t, contentURL); depth != 1 {
		t.Fatalf("content FIFO queue holds %d, want 1", depth)
	}
}

// J-STR-012 and decision 7 (OQ-STR-2): a repeated attribute name refuses the compose and nothing lands;
// unique names land as written.
func TestSqsDuplicateAttributeNameRefusesCompose(t *testing.T) {
	r := newSqsRig(t)
	url, path := r.queue(t, "attrs-q", map[string]string{"VisibilityTimeout": "60"})
	insert := func(headers string) []model.MutationRowOp {
		return []model.MutationRowOp{ins(vals("$body", "b", "$headers", headers))}
	}

	dup := insert(`{"trace":"a","trace":"b"}`)
	r.app.Contract(t, sqsScenario, "mutate:duplicateAttributeOps", flowharness.MutateWire(r.conn.ID, path, dup), contractMask)
	_, err := r.ds.Mutate(r.conn.ID, path, dup)
	derr := asDataError(t, err, "duplicate attribute name")
	if !strings.Contains(derr.Message, "trace") {
		t.Fatalf("refusal = %q, want it to name the attribute", derr.Message)
	}
	r.app.Contract(t, sqsScenario, "mutate:duplicateAttributeError", derr, contractMask)
	if depth := r.depth(t, url); depth != 0 {
		t.Fatalf("the refused compose left %d messages", depth)
	}

	if _, err := r.ds.Mutate(r.conn.ID, path, insert(`{"trace":"a","type":"x"}`)); err != nil {
		t.Fatalf("unique attributes: %v", err)
	}
	out, err := r.fx.Client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(url), MessageAttributeNames: []string{"All"}, WaitTimeSeconds: 2})
	if err != nil || len(out.Messages) != 1 {
		t.Fatalf("second client receive = %v, %v; want one message", out, err)
	}
	got := out.Messages[0].MessageAttributes
	if aws.ToString(got["trace"].StringValue) != "a" || aws.ToString(got["type"].StringValue) != "x" || len(got) != 2 {
		t.Fatalf("attributes = %v, want trace=a, type=x", got)
	}
}
