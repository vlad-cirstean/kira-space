package scripts

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Collections are the `collection` name in run files; an order row holds the drag order and is
// pruned when no run names it. Every action here is a file edit followed by a reload.

// setCollection edits the `collection` key of one run file ("" removes it).
func (l *Loader) setCollection(id, name string) error {
	file := FileName(id)
	e, ok := l.folder.Get(file)
	if !ok {
		return fmt.Errorf("scripts: file %s: %w", file, sql.ErrNoRows)
	}
	if e.Problem != nil {
		return fmt.Errorf("scripts: %s has a problem (line %d: %s); fix the file first: %w",
			file, e.Problem.Line, e.Problem.Message, configfs.ErrInvalid)
	}
	src, hash, err := l.folder.Read(file)
	if err != nil {
		return err
	}
	set := configfs.NodeSet{Path: []string{"collection"}, Value: name}
	if name == "" {
		set.Unset = true
	}
	out, err := configfs.EditYAML(src, []configfs.NodeSet{set})
	if err != nil {
		return fmt.Errorf("scripts: edit %s: %w", file, err)
	}
	_, err = l.folder.Save(file, hash, out)
	return err
}

// members lists the ids of the runs in a collection, in panel order.
func (l *Loader) members(collection string) ([]string, error) {
	list, err := l.o.Repo.List()
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, s := range list {
		if s.Collection == collection {
			ids = append(ids, s.ID)
		}
	}
	return ids, nil
}

func checkCollectionName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ipcerr.Required("collection")
	}
	if utf8.RuneCountInString(name) > MaxCollectionRunes {
		return "", ipcerr.Contract("collection name is longer than %d characters", MaxCollectionRunes)
	}
	return name, nil
}

type MoveArgs struct {
	ID string `json:"id"`
	// Collection is the target set, "" for ungrouped. A name no run uses yet creates the collection.
	Collection string `json:"collection"`
	// BeforeID places the run before this member of the target set; nil appends.
	BeforeID *string `json:"beforeId"`
}

// Move puts a run into a collection (or out of every one) and places it in that set's order.
func (s *Service) Move(args MoveArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	target := strings.TrimSpace(args.Collection)
	if target != "" {
		var err error
		if target, err = checkCollectionName(target); err != nil {
			return err
		}
	}
	l := s.Loader
	rec, err := l.o.Repo.Get(args.ID)
	if err != nil {
		return fail(err)
	}
	if rec == nil {
		return ipcerr.NotFound("run " + args.ID + " does not exist")
	}
	if args.BeforeID != nil && *args.BeforeID == args.ID {
		return ipcerr.Contract("beforeId must differ from id")
	}
	if rec.Collection != target {
		if err := l.setCollection(args.ID, target); err != nil {
			return fail(err)
		}
		l.Reload()
	}
	ids, err := l.members(target)
	if err != nil {
		return fail(err)
	}
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == args.ID })
	at := len(ids)
	if args.BeforeID != nil {
		i := slices.Index(ids, *args.BeforeID)
		if i < 0 {
			return ipcerr.NotFound("run " + *args.BeforeID + " is not in the target set")
		}
		at = i
	}
	ids = slices.Insert(ids, at, args.ID)
	if err := l.o.Repo.Reorder(target, ids); err != nil {
		return fail(err)
	}
	l.Broadcast()
	return nil
}

type RenameCollectionArgs struct {
	Name    string `json:"name"`
	NewName string `json:"newName"`
}

// RenameCollection renames a collection in every member file. The collection keeps its position.
// Nothing is written when a member file is broken or the new name is taken.
func (s *Service) RenameCollection(args RenameCollectionArgs) error {
	newName, err := checkCollectionName(args.NewName)
	if err != nil {
		return err
	}
	if args.Name == "" {
		return ipcerr.Required("name")
	}
	if newName == args.Name {
		return nil
	}
	l := s.Loader
	ids, err := l.members(args.Name)
	if err != nil {
		return fail(err)
	}
	if len(ids) == 0 {
		return ipcerr.NotFound("collection " + args.Name + " does not exist")
	}
	taken, err := l.members(newName)
	if err != nil {
		return fail(err)
	}
	if len(taken) > 0 {
		return ipcerr.BadRequest("collection " + newName + " already exists")
	}
	for _, id := range ids {
		if e, ok := l.folder.Get(FileName(id)); !ok || e.Problem != nil {
			return ipcerr.BadRequest(FileName(id) + " has a problem; fix the file before renaming the collection")
		}
	}
	// Held so a watcher reload cannot prune the renamed order row before a member names it.
	l.mu.Lock()
	err = s.renameLocked(args.Name, newName, ids)
	l.mu.Unlock()
	l.Reload()
	if err != nil {
		return fail(err)
	}
	return nil
}

func (s *Service) renameLocked(name, newName string, ids []string) error {
	l := s.Loader
	if _, err := l.o.Repo.DB.Exec(`UPDATE script_collections SET name = ? WHERE name = ?`, newName, name); err != nil {
		return fmt.Errorf("scripts: rename collection row: %w", err)
	}
	var done []string
	for _, id := range ids {
		if err := l.setCollection(id, newName); err != nil {
			return fmt.Errorf("scripts: rename stopped at %s (renamed: %s): %w", FileName(id), strings.Join(done, ", "), err)
		}
		done = append(done, FileName(id))
	}
	return nil
}

type DeleteCollectionArgs struct {
	Name string `json:"name"`
}

// DeleteCollection deletes every run in the collection: file, row, saved secrets and automations
// folder. Broken member files go too.
func (s *Service) DeleteCollection(args DeleteCollectionArgs) error {
	if args.Name == "" {
		return ipcerr.Required("name")
	}
	ids, err := s.Loader.members(args.Name)
	if err != nil {
		return fail(err)
	}
	if len(ids) == 0 {
		return ipcerr.NotFound("collection " + args.Name + " does not exist")
	}
	var done []string
	for _, id := range ids {
		if err := s.Loader.Delete(id); err != nil {
			return fail(fmt.Errorf("scripts: delete collection stopped at %s (deleted: %s): %w", FileName(id), strings.Join(done, ", "), err))
		}
		done = append(done, FileName(id))
	}
	return nil
}
