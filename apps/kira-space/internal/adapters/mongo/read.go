package mongo

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/page"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// idText is read.ts's idText: EJSON.stringify(doc._id, {relaxed:false}). bson.MarshalExtJSON
// cannot encode a bare scalar at the top level (only a document, the IDText probe finding),
// so id is wrapped in a one-field document, marshalled, and the wrapper text stripped back off.
// literal.go's ParseFilterObject/ParseJSON5Literal must accept whatever this emits.
func IDText(id any) (string, error) {
	data, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: id}}, true, false)
	if err != nil {
		return "", err
	}
	s := strings.TrimPrefix(string(data), `{"v":`)
	return strings.TrimSuffix(s, "}"), nil
}

// ejsonStringify is EJSON.stringify(v, opts). canonical mirrors the TS call sites exactly: every
// id/body text passes {relaxed: false} (canonical=true here) for byte-exact round-tripping through
// literal.go's parser, while op.SetCommand's own rendered filter text is bson.js's default
// EJSON.stringify(filter) with no options — relaxed (canonical=false here), the human-readable
// form (plain numbers instead of {"$numberInt":...} wrappers).
func ejsonStringify(v any, canonical bool) (string, error) {
	data, err := bson.MarshalExtJSON(v, canonical, false)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// readReq is the field subset of adapters.ReadRequest readPage needs, minus Path (already
// resolved by the caller into db/collection).
type readReq = adapters.ReadReq

// readSortPlan is readPage's sort/keyset eligibility resolution — the two directions
// (mongoDirection, the sort's own; scanDirection, what the find() itself issues) diverge only for a
// "before" keyset scan, which walks toward the boundary from the near side and is reversed back to
// display order after fetching.
type readSortPlan struct {
	sortTerms     []model.SortTerm
	idOnlySort    bool
	wantsKeyset   bool
	reverseRows   bool
	scanDirection int
}

func resolveReadSortPlan(req readReq) (readSortPlan, error) {
	if req.Sort != nil && req.Sort.Kind == "text" {
		return readSortPlan{}, adapters.Unsupported("mongodb", "a free-text sort expression")
	}
	var sortTerms []model.SortTerm
	if req.Sort != nil && req.Sort.Kind == "structured" {
		sortTerms = req.Sort.Terms
	}
	idOnlySort := len(sortTerms) == 0 || (len(sortTerms) == 1 && sortTerms[0].Column == "_id")
	direction := "asc"
	if len(sortTerms) > 0 {
		direction = sortTerms[0].Direction
	}
	wantsKeyset := req.Cursor.Mode == "after" || req.Cursor.Mode == "before"
	if wantsKeyset && !idOnlySort {
		return readSortPlan{}, adapters.Contract("keyset pagination is unavailable for this sort; the client must use an offset cursor", nil)
	}
	reverseRows := req.Cursor.Mode == "before" && idOnlySort
	mongoDirection := 1
	if direction == "desc" {
		mongoDirection = -1
	}
	scanDirection := mongoDirection
	if reverseRows {
		scanDirection = -mongoDirection
	}
	return readSortPlan{sortTerms, idOnlySort, wantsKeyset, reverseRows, scanDirection}, nil
}

// buildKeysetFilter extends baseFilter with the _id keyset boundary decoded from the cursor token,
// when the request actually wants one — the comparison operator tracks the scan's own direction
// (plan.scanDirection), not which user-facing request ('after'/'before') caused it.
func buildKeysetFilter(req readReq, plan readSortPlan, baseFilter bson.D, fingerprint string) (bson.D, error) {
	if !(plan.idOnlySort && plan.wantsKeyset && req.Cursor.Mode != "offset") {
		return baseFilter, nil
	}
	keyValues, err := adapters.DecodePageToken(req.Cursor.Token, fingerprint)
	if err != nil {
		return nil, err
	}
	if len(keyValues) != 1 {
		return nil, adapters.Contract("malformed page token", nil)
	}
	boundaryID, err := ParseJSON5Literal(keyValues[0])
	if err != nil {
		return nil, adapters.Contract("malformed page token", nil)
	}
	boundaryID = ResolveEJSONWrappers(boundaryID)
	cmpOp := "$gt"
	if plan.scanDirection == -1 {
		cmpOp = "$lt"
	}
	return mergeKeysetIDCondition(baseFilter, cmpOp, boundaryID), nil
}

// buildFindOptions assembles find()'s own options — limit, projection, sort, skip — from req and
// the already-resolved sort plan.
func buildFindOptions(req readReq, plan readSortPlan, op *adapters.OpCtx) (*options.FindOptionsBuilder, error) {
	limit, err := adapters.SafeInt(req.PageSize+1, "page size") // D24's +1 probe, mirroring the SQL adapters
	if err != nil {
		return nil, err
	}
	findOpts := options.Find().SetLimit(int64(limit)).SetComment(op.OpID)
	// req.Projection is the generic ReadRequest field every adapter shares (Adapter rule 7's
	// relational precedent) — Mongo's own shape for "return a field subset" is a find() options
	// projection document, {field: 1, ...}. _id is never listed here even when the caller omitted
	// it from the picker: an inclusion projection returns _id by default unless it is explicitly
	// excluded (_id: 0), and this adapter never sends that exclusion, so the document's identity
	// always survives regardless of which fields the UI's picker has checked.
	if len(req.Projection) > 0 {
		proj := make(bson.D, len(req.Projection))
		for i, field := range req.Projection {
			proj[i] = bson.E{Key: field, Value: 1}
		}
		findOpts.SetProjection(proj)
	}
	if plan.idOnlySort {
		findOpts.SetSort(bson.D{{Key: "_id", Value: plan.scanDirection}})
	} else if len(plan.sortTerms) > 0 {
		sortDoc := make(bson.D, len(plan.sortTerms))
		for i, t := range plan.sortTerms {
			d := 1
			if t.Direction == "desc" {
				d = -1
			}
			sortDoc[i] = bson.E{Key: t.Column, Value: d}
		}
		findOpts.SetSort(sortDoc)
	}
	// P43 iter2 D24: any offset cursor with a non-zero offset applies skip, not just a non-_id
	// sort — skip() has no relationship to which sort is in force. The > 0 test keeps the
	// ordinary first page issuing no skip at all.
	if req.Cursor.Mode == "offset" && req.Cursor.Offset > 0 {
		offset, err := adapters.SafeInt(req.Cursor.Offset, "offset")
		if err != nil {
			return nil, err
		}
		findOpts.SetSkip(int64(offset))
	}
	return findOpts, nil
}

// buildKeysetTokens computes readPage's next/prev page tokens from the already-reversed
// displayDocs — only meaningful under the _id-keyset strategy (plan.idOnlySort), nil/nil otherwise.
func buildKeysetTokens(req readReq, plan readSortPlan, displayDocs []bson.Raw, probedExtra bool, fingerprint string) (nextToken, prevToken *string, err error) {
	if !plan.idOnlySort || len(displayDocs) == 0 {
		return nil, nil, nil
	}
	rowCount := len(displayDocs)
	hasForward := probedExtra
	if req.Cursor.Mode == "before" {
		hasForward = true
	}
	var hasBackward bool
	switch req.Cursor.Mode {
	case "before":
		hasBackward = probedExtra
	case "after":
		hasBackward = true
	default:
		hasBackward = req.Cursor.Offset > 0
	}
	if hasForward {
		text, err := IDText(displayDocs[rowCount-1].Lookup("_id"))
		if err != nil {
			return nil, nil, mapError(err)
		}
		token := adapters.EncodePageToken([]string{text}, fingerprint)
		nextToken = &token
	}
	if hasBackward {
		text, err := IDText(displayDocs[0].Lookup("_id"))
		if err != nil {
			return nil, nil, mapError(err)
		}
		token := adapters.EncodePageToken([]string{text}, fingerprint)
		prevToken = &token
	}
	return nextToken, prevToken, nil
}

// buildReadPage is readPage's reverse/probe/token tail: turns the fetched (pageSize+1-probed) docs
// into a page.DocumentPage, including the _id-keyset next/prev tokens when idOnlySort applies.
func buildReadPage(req readReq, plan readSortPlan, docs []bson.Raw, fingerprint string) (page.DocumentPage, error) {
	probedExtra := len(docs) > req.PageSize
	keptDocs := docs
	if probedExtra {
		keptDocs = docs[:req.PageSize]
	}
	displayDocs := keptDocs
	if plan.reverseRows {
		displayDocs = make([]bson.Raw, len(keptDocs))
		for i, d := range keptDocs {
			displayDocs[len(keptDocs)-1-i] = d
		}
	}
	rowCount := len(displayDocs)

	builder := page.NewDocumentPageBuilder(false)
	// anyMissingID is a document with no _id at all — reachable via a $project: {_id: 0} view,
	// which this app lists and reads elsewhere — makes doc.Lookup("_id") return an invalid BSON
	// type IDText can't stringify, which used to fail this whole page outright. The console's own
	// docsToPage already treats a missing _id as an empty id string; this matches that, and also
	// forces the page below into offset pagination, since a keyset token needs a real _id boundary
	// to resume from and this result set doesn't reliably have one.
	anyMissingID := false
	for _, doc := range displayDocs {
		idStr := ""
		if idVal, err := doc.LookupErr("_id"); err == nil {
			idStr, err = IDText(idVal)
			if err != nil {
				return page.DocumentPage{}, mapError(err)
			}
		} else {
			anyMissingID = true
		}
		bodyStr, err := ejsonStringify(doc, true)
		if err != nil {
			return page.DocumentPage{}, mapError(err)
		}
		builder.Push(idStr, bodyStr)
	}

	strategy := "offset"
	if plan.idOnlySort && !anyMissingID {
		strategy = "keyset"
	}
	hasMore := false
	if rowCount > 0 {
		if req.Cursor.Mode == "before" {
			hasMore = true
		} else {
			hasMore = probedExtra
		}
	}

	var nextToken, prevToken *string
	var err error
	if !anyMissingID {
		nextToken, prevToken, err = buildKeysetTokens(req, plan, displayDocs, probedExtra, fingerprint)
		if err != nil {
			return page.DocumentPage{}, err
		}
	}

	var offsetPtr *int
	if req.Cursor.Mode == "offset" || anyMissingID {
		// A token-based (keyset) cursor that lands here with no known _id has no true offset to
		// report either — 0 is what a fresh offset-paginated browse would start from, the same
		// degraded-but-safe restart this adapter already gives up front for any sort this view
		// can't keyset at all.
		o := req.Cursor.Offset
		offsetPtr = &o
	}

	position := page.PagePosition{
		Offset: offsetPtr, PageSize: req.PageSize, HasMore: hasMore,
		NextToken: nextToken, PrevToken: prevToken, Strategy: strategy,
	}
	return builder.Finish(position), nil
}

// readPage ports read.ts's readPage. D6: _id-keyset when the request is unsorted or sorted purely
// by _id; skip/limit fallback for any other sort.
func readPage(ctx context.Context, db *mongodriver.Database, collectionName string, req readReq, op *adapters.OpCtx, track TrackQuery) (page.DocumentPage, error) {
	plan, err := resolveReadSortPlan(req)
	if err != nil {
		return page.DocumentPage{}, err
	}
	collection := db.Collection(collectionName)
	baseFilter, err := ParseFilterObject(req.Filter)
	if err != nil {
		return page.DocumentPage{}, err
	}

	fingerprint := adapters.RequestFingerprint(struct {
		Path     string          `json:"path"`
		Filter   *string         `json:"filter"`
		Sort     *model.SortSpec `json:"sort"`
		PageSize int             `json:"pageSize"`
	}{collectionName, req.Filter, req.Sort, req.PageSize})

	filter, err := buildKeysetFilter(req, plan, baseFilter, fingerprint)
	if err != nil {
		return page.DocumentPage{}, err
	}

	findOpts, err := buildFindOptions(req, plan, op)
	if err != nil {
		return page.DocumentPage{}, err
	}

	filterText, err := ejsonStringify(filter, false)
	if err != nil {
		return page.DocumentPage{}, mapError(err)
	}
	op.SetCommand("db." + collectionName + ".find(" + filterText + ")")

	// docs is left as bson.Raw — the driver's own wire bytes — rather than decoded into bson.D.
	// The id/body text below only ever re-marshals each document straight back
	// to EJSON (IDText/ejsonStringify), never inspects or mutates a decoded field, so decoding into
	// bson.D first paid a full reflection-driven tree build only to walk it again on the way back
	// out. bson.Raw/bson.RawValue have their own codecs (rawEncodeValue/rawValueEncodeValue) that
	// bson.MarshalExtJSON dispatches to automatically, streaming the wire bytes straight into the
	// EJSON writer with no intermediate tree at all.
	docs, err := runTracked(ctx, track, func(queryCtx context.Context) ([]bson.Raw, error) {
		cursor, err := collection.Find(queryCtx, filter, findOpts)
		if err != nil {
			return nil, mapError(err)
		}
		defer cursor.Close(queryCtx)
		var out []bson.Raw
		if err := cursor.All(queryCtx, &out); err != nil {
			return nil, mapError(err)
		}
		return out, nil
	})
	if err != nil {
		return page.DocumentPage{}, err
	}

	return buildReadPage(req, plan, docs, fingerprint)
}

// lookupField returns d's value for key, and whether the key was present.
func lookupField(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// bsonSortTiers is MongoDB's own documented cross-BSON-type comparison order ("Comparison/Sort
// Order"), collapsed into the tiers keysetIDCondition actually needs to reason about (F6): the four
// numeric aliases share one tier since Mongo already compares them by value regardless of subtype
// (a bare {_id:{$gt: NumberLong(5)}} already matches a double 6.0 with no help needed here) — the
// type-bracketing bug this fixes is real only *across* tiers, e.g. a string boundary's own $gt never
// matching an ObjectId-keyed document that sorts after every string. minKey/maxKey are structural
// sentinels no real _id ever holds, kept only so the ordering is total and callers need no special
// case at either end.
var bsonSortTiers = [][]string{
	{"minKey"},
	{"null"},
	{"double", "int", "long", "decimal"},
	{"string"},
	{"object"},
	{"array"},
	{"binData"},
	{"objectId"},
	{"bool"},
	{"date"},
	{"timestamp"},
	{"regex"},
	{"maxKey"},
}

func bsonTierIndex(alias string) int {
	for i, tier := range bsonSortTiers {
		for _, a := range tier {
			if a == alias {
				return i
			}
		}
	}
	return -1
}

// typeAliasesAfter/typeAliasesBefore return every $type alias MongoDB's query language accepts that
// sorts strictly after/before tierAlias's own tier — the widening keysetIDCondition's own $or arm
// needs, empty when tierAlias is already the outermost tier (never true for a real _id in
// practice).
func typeAliasesAfter(tierAlias string) []string {
	idx := bsonTierIndex(tierAlias)
	if idx < 0 {
		return nil
	}
	var out []string
	for _, tier := range bsonSortTiers[idx+1:] {
		out = append(out, tier...)
	}
	return out
}

func typeAliasesBefore(tierAlias string) []string {
	idx := bsonTierIndex(tierAlias)
	if idx <= 0 {
		return nil
	}
	var out []string
	for _, tier := range bsonSortTiers[:idx] {
		out = append(out, tier...)
	}
	return out
}

// bsonTypeAliasFor names the $type alias for boundaryID's own concrete Go type — exactly the
// concrete types ParseJSON5Literal/ResolveEJSONWrappers (literal.go) can hand back as a decoded
// page-token boundary value. false means the type genuinely isn't one this function recognizes
// (nothing in this package's own value set), not that the type is somehow invalid.
func bsonTypeAliasFor(v any) (string, bool) {
	switch v.(type) {
	case nil:
		return "null", true
	case string:
		return "string", true
	case bson.ObjectID:
		return "objectId", true
	case bool:
		return "bool", true
	case time.Time, bson.DateTime:
		return "date", true
	case bson.Timestamp:
		return "timestamp", true
	case bson.Binary:
		return "binData", true
	case bson.Regex:
		return "regex", true
	case bson.D:
		return "object", true
	case bson.A:
		return "array", true
	case int32, int64, float64, bson.Decimal128:
		// Any one numeric alias works: bsonSortTiers keeps all four in the same tier, and Mongo's
		// own $gt/$lt already compares numeric BSON subtypes by value with no widening needed.
		return "double", true
	default:
		return "", false
	}
}

// keysetIDCondition builds one page's own _id boundary condition: a bare {_id:{$gt/$lt:
// boundary}} only ever matches _id values of boundary's own BSON type — MongoDB's query-level
// comparison type-brackets a non-numeric type this way even though the underlying *sort* itself
// runs in one global cross-type order (bsonSortTiers above) — so once a page's boundary lands on
// the last _id of one type (e.g. the last string _id in a collection this app itself can leave with
// mixed _id types: applyInsert mints a fresh ObjectId whenever the inserted body omits _id), the
// next page's own boundary condition matches nothing from any other type, silently stranding every
// document past it. Widening with an $or over every type that sorts on the correct side of
// boundary's own tier closes that gap while staying indexable (an index range scan/OR-plan over the
// same field), unlike a filter with no type constraint at all, which would give up the _id index
// entirely.
func keysetIDCondition(cmpOp string, boundaryID any) bson.D {
	direct := bson.D{{Key: "_id", Value: bson.D{{Key: cmpOp, Value: boundaryID}}}}
	alias, ok := bsonTypeAliasFor(boundaryID)
	if !ok {
		return direct
	}
	var widerAliases []string
	if cmpOp == "$gt" {
		widerAliases = typeAliasesAfter(alias)
	} else {
		widerAliases = typeAliasesBefore(alias)
	}
	if len(widerAliases) == 0 {
		return direct
	}
	return bson.D{{Key: "$or", Value: bson.A{
		direct,
		bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: widerAliases}}}},
	}}}
}

