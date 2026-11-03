// Package repocfg is the repo settings config folder of Kira Space: one YAML file per repository,
// keyed by its path, read into an in-memory index and written back by git-ui's settings dialog.
package repocfg

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitpath"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// Folder is the config folder's name under the Space home and in config_last_valid.
const Folder = "repos"

// Limits shared by the schema, the rules and the board.
const (
	MaxNicknameRunes  = 40
	MaxIntegration    = 10
	MaxEnvironments   = 10
	MaxEnvScriptBytes = 4 << 10
	MaxPrepareBytes   = 64 << 10
)

// File is one repo file as written. A nil leaf is absent: the default applies.
type File struct {
	Path                string        `json:"path"`
	Nickname            string        `json:"nickname,omitempty"`
	IntegrationBranches []string      `json:"integration_branches,omitempty"`
	Environments        []Environment `json:"environments,omitempty"`
	Worktree            *Worktree     `json:"worktree,omitempty"`
	Graph               *Graph        `json:"graph,omitempty"`
	Stash               *Stash        `json:"stash,omitempty"`
	Review              *Review       `json:"review,omitempty"`
	Pull                *Pull         `json:"pull,omitempty"`
	GitHost             *GitHost      `json:"git_host,omitempty"`
	Checkout            *Checkout     `json:"checkout,omitempty"`
}

// Environment is one deploy target.
type Environment struct {
	Name              string `json:"name"`
	DeployedSHAScript string `json:"deployed_sha_script"`
}

// Worktree holds the worktree leaves.
type Worktree struct {
	PrepareScript  *string `json:"prepare_script,omitempty"`
	PrepareTimeout *string `json:"prepare_timeout,omitempty"`
	BasePath       *string `json:"base_path,omitempty"`
}

// Graph holds the commit graph leaves.
type Graph struct {
	PageSize *int    `json:"page_size,omitempty"`
	Scope    *string `json:"scope,omitempty"`
}

// Stash holds the stash leaves.
type Stash struct {
	ShowInGraph      *bool `json:"show_in_graph,omitempty"`
	IncludeUntracked *bool `json:"include_untracked,omitempty"`
}

// Review holds the review leaves.
type Review struct {
	BaseCandidates *[]string `json:"base_candidates,omitempty"`
}

// Pull holds the pull leaves.
type Pull struct {
	Strategy *string `json:"strategy,omitempty"`
}

// GitHost holds the Git host leaves.
type GitHost struct {
	Provider *string `json:"provider,omitempty"`
}

// Checkout holds the checkout leaves.
type Checkout struct {
	AutoStash *bool `json:"auto_stash,omitempty"`
}

// Config is a repo's effective config: the file's values over the defaults.
type Config struct {
	Path                string
	Settings            model.GitRepoSettings
	Nickname            string
	IntegrationBranches []string
	Environments        []model.AdeRepoEnv
}

// isDefault reports whether the file sets nothing beyond its path.
func (f File) isDefault() bool {
	return f.Nickname == "" && len(f.IntegrationBranches) == 0 && len(f.Environments) == 0 && f.Worktree == nil &&
		f.Graph == nil && f.Stash == nil && f.Review == nil && f.Pull == nil && f.GitHost == nil && f.Checkout == nil
}

// Key is the path as the index keys it.
func Key(path string) string { return gitpath.CleanNFC(path) }

// Defaults is the config of a repo without a file.
func Defaults(path string) Config {
	return Config{Path: path, Settings: model.DefaultGitRepoSettings(), IntegrationBranches: []string{}, Environments: []model.AdeRepoEnv{}}
}

