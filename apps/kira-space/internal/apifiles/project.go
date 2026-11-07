package apifiles

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ids"
	"github.com/kirathecat/kira-studio/internal/kiraschema"
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlb"
)

const (
	folderCollections  = "api-collections"
	folderEnvironments = "api-environments"
	// variableHistoryLimit bounds the prior values kept per variable.
	variableHistoryLimit = 20
)

// projection is a configfs.Projection for one file kind: it keeps the last good JSON and writes the
// domain rows of each recorded file in the same transaction.
type projection struct {
	s      *Store
	folder string
	ext    string
	record func(tx *sql.Tx, stem, valueJSON string, ch *Changes) error
	forget func(tx *sql.Tx, stem string, ch *Changes) error
}

func (p *projection) Snapshot() (map[string]string, error) {
	return configfs.TableProjection{DB: p.s.o.DB, Folder: p.folder}.Snapshot()
}

func (p *projection) Record(file, valueJSON string, now int64) error {
	stem, ok := StemOf(file, p.ext)
	if !ok {
		return nil
	}
	return p.s.tx(func(tx *sql.Tx, ch *Changes) error {
		c := kiraschema.ConfigLastValid
		ib := p.s.o.Dialect.InsertInto(c.Table).Cols(c.Folder, c.File, c.ValueJSON, c.RecordedAt).Values(p.folder, file, valueJSON, now)
		sqlb.Upsert(ib, []string{c.Folder, c.File}, []string{c.ValueJSON, c.RecordedAt}, "")
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("apifiles: record last valid %s: %w", file, err)
		}
		return p.record(tx, stem, valueJSON, ch)
	})
}

func (p *projection) Forget(file string) error {
	return p.s.tx(func(tx *sql.Tx, ch *Changes) error {
		c := kiraschema.ConfigLastValid
		del := p.s.o.Dialect.DeleteFrom(c.Table)
		del.Where(del.Equal(c.Folder, p.folder), del.Equal(c.File, file))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return fmt.Errorf("apifiles: forget last valid %s: %w", file, err)
		}
		if stem, ok := StemOf(file, p.ext); ok {
			return p.forget(tx, stem, ch)
		}
		return nil
	})
}

// tx runs fn in a transaction and, once it commits, adds the changes it reported to the store's pending set.
func (s *Store) tx(fn func(tx *sql.Tx, ch *Changes) error) error {
	tx, err := s.o.DB.Begin()
	if err != nil {
		return fmt.Errorf("apifiles: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var ch Changes
	if err := fn(tx, &ch); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("apifiles: commit: %w", err)
	}
	s.pend.merge(ch)
	return nil
}

func encodeJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// ---- collections ----

func (s *Store) recordCollection(tx *sql.Tx, stem, valueJSON string, ch *Changes) error {
	var f CollectionFile
	if err := json.Unmarshal([]byte(valueJSON), &f); err != nil {
		return fmt.Errorf("apifiles: decode %s: %w", stem, err)
	}
	now := kiratime.NowMs()
	d := s.o.Dialect
	c := kiraschema.APICollections
	var id, name string
	sel := d.Select(c.ID, c.Name).From(c.Table)
	sel.Where(sel.Equal(c.Stem, stem))
	err := sqlb.QueryRow(tx, sel).Scan(&id, &name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = ids.NewID()
		order, err := sqlb.NextSortOrder(tx, d, c.Table, c.SortOrder, nil)
		if err != nil {
			return err
		}
		ib := d.InsertInto(c.Table).Cols(c.ID, c.Name, c.SortOrder, c.OriginJSON, c.CreatedAt, c.UpdatedAt, c.VariablesPromoted, c.Stem).
			Values(id, f.Name, order, s.takeOrigin(stem, ""), now, now, 1, stem)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("apifiles: insert collection %s: %w", stem, err)
		}
		ch.Tree = true
	case err != nil:
		return fmt.Errorf("apifiles: read collection %s: %w", stem, err)
	case name != f.Name:
		ub := d.Update(c.Table)
		ub.Set(ub.Assign(c.Name, f.Name), ub.Assign(c.UpdatedAt, now)).Where(ub.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("apifiles: rename collection %s: %w", stem, err)
		}
		ch.Tree = true
	}
	if err := s.syncItems(tx, id, stem, f.Items, ch); err != nil {
		return err
	}
	if err := syncVariables(tx, d, OwnerCollection, id, f.Variables, ch); err != nil {
		return err
	}
	return nil
}

