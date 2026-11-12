package agentmcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const (
	gitTimeout   = 2 * time.Minute
	maxGitOutput = 64 << 10
)

// GitGrant lets a run call the git tool in the listed worktrees, and only there. Push allows
// `git push --force-with-lease` of named branches.
type GitGrant struct {
	Worktrees []string
	Push      bool
}

type gitArgs struct {
	Worktree string   `json:"worktree"`
	Args     []string `json:"args"`
}

var (
	gitRev  = regexp.MustCompile(`^[A-Za-z0-9_@][A-Za-z0-9._/@^~{}:+*-]*$`)
	gitName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/-]*$`)
	// gitRemote is a configured remote's name: no slash, so never a path.
	gitRemote = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
)

// posKind says what a command's non-option words are.
type posKind int

const (
	posNone posKind = iota
	posRev          // revisions before "--", paths after
	posPath         // paths, with or without "--"
)

// gitCmd is the exact shape of one allowed read or index command. An option is allowed only when it
// equals a flag or fully matches a pattern: git also accepts abbreviated long options, so a deny
// list on spellings cannot work, and anything unlisted (--output, --upload-pack, --exec, -c, ...)
// is refused.
type gitCmd struct {
	flags    []string
	patterns []*regexp.Regexp
	pos      posKind
	// plain adds --no-ext-diff and --no-textconv, so no configured program runs.
	plain bool
}

var logFlags = []string{
	"--oneline", "--graph", "--decorate", "--no-color", "--name-only", "--name-status", "--stat", "--summary",
	"--first-parent", "--merge", "--left-right", "--cherry-pick", "--cherry", "--reverse", "--no-merges",
	"--merges", "--all", "-p", "--patch", "-w", "--no-patch",
}

var logPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^-n?\d{1,4}$`),
	regexp.MustCompile(`^--max-count=\d{1,4}$`),
	regexp.MustCompile(`^--(pretty|format)=[^\x00]{0,200}$`),
	regexp.MustCompile(`^--diff-filter=[A-Za-z]{1,10}$`),
	regexp.MustCompile(`^-U\d{1,3}$`),
	regexp.MustCompile(`^--abbrev=\d{1,2}$`),
}

var gitCmds = map[string]gitCmd{
	"status": {flags: []string{"-s", "--short", "-b", "--branch", "--porcelain", "--porcelain=v1", "-uno", "-unormal", "--untracked-files=no"}, pos: posPath},
	"diff": {
		flags:    append([]string{"--cached", "--staged", "--check", "--ours", "--theirs", "--base", "--no-renames"}, logFlags...),
		patterns: logPatterns, pos: posRev, plain: true,
	},
	"log":  {flags: logFlags, patterns: logPatterns, pos: posRev, plain: true},
	"show": {flags: logFlags, patterns: logPatterns, pos: posRev, plain: true},
	"rev-parse": {flags: []string{
		"--verify", "-q", "--quiet", "--abbrev-ref", "--short", "--symbolic-full-name", "--show-toplevel", "--git-dir", "--is-inside-work-tree",
	}, pos: posRev},
	"merge-base": {flags: []string{"--is-ancestor", "--fork-point", "--all", "--octopus"}, pos: posRev},
	"ls-files": {flags: []string{
		"-u", "--unmerged", "-m", "--modified", "-d", "--deleted", "-o", "--others", "-s", "--stage", "-c", "--cached", "--exclude-standard",
	}, pos: posPath},
	"add": {flags: []string{"-u", "--update", "-A", "--all"}, pos: posPath},
	"rm":  {flags: []string{"--cached", "-r", "-q", "--quiet", "--ignore-unmatch"}, pos: posPath},
}

