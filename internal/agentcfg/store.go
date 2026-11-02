package agentcfg

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/agentctx"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// File is the settings file name in the agent folder.
const File = "settings.yaml"

// MaxBytes caps the settings file: it is read at every launch.
const MaxBytes = 256 << 10

//go:embed settings.schema.json
var baseSchema []byte

//go:embed agents_intro.md
var agentsIntro string

//go:embed settings_intro.md
var settingsIntro string

// Options builds a Store.
type Options struct {
	Dir        string              // shared agent folder
	Projection configfs.Projection // last good settings
	Providers  *harness.Registry
	// Secrets holds the credential values the file references; nil means none can be read or saved.
	Secrets *SecretStore
	// Env reads ${NAME} references; nil means the login shell, then the process environment.
	Env func(string) (string, bool)
	App string // "Space" or "Studio", AGENTS.md header
}

// Store reads and writes the settings file and prepares it for launches.
type Store struct {
	o         Options
	folder    *configfs.Folder[Settings]
	docModel  configfs.DocModel
	providers []string // provider ids with a settings section, registration order
	saveMu    sync.Mutex

	runMu sync.Mutex
	runs  map[*runFiles]struct{}

	envMu     sync.Mutex
	probed    map[string]string // login-shell values by name
	tried     map[string]bool   // names a probe already asked for
	probeWarn sync.Once
	warned    sync.Map // provider id: warned that it takes no settings
}

// FileView is the editor's state of the file.
type FileView struct {
	Folder   string       `json:"folder"`
	File     string       `json:"file"`
	Path     string       `json:"path"`
	Exists   bool         `json:"exists"`
	Text     string       `json:"text"`
	Hash     string       `json:"hash"`
	Editable bool         `json:"editable"`
	Problem  *ViewProblem `json:"problem"`
	LastGood bool         `json:"lastGood"`
	MaxBytes int          `json:"maxBytes"`
}

// ViewProblem is why the file is invalid.
type ViewProblem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// New builds a Store with the schema assembled from the registered providers; it creates nothing on disk.
func New(o Options) (*Store, error) {
	s := &Store{o: o, runs: map[*runFiles]struct{}{}, probed: map[string]string{}, tried: map[string]bool{}}
	schema, err := s.assembleSchema()
	if err != nil {
		return nil, err
	}
	decode, model, err := configfs.YAMLSchema[Settings](schema, rules(s.providers)...)
	if err != nil {
		return nil, err
	}
	model.Pattern, model.Format, model.Singleton, model.MaxBytes = File, "YAML", true, MaxBytes
	model.Intro, model.App = settingsIntro, o.App
	s.docModel = model
	s.folder = &configfs.Folder[Settings]{
		Dir: o.Dir, Ext: ".yaml", Name: regexp.MustCompile(`^settings\.yaml$`),
		Decode: func(file string, src []byte) (Settings, *configfs.Problem) {
			if len(src) > MaxBytes {
				return Settings{}, &configfs.Problem{Message: i18n.T("backend.core.config.textTooLarge.body", map[string]string{"limit": fmt.Sprint(MaxBytes)})}
			}
			return decode(file, src)
		},
		Projection: o.Projection,
	}
	return s, nil
}

// assembleSchema puts each SettingsHost provider's section schema under its id.
func (s *Store) assembleSchema() ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(baseSchema, &root); err != nil {
		return nil, fmt.Errorf("agentcfg: schema: %w", err)
	}
	props := root["properties"].(map[string]any)
	if s.o.Providers != nil {
		for _, id := range s.o.Providers.IDs() {
			p, _ := s.o.Providers.Get(id)
			host, ok := harness.SettingsHostOf(p)
			if !ok {
				continue
			}
			var sec map[string]any
			if err := json.Unmarshal(host.SettingsSchema(), &sec); err != nil {
				return nil, fmt.Errorf("agentcfg: %s settings schema: %w", id, err)
			}
			delete(sec, "$schema")
			props[id] = sec
			s.providers = append(s.providers, id)
		}
	}
	return json.Marshal(root)
}

func (s *Store) path() string { return filepath.Join(s.folder.Dir, File) }

// View returns the file's text, hash and state. It also records a valid file as last good.
func (s *Store) View() FileView {
	v := FileView{Folder: agentctx.Folder, File: File, Path: s.path(), MaxBytes: MaxBytes, Editable: true}
	e, ok := s.folder.Get(File)
	if !ok {
		return v
	}
	v.Exists = true
	v.Hash = e.Hash
	if e.Problem != nil {
		v.Problem = &ViewProblem{Line: e.Problem.Line, Message: e.Problem.Message}
		v.LastGood = e.Value != nil
	}
	raw, hash, err := s.folder.Read(File)
	if err != nil || !utf8.Valid(raw) {
		v.Editable = false
		return v
	}
	v.Text, v.Hash = string(raw), hash
	return v
}

// Settings is the settings in effect: the file's, or the last good one while the file is invalid. ok
// is false when there are none (no file, or invalid with no last good).
func (s *Store) Settings() (Settings, bool) {
	e, ok := s.folder.Get(File)
	if !ok || e.Value == nil {
		return Settings{}, false
	}
	return *e.Value, true
}

// Save writes text as the file. baseHash is the hash the editor loaded (configfs.TextHash(nil), "",
// for no file): a file changed since then is configfs.ErrConflict. Invalid text is saved too; the
// last good settings stay in use.
func (s *Store) Save(text, baseHash string) (FileView, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	cur, err := os.ReadFile(s.path())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return FileView{}, fmt.Errorf("agentcfg: read %s: %w", File, err)
	}
	if configfs.TextHash(cur) != baseHash {
		return FileView{}, fmt.Errorf("%w: the file changed since it was loaded", configfs.ErrConflict)
	}
	if _, err := s.folder.Save(File, "", []byte(text)); err != nil {
		return FileView{}, err
	}
	return s.View(), nil
}

// Watch calls onChange (debounced) when the folder's YAML files change.
func (s *Store) Watch(onChange func()) (stop func(), err error) { return s.folder.Watch(onChange) }

// WriteDocs writes the folder's AGENTS.md: the folder intro, the context file's section and this
// file's section.
func (s *Store) WriteDocs(context configfs.DocModel) error {
	m := configfs.DocModel{App: s.o.App, Title: context.Title, Intro: agentsIntro, Sections: []configfs.DocModel{context, s.docModel}}
	return s.folder.WriteDocs(m)
}

// Check decodes text as settings.yaml without saving it: nil when it is valid.
func (s *Store) Check(text []byte) *configfs.Problem {
	_, p := s.folder.Decode(File, text)
	return p
}

// ForProvider is the settings in effect as one provider's input, references verbatim, as the
// settings view renders them; ok is false when there are no settings.
func (s *Store) ForProvider(provider string) (as harness.AgentSettings, ok bool, err error) {
	set, ok := s.Settings()
	if !ok {
		return as, false, nil
	}
	as, err = set.agentSettings(provider, func(t string) (string, error) { return t, nil })
	return as, true, err
}

// NeutralEvent is the settings.yaml hook event name of a provider event kind.
func NeutralEvent(kind harness.EventKind) (string, bool) {
	for name, k := range hookEvents {
		if k == kind {
			return name, true
		}
	}
	return "", false
}

// HookEventKind is the provider event kind of a settings.yaml hook event name.
func HookEventKind(neutral string) (harness.EventKind, bool) {
	k, ok := hookEvents[neutral]
	return k, ok
}
