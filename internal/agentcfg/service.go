package agentcfg

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/internal/secrets"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// maxSecretBytes bounds one saved credential value.
const maxSecretBytes = 64 << 10

var secretNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// Service is the bound surface of the settings editor. The app's own bridge type embeds it so
// binding names stay app-local.
type Service struct {
	Store *Store
}

// SaveArgs is one save: the whole text and the hash of the text the editor loaded.
type SaveArgs struct {
	Text     string `json:"text"`
	BaseHash string `json:"baseHash"`
}

// SetSecretArgs saves one credential. The value is write-only: no method returns it.
type SetSecretArgs struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DeleteSecretArgs names the credential to remove.
type DeleteSecretArgs struct {
	Name string `json:"name"`
}

// View is everything the Settings page shows.
type View struct {
	FileView
	Storage   secrets.Status `json:"storage"`
	Secrets   []SecretView   `json:"secrets"`
	Providers []ProviderView `json:"providers"`
}

// SecretView is one credential name: saved, referenced by the file, or both. Never the value.
type SecretView struct {
	Name       string `json:"name"`
	Set        bool   `json:"set"`
	Referenced bool   `json:"referenced"`
}

// ProviderView is the resulting config of one provider, references shown as written.
type ProviderView struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	Files []FileText `json:"files"`
	Env   []EnvText  `json:"env"`
	Notes []string   `json:"notes"`
	Kira  []string   `json:"kira"`
}

// FileText is one rendered provider file.
type FileText struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// EnvText is one launch variable.
type EnvText struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Get returns the page's state.
func (s *Service) Get() (View, error) { return s.view() }

// Save writes the file. A file changed since BaseHash is a conflict error.
func (s *Service) Save(args SaveArgs) (View, error) {
	if s.Store == nil {
		return s.view()
	}
	if _, err := s.Store.Save(args.Text, args.BaseHash); err != nil {
		switch {
		case errors.Is(err, configfs.ErrConflict):
			return View{}, ipcerr.T(ipcerr.CodeConflict, "backend.core.agentSettings.changed.body", nil)
		case errors.Is(err, configfs.ErrInvalid):
			return View{}, ipcerr.BadRequest(err.Error())
		}
		return View{}, ipcerr.InternalErr(err)
	}
	return s.view()
}

// SetSecret seals one credential. A cipher failure keeps its E_SECRET_STORE code.
func (s *Service) SetSecret(args SetSecretArgs) (View, error) {
	if !secretNameRe.MatchString(args.Name) {
		return View{}, ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretName.body", nil))
	}
	if len(args.Value) == 0 || len(args.Value) > maxSecretBytes || strings.ContainsRune(args.Value, 0) {
		return View{}, ipcerr.BadRequest(i18n.T("backend.core.agentSettings.secretValue.body", nil))
	}
	if s.Store == nil {
		return s.view()
	}
	sec := s.Store.o.Secrets
	if sec == nil {
		return View{}, ipcerr.SecretStore(i18n.T("backend.core.agentSettings.noStore.body", nil))
	}
	if err := sec.Put(args.Name, args.Value); err != nil {
		return View{}, ipcerr.Wrap(err)
	}
	return s.view()
}

// DeleteSecret removes one credential; an absent name is not an error.
func (s *Service) DeleteSecret(args DeleteSecretArgs) (View, error) {
	if s.Store == nil {
		return s.view()
	}
	if sec := s.Store.o.Secrets; sec != nil {
		if err := sec.Delete(args.Name); err != nil {
			return View{}, ipcerr.Wrap(err)
		}
	}
	return s.view()
}

func (s *Service) view() (View, error) {
	st := s.Store
	if st == nil {
		return View{}, ipcerr.Internal(i18n.T("backend.core.agentSettings.unavailable.body", nil))
	}
	v := View{FileView: st.View(), Secrets: []SecretView{}, Providers: []ProviderView{}}
	set, ok := st.Settings()
	saved, err := st.savedNames()
	if err != nil {
		return View{}, ipcerr.Wrap(err)
	}
	v.Storage = st.storage()
	v.Secrets = secretViews(saved, referencedSecrets(set))
	if ok {
		pv, err := st.providerViews(set, v.Secrets)
		if err != nil {
			return View{}, ipcerr.InternalErr(err)
		}
		v.Providers = pv
	} else {
		v.Providers = st.providerViews0()
	}
	return v, nil
}

