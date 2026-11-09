package repocfg

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/gitremote"
)

// MaxOriginBytes bounds the origin of a shared file.
const MaxOriginBytes = 2048

// SharedFile is a repo file as a shared config repository carries it: the repository is named by its
// origin URL and only the keys that are safe to share are present.
type SharedFile struct {
	Origin              string          `json:"origin"`
	IntegrationBranches []string        `json:"integration_branches,omitempty"`
	Environments        []Environment   `json:"environments,omitempty"`
	Worktree            *SharedWorktree `json:"worktree,omitempty"`
	Review              *Review         `json:"review,omitempty"`
}

// SharedWorktree holds the worktree leaves a shared file may set; base_path stays local.
type SharedWorktree struct {
	PrepareScript  *string          `json:"prepare_script,omitempty"`
	PrepareRun     *model.ScriptRef `json:"prepare_run,omitempty"`
	PrepareTimeout *string          `json:"prepare_timeout,omitempty"`
}

// file is a File holding the shared leaves, so the repo rules run on a shared file unchanged.
func (sf SharedFile) file() File {
	f := File{IntegrationBranches: sf.IntegrationBranches, Environments: sf.Environments, Review: sf.Review}
	if w := sf.Worktree; w != nil {
		f.Worktree = &Worktree{PrepareScript: w.PrepareScript, PrepareRun: w.PrepareRun, PrepareTimeout: w.PrepareTimeout}
	}
	return f
}

// sharedOf is the shared leaves of f under origin.
func sharedOf(origin string, f File) SharedFile {
	sf := SharedFile{Origin: origin, IntegrationBranches: f.IntegrationBranches, Environments: f.Environments, Review: f.Review}
	if w := f.Worktree; w != nil && (w.PrepareScript != nil || w.PrepareRun != nil || w.PrepareTimeout != nil) {
		sf.Worktree = &SharedWorktree{PrepareScript: w.PrepareScript, PrepareRun: w.PrepareRun, PrepareTimeout: w.PrepareTimeout}
	}
	return sf
}

// sharedDropped are the top-level keys of the repo schema a shared file never carries.
var sharedDropped = []string{"path", "nickname", "graph", "stash", "pull", "git_host", "checkout"}

// assembleSharedSchema derives the shared file schema from the assembled repo schema, so the limits
// cannot drift: it drops the local-only keys and requires origin.
func assembleSharedSchema(hosts []string) ([]byte, error) {
	raw, err := assembleSchema(hosts)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("repocfg: shared schema: %w", err)
	}
	props := root["properties"].(map[string]any)
	for _, k := range sharedDropped {
		delete(props, k)
	}
	delete(props["worktree"].(map[string]any)["properties"].(map[string]any), "base_path")
	props["origin"] = map[string]any{
		"type":        "string",
		"minLength":   1,
		"maxLength":   MaxOriginBytes,
		"description": "The repository this file configures, by the URL of its origin remote. Every registered repository whose origin remote is this URL gets the keys below. The URL is compared without scheme, user, port and .git suffix, so https and ssh spellings match.",
	}
	root["required"] = []string{"origin"}
	root["description"] = "A shared repo settings file: the keys a team may share, bound to repositories by origin."
	return json.Marshal(root)
}

func originRule() configfs.Rule[SharedFile] {
	return configfs.Rule[SharedFile]{
		Doc: "`origin` is a remote URL with a host and a path and no password.",
		Check: func(sf SharedFile, _ configfs.Locator) *configfs.Problem {
			if _, ok := gitremote.Key(sf.Origin); !ok || originHasPassword(sf.Origin) {
				return problem("/origin", "backend.space.repoFiles.originInvalid.body")
			}
			return nil
		},
	}
}

func originHasPassword(raw string) bool {
	if !strings.Contains(raw, "://") {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.User == nil {
		return false
	}
	_, has := u.User.Password()
	return has
}

// sharedRules are the repo rules that apply to the shared keys, plus the origin check.
func sharedRules() []configfs.Rule[SharedFile] {
	out := []configfs.Rule[SharedFile]{originRule()}
	for _, r := range []configfs.Rule[File]{integrationBranchesRule(), environmentsRule(), prepareExclusiveRule(), scriptRefsRule(), prepareTimeoutRule()} {
		out = append(out, configfs.Rule[SharedFile]{
			Doc:   r.Doc,
			Check: func(sf SharedFile, at configfs.Locator) *configfs.Problem { return r.Check(sf.file(), at) },
		})
	}
	return out
}

// CheckShared is the first problem of a shared file, or "" when it is clean.
func (s *Store) CheckShared(data []byte) string {
	if _, p := s.shared("", data); p != nil {
		return p.Message
	}
	return ""
}

// CheckLocal is the first problem of data as the repo file named file, or "" when it is clean.
func (s *Store) CheckLocal(file string, data []byte) string {
	if _, p := s.folder.Decode(file, data); p != nil {
		return p.Message
	}
	return ""
}

// NewFile is the name and starting text of the file a repo without one gets: its path and nothing
// else. taken lists names that are free on disk but already claimed by an earlier new file.
func (s *Store) NewFile(repoID string, taken map[string]bool) (string, []byte, error) {
	id := Key(repoID)
	name, err := s.freeName(slug(id), taken)
	if err != nil {
		return "", nil, err
	}
	text, err := configfs.RenderYAML(File{Path: id}, header)
	if err != nil {
		return "", nil, err
	}
	return name, text, nil
}

// SharedText is the shared file of a repo under origin: origin plus the repo's shared keys from its
// file, never a local key.
func (s *Store) SharedText(repoID, origin string) ([]byte, error) {
	var f File
	if file := s.FileFor(repoID); file != "" {
		if v, ok := s.Value(file); ok {
			f = v
		}
	}
	return configfs.RenderYAML(sharedOf(origin, f), "Shared Kira repo settings: applies to every repository with this origin.")
}