// Config fills the defaults under the file's values.
func (f File) Config() Config {
	c := Defaults(Key(f.Path))
	c.Nickname = strings.TrimSpace(f.Nickname)
	for _, b := range f.IntegrationBranches {
		c.IntegrationBranches = append(c.IntegrationBranches, strings.TrimSpace(b))
	}
	for _, e := range f.Environments {
		c.Environments = append(c.Environments, model.AdeRepoEnv{Name: e.Name, DeployedShaScript: e.DeployedSHAScript})
	}
	s := &c.Settings
	if w := f.Worktree; w != nil {
		set(&s.WorktreePrepareScript, w.PrepareScript)
		set(&s.WorktreePrepareTimeout, w.PrepareTimeout)
		if w.BasePath != nil && *w.BasePath != "" {
			s.WorktreeBasePath = Key(*w.BasePath)
		}
	}
	if g := f.Graph; g != nil {
		set(&s.GraphPageSize, g.PageSize)
		set(&s.GraphScope, g.Scope)
	}
	if st := f.Stash; st != nil {
		set(&s.StashShowInGraph, st.ShowInGraph)
		set(&s.StashIncludeUntracked, st.IncludeUntracked)
	}
	if r := f.Review; r != nil && r.BaseCandidates != nil {
		s.ReviewBaseCandidates = append([]string{}, *r.BaseCandidates...)
	}
	if p := f.Pull; p != nil {
		set(&s.PullStrategy, p.Strategy)
	}
	if h := f.GitHost; h != nil {
		set(&s.GitHostProvider, h.Provider)
	}
	if c := f.Checkout; c != nil {
		set(&s.CheckoutAutoStash, c.AutoStash)
	}
	return c
}

func set[T any](dst *T, v *T) {
	if v != nil {
		*dst = *v
	}
}

// FileOf is the file of c, holding only the leaves that differ from the defaults.
func FileOf(c Config) File {
	f := File{Path: c.Path, Nickname: c.Nickname, IntegrationBranches: c.IntegrationBranches}
	for _, e := range c.Environments {
		f.Environments = append(f.Environments, Environment{Name: e.Name, DeployedSHAScript: e.DeployedShaScript})
	}
	d, s := model.DefaultGitRepoSettings(), c.Settings
	if s.WorktreePrepareScript != d.WorktreePrepareScript || s.WorktreePrepareTimeout != d.WorktreePrepareTimeout || s.WorktreeBasePath != d.WorktreeBasePath {
		w := &Worktree{}
		diff(&w.PrepareScript, s.WorktreePrepareScript, d.WorktreePrepareScript)
		diff(&w.PrepareTimeout, s.WorktreePrepareTimeout, d.WorktreePrepareTimeout)
		diff(&w.BasePath, s.WorktreeBasePath, d.WorktreeBasePath)
		f.Worktree = w
	}
	if s.GraphPageSize != d.GraphPageSize || s.GraphScope != d.GraphScope {
		g := &Graph{}
		diff(&g.PageSize, s.GraphPageSize, d.GraphPageSize)
		diff(&g.Scope, s.GraphScope, d.GraphScope)
		f.Graph = g
	}
	if s.StashShowInGraph != d.StashShowInGraph || s.StashIncludeUntracked != d.StashIncludeUntracked {
		st := &Stash{}
		diff(&st.ShowInGraph, s.StashShowInGraph, d.StashShowInGraph)
		diff(&st.IncludeUntracked, s.StashIncludeUntracked, d.StashIncludeUntracked)
		f.Stash = st
	}
	if !equalStrings(s.ReviewBaseCandidates, d.ReviewBaseCandidates) {
		f.Review = &Review{BaseCandidates: &s.ReviewBaseCandidates}
	}
	if s.PullStrategy != d.PullStrategy {
		f.Pull = &Pull{Strategy: &s.PullStrategy}
	}
	if s.GitHostProvider != d.GitHostProvider {
		f.GitHost = &GitHost{Provider: &s.GitHostProvider}
	}
	if s.CheckoutAutoStash != d.CheckoutAutoStash {
		f.Checkout = &Checkout{AutoStash: &s.CheckoutAutoStash}
	}
	return f
}

func diff[T comparable](dst **T, v, def T) {
	if v != def {
		*dst = &v
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
