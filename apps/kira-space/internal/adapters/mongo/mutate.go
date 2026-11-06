package mongo

import (
	"context"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// documentSentinel is mutate.ts's DOCUMENT_SENTINEL: the reserved key for a whole-document
// replace, expressed through the existing relational-shaped MutationRowOp rather than widening the
// shared mutation schema — '$' can never start a real top-level Mongo field name, so it can never
// collide with genuine data.
const documentSentinel = "$document"

// parseIdKey ports mutate.ts's parseIdKey.
func parseIdKey(key model.RowValues) (any, error) {
	if len(key) != 1 || key[0].Name != "_id" {
		return nil, adapters.Contract("a document mutation key must be exactly { _id }", nil)
	}
	raw := key[0].Value
	if raw == nil {
		return nil, nil
	}
	// P27 D16: an _id is a value, not a document — this composes the same two primitives
	// ParseDocumentLiteral does (JSON5-lite parse + wrapper resolution) rather than routing
	// through ParseDocumentLiteral's own object-only shape, since _id need not be an object.
	value, err := ParseJSON5Literal(*raw)
	if err != nil {
		return nil, adapters.Contract("malformed _id in mutation key", nil)
	}
	return ResolveEJSONWrappers(value), nil
}

func renderOpText(op model.MutationRowOp, collectionName string) (string, error) {
	switch op.Kind {
	case "update":
		doc, ok := op.Changes.Get(documentSentinel)
		if !ok || doc == nil {
			return "", adapters.Contract("document mutation requires a $document replacement", nil)
		}
		return "db." + collectionName + ".replaceOne({_id: ...}, " + *doc + ")", nil
	case "delete":
		return "db." + collectionName + ".deleteOne({_id: ...})", nil
	default: // insert
		doc, ok := op.Values.Get(documentSentinel)
		if !ok || doc == nil {
			return "", adapters.Contract("document mutation requires a $document body", nil)
		}
		return "db." + collectionName + ".insertOne(" + *doc + ")", nil
	}
}

// preview is mutate.ts's preview — synchronous (Adapter rule 3's discipline): no network, no
// catalog lookup.
func preview(plan model.MutationPlan) ([]string, error) {
	_, objectSegment, err := requireTwoSegmentObjectPath(plan.Path, "mutate")
	if err != nil {
		return nil, err
	}
	collection := objectSegment.Name
	out := make([]string, len(plan.Ops))
	for i, op := range plan.Ops {
		text, err := renderOpText(op, collection)
		if err != nil {
			return nil, err
		}
		out[i] = text
	}
	return out, nil
}

// applyUpdate is mutateDB's "update" arm: a whole-document replaceOne keyed by _id, driven by the
// $document sentinel both it and applyInsert share.
func applyUpdate(ctx context.Context, collection *mongodriver.Collection, op *adapters.OpCtx, rowOp model.MutationRowOp, track TrackQuery) (int, error) {
	id, err := parseIdKey(rowOp.Key)
	if err != nil {
		return 0, err
	}
	bodyText, ok := rowOp.Changes.Get(documentSentinel)
	if !ok || bodyText == nil {
		return 0, adapters.Contract("document mutation requires a $document replacement", nil)
	}
	parsed, err := ParseDocumentLiteral(*bodyText)
	if err != nil {
		return 0, err
	}
	replacement := setField(parsed, "_id", id)
	matchedCount, err := runTracked(ctx, track, func(qctx context.Context) (int64, error) {
		result, err := collection.ReplaceOne(qctx, bson.D{{Key: "_id", Value: id}}, replacement, options.Replace().SetComment(op.OpID))
		if err != nil {
			return 0, mapError(err)
		}
		return result.MatchedCount, nil
	})
	if err != nil {
		return 0, err
	}
	if matchedCount != 1 {
		return 0, adapters.New(adapters.CodeQuery,
			"expected update to affect exactly one document, matched "+strconv.FormatInt(matchedCount, 10), nil)
	}
	return int(matchedCount), nil
}

// applyDelete is mutateDB's "delete" arm.
func applyDelete(ctx context.Context, collection *mongodriver.Collection, op *adapters.OpCtx, rowOp model.MutationRowOp, track TrackQuery) (int, error) {
	id, err := parseIdKey(rowOp.Key)
	if err != nil {
		return 0, err
	}
	deletedCount, err := runTracked(ctx, track, func(qctx context.Context) (int64, error) {
		result, err := collection.DeleteOne(qctx, bson.D{{Key: "_id", Value: id}}, options.DeleteOne().SetComment(op.OpID))
		if err != nil {
			return 0, mapError(err)
		}
		return result.DeletedCount, nil
	})
	if err != nil {
		return 0, err
	}
	if deletedCount != 1 {
		return 0, adapters.New(adapters.CodeQuery,
			"expected delete to affect exactly one document, deleted "+strconv.FormatInt(deletedCount, 10), nil)
	}
	return int(deletedCount), nil
}

// applyInsert is mutateDB's default (insert) arm: the same $document sentinel the update branch
// uses, holding the new document's full EJSON body rather than a replacement for an existing one —
// no key to parse, since InsertOne assigns a fresh ObjectID when the body omits _id.
func applyInsert(ctx context.Context, collection *mongodriver.Collection, op *adapters.OpCtx, rowOp model.MutationRowOp, track TrackQuery) (int, error) {
	bodyText, ok := rowOp.Values.Get(documentSentinel)
	if !ok || bodyText == nil {
		return 0, adapters.Contract("document mutation requires a $document body", nil)
	}
	parsed, err := ParseDocumentLiteral(*bodyText)
	if err != nil {
		return 0, err
	}
	acknowledged, err := runTracked(ctx, track, func(qctx context.Context) (bool, error) {
		result, err := collection.InsertOne(qctx, parsed, options.InsertOne().SetComment(op.OpID))
		if err != nil {
			return false, mapError(err)
		}
		return result.Acknowledged, nil
	})
	if err != nil {
		return 0, err
	}
	if !acknowledged {
		return 0, adapters.T(adapters.CodeQuery, "backend.studio.adapter.insertNotAcknowledged.body", nil, nil)
	}
	return 1, nil
}

// mutateDB ports mutate.ts's mutate.
func mutateDB(ctx context.Context, db *mongodriver.Database, op *adapters.OpCtx, readOnly bool, plan model.MutationPlan, track TrackQuery) (model.MutationResult, error) {
	// §8.12's standard: enforced here, not only greyed out in the UI.
	if err := adapters.AssertWritable(readOnly); err != nil {
		return model.MutationResult{}, err
	}

	_, objectSegment, err := requireTwoSegmentObjectPath(plan.Path, "mutate")
	if err != nil {
		return model.MutationResult{}, err
	}
	collectionName := objectSegment.Name
	collection := db.Collection(collectionName)

	statements, err := preview(plan)
	if err != nil {
		return model.MutationResult{}, err
	}

	return adapters.RunKindDispatched(ctx, op, plan, readOnly, statements, adapters.DispatchUpdateDeleteInsert(
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyUpdate(ctx, collection, op, rowOp, track)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyDelete(ctx, collection, op, rowOp, track)
		},
		func(ctx context.Context, rowOp model.MutationRowOp) (int, error) {
			return applyInsert(ctx, collection, op, rowOp, track)
		},
	))
}