// CheckGitArgs reports why the git tool refuses args (the words after git); nil means they match an
// allowed shape exactly. push says whether `push --force-with-lease` is allowed.
func CheckGitArgs(args []string, push bool) error {
	if len(args) == 0 {
		return errors.New("args is empty")
	}
	for _, a := range args {
		if strings.ContainsRune(a, 0) {
			return errors.New("args hold a NUL byte")
		}
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "rebase":
		return checkRebase(rest)
	case "checkout":
		return checkCheckout(rest)
	case "fetch":
		return checkFetch(rest)
	case "push":
		if !push {
			return errors.New("push is not allowed in this run")
		}
		return checkPush(rest)
	}
	c, ok := gitCmds[sub]
	if !ok {
		return fmt.Errorf("git %s is not allowed", sub)
	}
	return c.check(sub, rest)
}

func (c gitCmd) allows(opt string) bool {
	if slices.Contains(c.flags, opt) {
		return true
	}
	for _, p := range c.patterns {
		if p.MatchString(opt) {
			return true
		}
	}
	return false
}

func (c gitCmd) check(sub string, rest []string) error {
	paths := false
	for _, a := range rest {
		switch {
		case paths:
			if a == "" {
				return errors.New("a path is empty")
			}
		case a == "--":
			if c.pos == posNone {
				return fmt.Errorf("git %s takes no paths", sub)
			}
			paths = true
		case strings.HasPrefix(a, "-"):
			if !c.allows(a) {
				return fmt.Errorf("git %s: option %q is not allowed", sub, a)
			}
		case c.pos == posNone:
			return fmt.Errorf("git %s: unexpected argument %q", sub, a)
		case c.pos == posRev && !gitRev.MatchString(a):
			return fmt.Errorf("git %s: %q is not a revision; put paths after a \"--\" entry", sub, a)
		case c.pos == posPath && a == "":
			return errors.New("a path is empty")
		}
	}
	return nil
}

var rebaseModes = []string{"--continue", "--skip", "--abort", "--quit"}

var rebaseFlags = []string{
	"-q", "--quiet", "--autostash", "--no-autostash", "--keep-base", "--rebase-merges", "--no-rebase-merges",
	"--update-refs", "--no-update-refs", "--no-verify", "--reapply-cherry-picks", "--no-reapply-cherry-picks",
}

var rebaseEmpty = regexp.MustCompile(`^--empty=(drop|keep|stop)$`)

func checkRebase(rest []string) error {
	if len(rest) == 1 && slices.Contains(rebaseModes, rest[0]) {
		return nil
	}
	var revs int
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case a == "--onto":
			i++
			if i == len(rest) || !gitRev.MatchString(rest[i]) {
				return errors.New("git rebase: --onto needs a revision")
			}
		case strings.HasPrefix(a, "--onto="):
			if !gitRev.MatchString(strings.TrimPrefix(a, "--onto=")) {
				return errors.New("git rebase: --onto needs a revision")
			}
		case strings.HasPrefix(a, "-"):
			if !slices.Contains(rebaseFlags, a) && !rebaseEmpty.MatchString(a) {
				return fmt.Errorf("git rebase: option %q is not allowed", a)
			}
		case gitRev.MatchString(a):
			revs++
		default:
			return fmt.Errorf("git rebase: %q is not a revision", a)
		}
	}
	if revs == 0 || revs > 2 {
		return errors.New("git rebase: give an upstream and optionally a branch")
	}
	return nil
}

func checkCheckout(rest []string) error {
	if len(rest) < 2 || (rest[0] != "--ours" && rest[0] != "--theirs") {
		return errors.New("git checkout is allowed only as: checkout --ours|--theirs -- <path>...")
	}
	paths, dashed := rest[1:], rest[1] == "--"
	if dashed {
		paths = paths[1:]
	}
	if len(paths) == 0 {
		return errors.New("git checkout: no path")
	}
	for _, p := range paths {
		if p == "" || (!dashed && strings.HasPrefix(p, "-")) {
			return fmt.Errorf("git checkout: put paths after a \"--\" entry (%q)", p)
		}
	}
	return nil
}

