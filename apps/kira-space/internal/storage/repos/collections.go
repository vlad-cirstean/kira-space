package repos

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/sqlb"
	"github.com/kirathecat/kira-studio/internal/sqlitex"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// CollectionsRepo reads api_collections and api_items. The rows are a projection of the api config
// files, written only by apifiles.Store; a read never touches request_json or origin_json on the
// list path.
type CollectionsRepo struct {
	DB      *sql.DB
	Dialect sqlb.Dialect
}

// The projections that never touch request_json/origin_json. SQLite stores a row's columns in
// declaration order and spills the tail into overflow pages, so stopping before them is the cheap
// path for the one query that runs on every panel mount.
var (
	collectionSelectColumns = []string{
		kiraschema.APICollections.ID, kiraschema.APICollections.Name, kiraschema.APICollections.SortOrder,
		kiraschema.APICollections.CreatedAt, kiraschema.APICollections.UpdatedAt,
		"COALESCE(" + kiraschema.APICollections.Stem + " || '.collection.yaml', '')",
	}
	itemSelectColumns = []string{
		kiraschema.APIItems.ID, kiraschema.APIItems.CollectionID, kiraschema.APIItems.ParentID, kiraschema.APIItems.Kind,
		kiraschema.APIItems.Name, kiraschema.APIItems.SortOrder, kiraschema.APIItems.Method, kiraschema.APIItems.URL,
		kiraschema.APIItems.Protocol, kiraschema.APIItems.CreatedAt, kiraschema.APIItems.UpdatedAt,
	}
)

// mirroring how TreeService.Children returns flat nodes. One call per panel mount; no N+1.
func (r *CollectionsRepo) List() ([]model.Collection, []model.CollectionItem, error) {
	collections, err := r.listCollections()
	if err != nil {
		return nil, nil, err
	}
	items, err := r.listItems()
	if err != nil {
		return nil, nil, err
	}
	return collections, items, nil
}

func (r *CollectionsRepo) listCollections() ([]model.Collection, error) {
	c := kiraschema.APICollections
	rows, err := sqlb.Query(r.DB, r.Dialect.Select(collectionSelectColumns...).From(c.Table).OrderBy(c.SortOrder, c.Name))
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.Collection, bool, error) {
		var c model.Collection
		err := rows.Scan(&c.ID, &c.Name, &c.SortOrder, msISO(&c.CreatedAt), msISO(&c.UpdatedAt), &c.File)
		return c, true, err
	})
}

func (r *CollectionsRepo) listItems() ([]model.CollectionItem, error) {
	c := kiraschema.APIItems
	rows, err := sqlb.Query(r.DB, r.Dialect.Select(itemSelectColumns...).From(c.Table).OrderBy(c.CollectionID, c.SortOrder))
	return sqlitex.QueryAll(rows, err, func(rows *sql.Rows) (model.CollectionItem, bool, error) {
		item, err := scanItem(rows)
		return item, true, err
	})
}

func scanItem(row rowScanner) (model.CollectionItem, error) {
	var (
		item   model.CollectionItem
		parent sql.NullString
	)
	if err := row.Scan(
		&item.ID, &item.CollectionID, &parent, &item.Kind, &item.Name,
		&item.SortOrder, &item.Method, &item.URL, &item.Protocol, msISO(&item.CreatedAt), msISO(&item.UpdatedAt),
	); err != nil {
		return model.CollectionItem{}, fmt.Errorf("repos/collections: scan item: %w", err)
	}
	if parent.Valid {
		item.ParentID = &parent.String
	}
	return item, nil
}

// ErrItemNotFound is a read of an item id that has no row; ErrNotARequest is a read that names a
// folder or a request of the other protocol. Bridges map them to E_NOT_FOUND and E_BAD_REQUEST.
var (
	ErrItemNotFound = errors.New("repos/collections: item not found")
	ErrNotARequest  = errors.New("repos/collections: item is not a request of this protocol")
)

// getRequestBody is GetRequest's and GetGrpcRequest's shared shape: read
// kind/protocol/request_json by item id, refuse a missing row, a non-request item or a protocol
// mismatch, then hand the raw body to decode. errLabel keeps the two callers' error text exactly
// as it was ("get request %s" for HTTP, "get grpc request %s" for gRPC) rather than merging them.
func getRequestBody[T any](db *sql.DB, d sqlb.Dialect, itemID, wantProtocol, errLabel string, decode func(itemID, body string) (T, error)) (T, error) {
	var (
		kind, protocol string
		body           string
		zero           T
	)
	c := kiraschema.APIItems
	sb := d.Select(c.Kind, c.Protocol, c.RequestJSON).From(c.Table)
	sb.Where(sb.Equal(c.ID, itemID))
	err := sqlb.QueryRow(db, sb).Scan(&kind, &protocol, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, fmt.Errorf("%w: %s", ErrItemNotFound, itemID)
	}
	if err != nil {
		return zero, fmt.Errorf("repos/collections: get %srequest %s: %w", errLabel, itemID, err)
	}
	if kind != model.CollectionItemRequest {
		return zero, fmt.Errorf("%w: item %s is a %s", ErrNotARequest, itemID, kind)
	}
	if protocol != wantProtocol {
		return zero, fmt.Errorf("%w: item %s is a %s request, not %s", ErrNotARequest, itemID, protocol, wantProtocol)
	}
	return decode(itemID, body)
}

