package configsync

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

//go:embed sync.schema.json
var syncSchema []byte

const (
	// DirName is the hidden sync folder under <home>/config. No module reads it.
	DirName      = ".sync"
	settingsFile = "sync.yaml"
	stateFile    = "state.json"
	journalFile  = "apply.json"
	mirrorDir    = "repo.git"
	baseDir      = "base"
	baseNextDir  = "base.next"

	defaultBranch       = "main"
	defaultFetchMinutes = 30
	minFetchMinutes     = 5
)

func syncDir(home string) string { return filepath.Join(configfs.Root(home), DirName) }

// Repo is the one shared repository.
type Repo struct {
	URL    string `json:"url" yaml:"url"`
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`
}

// Settings is .sync/sync.yaml: written only by the service.
type Settings struct {
	Repo         *Repo               `json:"repo,omitempty" yaml:"repo,omitempty"`
	FetchMinutes *int                `json:"fetch_minutes,omitempty" yaml:"fetch_minutes,omitempty"`
	Modules      map[string]string   `json:"modules,omitempty" yaml:"modules,omitempty"`
	Deactivated  map[string][]string `json:"deactivated,omitempty" yaml:"deactivated,omitempty"`
}

func (s Settings) branch() string {
	if s.Repo == nil || s.Repo.Branch == "" {
		return defaultBranch
	}
	return s.Repo.Branch
}

func (s Settings) minutes() int {
	if s.FetchMinutes == nil {
		return defaultFetchMinutes
	}
	return *s.FetchMinutes
}

func (s Settings) mode(module string) string {
	if m := s.Modules[module]; m != "" {
		return m
	}
	return defaultMode(module)
}

func (s Settings) deactivated(module string) map[string]bool {
	out := map[string]bool{}
	for _, u := range s.Deactivated[module] {
		out[u] = true
	}
	return out
}

var decodeSettings = func() configfs.DecodeFunc[Settings] {
	dec, _, err := configfs.YAMLSchema(syncSchema,
		configfs.Rule[Settings]{
			Doc: "repo.url is https, ssh, scp-like or file and holds no credentials; repo.branch is a plain branch name.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem {
				if s.Repo == nil {
					return nil
				}
				if validRepoURL(s.Repo.URL) != nil {
					return &configfs.Problem{Path: "/repo/url", Message: i18n.T("backend.space.configSync.url.invalid.body", nil)}
				}
				if s.Repo.Branch != "" && !validBranch(s.Repo.Branch) {
					return &configfs.Problem{Path: "/repo/branch", Message: i18n.T("backend.space.configSync.branch.invalid.body", nil)}
				}
				return nil
			},
		},
		configfs.Rule[Settings]{
			Doc: "fetch_minutes is 0 or 5 to 1440.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem {
				if s.FetchMinutes != nil && *s.FetchMinutes != 0 && *s.FetchMinutes < minFetchMinutes {
					return &configfs.Problem{Path: "/fetch_minutes", Message: i18n.T("backend.space.configSync.interval.invalid.body", nil)}
				}
				return nil
			},
		},
		configfs.Rule[Settings]{
			Doc: "Only agent and prompts may be combined.",
			Check: func(s Settings, _ configfs.Locator) *configfs.Problem {
				for m, mode := range s.Modules {
					if !validMode(m, mode) {
						return &configfs.Problem{Path: "/modules/" + m, Message: i18n.T("backend.space.configSync.mode.invalid.body", nil)}
					}
				}
				return nil
			},
		},
	)
	if err != nil {
		panic(fmt.Sprintf("configsync: settings schema: %v", err))
	}
	return dec
}()

// loadSettings reads sync.yaml; a missing file is no repository.
func loadSettings(home string) (Settings, error) {
	src, err := configfs.ReadCapped(filepath.Join(syncDir(home), settingsFile), configfs.MaxFileBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	s, prob := decodeSettings(settingsFile, src)
	if prob != nil {
		return Settings{}, fmt.Errorf("configsync: %s line %d: %s", settingsFile, prob.Line, prob.Message)
	}
	return s, nil
}

func saveSettings(home string, s Settings) error {
	for m, list := range s.Deactivated {
		if len(list) == 0 {
			delete(s.Deactivated, m)
		} else {
			sort.Strings(list)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return configfs.WriteAtomic(filepath.Join(syncDir(home), settingsFile), buf.Bytes(), 0o644)
}

// Problem is why sync is stuck, shown in Settings.
type Problem struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// State is .sync/state.json: runtime state written only by the service.
type State struct {
	Fetched   string   `json:"fetched"`
	Applied   string   `json:"applied"`
	FetchedAt string   `json:"fetchedAt"`
	AppliedAt string   `json:"appliedAt"`
	Problem   *Problem `json:"problem,omitempty"`
	Dismissed string   `json:"dismissed"`
	// Manifest lists the modules the fetched commit's manifest names.
	Manifest []string `json:"manifest,omitempty"`
}

var stateMu sync.Mutex

func loadState(home string) State {
	var st State
	b, err := os.ReadFile(filepath.Join(syncDir(home), stateFile))
	if err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func saveState(home string, st State) error {
	stateMu.Lock()
	defer stateMu.Unlock()
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return configfs.WriteAtomic(filepath.Join(syncDir(home), stateFile), b, 0o644)
}