func (s *Store) forgetCollection(tx *sql.Tx, stem string, ch *Changes) error {
	d := s.o.Dialect
	c := kiraschema.APICollections
	var id string
	sel := d.Select(c.ID).From(c.Table)
	sel.Where(sel.Equal(c.Stem, stem))
	err := sqlb.QueryRow(tx, sel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apifiles: read collection %s: %w", stem, err)
	}
	del := d.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, id))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return fmt.Errorf("apifiles: delete collection %s: %w", stem, err)
	}
	ch.Tree = true
	ch.variables(OwnerCollection, id)
	return reindex(tx, d, c.Table, c.ID, c.SortOrder, c.Name, c.Stem)
}

type flatItem struct {
	key, parentKey string
	folder         bool
	name           string
	order          int
	method, url    string
	protocol       string
	requestJSON    string
}

func flatten(items []Item, parentKey string, out *[]flatItem) error {
	for i, it := range items {
		f := flatItem{key: it.ID, parentKey: parentKey, name: it.Name, order: i, protocol: model.ItemProtocolHTTP}
		switch {
		case it.Items != nil:
			f.folder = true
		case it.HTTP != nil:
			req := it.HTTP.Saved()
			if err := req.Validate(); err != nil {
				return fmt.Errorf("apifiles: item %s: %w", it.ID, err)
			}
			js, err := encodeJSON(req)
			if err != nil {
				return fmt.Errorf("apifiles: encode item %s: %w", it.ID, err)
			}
			f.method, f.url, f.requestJSON = req.Method, req.URL, js
		case it.GRPC != nil:
			req := it.GRPC.Saved()
			if err := req.Validate(); err != nil {
				return fmt.Errorf("apifiles: item %s: %w", it.ID, err)
			}
			js, err := encodeJSON(req)
			if err != nil {
				return fmt.Errorf("apifiles: encode item %s: %w", it.ID, err)
			}
			f.protocol, f.method, f.url, f.requestJSON = model.ItemProtocolGrpc, req.Service+"/"+req.Method, req.Target, js
		}
		*out = append(*out, f)
		if it.Items != nil {
			if err := flatten(*it.Items, it.ID, out); err != nil {
				return err
			}
		}
	}
	return nil
}

type itemRow struct {
	id, parent, kind, name, method, url, protocol, requestJSON, origin string
	order                                                              int
}

func (s *Store) syncItems(tx *sql.Tx, collectionID, stem string, items []Item, ch *Changes) error {
	var flat []flatItem
	if err := flatten(items, "", &flat); err != nil {
		return err
	}
	d := s.o.Dialect
	c := kiraschema.APIItems
	existing, unkeyed, err := loadItemRows(tx, d, collectionID, stem)
	if err != nil {
		return err
	}
	kept := map[string]bool{}
	ids := map[string]string{}
	now := kiratime.NowMs()
	for _, f := range flat {
		parent := ids[f.parentKey]
		old, found := existing[f.key]
		if !found {
			id, err := s.insertItem(tx, collectionID, stem, parent, f, now, ch)
			if err != nil {
				return err
			}
			ids[f.key] = id
			kept[id] = true
			continue
		}
		ids[f.key] = old.id
		kept[old.id] = true
		if err := updateItem(tx, d, old, parent, f, now, ch); err != nil {
			return err
		}
	}
	for _, r := range existing {
		if !kept[r.id] {
			del := d.DeleteFrom(c.Table)
			del.Where(del.Equal(c.ID, r.id))
			if _, err := sqlb.Exec(tx, del); err != nil {
				return fmt.Errorf("apifiles: delete item: %w", err)
			}
			ch.Tree = true
		}
	}
	for _, id := range unkeyed {
		del := d.DeleteFrom(c.Table)
		del.Where(del.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return fmt.Errorf("apifiles: delete unkeyed item: %w", err)
		}
		ch.Tree = true
	}
	return nil
}

