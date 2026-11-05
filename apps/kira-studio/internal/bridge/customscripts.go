package bridge

import (
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/scripts"
)

// CustomScriptsService is this app's binding-name shim over internal/scripts.Service (the
// store, validation and the list-changed broadcast live there once, shared with the other app).
type CustomScriptsService struct {
	Svc *scripts.Service
}

type (
	CustomScriptsCreateArgs             = scripts.CreateArgs
	CustomScriptsDeleteArgs             = scripts.DeleteArgs
	CustomScriptsFileArgs               = scripts.FileArgs
	CustomScriptsFormArgs               = scripts.FormArgs
	CustomScriptsSaveFileArgs           = scripts.SaveFileArgs
	CustomScriptsSetSecretsArgs         = scripts.SetSecretsArgs
	CustomScriptsReorderArgs            = scripts.ReorderArgs
	CustomScriptsReorderCollectionsArgs = scripts.ReorderCollectionsArgs
	CustomScriptsMoveArgs               = scripts.MoveArgs
	CustomScriptsRenameCollectionArgs   = scripts.RenameCollectionArgs
	CustomScriptsDeleteCollectionArgs   = scripts.DeleteCollectionArgs
)

func (s *CustomScriptsService) List() (scripts.Snapshot, error) {
	return s.Svc.List()
}

func (s *CustomScriptsService) Create(args CustomScriptsCreateArgs) (scripts.CustomScript, error) {
	return s.Svc.Create(args)
}

func (s *CustomScriptsService) Delete(args CustomScriptsDeleteArgs) error {
	return s.Svc.Delete(args)
}

func (s *CustomScriptsService) File(args CustomScriptsFileArgs) (scripts.FileView, error) {
	return s.Svc.File(args)
}

func (s *CustomScriptsService) SaveFile(args CustomScriptsSaveFileArgs) (scripts.FileView, error) {
	return s.Svc.SaveFile(args)
}

func (s *CustomScriptsService) SetSecrets(args CustomScriptsSetSecretsArgs) (scripts.CustomScript, error) {
	return s.Svc.SetSecrets(args)
}

func (s *CustomScriptsService) Reorder(args CustomScriptsReorderArgs) error {
	return s.Svc.Reorder(args)
}

func (s *CustomScriptsService) ReorderCollections(args CustomScriptsReorderCollectionsArgs) error {
	return s.Svc.ReorderCollections(args)
}

func (s *CustomScriptsService) Form(args CustomScriptsFormArgs) (configfs.FormResult, error) {
	return s.Svc.Form(args)
}

func (s *CustomScriptsService) Move(args CustomScriptsMoveArgs) error {
	return s.Svc.Move(args)
}

func (s *CustomScriptsService) RenameCollection(args CustomScriptsRenameCollectionArgs) error {
	return s.Svc.RenameCollection(args)
}

func (s *CustomScriptsService) DeleteCollection(args CustomScriptsDeleteCollectionArgs) error {
	return s.Svc.DeleteCollection(args)
}
