package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitprepare"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/scripts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// envScriptTimeout is the fixed cap on one deploy-sha script (P145 F10).
const envScriptTimeout = 60 * time.Second

var shaLine = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func (b *TaskBoard) scriptRunner() gitprepare.Runner {
	if b.deps.Scripts != nil {
		return b.deps.Scripts
	}
	return gitprepare.NewOSRunner()
}

// runEnvScript runs one environment's script, inline or a referenced Run script, and returns the sha
// it prints: the first stdout line that looks like one. Inline scripts run in the repo root.
func (b *TaskBoard) runEnvScript(ctx context.Context, rec *model.CodeRepo, nick string, env model.AdeRepoEnv, all func() ([]scripts.CustomScript, error)) (string, error) {
	root := rec.Root
	command, dir, extra := env.DeployedShaScript, root, []string(nil)
	if ref := env.DeployedShaRun; ref != nil {
		list, err := all()
		if err != nil {
			return "", fmt.Errorf("Run script %q: %w", ref.Script, err)
		}
		vars := map[string]string{"repo": nick, "worktree": root, "base": b.mainShortName(ctx, rec.ID)}
		r, err := b.resolveScriptRef(list, repoFileUse, ref.Script, ref.Values(), vars, root)
		if err != nil {
			return "", fmt.Errorf("Run script %q: %w", ref.Script, err)
		}
		command, dir, extra = r.script.Command, r.dir, r.env
	}
	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	res, err := b.scriptRunner().Run(ctx, gitprepare.Spec{
		Shell: shell, LoginShell: login, Script: command, Dir: dir, Timeout: envScriptTimeout,
		Env: append(gitprepare.BuildEnv(os.Environ(), gitprepare.Vars{WorktreePath: root, RepoRoot: root}), extra...),
	})
	switch {
	case err != nil:
		return "", err
	case res.TimedOut:
		return "", fmt.Errorf("the script %s", runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndTimeout, Timeout: envScriptTimeout.String()}).Reason)
	case res.Cancelled:
		return "", errors.New("the script was cancelled")
	case res.ExitCode != 0:
		return "", fmt.Errorf("the script %s", runoutcome.ForProcess(runoutcome.Process{ExitCode: res.ExitCode}).Reason)
	}
	for _, l := range res.Output {
		if l.Stream != "stdout" {
			continue
		}
		if t := strings.TrimSpace(l.Text); shaLine.MatchString(t) {
			return t, nil
		}
	}
	return "", errors.New("the script printed no commit sha")
}

