package apifiles

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"
)

// ErrInvalidMove marks a placement that would nest a folder inside itself or names a neighbour that is
// not in the target folder.
var ErrInvalidMove = errors.New("apifiles: the placement is not valid")

// copySuffix names a duplicated environment.
const copySuffix = " copy"

func itemsSeq(item *yaml.Node) *yaml.Node {
	if i := mapIndex(item, "items"); i >= 0 {
		return item.Content[i]
	}
	return nil
}

func itemKey(item *yaml.Node) string {
	if i := mapIndex(item, "id"); i >= 0 {
		return item.Content[i].Value
	}
	return ""
}

func setScalar(m *yaml.Node, key, value string) {
	if i := mapIndex(m, key); i >= 0 {
		m.Content[i].Value, m.Content[i].Tag, m.Content[i].Style = value, "!!str", 0
		return
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

// cutPoint is one item node and the sequence that holds it.
type cutPoint struct {
	seq, node *yaml.Node
	key       string
}

// topmost finds the nodes of keys, outermost only: a key under another listed key is skipped. The result
// follows the order of keys. A key found nowhere is ErrItemGone.
func topmost(root *yaml.Node, keys []string) ([]cutPoint, error) {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	found := map[string]cutPoint{}
	var walk func(seq *yaml.Node)
	walk = func(seq *yaml.Node) {
		if seq == nil || seq.Kind != yaml.SequenceNode {
			return
		}
		for _, it := range seq.Content {
			if it.Kind != yaml.MappingNode {
				continue
			}
			if k := itemKey(it); want[k] {
				found[k] = cutPoint{seq: seq, node: it, key: k}
				continue
			}
			walk(itemsSeq(it))
		}
	}
	walk(rootItems(root))
	var out []cutPoint
	for _, k := range keys {
		if c, ok := found[k]; ok {
			out = append(out, c)
			delete(found, k)
			continue
		}
		covered := slices.ContainsFunc(out, func(c cutPoint) bool { return c.key == k || findItem(itemsSeq(c.node), k) != nil })
		if !covered && findItem(rootItems(root), k) == nil {
			return nil, ErrItemGone
		}
	}
	return out, nil
}

func (c cutPoint) remove() {
	c.seq.Content = slices.DeleteFunc(c.seq.Content, func(n *yaml.Node) bool { return n == c.node })
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	c := *n
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, ch := range n.Content {
		c.Content[i] = cloneNode(ch)
	}
	return &c
}

// targetSeq returns the sequence a move lands in: the folder parentKey, or the collection root.
func targetSeq(root *yaml.Node, parentKey string) (*yaml.Node, error) {
	if parentKey == "" {
		seq := rootItems(root)
		if seq == nil {
			seq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "items"}, seq)
		}
		return seq, nil
	}
	parent := findItem(rootItems(root), parentKey)
	if parent == nil {
		return nil, ErrItemGone
	}
	seq := itemsSeq(parent)
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil, ErrNotFolder
	}
	return seq, nil
}

// insertAt puts nodes before the item beforeKey of seq, or last when beforeKey is "".
func insertAt(seq *yaml.Node, beforeKey string, nodes []*yaml.Node) error {
	at := len(seq.Content)
	if beforeKey != "" {
		at = slices.IndexFunc(seq.Content, func(n *yaml.Node) bool { return n.Kind == yaml.MappingNode && itemKey(n) == beforeKey })
		if at < 0 {
			return ErrInvalidMove
		}
	}
	seq.Style = 0
	seq.Content = slices.Insert(seq.Content, at, nodes...)
	return nil
}

func collectKeys(seq *yaml.Node, into map[string]bool) {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for _, it := range seq.Content {
		if it.Kind != yaml.MappingNode {
			continue
		}
		into[itemKey(it)] = true
		collectKeys(itemsSeq(it), into)
	}
}

// ---- rename ----

