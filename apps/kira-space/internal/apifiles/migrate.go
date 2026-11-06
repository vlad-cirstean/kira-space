package apifiles

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/postman"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// migrate exports the rows that predate the api files: it binds each row to a stem, an item key and a
// secret ref, moves sealed secret values into the secret store as they are, and writes one file per
// collection and environment. A file that exists is never overwritten. A collection's promoted Postman
// variables leave its origin only after its file exists, so a rerun renders them again. A crash before the
// marker reruns it with the stored stems.
func (s *Store) migrate() error {
	m, err := s.bindLegacyRows()
	if err != nil {
		return err
	}
	for name, text := range m.files {
		if !fileExists(s.o.Dir, name) {
			if len(text) > configfs.MaxFileBytes {
				slog.Warn("api: migrated file is over the size cap and stays invalid until edited", "scope", "startup", "file", name)
			}
			if err := configfs.WriteAtomic(filepath.Join(s.o.Dir, name), text, 0o644); err != nil {
				return err
			}
		}
		if st, ok := m.stamps[name]; ok {
			if _, err := s.o.DB.Exec(`UPDATE api_collections SET origin_json = ?, variables_promoted = 1 WHERE id = ?`, st.origin, st.id); err != nil {
				return fmt.Errorf("apifiles: stamp promoted: %w", err)
			}
		}
	}
	return nil
}

// stamp is a collection's origin without its promoted variables, applied once its file exists.
type stamp struct{ id, origin string }

type legacyVar struct {
	id, name, value, desc, ref string
	secret                     bool
	sealed                     sql.NullString
}

type legacyItem struct {
	id, parent, kind, name, protocol, requestJSON, key string
}

