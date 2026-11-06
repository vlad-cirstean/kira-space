package configsync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/catfile"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitops"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

const maxTreeEntries = 5000

// Problem kinds.
const (
	ProblemAuth     = "auth"
	ProblemNetwork  = "network"
	ProblemNotFound = "notFound"
	ProblemBranch   = "branch"
	ProblemGit      = "git"
	ProblemManifest = "manifest"
	ProblemSettings = "settings"
)

// syncError is a failed fetch or tree read, classified for retry and display.
type syncError struct {
	kind      string
	message   string
	transient bool
}

func (e *syncError) Error() string { return e.kind + ": " + e.message }

func (e *syncError) problem() *Problem { return &Problem{Kind: e.kind, Message: e.message} }

func manifestError(key string, params map[string]string) *syncError {
	return &syncError{kind: ProblemManifest, message: i18n.T(manifestKeys[key], params)}
}

var manifestKeys = map[string]string{
	"missing":  "backend.space.configSync.manifest.missing.body",
	"invalid":  "backend.space.configSync.manifest.invalid.body",
	"version":  "backend.space.configSync.manifest.version.body",
	"module":   "backend.space.configSync.manifest.module.body",
	"repos":    "backend.space.configSync.manifest.repos.body",
	"tooMany":  "backend.space.configSync.manifest.tooMany.body",
	"tooLarge": "backend.space.configSync.manifest.tooLarge.body",
}

// mirror is the bare clone under .sync/repo.git: no worktree, no checkout, no hooks on fetch.
type mirror struct {
	dir     string
	runner  gitclient.Runner
	gitPath func(context.Context) (string, error)
}

// firstLine trims git's stderr to one line for display; the URL never holds a credential.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// run spawns git in dir; a nonzero exit comes back classified.
func (m *mirror) run(ctx context.Context, dir string, args, env []string, readOnly bool) (gitclient.Result, *syncError) {
	path, err := m.gitPath(ctx)
	if err != nil {
		return gitclient.Result{}, &syncError{kind: ProblemGit, message: i18n.T("backend.space.configSync.git.unavailable.body", nil)}
	}
	res, err := gitclient.Run(ctx, m.runner, path, gitclient.Spec{
		Dir: dir, Args: args, ReadOnly: readOnly, Setsid: true,
		Env: append([]string{"GIT_ALLOW_PROTOCOL=file:https:ssh"}, env...),
	})
	if err != nil {
		if ctx.Err() != nil {
			return res, &syncError{kind: ProblemNetwork, message: ctx.Err().Error(), transient: true}
		}
		return res, &syncError{kind: ProblemGit, message: firstLine(err.Error())}
	}
	if res.ExitCode == 0 {
		return res, nil
	}
	return res, classifyFetch(string(res.Stderr), res.ExitCode)
}

func classifyFetch(stderr string, exit int) *syncError {
	if strings.Contains(stderr, "couldn't find remote ref") {
		return &syncError{kind: ProblemBranch, message: i18n.T("backend.space.configSync.branch.missing.body", nil)}
	}
	kind, _ := gitops.ClassifyOpError(stderr, exit)
	switch kind {
	case "AuthFailed":
		return &syncError{kind: ProblemAuth, message: i18n.T("backend.space.configSync.auth.required.body", nil)}
	case "NetworkFailed", "LockHeld":
		return &syncError{kind: ProblemNetwork, message: firstLine(stderr), transient: true}
	case "RemoteNotFound":
		return &syncError{kind: ProblemNotFound, message: i18n.T("backend.space.configSync.repo.notFound.body", nil)}
	}
	return &syncError{kind: ProblemGit, message: firstLine(stderr)}
}

// fetch updates the mirror from url and returns the branch tip. env carries the askpass settings of
// an explicit fetch; a background fetch passes none and fails fast on a credential prompt.
func (m *mirror) fetch(ctx context.Context, url, branch string, env []string) (string, *syncError) {
	parent := filepath.Dir(m.dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", &syncError{kind: ProblemGit, message: err.Error()}
	}
	if _, err := os.Stat(filepath.Join(m.dir, "HEAD")); err != nil {
		if _, e := m.run(ctx, parent, []string{"init", "--bare", "--quiet", filepath.Base(m.dir)}, nil, false); e != nil {
			return "", e
		}
	}
	if _, e := m.run(ctx, m.dir, []string{"config", "remote.origin.url", url}, nil, false); e != nil {
		return "", e
	}
	ref := "refs/remotes/origin/" + branch
	spec := "+refs/heads/" + branch + ":" + ref
	args := append(gitops.FetchRefspecsArgs("origin", []string{spec}), "--no-tags")
	if _, e := m.run(ctx, m.dir, args, env, false); e != nil {
		return "", e
	}
	res, e := m.run(ctx, m.dir, []string{"rev-parse", "--verify", "--quiet", ref + "^{commit}"}, nil, true)
	if e != nil {
		return "", e
	}
	return strings.TrimSpace(string(res.Stdout)), nil
}