// mergeKeysetIDCondition combines the keyset boundary condition ($gt/$lt boundaryID, the
// type-widened form above) for _id with whatever _id constraint the caller's own filter already
// carries. The previous version only handled an existing operator document (e.g.
// {"_id": {"$in": [...]}}) — it type-asserted the existing value as bson.D and merged its operators
// in, but a scalar _id (a single-document lookup, e.g. {"_id": ObjectId("...")}) failed that
// assertion and was silently discarded when setField then overwrote the _id key outright. Wrapping
// in $and instead of trying to fold both into one _id document handles either shape without
// dropping anything, and also avoids two conflicting keys under the same _id document if the
// filter's own operator document already used the same comparison operator the keyset boundary
// needs. The flatten-into-one-document fast path below only ever fires for a bare (non-widened)
// idCond — realistically just boundary types with nothing on the widened side (never a real _id).
func mergeKeysetIDCondition(baseFilter bson.D, cmpOp string, boundaryID any) bson.D {
	idCond := keysetIDCondition(cmpOp, boundaryID)
	if len(idCond) == 1 && idCond[0].Key == "_id" {
		if _, hasExistingID := lookupField(baseFilter, "_id"); !hasExistingID {
			return setField(baseFilter, "_id", idCond[0].Value)
		}
	}
	return bson.D{{Key: "$and", Value: bson.A{baseFilter, idCond}}}
}

