package sqs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
)

const (
	receiveLimit    = 10 // ReceiveMessage's own hard per-call cap on MaxNumberOfMessages
	waitTimeSeconds = 1  // short poll per call; looped rather than one long wait
)

// receiptHandleCap is a bound on the adapter-local receiptHandles map — a receipt handle
// is only useful for as long as the message it names is still in flight, so unbounded growth
// across many polls would be a slow leak of tokens nothing will ever look up again. FIFO eviction
// (oldest first) approximates "least likely to still be wanted"; Go's own map has no insertion
// order to lean on the way JS's Map does, so receiptHandles is a mutex-guarded map plus an
// explicit eviction queue (see adapter.go's receiptHandles type).
const receiptHandleCap = 5000

// sqsMessageAttribute is the hand-written encoder over types.MessageAttributeValue: fields
// are emitted only when non-nil/non-empty, in this fixed order, so the headers cell matches the
// JavaScript adapter's JSON.stringify(message.MessageAttributes ?? {}) — which drops undefined
// fields entirely — rather than json.Marshal's own all-fields-with-nulls struct shape.
type sqsMessageAttribute struct {
	DataType         string   `json:"DataType,omitempty"`
	StringValue      *string  `json:"StringValue,omitempty"`
	BinaryValue      *string  `json:"BinaryValue,omitempty"` // base64
	StringListValues []string `json:"StringListValues,omitempty"`
	BinaryListValues []string `json:"BinaryListValues,omitempty"` // base64, one per entry
}