// sharedTree is the manifest-listed content of one shared commit.
type sharedTree struct {
	commit  string
	modules []string
	// files is module, then file name, to content.
	files map[string]map[string][]byte
}

type manifest struct {
	Version int      `yaml:"version"`
	Modules []string `yaml:"modules"`
}

func parseManifest(src []byte) (manifest, *syncError) {
	var mf manifest
	dec := yaml.NewDecoder(bytes.NewReader(src))
	dec.KnownFields(true)
	if err := dec.Decode(&mf); err != nil {
		return mf, manifestError("invalid", nil)
	}
	if mf.Version != 1 {
		return mf, manifestError("version", nil)
	}
	for _, name := range mf.Modules {
		if name == "repos" {
			return mf, manifestError("repos", nil)
		}
		if !slices.Contains(syncModules, name) {
			return mf, manifestError("module", map[string]string{"name": name})
		}
	}
	return mf, nil
}

// readTree lists commit's files and reads the manifest and the files of the modules it lists.
func (m *mirror) readTree(ctx context.Context, commit string) (*sharedTree, *syncError) {
	res, e := m.run(ctx, m.dir, []string{"ls-tree", "-r", "-z", "--full-tree", commit}, nil, true)
	if e != nil {
		return nil, e
	}
	type blob struct{ oid, module, file string }
	var blobs []blob
	var manifestOID string
	entries := bytes.Split(bytes.TrimRight(res.Stdout, "\x00"), []byte{0})
	if len(entries) > maxTreeEntries {
		return nil, manifestError("tooMany", nil)
	}
	for _, ent := range entries {
		meta, path, ok := bytes.Cut(ent, []byte{'\t'})
		if !ok {
			continue
		}
		f := strings.Fields(string(meta))
		if len(f) != 3 || f[1] != "blob" || f[0] == "120000" {
			continue
		}
		parts := strings.Split(string(path), "/")
		switch {
		case len(parts) == 1 && parts[0] == manifestName:
			manifestOID = f[2]
		case len(parts) == 2:
			blobs = append(blobs, blob{oid: f[2], module: parts[0], file: parts[1]})
		}
	}
	if manifestOID == "" {
		return nil, manifestError("missing", nil)
	}
	path, err := m.gitPath(ctx)
	if err != nil {
		return nil, &syncError{kind: ProblemGit, message: i18n.T("backend.space.configSync.git.unavailable.body", nil)}
	}
	cf := catfile.NewSession(catfile.Deps{Runner: m.runner, GitPath: path, Dir: m.dir}, configfs.MaxFileBytes)
	defer cf.Close()
	read := func(oid, name string) ([]byte, *syncError) {
		_, data, err := cf.Read(ctx, oid)
		switch {
		case errors.Is(err, catfile.ErrTooLarge):
			return nil, manifestError("tooLarge", map[string]string{"file": name})
		case err != nil:
			return nil, &syncError{kind: ProblemGit, message: firstLine(err.Error()), transient: ctx.Err() != nil}
		}
		return data, nil
	}
	mdata, se := read(manifestOID, manifestName)
	if se != nil {
		return nil, se
	}
	mf, se := parseManifest(mdata)
	if se != nil {
		return nil, se
	}
	t := &sharedTree{commit: commit, modules: mf.Modules, files: map[string]map[string][]byte{}}
	for _, b := range blobs {
		if !slices.Contains(mf.Modules, b.module) || !syncableFile(b.module, b.file) {
			continue
		}
		data, se := read(b.oid, b.module+"/"+b.file)
		if se != nil {
			return nil, se
		}
		if t.files[b.module] == nil {
			t.files[b.module] = map[string][]byte{}
		}
		t.files[b.module][b.file] = data
	}
	return t, nil
}
