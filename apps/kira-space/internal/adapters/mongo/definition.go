package mongo

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

const noOptionsNote = "This collection has no creation options set (no validator, no capped-collection settings, etc)."

// stringifyRelaxed is definition.ts's stringifyRelaxed: relaxed mode is deliberate —
// strict EJSON would render a schema's `minimum: 5` as `{"$numberInt":"5"}` in a pane whose whole
// job is readability, and a $jsonSchema is plain JSON by construction, so nothing is lost by
// relaxing it. Go's json.Indent does the 2-space pretty-printing MarshalExtJSON's own indent
// parameter would otherwise duplicate effort with.
func stringifyRelaxed(value any) (string, error) {
	compact, err := bson.MarshalExtJSON(value, false, false)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// buildDocumentSchema ports definition.ts's buildDocumentSchema: $jsonSchema renders as the
// Validation section's field table; any other validator document is real and genuinely allowed by
// Mongo — it renders as read-only JSON instead, never as an empty table pretending the schema fit
// a shape it doesn't have.
func buildDocumentSchema(validator any, validationLevel, validationAction any) (model.DocumentSchemaMeta, error) {
	var level, action *string
	if s, ok := validationLevel.(string); ok {
		level = &s
	}
	if s, ok := validationAction.(string); ok {
		action = &s
	}

	validatorDoc, ok := validator.(bson.D)
	if !ok {
		return model.DocumentSchemaMeta{IsJSONSchema: false, ValidationLevel: level, ValidationAction: action}, nil
	}
	if jsonSchema, ok := lookupField(validatorDoc, "$jsonSchema"); ok {
		text, err := stringifyRelaxed(jsonSchema)
		if err != nil {
			return model.DocumentSchemaMeta{}, err
		}
		return model.DocumentSchemaMeta{Validator: &text, IsJSONSchema: true, ValidationLevel: level, ValidationAction: action}, nil
	}
	text, err := stringifyRelaxed(validatorDoc)
	if err != nil {
		return model.DocumentSchemaMeta{}, err
	}
	return model.DocumentSchemaMeta{Validator: &text, IsJSONSchema: false, ValidationLevel: level, ValidationAction: action}, nil
}

// buildDefinition ports definition.ts's buildDefinition.
func buildDefinition(ctx context.Context, db *mongodriver.Database, segments []model.PathSegment, databaseName, collection string) (model.ObjectDefinition, error) {
	options, err := collectionOptions(ctx, db, collection)
	if err != nil {
		return model.ObjectDefinition{}, err
	}
	hasOptions := len(options) > 0

	statement := "{}"
	notes := []string{noOptionsNote}
	if hasOptions {
		text, err := stringifyRelaxed(options)
		if err != nil {
			return model.ObjectDefinition{}, err
		}
		statement = text
		notes = []string{}
	}

	var validator, validationLevel, validationAction any
	if hasOptions {
		validator, _ = lookupField(options, "validator")
		validationLevel, _ = lookupField(options, "validationLevel")
		validationAction, _ = lookupField(options, "validationAction")
	}
	documentSchema, err := buildDocumentSchema(validator, validationLevel, validationAction)
	if err != nil {
		return model.ObjectDefinition{}, err
	}

	return model.ObjectDefinition{
		Path:           model.EncodePath(segments),
		Kind:           "collection",
		QualifiedName:  databaseName + "." + collection,
		Language:       "json",
		Statements:     []string{statement},
		Origin:         "server",
		Notes:          notes,
		Constraints:    []model.ConstraintMeta{},
		DocumentSchema: &documentSchema,
		Sections:       []model.DefinitionSection{},
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