func (s *Store) bindLegacyRows() (*migration, error) {
	tx, err := s.o.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("apifiles: begin migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	m := &migration{s: s, tx: tx, files: map[string][]byte{}, stamps: map[string]stamp{}, usedSecrets: map[string]bool{}}
	if err := m.loadSecretNames(); err != nil {
		return nil, err
	}
	if err := m.collections(); err != nil {
		return nil, err
	}
	if err := m.environments(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("apifiles: commit migration: %w", err)
	}
	return m, nil
}

type migration struct {
	s           *Store
	tx          *sql.Tx
	files       map[string][]byte
	stamps      map[string]stamp
	usedSecrets map[string]bool
}

func (m *migration) loadSecretNames() error {
	rows, err := m.tx.Query(`SELECT name FROM secret_values WHERE scope = ? AND owner = ?`, string(secrets.ScopeVariable), SecretOwner)
	if err != nil {
		return fmt.Errorf("apifiles: read secret names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		m.usedSecrets[n] = true
	}
	return rows.Err()
}

func (m *migration) freeStem(table, ext, base string, taken map[string]bool) (string, error) {
	var err error
	stem := freeName(base, func(c string) bool {
		if taken[c] || fileExists(m.s.o.Dir, c+ext) {
			return true
		}
		var n int
		if qerr := m.tx.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE stem = ?`, c).Scan(&n); qerr != nil {
			err = qerr
		}
		return n > 0
	})
	return stem, err
}

// variables binds the variables of one owner and returns them in file form. Later rows of a name are
// dead (sends were first-wins) and come back as comment lines.
func (m *migration) variables(column, ownerID, ownerStem string, promoted []postman.Variable) ([]Variable, []string, error) {
	all, err := m.legacyVars(column, ownerID)
	if err != nil {
		return nil, nil, err
	}
	var out []Variable
	var dead []string
	seen := map[string]bool{}
	for _, v := range all {
		if seen[v.name] {
			dead = append(dead, deadVariable(v.name, v.value, v.secret))
			continue
		}
		seen[v.name] = true
		fv := Variable{Name: v.name, Description: v.desc}
		if !v.secret {
			fv.Value = v.value
			out = append(out, fv)
			continue
		}
		ref, err := m.bindLegacySecret(ownerStem, v)
		if err != nil {
			return nil, nil, err
		}
		fv.Secret = "${secret:" + ref + "}"
		out = append(out, fv)
	}
	for _, p := range promoted {
		if p.Name == "" {
			continue
		}
		if seen[p.Name] {
			dead = append(dead, deadVariable(p.Name, p.Value, p.Secret))
			continue
		}
		seen[p.Name] = true
		out = append(out, m.promotedVariable(ownerStem, p))
	}
	return out, dead, nil
}

func (m *migration) legacyVars(column, ownerID string) ([]legacyVar, error) {
	rows, err := m.tx.Query(`SELECT id, name, value, is_secret, secret_value, description, COALESCE(secret_ref, '') FROM api_variables
		WHERE `+column+` = ? ORDER BY sort_order, rowid`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("apifiles: read variables: %w", err)
	}
	defer rows.Close()
	var all []legacyVar
	for rows.Next() {
		var v legacyVar
		var secret int
		if err := rows.Scan(&v.id, &v.name, &v.value, &secret, &v.sealed, &v.desc, &v.ref); err != nil {
			return nil, err
		}
		v.secret = secret != 0
		all = append(all, v)
	}
	return all, rows.Err()
}

// bindLegacySecret moves a row's sealed value into the secret store and returns its secret name.
func (m *migration) bindLegacySecret(ownerStem string, v legacyVar) (string, error) {
	if v.ref != "" {
		return v.ref, nil
	}
	ref := m.newSecretName(ownerStem, v.name)
	if v.sealed.Valid {
		if _, err := m.tx.Exec(`INSERT INTO secret_values (scope, owner, name, value, updated_ms) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (scope, owner, name) DO UPDATE SET value = excluded.value, updated_ms = excluded.updated_ms`,
			string(secrets.ScopeVariable), SecretOwner, ref, v.sealed.String, nowMillis()); err != nil {
			return "", fmt.Errorf("apifiles: move secret: %w", err)
		}
	}
	if _, err := m.tx.Exec(`UPDATE api_variables SET secret_ref = ?, secret_value = NULL WHERE id = ?`, ref, v.id); err != nil {
		return "", fmt.Errorf("apifiles: bind secret ref: %w", err)
	}
	return ref, nil
}

func (m *migration) promotedVariable(ownerStem string, p postman.Variable) Variable {
	fv := Variable{Name: p.Name, Description: p.Description}
	if !p.Secret {
		fv.Value = p.Value
		return fv
	}
	ref := m.newSecretName(ownerStem, p.Name)
	if p.Value != "" {
		if err := m.s.o.Secrets.Store.Put(m.tx, secrets.ScopeVariable, SecretOwner, ref, p.Value); err != nil {
			slog.Warn("api: a promoted secret value could not be sealed and stays unset", "scope", "startup", "name", ref)
		}
	}
	fv.Secret = "${secret:" + ref + "}"
	return fv
}

func deadVariable(name, value string, secret bool) string {
	if secret {
		return fmt.Sprintf("Duplicate variable, not applied (secret): %s", name)
	}
	return fmt.Sprintf("Duplicate variable, not applied: %s = %s", name, strings.ReplaceAll(strings.ReplaceAll(value, "\r", " "), "\n", " "))
}

func (m *migration) newSecretName(ownerStem, variable string) string {
	name := freeName(SecretName(ownerStem, variable), func(c string) bool { return m.usedSecrets[c] })
	m.usedSecrets[name] = true
	return name
}

func nowMillis() int64 { return time.Now().UnixMilli() }

type legacyCollection struct {
	id, name, origin, stem string
	promoted               bool
}

func (m *migration) legacyCollections() ([]legacyCollection, map[string]bool, error) {
	rows, err := m.tx.Query(`SELECT id, name, origin_json, variables_promoted, COALESCE(stem, '') FROM api_collections ORDER BY sort_order, name`)
	if err != nil {
		return nil, nil, fmt.Errorf("apifiles: read collections: %w", err)
	}
	defer rows.Close()
	var cols []legacyCollection
	taken := map[string]bool{}
	for rows.Next() {
		var c legacyCollection
		var promoted int
		if err := rows.Scan(&c.id, &c.name, &c.origin, &promoted, &c.stem); err != nil {
			return nil, nil, err
		}
		c.promoted = promoted != 0
		if c.stem != "" {
			taken[c.stem] = true
		}
		cols = append(cols, c)
	}
	return cols, taken, rows.Err()
}

// promoteOrigin lifts a not-yet-promoted collection's Postman variables out of its origin and returns the
// stamp that makes it permanent. The caller applies the stamp after the file exists.
func (m *migration) promoteOrigin(c legacyCollection) ([]postman.Variable, *stamp, error) {
	if c.promoted {
		return nil, nil, nil
	}
	var promoted []postman.Variable
	origin := map[string]json.RawMessage{}
	_ = json.Unmarshal([]byte(c.origin), &origin)
	if raw, ok := origin["variable"]; ok {
		if decoded, isArray := postman.DecodeVariables(raw); isArray {
			promoted = decoded
			delete(origin, "variable")
		}
	}
	js, err := encodeJSON(origin)
	if err != nil {
		return nil, nil, err
	}
	return promoted, &stamp{id: c.id, origin: js}, nil
}

func (m *migration) collections() error {
	cols, taken, err := m.legacyCollections()
	if err != nil {
		return err
	}
	for _, c := range cols {
		if c.stem == "" {
			stem, err := m.freeStem("api_collections", CollectionExt, Slug(c.name, "collection"), taken)
			if err != nil {
				return err
			}
			c.stem = stem
			taken[stem] = true
			if _, err := m.tx.Exec(`UPDATE api_collections SET stem = ? WHERE id = ?`, stem, c.id); err != nil {
				return fmt.Errorf("apifiles: bind collection: %w", err)
			}
		}
		promoted, st, err := m.promoteOrigin(c)
		if err != nil {
			return err
		}
		vars, dead, err := m.variables("collection_id", c.id, c.stem, promoted)
		if err != nil {
			return err
		}
		items, err := m.items(c.id)
		if err != nil {
			return err
		}
		text, err := configfs.RenderYAML(CollectionFile{Name: c.name, Variables: vars, Items: items}, strings.Join(dead, "\n"))
		if err != nil {
			return err
		}
		m.files[c.stem+CollectionExt] = text
		if st != nil {
			m.stamps[c.stem+CollectionExt] = *st
		}
	}
	return nil
}

func (m *migration) items(collectionID string) ([]Item, error) {
	rows, err := m.tx.Query(`SELECT id, COALESCE(parent_id, ''), kind, name, protocol, request_json, COALESCE(item_key, '') FROM api_items
		WHERE collection_id = ? ORDER BY sort_order, created_at, id`, collectionID)
	if err != nil {
		return nil, fmt.Errorf("apifiles: read items: %w", err)
	}
	byParent := map[string][]*legacyItem{}
	taken := map[string]bool{}
	var all []*legacyItem
	for rows.Next() {
		it := &legacyItem{}
		if err := rows.Scan(&it.id, &it.parent, &it.kind, &it.name, &it.protocol, &it.requestJSON, &it.key); err != nil {
			rows.Close()
			return nil, err
		}
		byParent[it.parent] = append(byParent[it.parent], it)
		if it.key != "" {
			taken[it.key] = true
		}
		all = append(all, it)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, it := range all {
		if it.key != "" {
			continue
		}
		it.key = freeName(Slug(it.name, "item"), func(c string) bool { return taken[c] })
		taken[it.key] = true
		if _, err := m.tx.Exec(`UPDATE api_items SET item_key = ? WHERE id = ?`, it.key, it.id); err != nil {
			return nil, fmt.Errorf("apifiles: bind item key: %w", err)
		}
	}
	visited := map[string]bool{}
	var build func(parent string) []Item
	build = func(parent string) []Item {
		out := []Item{}
		for _, it := range byParent[parent] {
			if visited[it.id] {
				continue
			}
			visited[it.id] = true
			item := Item{ID: it.key, Name: it.name}
			switch {
			case it.kind == model.CollectionItemFolder:
				kids := build(it.id)
				item.Items = &kids
			case it.protocol == model.ItemProtocolGrpc:
				var req model.SavedGrpcRequest
				if json.Unmarshal([]byte(it.requestJSON), &req) != nil || req.Validate() != nil {
					slog.Warn("api: a legacy gRPC request is unreadable, migrated empty", "scope", "startup", "id", it.id)
					req = model.SavedGrpcRequest{TLSMode: tlsOn, DescriptorMode: descReflect}
				}
				g := GRPCOf(req)
				item.GRPC = &g
			default:
				var req model.SavedRequest
				if json.Unmarshal([]byte(it.requestJSON), &req) != nil || req.Validate() != nil {
					slog.Warn("api: a legacy request is unreadable, migrated empty", "scope", "startup", "id", it.id)
					req = model.SavedRequest{Method: defaultMethod, BodyMode: bodyModeNone, CodeLanguage: langJSON}
				}
				h := HTTPOf(req)
				item.HTTP = &h
			}
			out = append(out, item)
		}
		return out
	}
	return build(""), nil
}

func (m *migration) environments() error {
	rows, err := m.tx.Query(`SELECT id, name, description, color, COALESCE(stem, '') FROM api_environments ORDER BY sort_order, name`)
	if err != nil {
		return fmt.Errorf("apifiles: read environments: %w", err)
	}
	type env struct{ id, name, desc, color, stem string }
	var envs []env
	taken := map[string]bool{}
	for rows.Next() {
		var e env
		if err := rows.Scan(&e.id, &e.name, &e.desc, &e.color, &e.stem); err != nil {
			rows.Close()
			return err
		}
		if e.stem != "" {
			taken[e.stem] = true
		}
		envs = append(envs, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	sort.SliceStable(envs, func(i, j int) bool { return envs[i].stem != "" && envs[j].stem == "" })
	for _, e := range envs {
		if e.stem == "" {
			stem, err := m.freeStem("api_environments", EnvironmentExt, Slug(e.name, "environment"), taken)
			if err != nil {
				return err
			}
			e.stem = stem
			taken[stem] = true
			if _, err := m.tx.Exec(`UPDATE api_environments SET stem = ? WHERE id = ?`, stem, e.id); err != nil {
				return fmt.Errorf("apifiles: bind environment: %w", err)
			}
		}
		vars, dead, err := m.variables("environment_id", e.id, e.stem, nil)
		if err != nil {
			return err
		}
		color := e.color
		if !model.ValidPaletteColor(color) || color == "none" {
			color = ""
		}
		text, err := configfs.RenderYAML(EnvironmentFile{Name: e.name, Description: e.desc, Color: color, Variables: vars}, strings.Join(dead, "\n"))
		if err != nil {
			return err
		}
		m.files[e.stem+EnvironmentExt] = text
	}
	return nil
}