// RenameItem sets the name of a request or folder. Its id in the file stays.
func (s *Store) RenameItem(itemID, name string) error {
	file, key, _, err := s.locate(itemID)
	if err != nil {
		return err
	}
	return s.editCollection(file, func(src []byte) ([]byte, error) {
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		item := findItem(rootItems(root), key)
		if item == nil {
			return nil, ErrItemGone
		}
		setScalar(item, "name", name)
		return encodeDoc(doc)
	})
}

// RenameCollection sets the name of a collection. The file stem stays.
func (s *Store) RenameCollection(collectionID, name string) error {
	file, err := s.collectionFile(collectionID)
	if err != nil {
		return err
	}
	return s.editCollection(file, func(src []byte) ([]byte, error) {
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		setScalar(root, "name", name)
		return encodeDoc(doc)
	})
}

// ---- delete ----

// deleteFiles removes config files and reloads, so the rows of a removed file go with it.
func (s *Store) deleteFiles(files []string) error {
	s.mu.Lock()
	s.pend = &Changes{}
	var err error
	for _, f := range files {
		if rerr := os.Remove(filepath.Join(s.o.Dir, f)); rerr != nil && !os.IsNotExist(rerr) {
			err = fmt.Errorf("apifiles: remove %s: %w", f, rerr)
			break
		}
	}
	s.syncLocked()
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return err
}

// DeleteItems deletes whole collections (their files) and items, each with its subtree. An item under
// a deleted folder or collection is skipped. Every id is resolved before anything is written.
func (s *Store) DeleteItems(collectionIDs, itemIDs []string) error {
	var files []string
	dropped := map[string]bool{}
	for _, id := range collectionIDs {
		f, err := s.collectionFile(id)
		if err != nil {
			return err
		}
		dropped[id] = true
		files = append(files, f)
	}
	type group struct {
		file string
		keys []string
	}
	var groups []*group
	byFile := map[string]*group{}
	for _, id := range itemIDs {
		file, key, owner, err := s.locate(id)
		if err != nil {
			return err
		}
		if dropped[owner] {
			continue
		}
		g := byFile[file]
		if g == nil {
			g = &group{file: file}
			byFile[file] = g
			groups = append(groups, g)
		}
		g.keys = append(g.keys, key)
	}
	for _, g := range groups {
		err := s.editCollection(g.file, func(src []byte) ([]byte, error) {
			doc, root, err := parseDoc(src)
			if err != nil {
				return nil, err
			}
			cuts, err := topmost(root, g.keys)
			if err != nil {
				return nil, err
			}
			for _, c := range cuts {
				c.remove()
			}
			return encodeDoc(doc)
		})
		if err != nil {
			return err
		}
	}
	if len(files) == 0 {
		return nil
	}
	return s.deleteFiles(files)
}

// ---- move ----

type moveGroup struct {
	file       string
	collection string
	rows       []string
	keys       []string
}

// MoveItems moves requests and folders, each with its subtree, under parentID (the collection root when
// nil) of collectionID, before beforeID or last. Within a file the rows stay. Across files the items are
// copied into the target first, their history is re-pointed, and only then are they cut from the source,
// so a failed step never loses or doubles an item. The result maps each moved row id to its row id now.
func (s *Store) MoveItems(itemIDs []string, collectionID string, parentID, beforeID *string) (map[string]string, error) {
	dest, err := s.collectionFile(collectionID)
	if err != nil {
		return nil, err
	}
	parentKey, err := s.placement(parentID, collectionID)
	if err != nil {
		return nil, err
	}
	beforeKey, err := s.placement(beforeID, collectionID)
	if err != nil {
		return nil, err
	}
	var groups []*moveGroup
	byFile := map[string]*moveGroup{}
	seen := map[string]bool{}
	for _, id := range itemIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if beforeID != nil && *beforeID == id {
			return nil, ErrInvalidMove
		}
		file, key, owner, err := s.locate(id)
		if err != nil {
			return nil, err
		}
		g := byFile[file]
		if g == nil {
			g = &moveGroup{file: file, collection: owner}
			byFile[file] = g
			groups = append(groups, g)
		}
		g.rows = append(g.rows, id)
		g.keys = append(g.keys, key)
	}
	moved := map[string]string{}
	for _, g := range groups {
		if g.file == dest {
			err = s.moveWithin(g, dest, parentKey, beforeKey)
			for _, id := range g.rows {
				moved[id] = id
			}
		} else {
			err = s.moveAcross(g, dest, collectionID, parentKey, beforeKey, moved)
		}
		if err != nil {
			return moved, err
		}
	}
	return moved, nil
}

