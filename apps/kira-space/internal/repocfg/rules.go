package repocfg

import (
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

func problem(path, key string) *configfs.Problem {
	return &configfs.Problem{Path: path, Message: i18n.T(key, nil)}
}

var paramName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// refProblem is the first problem of a Run script reference at path at, or nil.
func refProblem(at string, r *model.ScriptRef) *configfs.Problem {
	if r == nil {
		return nil
	}
	name := strings.TrimSpace(r.Script)
	if name == "" || strings.ContainsAny(name, "\r\n") || utf8.RuneCountInString(name) > model.MaxScriptRefName {
		return problem(at+"/script", "backend.space.repoFiles.refScriptBlank.body")
	}
	for _, p := range slices.Sorted(maps.Keys(r.Params)) {
		if !paramName.MatchString(p) {
			return problem(at+"/params/"+p, "backend.space.repoFiles.refParamName.body")
		}
	}
	return nil
}

// absolutePath reports whether p is absolute with no `~` start and no `.` or `..` segment.
func absolutePath(p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// rules are the cross-field checks run after schema validation. Each Doc renders into AGENTS.md.
func rules() []configfs.Rule[File] {
	return []configfs.Rule[File]{
		pathRule(),
		nicknameRule(),
		integrationBranchesRule(),
		environmentsRule(),
		prepareExclusiveRule(),
		scriptRefsRule(),
		prepareTimeoutRule(),
		basePathRule(),
	}
}

func pathRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "`path` is absolute, with no `~` and no `.` or `..` segment.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if !absolutePath(f.Path) {
				return problem("/path", "backend.space.repoFiles.pathAbsolute.body")
			}
			return nil
		},
	}
}

func nicknameRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "`nickname` is at most " + strconv.Itoa(MaxNicknameRunes) + " characters on one line.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if utf8.RuneCountInString(strings.TrimSpace(f.Nickname)) > MaxNicknameRunes || strings.ContainsAny(strings.TrimSpace(f.Nickname), "\r\n") {
				return problem("/nickname", "backend.space.repoFiles.nicknameLong.body")
			}
			return nil
		},
	}
}

func integrationBranchesRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "Integration branch names are unique and not blank.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			seen := map[string]bool{}
			for i, b := range f.IntegrationBranches {
				name := strings.TrimSpace(b)
				at := "/integration_branches/" + strconv.Itoa(i)
				switch {
				case name == "":
					return problem(at, "backend.space.repoFiles.integrationBlank.body")
				case seen[name]:
					return problem(at, "backend.space.repoFiles.integrationDuplicate.body")
				}
				seen[name] = true
			}
			return nil
		},
	}
}

func environmentsRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "Environment names are unique and every environment sets exactly one of `deployed_sha_script` (not blank) or `deployed_sha_run`.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			seen := map[string]bool{}
			for i, e := range f.Environments {
				at := "/environments/" + strconv.Itoa(i)
				inline := strings.TrimSpace(e.DeployedSHAScript) != ""
				switch {
				case seen[e.Name]:
					return problem(at+"/name", "backend.space.repoFiles.envDuplicate.body")
				case inline == (e.DeployedSHARun != nil):
					return problem(at, "backend.space.repoFiles.envScriptOne.body")
				}
				seen[e.Name] = true
			}
			return nil
		},
	}
}

func prepareExclusiveRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "`worktree.prepare_script` and `worktree.prepare_run` are exclusive.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if w := f.Worktree; w != nil && w.PrepareScript != nil && w.PrepareRun != nil {
				return problem("/worktree/prepare_run", "backend.space.repoFiles.prepareBoth.body")
			}
			return nil
		},
	}
}

func scriptRefsRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "A `prepare_run` or `deployed_sha_run` block names a script on one line, at most " + strconv.Itoa(model.MaxScriptRefName) + " characters, and its param names are lowercase letters, digits and underscores, starting with a letter, at most 32 characters.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if f.Worktree != nil {
				if p := refProblem("/worktree/prepare_run", f.Worktree.PrepareRun); p != nil {
					return p
				}
			}
			for i, e := range f.Environments {
				if p := refProblem("/environments/"+strconv.Itoa(i)+"/deployed_sha_run", e.DeployedSHARun); p != nil {
					return p
				}
			}
			return nil
		},
	}
}

func prepareTimeoutRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "`worktree.prepare_timeout` is a duration such as `15m`, above 0 and at most " + model.MaxPrepareTimeout.String() + ".",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if f.Worktree != nil && f.Worktree.PrepareTimeout != nil {
				if _, err := model.ParsePrepareTimeout(*f.Worktree.PrepareTimeout); err != nil {
					return problem("/worktree/prepare_timeout", "backend.space.repoFiles.timeoutInvalid.body")
				}
			}
			return nil
		},
	}
}

func basePathRule() configfs.Rule[File] {
	return configfs.Rule[File]{
		Doc: "`worktree.base_path` is absolute when set.",
		Check: func(f File, _ configfs.Locator) *configfs.Problem {
			if f.Worktree != nil && f.Worktree.BasePath != nil && *f.Worktree.BasePath != "" && !absolutePath(*f.Worktree.BasePath) {
				return problem("/worktree/base_path", "backend.space.repoFiles.basePathAbsolute.body")
			}
			return nil
		},
	}
}
