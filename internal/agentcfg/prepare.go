package agentcfg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/harness"
	"github.com/kirathecat/kira-studio/internal/loginshell"
)

// Launch is one launch to prepare settings for.
type Launch struct {
	Provider harness.Provider
	Kind     harness.LaunchKind
	// Dir is Kira's own 0700 run dir (the ADE agent dir, automations-mcp).
	Dir string
	// Stem is the file name stem: the run id for a headless run, "tui-<uuid>" otherwise.
	Stem string
}

// Prepared is what a launch gets. Release removes the run's files and is safe to call twice.
type Prepared struct {
	// App is nil when there is nothing to apply.
	App *harness.AppConfig
	// Secrets are the resolved secret values, for a Masker. Never log them.
	Secrets []string
	Release func()
}

// PrepareFunc is the launch-time hook callers hold; nil means no settings.
type PrepareFunc func(ctx context.Context, l Launch) (Prepared, error)

// runFiles is the set of files one launch wrote.
type runFiles struct {
	once  sync.Once
	paths []string
}

func (r *runFiles) remove() {
	r.once.Do(func() {
		for _, p := range r.paths {
			if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
				slog.Warn("agent settings: remove run file", "scope", "agentcfg", "err", err)
			}
		}
	})
}

// Prepare resolves the settings in effect for one launch and writes the provider's files (0600). Any
// resolve or write error is returned: a launch never starts with a secret silently missing.
func (s *Store) Prepare(ctx context.Context, l Launch) (Prepared, error) {
	none := Prepared{Release: func() {}}
	hermetic := s.o.Hermetic != nil && s.o.Hermetic()
	set, ok := s.Settings()
	if !ok && !hermetic {
		return none, nil
	}
	host, ok := harness.SettingsHostOf(l.Provider)
	if !ok {
		if _, warned := s.warned.LoadOrStore(l.Provider.ID(), true); !warned {
			slog.Warn("agent settings: provider takes none", "scope", "agentcfg", "provider", l.Provider.ID())
		}
		return none, nil
	}
	resolve, values, err := s.resolver(ctx, set)
	if err != nil {
		return none, err
	}
	as, err := set.agentSettings(l.Provider.ID(), resolve)
	if err != nil {
		return none, fmt.Errorf("agent settings: %w", err)
	}
	as.Hermetic = hermetic
	cfg, err := host.RenderSettings(as, l.Kind)
	if err != nil {
		return none, fmt.Errorf("agent settings: %w", err)
	}
	if len(cfg.Files) == 0 && len(cfg.Env) == 0 {
		return none, nil
	}
	app, files, err := s.writeFiles(l, cfg)
	if err != nil {
		return none, err
	}
	s.track(files, true)
	return Prepared{App: app, Secrets: *values, Release: func() { files.remove(); s.track(files, false) }}, nil
}

// resolver resolves the references of one launch (env, secrets) and collects the secret values it
// used, for a Masker. Read the collected values after every resolve call.
func (s *Store) resolver(ctx context.Context, set Settings) (func(string) (string, error), *[]string, error) {
	env, err := s.envLookup(ctx, set)
	if err != nil {
		return nil, nil, err
	}
	res := &configfs.Resolver{Env: env, Namespace: secretNamespace}
	if s.o.Secrets != nil {
		res.Secrets = s.o.Secrets.Snapshot()
	}
	values := new([]string)
	return func(text string) (string, error) {
		v, used, err := res.Resolve(ctx, text)
		for _, u := range used {
			if !slices.Contains(*values, u.Value) {
				*values = append(*values, u.Value)
			}
		}
		return v, err
	}, values, nil
}

