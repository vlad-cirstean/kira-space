package bridge

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/apifiles"
	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/appcore"
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