// GetRequest reads one saved request on demand, so the list path never touches request_json. A row
// whose stored document is unparseable or fails model.SavedRequest.Validate is refused with a
// legible error rather than handed to the renderer half-formed — the write-side counterpart of
// repos/saved_queries.go's drop-and-log on read.
func (r *CollectionsRepo) GetRequest(itemID string) (model.SavedRequest, error) {
	return getRequestBody(r.DB, r.Dialect, itemID, model.ItemProtocolHTTP, "", decodeSavedRequest)
}

// GetGrpcRequest is GetRequest's own gRPC sibling (D12).
func (r *CollectionsRepo) GetGrpcRequest(itemID string) (model.SavedGrpcRequest, error) {
	return getRequestBody(r.DB, r.Dialect, itemID, model.ItemProtocolGrpc, "grpc ", decodeSavedGrpcRequest)
}

func decodeSavedGrpcRequest(itemID, body string) (model.SavedGrpcRequest, error) {
	var req model.SavedGrpcRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		slog.Warn("collection grpc request is not valid JSON", "scope", "storage/collections", "id", itemID)
		return model.SavedGrpcRequest{}, fmt.Errorf("repos/collections: item %s: request is not valid JSON", itemID)
	}
	if err := req.Validate(); err != nil {
		slog.Warn("collection grpc request failed validation", "scope", "storage/collections", "id", itemID, "err", err)
		return model.SavedGrpcRequest{}, fmt.Errorf("repos/collections: item %s: %w", itemID, err)
	}
	return req, nil
}

func decodeSavedRequest(itemID, body string) (model.SavedRequest, error) {
	var req model.SavedRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		slog.Warn("collection request is not valid JSON", "scope", "storage/collections", "id", itemID)
		return model.SavedRequest{}, fmt.Errorf("repos/collections: item %s: request is not valid JSON", itemID)
	}
	if err := req.Validate(); err != nil {
		slog.Warn("collection request failed validation", "scope", "storage/collections", "id", itemID, "err", err)
		return model.SavedRequest{}, fmt.Errorf("repos/collections: item %s: %w", itemID, err)
	}
	// ParamDescriptions is brand new — every row saved earlier has no such key
	// at all, so json.Unmarshal leaves the field nil, which re-marshals as JSON `null` rather than
	// an absent key (no `omitempty` tag, matching this struct's other map/slice fields). Normalized
	// here rather than relying on the frontend's own Zod default, which only ever fires for
	// `undefined`, never `null`.
	if req.ParamDescriptions == nil {
		req.ParamDescriptions = map[string]string{}
	}
	return req, nil
}

// CreateCollection appends an empty collection created in this app — origin_json '{}', so export

// GetItem reads one item row.
func (r *CollectionsRepo) GetItem(itemID string) (model.CollectionItem, error) {
	sb := r.Dialect.Select(itemSelectColumns...).From(kiraschema.APIItems.Table)
	sb.Where(sb.Equal(kiraschema.APIItems.ID, itemID))
	item, err := scanItem(sqlb.QueryRow(r.DB, sb))
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return model.CollectionItem{}, fmt.Errorf("%w: %s", ErrItemNotFound, itemID)
	}
	return item, err
}

// GetCollection reads one collection row.
func (r *CollectionsRepo) GetCollection(id string) (model.Collection, error) {
	sb := r.Dialect.Select(collectionSelectColumns...).From(kiraschema.APICollections.Table)
	sb.Where(sb.Equal(kiraschema.APICollections.ID, id))
	var c model.Collection
	if err := sqlb.QueryRow(r.DB, sb).Scan(&c.ID, &c.Name, &c.SortOrder, msISO(&c.CreatedAt), msISO(&c.UpdatedAt), &c.File); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Collection{}, fmt.Errorf("repos/collections: no collection %s", id)
		}
		return model.Collection{}, fmt.Errorf("repos/collections: read collection %s: %w", id, err)
	}
	return c, nil
}

