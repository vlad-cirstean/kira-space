package bridge

import (
	"github.com/kirathecat/kira-studio/internal/appevent"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// CustomScriptsService is this app's binding-name shim over internal/scripts.Service.
type CustomScriptsService struct {
	Scripts *scripts.Service
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

// Publish emits the latest snapshot on ChannelCustomScriptsChanged. The loader's Emit hook calls it.
//
//wails:ignore
func (s *CustomScriptsService) Publish(emit appevent.Emitter, snap scripts.Snapshot) {
	emit.Emit(ChannelCustomScriptsChanged, snap)
}

func (s *CustomScriptsService) List() (scripts.Snapshot, error) { return s.Scripts.List() }

func (s *CustomScriptsService) Create(args CustomScriptsCreateArgs) (scripts.CustomScript, error) {
	return s.Scripts.Create(args)
}

func (s *CustomScriptsService) Delete(args CustomScriptsDeleteArgs) error {
	return s.Scripts.Delete(args)
}

func (s *CustomScriptsService) File(args CustomScriptsFileArgs) (scripts.FileView, error) {
	return s.Scripts.File(args)
}

func (s *CustomScriptsService) SaveFile(args CustomScriptsSaveFileArgs) (scripts.FileView, error) {
	return s.Scripts.SaveFile(args)
}

func (s *CustomScriptsService) SetSecrets(args CustomScriptsSetSecretsArgs) (scripts.CustomScript, error) {
	return s.Scripts.SetSecrets(args)
}

func (s *CustomScriptsService) SetScheduleTarget(args CustomScriptsSetScheduleTargetArgs) (scripts.CustomScript, error) {
	return s.Scripts.SetScheduleTarget(args)
}

func (s *CustomScriptsService) Reorder(args CustomScriptsReorderArgs) error {
	return s.Scripts.Reorder(args)
}

func (s *CustomScriptsService) ReorderCollections(args CustomScriptsReorderCollectionsArgs) error {
	return s.Scripts.ReorderCollections(args)
}

func (s *CustomScriptsService) Form(args CustomScriptsFormArgs) (configfs.FormResult, error) {
	return s.Scripts.Form(args)
}

func (s *CustomScriptsService) Move(args CustomScriptsMoveArgs) error { return s.Scripts.Move(args) }

func (s *CustomScriptsService) RenameCollection(args CustomScriptsRenameCollectionArgs) error {
	return s.Scripts.RenameCollection(args)
}

func (s *CustomScriptsService) DeleteCollection(args CustomScriptsDeleteCollectionArgs) error {
	return s.Scripts.DeleteCollection(args)
}
