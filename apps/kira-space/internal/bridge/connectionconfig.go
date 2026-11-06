package bridge

import (
	"errors"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/appcore"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/maskrules"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// maxConnectionSecretBytes bounds one saved secret value.
const maxConnectionSecretBytes = 64 << 10

// ConnectionConfigService is the connections config files surface: each file's status and text, the
// secret names (never the values), and write-only secret edits.
type ConnectionConfigService struct {
	Deps appcore.StudioDeps
	// Files is the connections folder.
	Files *connfiles.Loader
}

// ConnectionFileArgs names one file.
type ConnectionFileArgs struct {
	File string `json:"file"`
}

// ConnectionSaveFileArgs saves the text of one file. BaseHash is the hash of the text the edit started from.
type ConnectionSaveFileArgs struct {
	File     string `json:"file"`
	Text     string `json:"text"`
	BaseHash string `json:"baseHash"`
}

// ConnectionSecretArgs saves one secret. The value is write-only: no method returns it.
type ConnectionSecretArgs struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ConnectionSecretNameArgs names the secret to remove.
type ConnectionSecretNameArgs struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// View answers every file's state and the secret names.
func (s *ConnectionConfigService) View() (connfiles.View, error) {
	v, err := s.Files.View()
	if err != nil {
		return connfiles.View{}, ipcerr.InternalErr(err)
	}
	return v, nil
}

// File answers one file's text and state.
func (s *ConnectionConfigService) File(args ConnectionFileArgs) (connfiles.FileDetail, error) {
	if args.File == "" {
		return connfiles.FileDetail{}, ipcerr.Required("file")
	}
	d, err := s.Files.File(args.File)
	if err != nil {
		return connfiles.FileDetail{}, connfiles.IPCError(err)
	}
	return d, nil
}

// SaveFile writes the text. Invalid text is saved too and comes back with its problem; the last good
// content keeps serving.
func (s *ConnectionConfigService) SaveFile(args ConnectionSaveFileArgs) (connfiles.FileDetail, error) {
	if args.File == "" {
		return connfiles.FileDetail{}, ipcerr.Required("file")
	}
	if args.BaseHash == "" {
		return connfiles.FileDetail{}, ipcerr.Required("baseHash")
	}
	d, err := s.Files.SaveFile(args.File, args.BaseHash, args.Text)
	if err != nil {
		return connfiles.FileDetail{}, connfiles.IPCError(err)
	}
	return d, nil
}

// DeleteFile removes a file that never held a valid connection. A file with a connection is refused;
// Connections.Delete removes that one.
func (s *ConnectionConfigService) DeleteFile(args ConnectionFileArgs) error {
	if args.File == "" {
		return ipcerr.Required("file")
	}
	if err := s.Files.DeleteFile(args.File); err != nil {
		if errors.Is(err, connfiles.ErrExists) {
			return ipcerr.BadRequest(err.Error())
		}
		return connfiles.IPCError(err)
	}
	s.Deps.Events.Emit(ChannelConnectionConfigChanged, struct{}{})
	return nil
}

// ConnectionFormArgs is one form call: the draft text and the edits to apply to it.
type ConnectionFormArgs struct {
	File  string          `json:"file"`
	Text  string          `json:"text"`
	Edits []configfs.Edit `json:"edits"`
}

// Form applies the form's edits to the draft and decodes the result. Stateless: nothing is
// written; the dialog saves the returned text with SaveFile.
func (s *ConnectionConfigService) Form(args ConnectionFormArgs) (configfs.FormResult, error) {
	if args.File == "" {
		return configfs.FormResult{}, ipcerr.Required("file")
	}
	if len(args.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	res := s.Files.Form(args.File, args.Text, args.Edits)
	if len(res.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	return res, nil
}

func validSecret(kind, name string) error {
	if !slices.Contains(connfiles.Kinds, kind) {
		return ipcerr.Contract("invalid secret kind")
	}
	if !connfiles.ValidSecretName(name) {
		return ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretName.body", nil))
	}
	return nil
}

// SetSecret seals one secret value under name. A secret store that cannot seal keeps its E_SECRET_STORE code.
func (s *ConnectionConfigService) SetSecret(args ConnectionSecretArgs) (connfiles.View, error) {
	if err := validSecret(args.Kind, args.Name); err != nil {
		return connfiles.View{}, err
	}
	if len(args.Value) == 0 || len(args.Value) > maxConnectionSecretBytes || strings.ContainsRune(args.Value, 0) {
		return connfiles.View{}, ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretValue.body", nil))
	}
	if err := s.Files.PutSecret(args.Kind, args.Name, args.Value); err != nil {
		return connfiles.View{}, ipcerr.Wrap(err)
	}
	s.Deps.Events.Emit(ChannelConnectionConfigChanged, struct{}{})
	return s.View()
}

// DeleteSecret removes one secret; an absent name is not an error.
func (s *ConnectionConfigService) DeleteSecret(args ConnectionSecretNameArgs) (connfiles.View, error) {
	if err := validSecret(args.Kind, args.Name); err != nil {
		return connfiles.View{}, err
	}
	if err := s.Files.DeleteSecret(args.Kind, args.Name); err != nil {
		return connfiles.View{}, ipcerr.Wrap(err)
	}
	s.Deps.Events.Emit(ChannelConnectionConfigChanged, struct{}{})
	return s.View()
}

// ConnectionFiltersChangedEvent is ChannelConnectionFiltersChanged's payload: a connection's left
// panel visibility after its file changed it.
type ConnectionFiltersChangedEvent struct {
	ConnectionID string               `json:"connectionId"`
	Visibility   model.TreeVisibility `json:"visibility"`
}

// EmitConnectionChanges pushes what a connection file load or write made stale. The connection list
// goes out through connections.Service.ConfigChanged. Rule rows and the DB are already projected when
// this runs.
func EmitConnectionChanges(e appcore.Emitter, masks *maskrules.Service, review *maskreview.Service, ch connfiles.Changes) {
	for _, c := range ch.Items {
		if c.New == nil {
			continue
		}
		if c.Masks() {
			masks.Invalidate(c.ID)
			if rules, err := masks.List(c.ID); err == nil {
				e.Emit(ChannelMaskRulesChanged, MaskRulesChangedEvent{ConnectionID: c.ID, Rules: rules})
			}
		}
		if review != nil {
			review.FileChanged(c.ID)
		}
		if c.DDL() {
			e.Emit(ChannelSchemaChanged, model.ConnectionDDL{ConnectionID: c.ID, DDL: c.New.SchemaDDL})
		}
		if c.Tree() {
			e.Emit(ChannelConnectionFiltersChanged, ConnectionFiltersChangedEvent{ConnectionID: c.ID, Visibility: c.New.Visibility()})
		}
	}
	if !ch.Empty() {
		e.Emit(ChannelConnectionConfigChanged, struct{}{})
	}
}