// placement resolves an item row of the target collection to its id in the file; nil or "" is the root.
func (s *Store) placement(id *string, collectionID string) (string, error) {
	if id == nil || *id == "" {
		return "", nil
	}
	_, key, owner, err := s.locate(*id)
	if err != nil {
		return "", err
	}
	if owner != collectionID {
		return "", fmt.Errorf("%w: item %s is not in collection %s", ErrInvalidMove, *id, collectionID)
	}
	return key, nil
}

func (s *Store) moveWithin(g *moveGroup, file, parentKey, beforeKey string) error {
	return s.editCollection(file, func(src []byte) ([]byte, error) {
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		cuts, err := topmost(root, g.keys)
		if err != nil {
			return nil, err
		}
		seq, err := targetSeq(root, parentKey)
		if err != nil {
			return nil, err
		}
		nodes := make([]*yaml.Node, len(cuts))
		for i, c := range cuts {
			if parentKey != "" && (c.key == parentKey || findItem(itemsSeq(c.node), parentKey) != nil) {
				return nil, ErrInvalidMove
			}
			nodes[i] = c.node
		}
		for _, c := range cuts {
			c.remove()
		}
		if err := insertAt(seq, beforeKey, nodes); err != nil {
			return nil, err
		}
		return encodeDoc(doc)
	})
}

type keyPair struct{ from, to string }

func (s *Store) moveAcross(g *moveGroup, dest, destCollection, parentKey, beforeKey string, moved map[string]string) error {
	srcText, _, err := s.collections.Read(g.file)
	if err != nil {
		return err
	}
	_, srcRoot, err := parseDoc(srcText)
	if err != nil {
		return err
	}
	cuts, err := topmost(srcRoot, g.keys)
	if err != nil {
		return err
	}
	var pairs []keyPair
	var tops []string
	err = s.editCollection(dest, func(src []byte) ([]byte, error) {
		pairs, tops = nil, nil
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		seq, err := targetSeq(root, parentKey)
		if err != nil {
			return nil, err
		}
		taken := map[string]bool{}
		collectKeys(rootItems(root), taken)
		nodes := make([]*yaml.Node, len(cuts))
		for i, c := range cuts {
			nodes[i] = cloneNode(c.node)
			rekey(nodes[i], taken, &pairs)
			tops = append(tops, itemKey(nodes[i]))
		}
		if err := insertAt(seq, beforeKey, nodes); err != nil {
			return nil, err
		}
		return encodeDoc(doc)
	})
	if err != nil {
		return err
	}
	rows := make([]keyPair, 0, len(pairs))
	for _, p := range pairs {
		from, err := s.rowID(`SELECT id FROM api_items WHERE collection_id = ? AND item_key = ?`, g.collection, p.from)
		if err != nil {
			return s.undoCopies(dest, tops, err)
		}
		to, err := s.rowID(`SELECT id FROM api_items WHERE collection_id = ? AND item_key = ?`, destCollection, p.to)
		if err != nil {
			return s.undoCopies(dest, tops, err)
		}
		rows = append(rows, keyPair{from, to})
	}
	if err := s.repointHistory(rows, false); err != nil {
		return s.undoCopies(dest, tops, err)
	}
	err = s.editCollection(g.file, func(src []byte) ([]byte, error) {
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		cuts, err := topmost(root, g.keys)
		if err != nil {
			return nil, err
		}
		for _, c := range cuts {
			c.remove()
		}
		return encodeDoc(doc)
	})
	if err != nil {
		if rerr := s.repointHistory(rows, true); rerr != nil {
			err = errors.Join(err, rerr)
		}
		return s.undoCopies(dest, tops, err)
	}
	for _, r := range rows {
		moved[r.from] = r.to
	}
	return nil
}