// setField returns a copy of d with key set to value, appended if absent.
func setField(d bson.D, key string, value any) bson.D {
	out := make(bson.D, 0, len(d)+1)
	replaced := false
	for _, e := range d {
		if e.Key == key {
			out = append(out, bson.E{Key: key, Value: value})
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, bson.E{Key: key, Value: value})
	}
	return out
}

// countRows ports read.ts's countRows: estimatedDocumentCount() by default (Caps.ExactCount
// == false), countDocuments() (exact) only when the caller passes a non-empty filter — an
// unfiltered estimate is what the pager wants by default, but an estimate ignores any filter
// entirely, so a filtered count must always go through the exact (slow) path.
func countRows(ctx context.Context, db *mongodriver.Database, collectionName string, filter *string, op *adapters.OpCtx, track TrackQuery) (adapters.CountResult, error) {
	collection := db.Collection(collectionName)
	parsedFilter, err := ParseFilterObject(filter)
	if err != nil {
		return adapters.CountResult{}, err
	}
	wantsExact := filter != nil && strings.TrimSpace(*filter) != ""

	if wantsExact {
		value, err := runTracked(ctx, track, func(queryCtx context.Context) (int64, error) {
			n, err := collection.CountDocuments(queryCtx, parsedFilter, options.Count().SetComment(op.OpID))
			if err != nil {
				return 0, mapError(err)
			}
			return n, nil
		})
		if err != nil {
			return adapters.CountResult{}, err
		}
		return adapters.CountResult{Value: value, Exact: true}, nil
	}

	// EstimatedDocumentCount has no per-call comment/killOp-fallback tag in the driver — a single
	// fast metadata command, so this is not a gap in the cancel coverage in practice (mirrors
	// read.ts's own note that estimatedDocumentCount() has no AbortSignal support either).
	value, err := runTracked(ctx, track, func(queryCtx context.Context) (int64, error) {
		n, err := collection.EstimatedDocumentCount(queryCtx)
		if err != nil {
			return 0, mapError(err)
		}
		return n, nil
	})
	if err != nil {
		return adapters.CountResult{}, err
	}
	return adapters.CountResult{Value: value, Exact: false}, nil
}