// loadItemRows returns the collection's rows by item key and the ids of rows with no key.
func loadItemRows(tx *sql.Tx, d sqlb.Dialect, collectionID, stem string) (map[string]itemRow, []string, error) {
	c := kiraschema.APIItems
	sel := d.Select(c.ID, "COALESCE("+c.ItemKey+", '')", "COALESCE("+c.ParentID+", '')", c.Kind, c.Name, c.SortOrder, c.Method, c.URL,
		c.Protocol, c.RequestJSON, c.OriginJSON).From(c.Table)
	sel.Where(sel.Equal(c.CollectionID, collectionID))
	rows, err := sqlb.Query(tx, sel)
	if err != nil {
		return nil, nil, fmt.Errorf("apifiles: read items of %s: %w", stem, err)
	}
	defer rows.Close()
	existing := map[string]itemRow{}
	var unkeyed []string
	for rows.Next() {
		var key string
		var r itemRow
		if err := rows.Scan(&r.id, &key, &r.parent, &r.kind, &r.name, &r.order, &r.method, &r.url, &r.protocol, &r.requestJSON, &r.origin); err != nil {
			return nil, nil, fmt.Errorf("apifiles: scan item of %s: %w", stem, err)
		}
		if key == "" {
			unkeyed = append(unkeyed, r.id)
			continue
		}
		existing[key] = r
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("apifiles: items of %s: %w", stem, err)
	}
	return existing, unkeyed, nil
}

func itemKind(f flatItem) string {
	if f.folder {
		return model.CollectionItemFolder
	}
	return model.CollectionItemRequest
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) insertItem(tx *sql.Tx, collectionID, stem, parent string, f flatItem, now int64, ch *Changes) (string, error) {
	id := ids.NewID()
	kind := itemKind(f)
	c := kiraschema.APIItems
	ib := s.o.Dialect.InsertInto(c.Table).Cols(c.ID, c.CollectionID, c.ParentID, c.Kind, c.Name, c.SortOrder, c.Method, c.URL, c.Protocol,
		c.RequestJSON, c.OriginJSON, c.CreatedAt, c.UpdatedAt, c.ItemKey).
		Values(id, collectionID, nullable(parent), kind, f.name, f.order, f.method, f.url, f.protocol, f.requestJSON,
			s.takeOrigin(stem, f.key), now, now, f.key)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return "", fmt.Errorf("apifiles: insert item %s: %w", f.key, err)
	}
	ch.Tree = true
	if kind == model.CollectionItemRequest {
		ch.request(id)
	}
	return id, nil
}

func updateItem(tx *sql.Tx, d sqlb.Dialect, old itemRow, parent string, f flatItem, now int64, ch *Changes) error {
	kind := itemKind(f)
	structural := old.parent != parent || old.kind != kind || old.name != f.name || old.order != f.order ||
		old.method != f.method || old.url != f.url || old.protocol != f.protocol
	requestChanged := old.requestJSON != f.requestJSON
	if !structural && !requestChanged {
		return nil
	}
	origin := old.origin
	if requestChanged && f.protocol == model.ItemProtocolHTTP && f.requestJSON != "" {
		var req model.SavedRequest
		if err := json.Unmarshal([]byte(f.requestJSON), &req); err == nil {
			origin = shedOrigin(old.origin, req)
		}
	}
	c := kiraschema.APIItems
	ub := d.Update(c.Table)
	ub.Set(ub.Assign(c.ParentID, nullable(parent)), ub.Assign(c.Kind, kind), ub.Assign(c.Name, f.name), ub.Assign(c.SortOrder, f.order),
		ub.Assign(c.Method, f.method), ub.Assign(c.URL, f.url), ub.Assign(c.Protocol, f.protocol),
		ub.Assign(c.RequestJSON, f.requestJSON), ub.Assign(c.OriginJSON, origin), ub.Assign(c.UpdatedAt, now)).
		Where(ub.Equal(c.ID, old.id))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return fmt.Errorf("apifiles: update item %s: %w", f.key, err)
	}
	if structural {
		ch.Tree = true
	}
	if requestChanged {
		ch.request(old.id)
	}
	return nil
}

// shedOrigin drops the members of an item's Postman origin the edited request now overrides.
func shedOrigin(originJSON string, req model.SavedRequest) string {
	origin := map[string]json.RawMessage{}
	if originJSON != "" {
		if err := json.Unmarshal([]byte(originJSON), &origin); err != nil {
			slog.Warn("collection item origin is not a JSON object; discarding it", "scope", "apifiles")
			origin = map[string]json.RawMessage{}
		}
	}
	js, err := encodeJSON(postman.ShedOrigin(origin, req))
	if err != nil {
		return "{}"
	}
	return js
}

// ---- environments ----

