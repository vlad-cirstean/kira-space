package scripts

import (
	"errors"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Service holds every method body once. Each app's bridge.CustomScriptsService is a wrapper struct
// with forwarding methods, so Wails builds binding names from the app's own package. Every mutation broadcasts the full snapshot
// through Emit so a second window's Scripts list stays live.
type Service struct {
	Repo *Repo
	Emit func(snapshot Snapshot)
	// Home is the app data folder; removing a script deletes its automations folder under it.
	Home string
	// OnChange runs after every mutation's broadcast (the scheduler reloads).
	OnChange func()
}

func fail(err error) error {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie
	}
	if IsCallerError(err) {
		return ipcerr.BadRequest(err.Error())
	}
	return ipcerr.InternalErr(err)
}

func (s *Service) snapshot() (Snapshot, error) {
	collections, err := s.Repo.ListCollections()
	if err != nil {
		return Snapshot{}, err
	}
	scripts, err := s.Repo.List()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Collections: collections, Scripts: scripts, SecretStorage: s.Repo.secretStatus()}, nil
}

func (s *Service) List() (Snapshot, error) {
	return ipcerr.InternalResult(s.snapshot())
}

func (s *Service) broadcast() {
	snap, err := s.snapshot()
	if err != nil {
		return
	}
	s.Emit(snap)
	if s.OnChange != nil {
		s.OnChange()
	}
}

type CreateArgs struct {
	Fields CustomScriptFields `json:"fields"`
	// SecretValues saves a secret param's value by name; absent saves nothing.
	SecretValues map[string]*string `json:"secretValues"`
}

func (s *Service) Create(args CreateArgs) (CustomScript, error) {
	rec, err := s.Repo.Create(args.Fields, args.SecretValues)
	if err != nil {
		return CustomScript{}, fail(err)
	}
	s.broadcast()
	return rec, nil
}

type UpdateArgs struct {
	ID     string             `json:"id"`
	Fields CustomScriptFields `json:"fields"`
	// SecretValues: a string saves, null clears, an absent name keeps the saved value.
	SecretValues map[string]*string `json:"secretValues"`
}

func (s *Service) Update(args UpdateArgs) (CustomScript, error) {
	if args.ID == "" {
		return CustomScript{}, ipcerr.Required("id")
	}
	rec, err := s.Repo.Update(args.ID, args.Fields, args.SecretValues)
	if err != nil {
		return CustomScript{}, fail(err)
	}
	s.broadcast()
	return rec, nil
}

type DeleteArgs struct {
	ID string `json:"id"`
}

func (s *Service) Delete(args DeleteArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Repo.Delete(args.ID); err != nil {
		return fail(err)
	}
	if s.Home != "" {
		RemoveDir(s.Home, args.ID)
	}
	s.broadcast()
	return nil
}

type CreateCollectionArgs struct {
	Name string `json:"name"`
}

func (s *Service) CreateCollection(args CreateCollectionArgs) (Collection, error) {
	c, err := s.Repo.CreateCollection(args.Name)
	if err != nil {
		return Collection{}, fail(err)
	}
	s.broadcast()
	return c, nil
}

type RenameCollectionArgs struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Service) RenameCollection(args RenameCollectionArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Repo.RenameCollection(args.ID, args.Name); err != nil {
		return fail(err)
	}
	s.broadcast()
	return nil
}

type DeleteCollectionArgs struct {
	ID string `json:"id"`
}

func (s *Service) DeleteCollection(args DeleteCollectionArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Repo.DeleteCollection(args.ID); err != nil {
		return fail(err)
	}
	s.broadcast()
	return nil
}

type MoveArgs struct {
	ID           string  `json:"id"`
	CollectionID *string `json:"collectionId"`
	// BeforeID places the script before that sibling of the target set; null appends.
	BeforeID *string `json:"beforeId"`
}

func (s *Service) Move(args MoveArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Repo.Move(args.ID, args.CollectionID, args.BeforeID); err != nil {
		return fail(err)
	}
	s.broadcast()
	return nil
}

type ReorderArgs struct {
	CollectionID *string  `json:"collectionId"`
	IDs          []string `json:"ids"`
}

// Reorder persists a drag order within one set: ungrouped (nil collectionId) or one collection.
func (s *Service) Reorder(args ReorderArgs) error {
	if err := checkOrderIDs(args.IDs); err != nil {
		return err
	}
	if err := s.Repo.Reorder(args.CollectionID, args.IDs); err != nil {
		return fail(err)
	}
	s.broadcast()
	return nil
}

type ReorderCollectionsArgs struct {
	IDs []string `json:"ids"`
}

func (s *Service) ReorderCollections(args ReorderCollectionsArgs) error {
	if err := checkOrderIDs(args.IDs); err != nil {
		return err
	}
	if err := s.Repo.ReorderCollections(args.IDs); err != nil {
		return fail(err)
	}
	s.broadcast()
	return nil
}

func checkOrderIDs(ids []string) error {
	if len(ids) == 0 {
		return ipcerr.Required("ids")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return ipcerr.Contract("ids must be unique")
		}
		seen[id] = true
	}
	return nil
}
