package bridge

import (
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// CustomScriptsService is this app's binding-name shim over internal/scripts.Service. It routes
// over both homes' stacks until the home fold: List is the union, a call that names a run or file
// goes to the stack that owns it, Create goes to Space.
type CustomScriptsService struct {
	Space  *scripts.Service
	Studio *scripts.Service

	mu        sync.Mutex
	last      [2]scripts.Snapshot
	published [2]bool
}

type (
	CustomScriptsCreateArgs             = scripts.CreateArgs
	CustomScriptsDeleteArgs             = scripts.DeleteArgs
	CustomScriptsFileArgs               = scripts.FileArgs
	CustomScriptsFormArgs               = scripts.FormArgs
	CustomScriptsSaveFileArgs           = scripts.SaveFileArgs
	CustomScriptsSetSecretsArgs         = scripts.SetSecretsArgs
	CustomScriptsSetScheduleTargetArgs  = scripts.SetScheduleTargetArgs
	CustomScriptsReorderArgs            = scripts.ReorderArgs
	CustomScriptsReorderCollectionsArgs = scripts.ReorderCollectionsArgs
	CustomScriptsMoveArgs               = scripts.MoveArgs
	CustomScriptsRenameCollectionArgs   = scripts.RenameCollectionArgs
	CustomScriptsDeleteCollectionArgs   = scripts.DeleteCollectionArgs
)

// owner picks the stack holding run id; Space when neither does, so the error is Space's.
func (s *CustomScriptsService) owner(id string) *scripts.Service {
	if !s.Space.Loader.Has(id) && s.Studio.Loader.Has(id) {
		return s.Studio
	}
	return s.Space
}

func (s *CustomScriptsService) ownerOfFile(file string) *scripts.Service {
	if _, err := os.Stat(filepath.Join(s.Space.Loader.Dir(), file)); err != nil {
		if _, err := os.Stat(filepath.Join(s.Studio.Loader.Dir(), file)); err == nil {
			return s.Studio
		}
	}
	return s.Space
}

// Publish stores one stack's latest snapshot (studio picks the stack) and emits the merged list on
// ChannelCustomScriptsChanged. Both loaders' Emit hooks call it.
//
//wails:ignore
func (s *CustomScriptsService) Publish(emit appevent.Emitter, studio bool, snap scripts.Snapshot) {
	i := 0
	if studio {
		i = 1
	}
	s.mu.Lock()
	s.last[i], s.published[i] = snap, true
	merged := mergeSnapshots(s.last[0], s.last[1], s.published[1])
	s.mu.Unlock()
	emit.Emit(ChannelCustomScriptsChanged, merged)
}

func mergeSnapshots(space, studio scripts.Snapshot, withStudio bool) scripts.Snapshot {
	if !withStudio {
		return space
	}
	out := space
	out.Collections = slices.Clone(space.Collections)
	for _, c := range studio.Collections {
		if !slices.ContainsFunc(out.Collections, func(o scripts.Collection) bool { return o.Name == c.Name }) {
			out.Collections = append(out.Collections, c)
		}
	}
	out.Scripts = append(slices.Clone(space.Scripts), studio.Scripts...)
	out.Problems = append(slices.Clone(space.Problems), studio.Problems...)
	return out
}

func (s *CustomScriptsService) List() (scripts.Snapshot, error) {
	space, err := s.Space.List()
	if err != nil {
		return scripts.Snapshot{}, err
	}
	studio, err := s.Studio.List()
	if err != nil {
		return scripts.Snapshot{}, err
	}
	return mergeSnapshots(space, studio, true), nil
}

func (s *CustomScriptsService) Create(args CustomScriptsCreateArgs) (scripts.CustomScript, error) {
	return s.Space.Create(args)
}

func (s *CustomScriptsService) Delete(args CustomScriptsDeleteArgs) error {
	return s.owner(args.ID).Delete(args)
}

func (s *CustomScriptsService) File(args CustomScriptsFileArgs) (scripts.FileView, error) {
	return s.ownerOfFile(args.File).File(args)
}

func (s *CustomScriptsService) SaveFile(args CustomScriptsSaveFileArgs) (scripts.FileView, error) {
	return s.ownerOfFile(args.File).SaveFile(args)
}

func (s *CustomScriptsService) SetSecrets(args CustomScriptsSetSecretsArgs) (scripts.CustomScript, error) {
	return s.owner(args.ID).SetSecrets(args)
}

func (s *CustomScriptsService) SetScheduleTarget(args CustomScriptsSetScheduleTargetArgs) (scripts.CustomScript, error) {
	return s.owner(args.ID).SetScheduleTarget(args)
}

// Reorder splits the ids by owner and persists each stack's share.
func (s *CustomScriptsService) Reorder(args CustomScriptsReorderArgs) error {
	var space, studio []string
	for _, id := range args.IDs {
		if s.owner(id) == s.Studio {
			studio = append(studio, id)
		} else {
			space = append(space, id)
		}
	}
	if len(studio) == 0 || len(space) == 0 {
		return s.owner(firstOr(args.IDs)).Reorder(args)
	}
	if err := s.Space.Reorder(scripts.ReorderArgs{Collection: args.Collection, IDs: space}); err != nil {
		return err
	}
	return s.Studio.Reorder(scripts.ReorderArgs{Collection: args.Collection, IDs: studio})
}

func firstOr(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// ReorderCollections persists each stack's own collections in the given order.
func (s *CustomScriptsService) ReorderCollections(args CustomScriptsReorderCollectionsArgs) error {
	if len(args.Names) == 0 {
		return s.Space.ReorderCollections(args)
	}
	var err error
	for _, svc := range []*scripts.Service{s.Space, s.Studio} {
		snap, e := svc.List()
		if e != nil {
			return e
		}
		var names []string
		for _, n := range args.Names {
			if slices.ContainsFunc(snap.Collections, func(c scripts.Collection) bool { return c.Name == n }) {
				names = append(names, n)
			}
		}
		if len(names) == 0 {
			continue
		}
		if e := svc.ReorderCollections(scripts.ReorderCollectionsArgs{Names: names}); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (s *CustomScriptsService) Form(args CustomScriptsFormArgs) (configfs.FormResult, error) {
	return s.ownerOfFile(args.File).Form(args)
}

func (s *CustomScriptsService) Move(args CustomScriptsMoveArgs) error {
	svc := s.owner(args.ID)
	if args.BeforeID != nil && s.owner(*args.BeforeID) != svc {
		args.BeforeID = nil
	}
	return svc.Move(args)
}

// RenameCollection renames the collection in every stack that has it.
func (s *CustomScriptsService) RenameCollection(args CustomScriptsRenameCollectionArgs) error {
	return s.eachWithCollection(args.Name, func(svc *scripts.Service) error { return svc.RenameCollection(args) }, func() error { return s.Space.RenameCollection(args) })
}

// DeleteCollection deletes the collection in every stack that has it.
func (s *CustomScriptsService) DeleteCollection(args CustomScriptsDeleteCollectionArgs) error {
	return s.eachWithCollection(args.Name, func(svc *scripts.Service) error { return svc.DeleteCollection(args) }, func() error { return s.Space.DeleteCollection(args) })
}

// eachWithCollection runs fn on each stack that lists the collection; none run absent falls to
// Space so the error is Space's.
func (s *CustomScriptsService) eachWithCollection(name string, fn func(*scripts.Service) error, absent func() error) error {
	ran := false
	for _, svc := range []*scripts.Service{s.Space, s.Studio} {
		snap, err := svc.List()
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(snap.Collections, func(c scripts.Collection) bool { return c.Name == name }) {
			continue
		}
		ran = true
		if err := fn(svc); err != nil {
			return err
		}
	}
	if !ran {
		return absent()
	}
	return nil
}