// encodeHeaders is P58d D8. The outer map is a plain map[string]sqsMessageAttribute, which
// encoding/json marshals with sorted keys — a harmless difference from JS's insertion order that
// no consumer observes (the cell is parsed, not compared byte-for-byte against a live order).
func encodeHeaders(attrs map[string]types.MessageAttributeValue) (string, error) {
	out := make(map[string]sqsMessageAttribute, len(attrs))
	for name, v := range attrs {
		enc := sqsMessageAttribute{DataType: aws.ToString(v.DataType), StringValue: v.StringValue}
		if len(v.BinaryValue) > 0 {
			s := base64.StdEncoding.EncodeToString(v.BinaryValue)
			enc.BinaryValue = &s
		}
		if len(v.StringListValues) > 0 {
			enc.StringListValues = v.StringListValues
		}
		if len(v.BinaryListValues) > 0 {
			list := make([]string, len(v.BinaryListValues))
			for i, b := range v.BinaryListValues {
				list[i] = base64.StdEncoding.EncodeToString(b)
			}
			enc.BinaryListValues = list
		}
		out[name] = enc
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// defaultVisibilityTimeout is F10's own fallback when fetchVisibilityTimeout couldn't read the
// queue's own attribute (a best-effort call, see its own comment) — AWS's own documented default
// for a queue whose VisibilityTimeout was never explicitly set.
const defaultVisibilityTimeout = 30 * time.Second

// receiptHandleEntry is one cached receipt handle plus the bookkeeping F10 needs to tell a still-
// valid handle from one whose visibility timeout has already passed: receivedAt (when this poll
// received the message) and visibilityTimeout (the queue's own attribute at receive time).
type receiptHandleEntry struct {
	handle            string
	receivedAt        time.Time
	visibilityTimeout time.Duration
}

// receiptHandles is the adapter-local, mutex-guarded FIFO map SqsAdapter threads through from
// pollQueue (keyed by MessageId) to mutate.go — P58d D9. A receipt handle is an AWS-internal token
// with no reason to round-trip through the wire protocol, and it is only ever valid for the
// message that was actually received, not a stable identifier of the message itself. Two
// JavaScript guarantees this type must reproduce by hand: single-threadedness (a mutex) and Map's
// insertion order (an explicit eviction queue) — Go's own map iteration order is deliberately
// randomised, so a literal port would evict an arbitrary handle rather than the oldest one.
type receiptHandles struct {
	mu      sync.Mutex
	entries map[string]receiptHandleEntry
	order   []string
}

func newReceiptHandles() *receiptHandles {
	return &receiptHandles{entries: map[string]receiptHandleEntry{}}
}

// set records handle as messageID's own current receipt handle (F10: alongside the moment it was
// received and the queue's own visibility timeout at that moment), evicting the oldest entry once
// the cap is exceeded.
func (r *receiptHandles) set(messageID, handle string, visibilityTimeout time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[messageID]; !exists {
		r.order = append(r.order, messageID)
	}
	r.entries[messageID] = receiptHandleEntry{handle: handle, receivedAt: time.Now(), visibilityTimeout: visibilityTimeout}
	for len(r.order) > receiptHandleCap {
		oldest := r.order[0]
		r.order = r.order[1:]
		delete(r.entries, oldest)
	}
}

// forDelete is mutate.go's own lookup: refuses a delete outright, with a clear message,
// rather than letting AWS silently accept a stale receipt handle and report success while another
// consumer holds the message — once past the visibility timeout the handle recorded at receive
// time, AWS can accept a DeleteMessage call using it with no guarantee it still names the same
// in-flight receipt.
func (r *receiptHandles) forDelete(messageID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[messageID]
	if !ok {
		return "", adapters.T(adapters.CodeQuery, "backend.studio.adapter.sqsReceiptGone.body", nil, nil)
	}
	if time.Since(e.receivedAt) >= e.visibilityTimeout {
		return "", adapters.T(adapters.CodeQuery, "backend.studio.adapter.sqsReceiptExpired.body", nil, nil)
	}
	return e.handle, nil
}

func (r *receiptHandles) delete(messageID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, messageID)
	for i, id := range r.order {
		if id == messageID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

func (r *receiptHandles) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = map[string]receiptHandleEntry{}
	r.order = nil
}

func pushMessage(builder *page.StreamPageBuilder, m types.Message, handles *receiptHandles, visibilityTimeout time.Duration) error {
	attrs := m.Attributes
	var timestamp *string
	if raw, ok := attrs["SentTimestamp"]; ok {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
			// P58d D11: JS toISOString()'s exact format.
			s := time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z07:00")
			timestamp = &s
		}
	}
	headers, err := encodeHeaders(m.MessageAttributes)
	if err != nil {
		return err
	}
	attrsJSON, err := json.Marshal(attrs)
	if err != nil {
		return err
	}
	// SQS rejects a body outside its Unicode set at send, so this never reports binary; it keeps one
	// cell contract with Kafka.
	var body *string
	var bodyBinary bool
	if m.Body != nil {
		text, binary := page.BytesCell([]byte(*m.Body))
		body, bodyBinary = &text, binary
	}
	builder.Push(page.StreamRow{
		Key:        m.MessageId,
		Headers:    headers,
		Attrs:      string(attrsJSON),
		Timestamp:  timestamp,
		Body:       body,
		BodyBinary: bodyBinary,
	})
	if handles != nil && m.MessageId != nil && m.ReceiptHandle != nil {
		handles.set(*m.MessageId, *m.ReceiptHandle, visibilityTimeout)
	}
	return nil
}

// position is P58d D11 (§4.5): SQS has no addressable position at all — every poll is an
// independent, non-resumable snapshot.
func position(pageSize int) page.PagePosition {
	return page.PagePosition{Offset: nil, PageSize: pageSize, HasMore: false, NextToken: nil, PrevToken: nil, Strategy: "batch"}
}

// queueAttributes is what one GetQueueAttributes call tells a poll: the queue's visibility timeout
// and, when a redrive policy exists, how many receives move a message to its dead-letter queue.
type queueAttributes struct {
	visibilityTimeoutSeconds *int
	maxReceiveCount          *int
}

func fetchQueueAttributes(ctx context.Context, client *sqs.Client, queueURL string) queueAttributes {
	result, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameVisibilityTimeout, types.QueueAttributeNameRedrivePolicy,
		},
	})
	if err != nil {
		return queueAttributes{} // best-effort, mirrors redis/read.go's MEMORY USAGE fallback
	}
	var out queueAttributes
	if n, err := strconv.Atoi(result.Attributes["VisibilityTimeout"]); err == nil {
		out.visibilityTimeoutSeconds = &n
	}
	if raw := result.Attributes["RedrivePolicy"]; raw != "" {
		var policy struct {
			MaxReceiveCount json.Number `json:"maxReceiveCount"`
		}
		if json.Unmarshal([]byte(raw), &policy) == nil {
			if n, err := strconv.Atoi(policy.MaxReceiveCount.String()); err == nil {
				out.maxReceiveCount = &n
			}
		}
	}
	return out
}

// A read-only poll hides received messages for a user-set time. SQS allows 0 to 12 h, but the SDK
// omits a zero VisibilityTimeout from the request (indistinguishable from unset), so 0 would
// silently keep the queue's own timeout; 1 second is the shortest value that takes effect.
const (
	minBrowseVisibilityTimeout     = 1
	maxVisibilityTimeout           = 43200
	defaultBrowseVisibilityTimeout = 1
)

// sqsStreamFilter is the request filter's SQS shape; only a read-only poll applies it.
type sqsStreamFilter struct {
	VisibilityTimeoutSeconds *int `json:"visibilityTimeoutSeconds"`
}

