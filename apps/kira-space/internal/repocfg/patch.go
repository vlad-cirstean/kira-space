package repocfg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

const header = "Kira repo settings. Options: AGENTS.md in this folder."

// leafSets is the one table of patch leaves and their key paths in the file.
func leafSets(p model.GitRepoSettingsPatch) []configfs.NodeSet {
	var out []configfs.NodeSet
	add := func(v any, path ...string) { out = append(out, configfs.NodeSet{Path: path, Value: v}) }
	if p.GraphPageSize != nil {
		add(*p.GraphPageSize, "graph", "page_size")
	}
	if p.GraphScope != nil {
		add(*p.GraphScope, "graph", "scope")
	}
	if p.StashShowInGraph != nil {
		add(*p.StashShowInGraph, "stash", "show_in_graph")
	}
	if p.StashIncludeUntracked != nil {
		add(*p.StashIncludeUntracked, "stash", "include_untracked")
	}
	if p.ReviewBaseCandidates != nil {
		add(*p.ReviewBaseCandidates, "review", "base_candidates")
	}
	if p.PullStrategy != nil {
		add(*p.PullStrategy, "pull", "strategy")
	}
	if p.GitHostProvider != nil {
		add(*p.GitHostProvider, "git_host", "provider")
	}
	if p.WorktreePrepareScript != nil {
		add(*p.WorktreePrepareScript, "worktree", "prepare_script")
	}
	if p.WorktreePrepareTimeout != nil {
		add(*p.WorktreePrepareTimeout, "worktree", "prepare_timeout")
	}
	if p.WorktreeBasePath != nil {
		base := *p.WorktreeBasePath
		if base != "" {
			base = Key(base)
		}
		add(base, "worktree", "base_path")
	}
	if p.CheckoutAutoStash != nil {
		add(*p.CheckoutAutoStash, "checkout", "auto_stash")
	}
	return out
}

// Patch writes the patched leaves into the repo's file, creating the file when the repo has none, and
// returns the settings now in effect. A file with an error is refused and left untouched. Comments
// and key order in the file stay.
func (s *Store) Patch(repoID string, p model.GitRepoSettingsPatch) (model.GitRepoSettings, error) {
	if err := p.Validate(); err != nil {
		return model.GitRepoSettings{}, err
	}
	id := Key(repoID)
	s.op.Lock()
	first := s.load("")
	l, err := s.patchLocked(id, p)
	s.op.Unlock()
	s.finish(first)
	s.finish(l)
	if err != nil {
		return model.GitRepoSettings{}, err
	}
	return s.Settings(id)
}

func (s *Store) patchLocked(id string, p model.GitRepoSettingsPatch) (loaded, error) {
	sets := leafSets(p)
	file := s.FileFor(id)
	if file == "" {
		name, err := s.freeName(slug(id))
		if err != nil {
			return loaded{}, err
		}
		base, err := configfs.RenderYAML(File{Path: id}, header)
		if err != nil {
			return loaded{}, err
		}
		out, err := configfs.EditYAML(base, sets)
		if err != nil {
			return loaded{}, err
		}
		if _, err := s.folder.Save(name, "", out); err != nil {
			return loaded{}, err
		}
		return s.load(id), nil
	}
	for attempt := 0; ; attempt++ {
		if e, ok := s.folder.Get(file); ok && e.Problem != nil {
			return loaded{}, ipcerr.T(ipcerr.CodeBadRequest, "backend.space.repoFiles.fileBroken.body",
				map[string]string{"file": file, "line": strconv.Itoa(e.Problem.Line)})
		}
		src, hash, err := s.folder.Read(file)
		if err != nil {
			return loaded{}, fmt.Errorf("repocfg: read %s: %w", file, err)
		}
		out, err := configfs.EditYAML(src, sets)
		if err != nil {
			return loaded{}, fmt.Errorf("repocfg: edit %s: %w", file, err)
		}
		_, err = s.folder.Save(file, hash, out)
		switch {
		case err == nil:
			return s.load(id), nil
		case errors.Is(err, configfs.ErrConflict) && attempt == 0:
		case errors.Is(err, configfs.ErrConflict):
			return loaded{}, ipcerr.T(ipcerr.CodeConflict, "backend.space.repoFiles.changed.body", nil)
		default:
			return loaded{}, err
		}
	}
}

var slugRuns = regexp.MustCompile(`[^a-z0-9]+`)

// slug is a readable file stem for a repo path: its base name, lowercase, letters and digits joined by dashes.
func slug(path string) string {
	s := strings.Trim(slugRuns.ReplaceAllString(strings.ToLower(filepath.Base(path)), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		return "repo"
	}
	return s
}

// freeName is the first of stem.yaml, stem-2.yaml, ... with no file of that name.
func (s *Store) freeName(stem string) (string, error) {
	for n := 1; n < 10000; n++ {
		name := stem + ".yaml"
		if n > 1 {
			name = stem + "-" + strconv.Itoa(n) + ".yaml"
		}
		if _, err := os.Stat(filepath.Join(s.folder.Dir, name)); errors.Is(err, os.ErrNotExist) {
			return name, nil
		} else if err != nil {
			return "", fmt.Errorf("repocfg: stat %s: %w", name, err)
		}
	}
	return "", fmt.Errorf("repocfg: no free file name for %s", stem)
}