// storage is the credential store's availability.
func (s *Store) storage() secrets.Status {
	if s.o.Secrets == nil {
		r := i18n.T("backend.core.agentSettings.noStore.body", nil)
		return secrets.Status{Backend: "none", Reason: &r}
	}
	return s.o.Secrets.Store.Status()
}

func (s *Store) savedNames() ([]string, error) {
	if s.o.Secrets == nil {
		return nil, nil
	}
	return s.o.Secrets.Names()
}

func secretViews(saved, referenced []string) []SecretView {
	names := slices.Clone(saved)
	for _, n := range referenced {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	out := make([]SecretView, 0, len(names))
	for _, n := range names {
		out = append(out, SecretView{Name: n, Set: slices.Contains(saved, n), Referenced: slices.Contains(referenced, n)})
	}
	return out
}

// referencedSecrets lists the ${secret:name} names anywhere in the settings, sorted.
func referencedSecrets(s Settings) []string {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	var doc any
	if json.Unmarshal(b, &doc) != nil {
		return nil
	}
	var names []string
	walkStrings(doc, func(text string) {
		refs, _ := configfs.Refs(text)
		for _, r := range refs {
			if r.Kind == configfs.RefSecret && !slices.Contains(names, r.Name) {
				names = append(names, r.Name)
			}
		}
	})
	slices.Sort(names)
	return names
}

func walkStrings(v any, f func(string)) {
	switch t := v.(type) {
	case string:
		f(t)
	case []any:
		for _, e := range t {
			walkStrings(e, f)
		}
	case map[string]any:
		for _, e := range t {
			walkStrings(e, f)
		}
	}
}

// providerViews0 lists each provider with no settings: nothing rendered, only what Kira adds.
func (s *Store) providerViews0() []ProviderView {
	out := []ProviderView{}
	s.eachHost(func(id string, p harness.Provider, host harness.SettingsHost) {
		out = append(out, ProviderView{ID: id, Label: p.Label(), Files: []FileText{}, Env: []EnvText{}, Notes: []string{}, Kira: host.KiraAdds()})
	})
	return out
}

func (s *Store) eachHost(f func(id string, p harness.Provider, host harness.SettingsHost)) {
	if s.o.Providers == nil {
		return
	}
	for _, id := range s.providers {
		p, ok := s.o.Providers.Get(id)
		if !ok {
			continue
		}
		if host, ok := harness.SettingsHostOf(p); ok {
			f(id, p, host)
		}
	}
}

// providerViews renders each provider's launch config with references left verbatim: no decrypt, no
// environment read.
func (s *Store) providerViews(set Settings, secs []SecretView) ([]ProviderView, error) {
	out := []ProviderView{}
	var firstErr error
	s.eachHost(func(id string, p harness.Provider, host harness.SettingsHost) {
		if firstErr != nil {
			return
		}
		as, err := set.agentSettings(id, func(t string) (string, error) { return t, nil })
		if err != nil {
			firstErr = err
			return
		}
		cfg, err := host.RenderSettings(as, harness.LaunchInteractive)
		if err != nil {
			firstErr = err
			return
		}
		pv := ProviderView{ID: id, Label: p.Label(), Files: []FileText{}, Env: []EnvText{}, Notes: []string{}, Kira: host.KiraAdds()}
		for _, f := range cfg.Files {
			pv.Files = append(pv.Files, FileText{Name: f.Name, Text: string(f.Data)})
		}
		for _, kv := range cfg.Env {
			name, value, _ := strings.Cut(kv, "=")
			pv.Env = append(pv.Env, EnvText{Name: name, Value: value})
		}
		for _, sv := range secs {
			if sv.Referenced && !sv.Set {
				pv.Notes = append(pv.Notes, i18n.T("backend.core.agentSettings.secretUnset.body", map[string]string{"name": sv.Name}))
			}
		}
		pv.Notes = append(pv.Notes, cfg.Notes...)
		pv.Notes = append(pv.Notes, i18n.T("backend.core.agentSettings.headlessNote.body", nil))
		out = append(out, pv)
	})
	return out, firstErr
}
