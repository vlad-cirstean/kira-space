package repocfg

import (
	"path/filepath"
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
		{
			Doc: "`path` is absolute, with no `~` and no `.` or `..` segment.",
			Check: func(f File, _ configfs.Locator) *configfs.Problem {
				if !absolutePath(f.Path) {
					return problem("/path", "backend.space.repoFiles.pathAbsolute.body")
				}
				return nil
			},
		},
		{
			Doc: "`nickname` is at most " + strconv.Itoa(MaxNicknameRunes) + " characters on one line.",
			Check: func(f File, _ configfs.Locator) *configfs.Problem {
				if utf8.RuneCountInString(strings.TrimSpace(f.Nickname)) > MaxNicknameRunes || strings.ContainsAny(strings.TrimSpace(f.Nickname), "\r\n") {
					return problem("/nickname", "backend.space.repoFiles.nicknameLong.body")
				}
				return nil
			},
		},
		{
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
		},
		{
			Doc: "Environment names are unique and every `deployed_sha_script` is not blank.",
			Check: func(f File, _ configfs.Locator) *configfs.Problem {
				seen := map[string]bool{}
				for i, e := range f.Environments {
					at := "/environments/" + strconv.Itoa(i)
					switch {
					case seen[e.Name]:
						return problem(at+"/name", "backend.space.repoFiles.envDuplicate.body")
					case strings.TrimSpace(e.DeployedSHAScript) == "":
						return problem(at+"/deployed_sha_script", "backend.space.repoFiles.envScriptBlank.body")
					}
					seen[e.Name] = true
				}
				return nil
			},
		},
		{
			Doc: "`worktree.prepare_timeout` is a duration such as `15m`, above 0 and at most " + model.MaxPrepareTimeout.String() + ".",
			Check: func(f File, _ configfs.Locator) *configfs.Problem {
				if f.Worktree != nil && f.Worktree.PrepareTimeout != nil {
					if _, err := model.ParsePrepareTimeout(*f.Worktree.PrepareTimeout); err != nil {
						return problem("/worktree/prepare_timeout", "backend.space.repoFiles.timeoutInvalid.body")
					}
				}
				return nil
			},
		},
		{
			Doc: "`worktree.base_path` is absolute when set.",
			Check: func(f File, _ configfs.Locator) *configfs.Problem {
				if f.Worktree != nil && f.Worktree.BasePath != nil && *f.Worktree.BasePath != "" && !absolutePath(*f.Worktree.BasePath) {
					return problem("/worktree/base_path", "backend.space.repoFiles.basePathAbsolute.body")
				}
				return nil
			},
		},
	}
}