func (s *Store) recordEnvironment(tx *sql.Tx, stem, valueJSON string, ch *Changes) error {
	var f EnvironmentFile
	if err := json.Unmarshal([]byte(valueJSON), &f); err != nil {
		return fmt.Errorf("apifiles: decode %s: %w", stem, err)
	}
	color := orDefault(f.Color, "none")
	now := kiratime.NowMs()
	d := s.o.Dialect
	c := kiraschema.APIEnvironments
	var id, name, description, oldColor string
	sel := d.Select(c.ID, c.Name, c.Description, c.Color).From(c.Table)
	sel.Where(sel.Equal(c.Stem, stem))
	err := sqlb.QueryRow(tx, sel).Scan(&id, &name, &description, &oldColor)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = ids.NewID()
		order, err := sqlb.NextSortOrder(tx, d, c.Table, c.SortOrder, nil)
		if err != nil {
			return err
		}
		ib := d.InsertInto(c.Table).Cols(c.ID, c.Name, c.SortOrder, c.IsActive, c.Description, c.Color, c.CreatedAt, c.UpdatedAt, c.Stem).
			Values(id, f.Name, order, 0, f.Description, color, now, now, stem)
		if _, err := sqlb.Exec(tx, ib); err != nil {
			return fmt.Errorf("apifiles: insert environment %s: %w", stem, err)
		}
		ch.Environments = true
	case err != nil:
		return fmt.Errorf("apifiles: read environment %s: %w", stem, err)
	case name != f.Name || description != f.Description || oldColor != color:
		ub := d.Update(c.Table)
		ub.Set(ub.Assign(c.Name, f.Name), ub.Assign(c.Description, f.Description), ub.Assign(c.Color, color), ub.Assign(c.UpdatedAt, now)).
			Where(ub.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, ub); err != nil {
			return fmt.Errorf("apifiles: update environment %s: %w", stem, err)
		}
		ch.Environments = true
	}
	if err := syncVariables(tx, d, OwnerEnvironment, id, f.Variables, ch); err != nil {
		return err
	}
	return nil
}

func (s *Store) forgetEnvironment(tx *sql.Tx, stem string, ch *Changes) error {
	d := s.o.Dialect
	c := kiraschema.APIEnvironments
	var id string
	sel := d.Select(c.ID).From(c.Table)
	sel.Where(sel.Equal(c.Stem, stem))
	err := sqlb.QueryRow(tx, sel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apifiles: read environment %s: %w", stem, err)
	}
	del := d.DeleteFrom(c.Table)
	del.Where(del.Equal(c.ID, id))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return fmt.Errorf("apifiles: delete environment %s: %w", stem, err)
	}
	ch.Environments = true
	ch.variables(OwnerEnvironment, id)
	return reindex(tx, d, c.Table, c.ID, c.SortOrder, c.Name, c.Stem)
}

// reindex keeps table's sort_order dense after a delete, in the order the rows already have.
func reindex(tx *sql.Tx, d sqlb.Dialect, table, key, order string, orderBy ...string) error {
	return sqlb.ReindexSortOrder(tx, d, sqlb.SortSet{
		Table: table, Key: key, Order: order,
		Select: d.Select(key).From(table).OrderBy(orderBy...),
	})
}

// ---- variables ----

type varRow struct {
	id, value, ref, description string
	secret                      bool
	order                       int
}

// secretRef returns the secret name of a `${secret:name}` reference, "" for no reference.
func secretRef(ref string) string {
	refs, p := configfs.Refs(ref)
	if p != nil || len(refs) != 1 || refs[0].Kind != configfs.RefSecret {
		return ""
	}
	return refs[0].Name
}

// SecretNameOf is the secret name of a variable's `${secret:name}` reference, "" for none.
func SecretNameOf(ref string) string { return secretRef(ref) }

func syncVariables(tx *sql.Tx, d sqlb.Dialect, scope, ownerID string, vars []Variable, ch *Changes) error {
	c := kiraschema.APIVariables
	column := c.CollectionID
	if scope == OwnerEnvironment {
		column = c.EnvironmentID
	}
	existing, all, err := loadVariableRows(tx, d, column, ownerID)
	if err != nil {
		return err
	}
	kept := map[string]bool{}
	now := kiratime.NowMs()
	changed := false
	for i, v := range vars {
		secret := v.Secret != ""
		ref := secretRef(v.Secret)
		value := v.Value
		if secret {
			value = ""
		}
		var refArg any
		if secret {
			refArg = ref
		}
		old, found := existing[v.Name]
		if !found {
			ib := d.InsertInto(c.Table).Cols(c.ID, column, c.Name, c.Value, c.IsSecret, c.SecretValue, c.SortOrder, c.Description, c.SecretRef,
				c.CreatedAt, c.UpdatedAt).
				Values(ids.NewID(), ownerID, v.Name, value, boolInt(secret), nil, i, v.Description, refArg, now, now)
			if _, err := sqlb.Exec(tx, ib); err != nil {
				return fmt.Errorf("apifiles: insert variable: %w", err)
			}
			changed = true
			continue
		}
		kept[old.id] = true
		if old.secret == secret && old.value == value && old.ref == ref && old.description == v.Description && old.order == i {
			continue
		}
		if err := updateVariable(tx, d, old, v, value, secret, refArg, i, now); err != nil {
			return err
		}
		changed = true
	}
	for _, id := range all {
		if kept[id] {
			continue
		}
		del := d.DeleteFrom(c.Table)
		del.Where(del.Equal(c.ID, id))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return fmt.Errorf("apifiles: delete variable: %w", err)
		}
		changed = true
	}
	if changed {
		ch.variables(scope, ownerID)
	}
	return nil
}