// LoadTree rebuilds a postman.Tree from rows, with the id→index mapping restored so Write can
// rebuild the nested arrays.
func (r *CollectionsRepo) LoadTree(collectionID string) (*postman.Tree, error) {
	var (
		name       string
		originJSON string
	)
	cc := kiraschema.APICollections
	csb := r.Dialect.Select(cc.Name, cc.OriginJSON).From(cc.Table)
	csb.Where(csb.Equal(cc.ID, collectionID))
	err := sqlb.QueryRow(r.DB, csb).Scan(&name, &originJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("repos/collections: no collection %s", collectionID)
	}
	if err != nil {
		return nil, fmt.Errorf("repos/collections: read collection %s: %w", collectionID, err)
	}

	tree := &postman.Tree{Name: name, Origin: decodeOrigin(originJSON, collectionID)}

	// D15/D16: the collection's own promoted variables, re-emitted from these rows — a secret
	// row's `value` is already '' by construction (D4's own CHECK constraint), which is what
	// makes D16's "a secret exports valueless" true here with no decrypt and no extra branch.
	vc := kiraschema.APIVariables
	vsb := r.Dialect.Select(vc.Name, vc.Value, vc.IsSecret).From(vc.Table)
	vsb.Where(vsb.Equal(vc.CollectionID, collectionID)).OrderBy(vc.SortOrder, vc.Name)
	varRows, err := sqlb.Query(r.DB, vsb)
	if err != nil {
		return nil, fmt.Errorf("repos/collections: query variables: %w", err)
	}
	for varRows.Next() {
		var (
			varName, varValue string
			isSecretInt       int
		)
		if err := varRows.Scan(&varName, &varValue, &isSecretInt); err != nil {
			varRows.Close()
			return nil, fmt.Errorf("repos/collections: scan variable: %w", err)
		}
		secret := isSecretInt != 0
		typ := ""
		if secret {
			typ = "secret"
		}
		tree.Variables = append(tree.Variables, postman.Variable{Name: varName, Value: varValue, Secret: secret, Type: typ})
	}
	if err := varRows.Err(); err != nil {
		varRows.Close()
		return nil, fmt.Errorf("repos/collections: variable rows: %w", err)
	}
	varRows.Close()

	ic := kiraschema.APIItems
	isb := r.Dialect.Select(ic.ID, ic.ParentID, ic.Kind, ic.Name, ic.SortOrder, ic.Protocol, ic.RequestJSON, ic.OriginJSON).From(ic.Table)
	isb.Where(isb.Equal(ic.CollectionID, collectionID)).OrderBy(ic.SortOrder, ic.CreatedAt, ic.ID)
	rows, err := sqlb.Query(r.DB, isb)
	if err != nil {
		return nil, fmt.Errorf("repos/collections: query items: %w", err)
	}
	defer rows.Close()

	type row struct {
		id, kind, name, protocol, requestJSON, originJSON string
		parent                                            *string
		order                                             int
	}
	all := []row{}
	for rows.Next() {
		var (
			rec    row
			parent sql.NullString
		)
		if err := rows.Scan(&rec.id, &parent, &rec.kind, &rec.name, &rec.order, &rec.protocol, &rec.requestJSON, &rec.originJSON); err != nil {
			return nil, fmt.Errorf("repos/collections: scan item: %w", err)
		}
		if parent.Valid {
			rec.parent = &parent.String
		}
		all = append(all, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repos/collections: item rows: %w", err)
	}

	// One pass to claim an index per id, a second to resolve parents — a row's parent can sort
	// after it (sort_order is dense per parent, not global), so the two cannot be merged.
	indexByID := make(map[string]int, len(all))
	for i, rec := range all {
		indexByID[rec.id] = i
	}
	tree.Items = make([]postman.Item, 0, len(all))
	for _, rec := range all {
		item := postman.Item{
			Parent: postman.RootParent, Kind: postman.ItemKind(rec.kind), Name: rec.name,
			Order: rec.order, Protocol: rec.protocol, Origin: decodeOrigin(rec.originJSON, rec.id),
		}
		if rec.parent != nil {
			if idx, ok := indexByID[*rec.parent]; ok {
				item.Parent = idx
			}
		}
		// F22/D12: a gRPC item's request_json is a model.SavedGrpcRequest, not a SavedRequest —
		// write.go skips every protocol='grpc' item before it ever reads .Request (postman has no
		// representation for one), so it is left at its zero value here rather than decoded wrong.
		if item.Kind == postman.KindRequest && rec.protocol == model.ItemProtocolHTTP {
			req, err := decodeSavedRequest(rec.id, rec.requestJSON)
			if err != nil {
				return nil, err
			}
			item.Request = req
		}
		tree.Items = append(tree.Items, item)
	}
	return tree, nil
}

// decodeOrigin never fails the load: an origin that no longer parses is treated as absent, so the
// export builds every member canonically rather than refusing to write the file at all.
func decodeOrigin(originJSON, id string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	if originJSON == "" {
		return out
	}
	if err := json.Unmarshal([]byte(originJSON), &out); err != nil {
		slog.Warn("collection origin is not a JSON object; exporting canonically instead",
			"scope", "storage/collections", "id", id)
		return map[string]json.RawMessage{}
	}
	return out
}