// undoCopies removes the copies a failed move put in the target, and answers with cause.
func (s *Store) undoCopies(dest string, tops []string, cause error) error {
	err := s.editCollection(dest, func(src []byte) ([]byte, error) {
		doc, root, err := parseDoc(src)
		if err != nil {
			return nil, err
		}
		cuts, err := topmost(root, tops)
		if err != nil {
			return nil, err
		}
		for _, c := range cuts {
			c.remove()
		}
		return encodeDoc(doc)
	})
	if err != nil {
		return errors.Join(cause, fmt.Errorf("apifiles: remove the copied items from %s: %w", dest, err))
	}
	return cause
}

// rekey gives a copied subtree ids free in the target file, recording each changed or kept pair.
func rekey(n *yaml.Node, taken map[string]bool, pairs *[]keyPair) {
	old := itemKey(n)
	id := freeName(old, func(c string) bool { return taken[c] })
	taken[id] = true
	setScalar(n, "id", id)
	*pairs = append(*pairs, keyPair{old, id})
	if seq := itemsSeq(n); seq != nil && seq.Kind == yaml.SequenceNode {
		for _, ch := range seq.Content {
			if ch.Kind == yaml.MappingNode {
				rekey(ch, taken, pairs)
			}
		}
	}
}

// repointHistory moves the response and call history of each row pair to the new row, or back.
func (s *Store) repointHistory(rows []keyPair, reverse bool) error {
	tx, err := s.o.DB.Begin()
	if err != nil {
		return fmt.Errorf("apifiles: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range rows {
		from, to := r.from, r.to
		if reverse {
			from, to = to, from
		}
		for _, table := range []string{"api_response_history", "grpc_call_history"} {
			if _, err := tx.Exec(`UPDATE `+table+` SET item_id = ? WHERE item_id = ?`, to, from); err != nil {
				return fmt.Errorf("apifiles: move history of %s: %w", from, err)
			}
		}
	}
	return tx.Commit()
}

// ---- environments ----

func (s *Store) environmentFile(id string) (string, error) {
	var stem sql.NullString
	err := s.o.DB.QueryRow(`SELECT stem FROM api_environments WHERE id = ?`, id).Scan(&stem)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !stem.Valid {
		return "", fmt.Errorf("%w: environment %s", ErrNotFound, id)
	}
	if err != nil {
		return "", fmt.Errorf("apifiles: locate environment %s: %w", id, err)
	}
	return stem.String + EnvironmentExt, nil
}

// DeleteEnvironment removes the environment's file; the active selection clears with its row.
func (s *Store) DeleteEnvironment(id string) error {
	file, err := s.environmentFile(id)
	if err != nil {
		return err
	}
	return s.deleteFiles([]string{file})
}

// DuplicateEnvironment copies the environment file under a free stem, named "<name> copy", and returns
// the new row id. Secret variables keep naming the same secrets.
func (s *Store) DuplicateEnvironment(id string) (string, error) {
	file, err := s.environmentFile(id)
	if err != nil {
		return "", err
	}
	if e, ok := s.environments.Get(file); !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, file)
	} else if e.Problem != nil {
		return "", &BrokenError{File: file, Line: e.Problem.Line}
	}
	src, _, err := s.environments.Read(file)
	if err != nil {
		return "", err
	}
	doc, root, err := parseDoc(src)
	if err != nil {
		return "", err
	}
	name := ""
	if i := mapIndex(root, "name"); i >= 0 {
		name = root.Content[i].Value
	}
	name += copySuffix
	setScalar(root, "name", name)
	text, err := encodeDoc(doc)
	if err != nil {
		return "", err
	}
	stem, err := createFile(s, s.environments, EnvironmentExt, Slug(name, "environment"), func(string) ([]byte, error) { return text, nil })
	if err != nil {
		return "", err
	}
	return s.rowID(`SELECT id FROM api_environments WHERE stem = ?`, stem)
}