func checkFetch(rest []string) error {
	var words []string
	for _, a := range rest {
		switch {
		case a == "-q" || a == "--quiet" || a == "--prune" || a == "-p" || a == "--no-tags":
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("git fetch: option %q is not allowed", a)
		default:
			words = append(words, a)
		}
	}
	if len(words) == 0 || !gitRemote.MatchString(words[0]) {
		return errors.New("git fetch: name a configured remote")
	}
	for _, w := range words[1:] {
		if !gitName.MatchString(w) {
			return fmt.Errorf("git fetch: %q is not a branch name", w)
		}
	}
	return nil
}

func checkPush(rest []string) error {
	var lease bool
	var words []string
	for _, a := range rest {
		switch {
		case a == "--force-with-lease":
			lease = true
		case a == "-q" || a == "--quiet" || a == "-u" || a == "--set-upstream":
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("git push: option %q is not allowed", a)
		default:
			words = append(words, a)
		}
	}
	if !lease {
		return errors.New("git push needs --force-with-lease")
	}
	if len(words) < 2 || !gitRemote.MatchString(words[0]) {
		return errors.New("git push: give a configured remote and at least one branch")
	}
	for _, w := range words[1:] {
		for _, ref := range strings.Split(w, ":") {
			if !gitName.MatchString(ref) || (strings.HasPrefix(ref, "refs/") && !strings.HasPrefix(ref, "refs/heads/")) {
				return fmt.Errorf("git push: %q is not a branch", w)
			}
		}
		if strings.Count(w, ":") > 1 {
			return fmt.Errorf("git push: %q is not a branch", w)
		}
	}
	return nil
}

// GitRunner runs one checked git command in a worktree through the app's shared git runner.
// argv is the subcommand and its arguments (no -C, no --no-pager: the runner adds them). A non-zero
// exit is reported through exitCode with a nil err; err means git did not run to completion.
type GitRunner interface {
	RunGit(ctx context.Context, dir string, argv []string) (output string, exitCode int, err error)
}

// gitArgv is the argv after `git` for a checked args.
func gitArgv(args []string) []string {
	out := []string{args[0]}
	if c, ok := gitCmds[args[0]]; ok && c.plain {
		out = append(out, "--no-ext-diff", "--no-textconv")
	}
	return append(out, args[1:]...)
}

func (s *Server) addGitTool(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "git",
		Description: llmprompts.HeadlessToolGit.Text(s.prompts, llmprompts.NoVars{}),
		InputSchema: llmprompts.RequireNonEmpty(llmprompts.InputSchema[gitArgs](s.prompts, llmprompts.HeadlessToolGit.ID(), nil), "worktree", "args"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, a gitArgs) (*mcp.CallToolResult, any, error) {
		var g Grant
		var ok bool
		if req.Extra != nil {
			g, ok = s.grant(req.Extra.TokenInfo)
		}
		if !ok || g.Git == nil {
			return toolError("The git tool is not available for this session."), nil, nil
		}
		wt := filepath.Clean(a.Worktree)
		if !slices.Contains(g.Git.Worktrees, wt) {
			return toolError(fmt.Sprintf("worktree must be one of: %s", strings.Join(g.Git.Worktrees, ", "))), nil, nil
		}
		if err := CheckGitArgs(a.Args, g.Git.Push); err != nil {
			return toolError(err.Error()), nil, nil
		}
		ctx, done := s.callCtx(ctx, req)
		defer done()
		runCtx, cancel := context.WithTimeout(ctx, gitTimeout)
		defer cancel()
		out, code, err := s.git.RunGit(runCtx, wt, gitArgv(a.Args))
		if len(out) > maxGitOutput {
			out = out[:maxGitOutput] + "\n[output truncated]"
		}
		if err == nil && code != 0 {
			err = fmt.Errorf("exit status %d", code)
		}
		if err != nil {
			return toolError(fmt.Sprintf("git %s failed: %v\n%s", a.Args[0], err, out)), nil, nil
		}
		if out == "" {
			out = "ok"
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
	})
}