// parseStreamFilter returns the hide seconds a read-only poll asks for. A nil filter or missing
// field gives the default; unknown fields, non-integers and out-of-range values are query errors.
func parseStreamFilter(raw *string) (int, error) {
	if raw == nil {
		return defaultBrowseVisibilityTimeout, nil
	}
	var f sqsStreamFilter
	dec := json.NewDecoder(strings.NewReader(*raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return 0, adapters.Contract("malformed stream filter", err)
	}
	if dec.More() {
		return 0, adapters.Contract("malformed stream filter", nil)
	}
	if f.VisibilityTimeoutSeconds == nil {
		return defaultBrowseVisibilityTimeout, nil
	}
	n := *f.VisibilityTimeoutSeconds
	if n < minBrowseVisibilityTimeout || n > maxVisibilityTimeout {
		return 0, adapters.T(adapters.CodeQuery, "backend.studio.adapter.sqsVisibilityTimeout.body", nil, nil)
	}
	return n, nil
}

// pollQueue is read.ts's pollQueue. Never called automatically — always an explicit user-
// initiated poll. Loops ReceiveMessage (hard-capped at 10 messages per call) up to
// ceil(pageSize/10) times, stopping early on any short/empty batch (the queue is plausibly
// drained for this poll). One GetQueueAttributes call per poll feeds both this page's
// visibilityTimeoutSeconds and countQueue's approximate count. Every SDK call takes ctx directly:
// SQS has no server-side kill mechanism, so the op's own context is the entire
// cancellation story.
func pollQueue(ctx context.Context, client *sqs.Client, queueURL string, req adapters.ReadRequest, op *adapters.OpCtx, handles *receiptHandles, readOnly bool, browseSeconds int) (page.StreamPage, error) {
	attrs := fetchQueueAttributes(ctx, client, queueURL)
	visibilityTimeoutSeconds := attrs.visibilityTimeoutSeconds
	builder := page.NewStreamPageBuilder(visibilityTimeoutSeconds)
	builder.SetMaxReceiveCount(attrs.maxReceiveCount)
	collected := 0
	// A read-only poll hides messages for only browseSeconds, so a later batch of the same
	// poll can receive them again; seen keeps each message once and ends the poll at the first batch holding a repeat.
	seen := map[string]struct{}{}

	// F10: the duration a cached receipt handle stays trustworthy for — the queue's own attribute
	// when it was readable, defaultVisibilityTimeout (AWS's own documented default) otherwise.
	visibilityTimeout := defaultVisibilityTimeout
	if visibilityTimeoutSeconds != nil {
		visibilityTimeout = time.Duration(*visibilityTimeoutSeconds) * time.Second
	}

	command := "ReceiveMessage " + queueURL
	if readOnly {
		command += " VisibilityTimeout=" + strconv.Itoa(browseSeconds)
	}
	op.SetCommand(command)
	for collected < req.PageSize {
		if err := adapters.CheckCancelled(ctx); err != nil {
			return page.StreamPage{}, err
		}
		batchLimit := receiveLimit
		if remaining := req.PageSize - collected; remaining < batchLimit {
			batchLimit = remaining
		}
		input := receiveInput(queueURL, batchLimit, readOnly, browseSeconds)
		result, err := client.ReceiveMessage(ctx, input)
		if err != nil {
			return page.StreamPage{}, mapError(err)
		}
		added, sawDup, err := pushBatch(builder, result.Messages, handles, visibilityTimeout, readOnly, seen)
		if err != nil {
			return page.StreamPage{}, err
		}
		collected += added
		if readOnly && (added == 0 || sawDup) {
			break // a repeat means the visibility window lapsed: stop before raising receive counts again
		}
		if len(result.Messages) < batchLimit {
			break // short of a full batch — queue is likely drained
		}
	}

	return builder.Finish(position(req.PageSize)), nil
}

// receiveInput builds one ReceiveMessage request; a read-only poll hides messages for browseSeconds.
func receiveInput(queueURL string, batchLimit int, readOnly bool, browseSeconds int) *sqs.ReceiveMessageInput {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:                    aws.String(queueURL),
		MaxNumberOfMessages:         int32(batchLimit),
		WaitTimeSeconds:             waitTimeSeconds,
		MessageAttributeNames:       []string{"All"},
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{types.MessageSystemAttributeNameAll},
	}
	if readOnly {
		input.VisibilityTimeout = int32(browseSeconds)
	}
	return input
}

// pushBatch adds one received batch to the page and returns how many messages it added and whether
// it held an already-seen one. A read-only poll skips messages already seen and keeps no receipt
// handles (it can never delete).
func pushBatch(builder *page.StreamPageBuilder, messages []types.Message, handles *receiptHandles, visibilityTimeout time.Duration, readOnly bool, seen map[string]struct{}) (added int, sawDup bool, err error) {
	for _, m := range messages {
		if readOnly && m.MessageId != nil {
			if _, dup := seen[*m.MessageId]; dup {
				sawDup = true
				continue
			}
			seen[*m.MessageId] = struct{}{}
		}
		var h *receiptHandles
		if !readOnly {
			h = handles
		}
		if err := pushMessage(builder, m, h, visibilityTimeout); err != nil {
			return added, sawDup, mapError(err)
		}
		added++
	}
	return added, sawDup, nil
}

// countQueue is read.ts's countQueue — approximate only; SQS has no exact-count operation.
func countQueue(ctx context.Context, client *sqs.Client, queueURL string) (adapters.CountResult, error) {
	result, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	})
	if err != nil {
		return adapters.CountResult{}, mapError(err)
	}
	raw, ok := result.Attributes["ApproximateNumberOfMessages"]
	if !ok {
		return adapters.CountResult{Value: 0, Exact: false}, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return adapters.CountResult{Value: 0, Exact: false}, nil
	}
	return adapters.CountResult{Value: n, Exact: false}, nil
}