// RunEnvScripts runs every environment script of one repo, sequentially, and stores each result.
func (b *TaskBoard) RunEnvScripts(ctx context.Context, codeRepoID string) error {
	if b.deps.Facts == nil {
		return nil
	}
	// A pass that starts after a config edit reads the new config, so the last pass never stores a stale result.
	mu, _ := b.envScriptMus.LoadOrStore(codeRepoID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	cfg, err := b.repoConfig(codeRepoID)
	if err != nil {
		return err
	}
	if len(cfg.Environments) == 0 {
		return nil
	}
	rec, err := b.deps.CodeRepos.Get(codeRepoID)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("ade: code repo %s not found", codeRepoID)
	}
	var (
		listed  []scripts.CustomScript
		listErr error
		loaded  bool
	)
	all := func() ([]scripts.CustomScript, error) {
		if !loaded {
			listed, listErr = b.listScripts()
			loaded = true
		}
		return listed, listErr
	}
	for _, env := range cfg.Environments {
		st := model.AdeEnvState{CodeRepoID: codeRepoID, Env: env.Name, CheckedAt: b.deps.Now().UnixMilli()}
		sha, err := b.runEnvScript(ctx, rec, cmpNonEmpty(cfg.Nickname, cfg.Name), env, all)
		if err != nil {
			st.Error = err.Error()
		} else {
			st.Sha = sha
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := b.deps.Facts.SetEnvState(st); err != nil {
			return err
		}
	}
	return nil
}

// RunAllEnvScripts runs the scripts of every repo a live task uses, four repos at a time, then
// refreshes the board.
func (b *TaskBoard) RunAllEnvScripts(ctx context.Context) {
	branches, err := b.deps.Tasks.BranchesLive()
	if err != nil {
		slog.Warn("ade env scripts: list branches", "scope", "ade", "err", err)
		return
	}
	ids, _ := branchesByRepo(branches)
	var g errgroup.Group
	g.SetLimit(boardFanOut)
	for _, id := range ids {
		g.Go(func() error {
			if err := b.RunEnvScripts(ctx, id); err != nil {
				slog.Warn("ade env scripts", "scope", "ade", "repo", id, "err", err)
			}
			return nil
		})
	}
	_ = g.Wait()
	if ctx.Err() == nil {
		b.notifyBoard()
	}
}

// refreshEnvScripts reruns one repo's scripts after its environment list changed.
func (b *TaskBoard) refreshEnvScripts(codeRepoID string) {
	if err := b.RunEnvScripts(b.ctx, codeRepoID); err != nil {
		slog.Warn("ade env scripts", "scope", "ade", "repo", codeRepoID, "err", err)
	}
	if b.ctx.Err() == nil {
		b.notifyBoard()
	}
}

// resolveCommit resolves a stored sha in this clone; only found shas are cached (a later fetch may
// bring a missing one).
func (sc *boardCtx) resolveCommit(ctx context.Context, sha string) (string, bool, error) {
	if full, ok := sc.caches.commits.Get(sha); ok {
		return full, true, nil
	}
	full, ok, err := sc.entry.ResolveCommit(ctx, sha)
	if err != nil || !ok {
		return "", false, err
	}
	sc.caches.commits.Add(sha, full)
	return full, true, nil
}

// deploymentFacts is one `mine` branch's row per environment that has reported a state.
func (b *TaskBoard) deploymentFacts(ctx context.Context, sc *boardCtx, sb model.AdeTaskBranch, baseTip, tip string) []adewire.Deployment {
	out := make([]adewire.Deployment, 0, len(sc.envs))
	for _, env := range sc.envs {
		st, ok := sc.envState[env.Name]
		if !ok {
			continue
		}
		d, err := b.deploymentRow(ctx, sc, sb, baseTip, tip, env.Name, st)
		if err != nil {
			slog.Warn("ade board: deployment", "scope", "ade", "repo", sc.repoID, "branch", sb.Name, "env", env.Name, "err", err)
			d = adewire.Deployment{Env: env.Name, DeployedSha: st.Sha, CheckedAt: st.CheckedAt, Status: "unknown", Error: i18n.T("backend.space.ade.unchecked.body", nil)}
		}
		out = append(out, d)
	}
	return out
}

func (b *TaskBoard) deploymentRow(ctx context.Context, sc *boardCtx, sb model.AdeTaskBranch, baseTip, tip, env string, st model.AdeEnvState) (adewire.Deployment, error) {
	d := adewire.Deployment{Env: env, DeployedSha: st.Sha, CheckedAt: st.CheckedAt, Status: "not deployed"}
	if st.Error != "" {
		d.Status, d.Error = "unknown", st.Error
		return d, nil
	}
	full, ok, err := sc.resolveCommit(ctx, st.Sha)
	if err != nil {
		return d, err
	}
	if !ok {
		d.Status, d.Error = "unknown", fmt.Sprintf("deployed SHA %s is not in this clone", gitclient.ShortSHA(st.Sha))
		return d, nil
	}
	d.DeployedSha = full
	short := gitclient.ShortSHA(full)
	mark, hasMark := sc.markFor(sb.ID, "env", env)

	if hasMark && mark.MergedTip != tip {
		descends, err := sc.entry.IsAncestor(ctx, mark.MergedTip, tip)
		if err != nil {
			return d, err
		}
		inEnv, err := sc.entry.IsAncestor(ctx, tip, full)
		if err != nil {
			return d, err
		}
		if !descends && !inEnv {
			d.Status, d.Note = "stale", fmt.Sprintf("%s runs %s: rebased since it was deployed", env, short)
			return d, nil
		}
	}
	c, err := contains(ctx, sc.entry, sc.caches, baseTip, tip, full)
	if err != nil {
		return d, err
	}
	d.MissingCommits = c.missing
	switch {
	case c.total == 0:
		return d, nil
	case c.state == containAll:
		d.MissingCommits = 0
		d.Status, d.Note = "deployed", fmt.Sprintf("runs %s, contains this branch", short)
		if !hasMark || mark.MergedTip != tip {
			b.saveMark(sc, sb.ID, "env", env, tip)
		}
		return d, nil
	}
	back, err := b.movedBack(ctx, sc, baseTip, tip, full, st.PrevSha)
	if err != nil {
		return d, err
	}
	switch {
	case back:
		d.Status, d.Note = "stale", fmt.Sprintf("%s moved back to %s", env, short)
	case c.state == containSome:
		d.Status, d.Note = "stale", missingNote(env, short, c.missing)
	case hasMark:
		d.Status = "stale"
		since, err := sc.entry.CountRange(ctx, mark.MergedTip, tip)
		if err != nil {
			return d, err
		}
		if since > 0 {
			d.MissingCommits = since
			d.Note = missingNote(env, short, since)
		} else {
			d.Note = fmt.Sprintf("%s runs %s, which no longer contains this branch", env, short)
		}
	}
	return d, nil
}

// movedBack: the previous deployed sha held the branch, the current one does not and is older.
func (b *TaskBoard) movedBack(ctx context.Context, sc *boardCtx, baseTip, tip, current, prevSha string) (bool, error) {
	if prevSha == "" {
		return false, nil
	}
	prev, ok, err := sc.resolveCommit(ctx, prevSha)
	if err != nil || !ok || prev == current {
		return false, err
	}
	pc, err := contains(ctx, sc.entry, sc.caches, baseTip, tip, prev)
	if err != nil || pc.state != containAll {
		return false, err
	}
	return sc.entry.IsAncestor(ctx, current, prev)
}

// missingNote is the stale-deploy note; the verb agrees with the count (mockup: "is missing" for one).
func missingNote(env, short string, n int) string {
	verb := "are"
	if n == 1 {
		verb = "is"
	}
	return fmt.Sprintf("%s runs %s: %s of this branch %s missing", env, short, plural(n, "commit", "commits"), verb)
}
