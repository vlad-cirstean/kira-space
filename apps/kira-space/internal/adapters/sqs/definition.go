package sqs

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// jsonAttributes mirrors definition.ts's JSON_ATTRIBUTES — a few attribute values are themselves
// JSON policy documents; pretty-print those specifically rather than every attribute, so a plain
// string like an ARN or a timestamp stays a plain string.
var jsonAttributes = map[string]bool{"RedrivePolicy": true, "Policy": true, "RedriveAllowPolicy": true}

func formatAttributeValue(name, value string) string {
	if !jsonAttributes[name] {
		return value
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(value), "", "  "); err != nil {
		return value // swallow a parse failure back to the raw string, mirroring definition.ts
	}
	return buf.String()
}

// buildQueueDefinition is definition.ts's buildQueueDefinition — P23 D9: a queue genuinely *is*
// its attributes. One GetQueueAttributes(All) call, no automatic message read.
func buildQueueDefinition(ctx context.Context, client *sqs.Client, queueURL, name string) (model.ObjectDefinition, error) {
	result, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}
	attributes := result.Attributes

	names := make([]string, 0, len(attributes))
	for n := range attributes {
		names = append(names, n)
	}
	sort.Strings(names)

	rows := make([]model.DefinitionSectionRow, len(names))
	for i, n := range names {
		rows[i] = model.DefinitionSectionRow{Name: n, Value: formatAttributeValue(n, attributes[n]), Detail: nil}
	}

	// json.MarshalIndent of a map[string]string sorts keys — matching Object.keys().sort() above
	// for the rows, but not JS's JSON.stringify(attributes, null, 2) insertion order for this
	// statement text (the recorded divergence; tests/ipc/sqs/sqs.fixture.ts freezes this text
	// against the TypeScript producer, and the generator port will re-derive it).
	statementBytes, err := json.MarshalIndent(attributes, "", "  ")
	if err != nil {
		return model.ObjectDefinition{}, mapError(err)
	}

	return model.ObjectDefinition{
		Path:           model.EncodePath([]model.PathSegment{{Kind: "queue", Name: name}}),
		Kind:           "queue",
		QualifiedName:  name,
		Language:       "json",
		Statements:     []string{string(statementBytes)},
		Origin:         "server",
		Notes:          []string{},
		Constraints:    []model.ConstraintMeta{},
		DocumentSchema: nil,
		Sections:       []model.DefinitionSection{{Title: i18n.T("backend.definition.section.attributes.title", nil), Rows: rows}},
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
