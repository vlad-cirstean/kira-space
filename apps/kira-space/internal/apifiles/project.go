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
	"github.com/kirathecat/kira-studio/internal/kiratime"
	"github.com/kirathecat/kira-studio/internal/sqlitex"
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
		if _, err := tx.Exec(`INSERT INTO config_last_valid (folder, file, value_json, recorded_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(folder, file) DO UPDATE SET value_json = excluded.value_json, recorded_at = excluded.recorded_at`,
			p.folder, file, valueJSON, now); err != nil {
			return fmt.Errorf("apifiles: record last valid %s: %w", file, err)
		}
		return p.record(tx, stem, valueJSON, ch)
	})
}

func (p *projection) Forget(file string) error {
	return p.s.tx(func(tx *sql.Tx, ch *Changes) error {
		if _, err := tx.Exec(`DELETE FROM config_last_valid WHERE folder = ? AND file = ?`, p.folder, file); err != nil {
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
	var id, name string
	err := tx.QueryRow(`SELECT id, name FROM api_collections WHERE stem = ?`, stem).Scan(&id, &name)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = ids.NewID()
		if _, err := tx.Exec(`INSERT INTO api_collections (id, name, sort_order, origin_json, created_at, updated_at, variables_promoted, stem)
			VALUES (?, ?, (SELECT COALESCE(MAX(sort_order) + 1, 0) FROM api_collections), ?, ?, ?, 1, ?)`, id, f.Name, s.takeOrigin(stem, ""), now, now, stem); err != nil {
			return fmt.Errorf("apifiles: insert collection %s: %w", stem, err)
		}
		ch.Tree = true
	case err != nil:
		return fmt.Errorf("apifiles: read collection %s: %w", stem, err)
	case name != f.Name:
		if _, err := tx.Exec(`UPDATE api_collections SET name = ?, updated_at = ? WHERE id = ?`, f.Name, now, id); err != nil {
			return fmt.Errorf("apifiles: rename collection %s: %w", stem, err)
		}
		ch.Tree = true
	}
	if err := s.syncItems(tx, id, stem, f.Items, ch); err != nil {
		return err
	}
	if err := syncVariables(tx, OwnerCollection, id, f.Variables, ch); err != nil {
		return err
	}
	return nil
}

func (s *Store) forgetCollection(tx *sql.Tx, stem string, ch *Changes) error {
	var id string
	err := tx.QueryRow(`SELECT id FROM api_collections WHERE stem = ?`, stem).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apifiles: read collection %s: %w", stem, err)
	}
	if _, err := tx.Exec(`DELETE FROM api_collections WHERE id = ?`, id); err != nil {
		return fmt.Errorf("apifiles: delete collection %s: %w", stem, err)
	}
	ch.Tree = true
	ch.variables(OwnerCollection, id)
	return reindex(tx, "api_collections")
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
	existing, unkeyed, err := loadItemRows(tx, collectionID, stem)
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
		if err := updateItem(tx, old, parent, f, now, ch); err != nil {
			return err
		}
	}
	for _, r := range existing {
		if !kept[r.id] {
			if _, err := tx.Exec(`DELETE FROM api_items WHERE id = ?`, r.id); err != nil {
				return fmt.Errorf("apifiles: delete item: %w", err)
			}
			ch.Tree = true
		}
	}
	for _, id := range unkeyed {
		if _, err := tx.Exec(`DELETE FROM api_items WHERE id = ?`, id); err != nil {
			return fmt.Errorf("apifiles: delete unkeyed item: %w", err)
		}
		ch.Tree = true
	}
	return nil
}