// loadVariableRows returns the owner's rows by name (first wins) and every row id.
func loadVariableRows(tx *sql.Tx, d sqlb.Dialect, column, ownerID string) (map[string]varRow, []string, error) {
	c := kiraschema.APIVariables
	sel := d.Select(c.ID, c.Name, c.Value, c.IsSecret, "COALESCE("+c.SecretRef+", '')", c.Description, c.SortOrder).From(c.Table)
	sel.Where(sel.Equal(column, ownerID)).OrderBy(c.SortOrder, sqlb.RowID)
	rows, err := sqlb.Query(tx, sel)
	if err != nil {
		return nil, nil, fmt.Errorf("apifiles: read variables: %w", err)
	}
	defer rows.Close()
	existing := map[string]varRow{}
	var all []string
	for rows.Next() {
		var name string
		var r varRow
		var secret int
		if err := rows.Scan(&r.id, &name, &r.value, &secret, &r.ref, &r.description, &r.order); err != nil {
			return nil, nil, fmt.Errorf("apifiles: scan variable: %w", err)
		}
		r.secret = secret != 0
		all = append(all, r.id)
		if _, dup := existing[name]; !dup {
			existing[name] = r
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("apifiles: variables: %w", err)
	}
	return existing, all, nil
}

func updateVariable(tx *sql.Tx, d sqlb.Dialect, old varRow, v Variable, value string, secret bool, refArg any, order int, now int64) error {
	if !old.secret && !secret && old.value != value {
		if err := recordHistory(tx, d, old.id, old.value, now); err != nil {
			return err
		}
	}
	if secret && !old.secret {
		h := kiraschema.APIVariableHistory
		del := d.DeleteFrom(h.Table)
		del.Where(del.Equal(h.VariableID, old.id), del.Equal(h.IsSecret, 0))
		if _, err := sqlb.Exec(tx, del); err != nil {
			return fmt.Errorf("apifiles: purge plain history: %w", err)
		}
	}
	c := kiraschema.APIVariables
	ub := d.Update(c.Table)
	ub.Set(ub.Assign(c.Value, value), ub.Assign(c.IsSecret, boolInt(secret)), ub.Assign(c.SecretValue, nil), ub.Assign(c.SecretRef, refArg),
		ub.Assign(c.Description, v.Description), ub.Assign(c.SortOrder, order), ub.Assign(c.UpdatedAt, now)).
		Where(ub.Equal(c.ID, old.id))
	if _, err := sqlb.Exec(tx, ub); err != nil {
		return fmt.Errorf("apifiles: update variable: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// recordHistory keeps the value a plain variable had before an edit, then trims to the limit.
func recordHistory(tx *sql.Tx, d sqlb.Dialect, variableID, old string, now int64) error {
	h := kiraschema.APIVariableHistory
	ib := d.InsertInto(h.Table).Cols(h.ID, h.VariableID, h.Value, h.IsSecret, h.SecretValue, h.RecordedAt).
		Values(ids.NewID(), variableID, old, 0, nil, now)
	if _, err := sqlb.Exec(tx, ib); err != nil {
		return fmt.Errorf("apifiles: insert history: %w", err)
	}
	keep := d.Select(h.ID).From(h.Table)
	keep.Where(keep.Equal(h.VariableID, variableID)).OrderBy(h.RecordedAt+" DESC", sqlb.RowID+" DESC").Limit(variableHistoryLimit)
	del := d.DeleteFrom(h.Table)
	del.Where(del.Equal(h.VariableID, variableID), del.NotIn(h.ID, keep))
	if _, err := sqlb.Exec(tx, del); err != nil {
		return fmt.Errorf("apifiles: trim history: %w", err)
	}
	return nil
}
