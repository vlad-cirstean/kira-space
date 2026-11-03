package scripts

import (
	"errors"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Service holds every method body once. Each app's bridge.CustomScriptsService is a wrapper struct
// with forwarding methods, so Wails builds binding names from the app's own package. Every change
// reaches the panel through the Loader's snapshot broadcast.
type Service struct {
	Loader *Loader
}

func fail(err error) error {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		return ie
	}
	switch {
	case errors.Is(err, configfs.ErrConflict):
		return ipcerr.T(ipcerr.CodeConflict, "backend.core.runs.fileChanged.body", nil)
	case errors.Is(err, configfs.ErrInvalid):
		return ipcerr.BadRequest(err.Error())
	case IsCallerError(err):
		return ipcerr.BadRequest(err.Error())
	}
	return ipcerr.InternalErr(err)
}

func (s *Service) List() (Snapshot, error) {
	return ipcerr.InternalResult(s.Loader.Snapshot())
}

type CreateArgs struct {
	Fields CustomScriptFields `json:"fields"`
	// SecretValues saves a secret param's value by name; absent saves nothing.
	SecretValues map[string]*string `json:"secretValues"`
}

// Create writes a new run file from fields.
func (s *Service) Create(args CreateArgs) (CustomScript, error) {
	rec, err := s.Loader.Create(args.Fields, args.SecretValues)
	if err != nil {
		return CustomScript{}, fail(err)
	}
	return rec, nil
}

type DeleteArgs struct {
	ID string `json:"id"`
}

func (s *Service) Delete(args DeleteArgs) error {
	if args.ID == "" {
		return ipcerr.Required("id")
	}
	if err := s.Loader.Delete(args.ID); err != nil {
		return fail(err)
	}
	return nil
}

type FileArgs struct {
	File string `json:"file"`
}

// File returns one run file's text and state.
func (s *Service) File(args FileArgs) (FileView, error) {
	if args.File == "" {
		return FileView{}, ipcerr.Required("file")
	}
	v, err := s.Loader.View(args.File)
	if err != nil {
		if IsCallerError(err) {
			return FileView{}, ipcerr.NotFound(err.Error())
		}
		return FileView{}, fail(err)
	}
	return v, nil
}

type SaveFileArgs struct {
	File string `json:"file"`
	Text string `json:"text"`
	// BaseHash is the hash of the text the editor loaded.
	BaseHash string `json:"baseHash"`
}

// SaveFile writes the whole text; a file changed since BaseHash is a conflict.
func (s *Service) SaveFile(args SaveFileArgs) (FileView, error) {
	if args.File == "" {
		return FileView{}, ipcerr.Required("file")
	}
	v, err := s.Loader.SaveFile(args.File, args.Text, args.BaseHash)
	if err != nil {
		if IsCallerError(err) {
			return FileView{}, ipcerr.NotFound(err.Error())
		}
		return FileView{}, fail(err)
	}
	return v, nil
}

type SetSecretsArgs struct {
	ID string `json:"id"`
	// Values: a string saves, null clears, an absent name keeps the saved value.
	Values map[string]*string `json:"values"`
}

// SetSecrets saves or clears the values of a run's secret params.
func (s *Service) SetSecrets(args SetSecretsArgs) (CustomScript, error) {
	if args.ID == "" {
		return CustomScript{}, ipcerr.Required("id")
	}
	if err := s.Loader.o.Repo.SetSecrets(args.ID, args.Values); err != nil {
		return CustomScript{}, fail(err)
	}
	s.Loader.Broadcast()
	return ipcerr.InternalResult(s.Loader.get(args.ID))
}

type SetScheduleTargetArgs struct {
	ID       string `json:"id"`
	TaskID   string `json:"taskId"`
	BranchID string `json:"branchId"`
}

// SetScheduleTarget sets the ADE task and branch a schedule runs in (Kira Space).
func (s *Service) SetScheduleTarget(args SetScheduleTargetArgs) (CustomScript, error) {
	if args.ID == "" {
		return CustomScript{}, ipcerr.Required("id")
	}
	if err := s.Loader.o.Repo.SetScheduleTarget(args.ID, args.TaskID, args.BranchID); err != nil {
		return CustomScript{}, fail(err)
	}
	s.Loader.Broadcast()
	return ipcerr.InternalResult(s.Loader.get(args.ID))
}

type ReorderArgs struct {
	// Collection is the group, "" for the ungrouped runs.
	Collection string   `json:"collection"`
	IDs        []string `json:"ids"`
}

// Reorder persists a drag order within one set: ungrouped ("") or one collection.
func (s *Service) Reorder(args ReorderArgs) error {
	if err := checkOrderIDs(args.IDs); err != nil {
		return err
	}
	if err := s.Loader.o.Repo.Reorder(args.Collection, args.IDs); err != nil {
		return fail(err)
	}
	s.Loader.Broadcast()
	return nil
}

type ReorderCollectionsArgs struct {
	Names []string `json:"names"`
}

func (s *Service) ReorderCollections(args ReorderCollectionsArgs) error {
	if err := checkOrderIDs(args.Names); err != nil {
		return err
	}
	if err := s.Loader.o.Repo.ReorderCollections(args.Names); err != nil {
		return fail(err)
	}
	s.Loader.Broadcast()
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
