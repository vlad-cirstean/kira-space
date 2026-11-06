package bridge

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/agentapi"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiotmp/appcore"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// maxApiSecretBytes bounds one saved secret value.
const maxApiSecretBytes = 64 << 10

// ApiConfigService is the Api module's config files surface: each file's status and the secret
// names (never the values), plus write-only secret edits. Mirrors the agent settings page.
type ApiConfigService struct {
	Deps  appcore.Deps
	Files *apifiles.Store
	Agent *agentapi.Service
}

// ApiSecretArgs saves one secret. The value is write-only: no method returns it.
type ApiSecretArgs struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ApiSecretNameArgs names the secret to remove.
type ApiSecretNameArgs struct {
	Name string `json:"name"`
}

// View answers every file's state and the secret names.
func (s *ApiConfigService) View() (apifiles.View, error) {
	v, err := s.Files.View()
	if err != nil {
		return apifiles.View{}, ipcerr.InternalErr(err)
	}
	return v, nil
}

// AgentAccess lists the environments that share variables with agents: names and allowed hosts, never
// a value. It is the same data the API request tools list.
func (s *ApiConfigService) AgentAccess() ([]agentapi.EnvView, error) {
	envs, err := s.Agent.Environments()
	if err != nil {
		return nil, ipcerr.InternalErr(err)
	}
	return envs, nil
}

// SetSecret seals one secret value under name. A secret store that cannot seal keeps its E_SECRET_STORE code.
func (s *ApiConfigService) SetSecret(args ApiSecretArgs) (apifiles.View, error) {
	if !apifiles.ValidSecretName(args.Name) {
		return apifiles.View{}, ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretName.body", nil))
	}
	if len(args.Value) == 0 || len(args.Value) > maxApiSecretBytes || strings.ContainsRune(args.Value, 0) {
		return apifiles.View{}, ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretValue.body", nil))
	}
	if err := s.Files.PutSecret(args.Name, args.Value); err != nil {
		return apifiles.View{}, ipcerr.Wrap(err)
	}
	emitApiData(s.Deps.Events, configChange())
	return s.View()
}

// DeleteSecret removes one secret; an absent name is not an error.
func (s *ApiConfigService) DeleteSecret(args ApiSecretNameArgs) (apifiles.View, error) {
	if err := s.Files.DeleteSecret(args.Name); err != nil {
		return apifiles.View{}, ipcerr.Wrap(err)
	}
	emitApiData(s.Deps.Events, configChange())
	return s.View()
}

// ApiFileArgs names one collection or environment file.
type ApiFileArgs struct {
	File string `json:"file"`
}

// ApiSaveFileArgs saves the whole text of a file built on the text with BaseHash.
type ApiSaveFileArgs struct {
	File     string `json:"file"`
	Text     string `json:"text"`
	BaseHash string `json:"baseHash"`
}

// ApiFormArgs is one form call: the draft text and the edits to apply to it.
type ApiFormArgs struct {
	File  string          `json:"file"`
	Text  string          `json:"text"`
	Edits []configfs.Edit `json:"edits"`
}

// File answers one file's text and state.
func (s *ApiConfigService) File(args ApiFileArgs) (apifiles.FileDetail, error) {
	if args.File == "" {
		return apifiles.FileDetail{}, ipcerr.Required("file")
	}
	d, err := s.Files.FileText(args.File)
	if err != nil {
		return apifiles.FileDetail{}, apiFilesErr(err)
	}
	return d, nil
}

// SaveFile writes the text. Invalid text is saved too and comes back with its problem; the last good
// content keeps serving.
func (s *ApiConfigService) SaveFile(args ApiSaveFileArgs) (apifiles.FileDetail, error) {
	if args.File == "" {
		return apifiles.FileDetail{}, ipcerr.Required("file")
	}
	if args.BaseHash == "" {
		return apifiles.FileDetail{}, ipcerr.Required("baseHash")
	}
	d, err := s.Files.SaveFile(args.File, args.BaseHash, args.Text)
	if err != nil {
		return apifiles.FileDetail{}, apiFilesErr(err)
	}
	return d, nil
}

// Form applies the form's edits to the draft and decodes the result. Stateless: nothing is written;
// the dialog saves the returned text with SaveFile.
func (s *ApiConfigService) Form(args ApiFormArgs) (configfs.FormResult, error) {
	if args.File == "" {
		return configfs.FormResult{}, ipcerr.Required("file")
	}
	if len(args.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	res, err := s.Files.Form(args.File, args.Text, args.Edits)
	if err != nil {
		return configfs.FormResult{}, apiFilesErr(err)
	}
	if len(res.Text) > configfs.MaxFileBytes {
		return configfs.FormResult{}, ipcerr.BadRequest(configfs.ErrTooLarge.Error())
	}
	return res, nil
}