// writeFiles writes each non-inline file of cfg into the launch dir; on an error it removes what it wrote.
func (s *Store) writeFiles(l Launch, cfg harness.NativeConfig) (*harness.AppConfig, *runFiles, error) {
	app := &harness.AppConfig{Files: map[string]string{}, Inline: map[string][]byte{}, Env: cfg.Env}
	files := &runFiles{}
	for _, f := range cfg.Files {
		if f.Inline {
			app.Inline[f.Role] = f.Data
			continue
		}
		path := filepath.Join(l.Dir, fmt.Sprintf("%s-app.%s%s", l.Stem, f.Role, filepath.Ext(f.Name)))
		if err := os.MkdirAll(l.Dir, 0o700); err != nil {
			files.remove()
			return nil, nil, fmt.Errorf("agent settings: create %s: %w", l.Dir, err)
		}
		if err := configfs.WriteAtomic(path, f.Data, 0o600); err != nil {
			files.remove()
			return nil, nil, fmt.Errorf("agent settings: %w", err)
		}
		files.paths = append(files.paths, path)
		app.Files[f.Role] = path
	}
	return app, files, nil
}

func (s *Store) track(r *runFiles, add bool) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if add {
		s.runs[r] = struct{}{}
	} else {
		delete(s.runs, r)
	}
}

// Close removes every run file not released yet (a quit with agents running).
func (s *Store) Close() error {
	s.runMu.Lock()
	runs := s.runs
	s.runs = map[*runFiles]struct{}{}
	s.runMu.Unlock()
	for r := range runs {
		r.remove()
	}
	return nil
}

// Sweep removes leftover run files and hermetic isolation dirs in dirs (a crash skipped Release).
// Call it at start, before any launch.
func (s *Store) Sweep(dirs ...string) error {
	var errs []error
	for _, d := range dirs {
		isos, err := filepath.Glob(filepath.Join(d, harness.IsolationGlob))
		if err != nil {
			errs = append(errs, err)
		}
		for _, m := range isos {
			if err := os.RemoveAll(m); err != nil {
				errs = append(errs, err)
			}
		}
		matches, err := filepath.Glob(filepath.Join(d, "*-app.*.json"))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, m := range matches {
			if err := os.Remove(m); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// envLookup is the resolver's env source for set: the user's login shell first, then the process
// environment. A shell probe failure warns once and falls back to the process environment.
func (s *Store) envLookup(ctx context.Context, set Settings) (func(string) (string, bool), error) {
	if s.o.Env != nil {
		return s.o.Env, nil
	}
	s.envMu.Lock()
	defer s.envMu.Unlock()
	var want []string
	for _, n := range envRefNames(set) {
		if !s.tried[n] {
			want = append(want, n)
		}
	}
	if len(want) > 0 {
		got, err := loginshell.ProbeEnv(ctx, want)
		if err != nil {
			s.probeWarn.Do(func() {
				slog.Warn("agent settings: login shell probe failed, using the process environment", "scope", "agentcfg", "err", err)
			})
		}
		for _, n := range want {
			s.tried[n] = true
			if v, ok := got[n]; ok {
				s.probed[n] = v
			}
		}
	}
	probed := cloneMap(s.probed)
	return func(name string) (string, bool) {
		if v, ok := probed[name]; ok {
			return v, true
		}
		return os.LookupEnv(name)
	}, nil
}

func cloneMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// envRefNames are the ${NAME} references in the settings' reference fields.
func envRefNames(set Settings) []string {
	var names []string
	add := func(text string) {
		refs, p := configfs.Refs(text)
		if p != nil {
			return
		}
		for _, r := range refs {
			if r.Kind == configfs.RefEnv && !slices.Contains(names, r.Name) {
				names = append(names, r.Name)
			}
		}
	}
	addMap := func(m map[string]string) {
		for _, v := range m {
			add(v)
		}
	}
	addMap(set.Env)
	for _, m := range set.MCPServers {
		add(m.URL)
		add(m.Command)
		addMap(m.Headers)
		addMap(m.Env)
		for _, a := range m.Args {
			add(a)
		}
	}
	slices.Sort(names)
	return names
}