// loadItemRows returns the collection's rows by item key and the ids of rows with no key.
func loadItemRows(tx *sql.Tx, collectionID, stem string) (map[string]itemRow, []string, error) {
	rows, err := tx.Query(`SELECT id, COALESCE(item_key, ''), COALESCE(parent_id, ''), kind, name, sort_order, method, url, protocol, request_json, origin_json
		FROM api_items WHERE collection_id = ?`, collectionID)
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
	if _, err := tx.Exec(`INSERT INTO api_items (id, collection_id, parent_id, kind, name, sort_order, method, url, protocol, request_json, origin_json, created_at, updated_at, item_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, collectionID, nullable(parent), kind, f.name, f.order, f.method, f.url, f.protocol, f.requestJSON, s.takeOrigin(stem, f.key), now, now, f.key); err != nil {
		return "", fmt.Errorf("apifiles: insert item %s: %w", f.key, err)
	}
	ch.Tree = true
	if kind == model.CollectionItemRequest {
		ch.request(id)
	}
	return id, nil
}

func updateItem(tx *sql.Tx, old itemRow, parent string, f flatItem, now int64, ch *Changes) error {
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
	if _, err := tx.Exec(`UPDATE api_items SET parent_id = ?, kind = ?, name = ?, sort_order = ?, method = ?, url = ?, protocol = ?, request_json = ?, origin_json = ?, updated_at = ?
		WHERE id = ?`, nullable(parent), kind, f.name, f.order, f.method, f.url, f.protocol, f.requestJSON, origin, now, old.id); err != nil {
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
	var id, name, description, oldColor string
	err := tx.QueryRow(`SELECT id, name, description, color FROM api_environments WHERE stem = ?`, stem).Scan(&id, &name, &description, &oldColor)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = ids.NewID()
		if _, err := tx.Exec(`INSERT INTO api_environments (id, name, sort_order, is_active, description, color, created_at, updated_at, stem)
			VALUES (?, ?, (SELECT COALESCE(MAX(sort_order) + 1, 0) FROM api_environments), 0, ?, ?, ?, ?, ?)`, id, f.Name, f.Description, color, now, now, stem); err != nil {
			return fmt.Errorf("apifiles: insert environment %s: %w", stem, err)
		}
		ch.Environments = true
	case err != nil:
		return fmt.Errorf("apifiles: read environment %s: %w", stem, err)
	case name != f.Name || description != f.Description || oldColor != color:
		if _, err := tx.Exec(`UPDATE api_environments SET name = ?, description = ?, color = ?, updated_at = ? WHERE id = ?`,
			f.Name, f.Description, color, now, id); err != nil {
			return fmt.Errorf("apifiles: update environment %s: %w", stem, err)
		}
		ch.Environments = true
	}
	if err := syncVariables(tx, OwnerEnvironment, id, f.Variables, ch); err != nil {
		return err
	}
	return nil
}

func (s *Store) forgetEnvironment(tx *sql.Tx, stem string, ch *Changes) error {
	var id string
	err := tx.QueryRow(`SELECT id FROM api_environments WHERE stem = ?`, stem).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apifiles: read environment %s: %w", stem, err)
	}
	if _, err := tx.Exec(`DELETE FROM api_environments WHERE id = ?`, id); err != nil {
		return fmt.Errorf("apifiles: delete environment %s: %w", stem, err)
	}
	ch.Environments = true
	ch.variables(OwnerEnvironment, id)
	return reindex(tx, "api_environments")
}

// reindex keeps table's sort_order dense after a delete, in the order the rows already have.
func reindex(tx *sql.Tx, table string) error {
	return sqlitex.ReindexSortOrder(tx, `SELECT id FROM `+table+` ORDER BY sort_order, name, stem`,
		`UPDATE `+table+` SET sort_order = ? WHERE id = ?`)
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

func syncVariables(tx *sql.Tx, scope, ownerID string, vars []Variable, ch *Changes) error {
	column := "collection_id"
	if scope == OwnerEnvironment {
		column = "environment_id"
	}
	existing, all, err := loadVariableRows(tx, column, ownerID)
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
			if _, err := tx.Exec(`INSERT INTO api_variables (id, `+column+`, name, value, is_secret, secret_value, sort_order, description, secret_ref, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?, ?, ?)`,
				ids.NewID(), ownerID, v.Name, value, boolInt(secret), i, v.Description, refArg, now, now); err != nil {
				return fmt.Errorf("apifiles: insert variable: %w", err)
			}
			changed = true
			continue
		}
		kept[old.id] = true
		if old.secret == secret && old.value == value && old.ref == ref && old.description == v.Description && old.order == i {
			continue
		}
		if err := updateVariable(tx, old, v, value, secret, refArg, i, now); err != nil {
			return err
		}
		changed = true
	}
	for _, id := range all {
		if kept[id] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM api_variables WHERE id = ?`, id); err != nil {
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
func loadVariableRows(tx *sql.Tx, column, ownerID string) (map[string]varRow, []string, error) {
	rows, err := tx.Query(`SELECT id, name, value, is_secret, COALESCE(secret_ref, ''), description, sort_order FROM api_variables
		WHERE `+column+` = ? ORDER BY sort_order, rowid`, ownerID)
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

func updateVariable(tx *sql.Tx, old varRow, v Variable, value string, secret bool, refArg any, order int, now int64) error {
	if !old.secret && !secret && old.value != value {
		if err := recordHistory(tx, old.id, old.value, now); err != nil {
			return err
		}
	}
	if secret && !old.secret {
		if _, err := tx.Exec(`DELETE FROM api_variable_history WHERE variable_id = ? AND is_secret = 0`, old.id); err != nil {
			return fmt.Errorf("apifiles: purge plain history: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE api_variables SET value = ?, is_secret = ?, secret_value = NULL, secret_ref = ?, description = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
		value, boolInt(secret), refArg, v.Description, order, now, old.id); err != nil {
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
func recordHistory(tx *sql.Tx, variableID, old string, now int64) error {
	if _, err := tx.Exec(`INSERT INTO api_variable_history (id, variable_id, value, is_secret, secret_value, recorded_at) VALUES (?, ?, ?, 0, NULL, ?)`,
		ids.NewID(), variableID, old, now); err != nil {
		return fmt.Errorf("apifiles: insert history: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM api_variable_history WHERE variable_id = ? AND id NOT IN (
		SELECT id FROM api_variable_history WHERE variable_id = ? ORDER BY recorded_at DESC, rowid DESC LIMIT ?)`,
		variableID, variableID, variableHistoryLimit); err != nil {
		return fmt.Errorf("apifiles: trim history: %w", err)
	}
	return nil
}
