package sqs

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Sentinel keys (mirrors mongo's $document, kafka's $body): a new message is expressed through
// the existing relational-shaped RowValues rather than widening the shared mutation schema.
const (
	bodyField    = "$body"
	headersField = "$headers"
	idField      = "messageId" // the row's key column is already the MessageId (read.go's pushMessage)
	// F9: FIFO-only sentinels — AWS requires MessageGroupId on every SendMessage to a .fifo queue,
	// and MessageDeduplicationId unless the queue has content-based dedup enabled. Neither has any
	// non-FIFO meaning, so both stay optional/absent for a standard queue's own insert.
	messageGroupIDField         = "$messageGroupId"
	messageDeduplicationIDField = "$messageDeduplicationId"
)

// isFIFOQueueName reports whether name is a FIFO queue, per AWS's own naming convention: every
// FIFO queue's name ends in the literal ".fifo" suffix, case-sensitive.
func isFIFOQueueName(name string) bool {
	return strings.HasSuffix(name, ".fifo")
}

// queueContentBasedDedupEnabled reports whether queueURL has ContentBasedDeduplication turned on —
// the one condition under which a FIFO SendMessage can omit MessageDeduplicationId.
func queueContentBasedDedupEnabled(ctx context.Context, client *sqs.Client, queueURL string) (bool, error) {
	result, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameContentBasedDeduplication},
	})
	if err != nil {
		return false, mapError(err)
	}
	return result.Attributes["ContentBasedDeduplication"] == "true", nil
}

// resolveFIFOInsertFields is the fix: SendMessage never set MessageGroupId or
// MessageDeduplicationId, both required by AWS on a .fifo queue (the former unconditionally, the
// latter unless content-based dedup is on) — every FIFO insert failed with AWS's own opaque
// rejection despite caps.CanInsert reporting true for every queue, FIFO included. A non-FIFO queue
// name is untouched (nil, nil, no extra GetQueueAttributes round trip).
func resolveFIFOInsertFields(ctx context.Context, client *sqs.Client, queueURL, queueName string, values model.RowValues) (groupID, dedupID *string, err error) {
	if !isFIFOQueueName(queueName) {
		return nil, nil, nil
	}
	group, ok := values.Get(messageGroupIDField)
	if !ok || group == nil || strings.TrimSpace(*group) == "" {
		return nil, nil, adapters.New(adapters.CodeQuery,
			"a FIFO queue requires "+messageGroupIDField+" on every new message", nil)
	}
	dedup, hasDedup := values.Get(messageDeduplicationIDField)
	if hasDedup && dedup != nil && strings.TrimSpace(*dedup) != "" {
		return group, dedup, nil
	}
	contentBased, err := queueContentBasedDedupEnabled(ctx, client, queueURL)
	if err != nil {
		return nil, nil, err
	}
	if !contentBased {
		return nil, nil, adapters.New(adapters.CodeQuery,
			"a FIFO queue without content-based deduplication requires "+messageDeduplicationIDField+" on every new message", nil)
	}
	return group, nil, nil
}

func toMessageAttributes(headers map[string]string) map[string]types.MessageAttributeValue {
	if headers == nil {
		return nil
	}
	out := make(map[string]types.MessageAttributeValue, len(headers))
	for name, value := range headers {
		out[name] = types.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(value)}
	}
	return out
}

func renderOpText(op model.MutationRowOp, queueName string) (string, error) {
	switch op.Kind {
	case "insert":
		return "SendMessage(" + queueName + ")", nil
	case "delete":
		return "DeleteMessage(" + queueName + ")", nil
	default: // update — a delivered message can't be edited in place, only replaced by delete+resend.
		return "", adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.sqsNoUpdate.body", nil, nil)
	}
}

// preview is mutate.ts's preview — synchronous (Adapter rule 3): no network, no queue-URL
// resolution.
func preview(plan model.MutationPlan, queueName string) ([]string, error) {
	return adapters.PreviewProduce(plan, queueName, renderOpText)
}

// mutateQueue is mutate.ts's mutateQueue. handles is the adapter-local, mutex-guarded receipt-
// handle cache read.go populates — a receipt handle is only ever valid for the message that was
// received, not a stable identifier of the message itself; deleting a message the
// current session never polled is reported as E_QUERY rather than silently doing nothing.
func mutateQueue(ctx context.Context, client *sqs.Client, queueURL, queueName string, readOnly bool, plan model.MutationPlan, handles *receiptHandles, op *adapters.OpCtx) (model.MutationResult, error) {
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}

	statements, err := preview(plan, queueName)
	if err != nil {
		return model.MutationResult{}, err
	}

	return adapters.RunKindDispatched(ctx, op, plan, readOnly, statements, func(ctx context.Context, _ int, rowOp model.MutationRowOp) (int, error) {
		switch rowOp.Kind {
		case "insert":
			body, ok := rowOp.Values.Get(bodyField)
			if !ok || body == nil {
				return 0, adapters.New(adapters.CodeQuery, "a new message requires a "+bodyField, nil)
			}
			headersRaw, _ := rowOp.Values.Get(headersField)
			headers, err := adapters.ParseHeaderJSON(headersRaw)
			if err != nil {
				return 0, err
			}
			groupID, dedupID, err := resolveFIFOInsertFields(ctx, client, queueURL, queueName, rowOp.Values)
			if err != nil {
				return 0, err
			}
			_, err = client.SendMessage(ctx, &sqs.SendMessageInput{
				QueueUrl:               aws.String(queueURL),
				MessageBody:            body,
				MessageAttributes:      toMessageAttributes(headers),
				MessageGroupId:         groupID,
				MessageDeduplicationId: dedupID,
			})
			if err != nil {
				return 0, mapError(err)
			}
			return 1, nil

		case "delete":
			messageID, ok := rowOp.Key.Get(idField)
			if !ok || messageID == nil {
				return 0, adapters.New(adapters.CodeQuery, "a delete requires the message's "+idField, nil)
			}
			if handles == nil {
				return 0, adapters.T(adapters.CodeConnect, "backend.studio.adapter.notConnected.body", nil, nil)
			}
			// F10: forDelete refuses a stale receipt handle outright (past its own visibility
			// timeout) rather than letting AWS silently accept it and report a false success.
			handle, err := handles.forDelete(*messageID)
			if err != nil {
				return 0, err
			}
			_, err = client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(queueURL), ReceiptHandle: aws.String(handle)})
			if err != nil {
				return 0, mapError(err)
			}
			handles.delete(*messageID)
			return 1, nil

		default:
			return 0, adapters.T(adapters.CodeUnsupported, "backend.studio.adapter.sqsNoUpdate.body", nil, nil)
		}
	})
}
