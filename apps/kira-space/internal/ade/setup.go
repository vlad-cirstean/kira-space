package ade

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitclient/porcelain"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitprepare"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitsession"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/oplog"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
	"github.com/kirathecat/kira-studio/internal/loginshell"
	"github.com/kirathecat/kira-studio/internal/runoutcome"
	"github.com/kirathecat/kira-studio/internal/secrets"
)

// setup.go creates the worktrees a run needs and runs each repo's prepare script in a new one. The
// gate (setupReady) keeps a branch's runs waiting until the script finished (SPEC2 section 6.1).

const maxBranchSlug = 40

// slug lowercases s to [a-z0-9-], collapsed, trimmed and cut to maxBranchSlug.
func slug(s string) string {
	var sb strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			dash = false
		} else if !dash {
			sb.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(sb.String(), "-")
	if len(out) > maxBranchSlug {
		out = strings.Trim(out[:maxBranchSlug], "-")
	}
	if out == "" {
		return "task"
	}
	return out
}

// pathSegment makes a repo name safe as one path segment.
func pathSegment(s string) string {
	s = strings.NewReplacer("/", "-", "\\", "-", "\x00", "").Replace(s)
	if s == "" || s == "." || s == ".." {
		return "repo"
	}
	return s
}

// taskTitle mirrors labels.ts taskTitle: title, else Jira key, else first branch name, else a default.
func taskTitle(t model.AdeTask, branches []model.AdeTaskBranch) string {
	switch {
	case t.Title != "":
		return t.Title
	case t.JiraKey != "":
		return t.JiraKey
	}
	for _, br := range branches {
		if br.Name != "" {
			return br.Name
		}
	}
	return "New task"
}

func localBranch(inv []porcelain.InventoryRef, short string) (porcelain.InventoryRef, bool) {
	for _, r := range inv {
		if r.Remote == "" && r.Short == short {
			return r, true
		}
	}
	return porcelain.InventoryRef{}, false
}

// remoteBranch finds the remote-tracking ref for short, preferring origin.
func remoteBranch(inv []porcelain.InventoryRef, short string) (porcelain.InventoryRef, bool) {
	var found porcelain.InventoryRef
	ok := false
	for _, r := range inv {
		if r.Remote != "" && r.Short == short && (!ok || r.Remote == "origin") {
			found, ok = r, true
		}
	}
	return found, ok
}

func branchExists(inv []porcelain.InventoryRef, short string) bool {
	for _, r := range inv {
		if r.Short == short {
			return true
		}
	}
	return false
}

// worktreeResult is ensureWorktree's outcome: Fresh is true when this call created the worktree.
type worktreeResult struct {
	Path  string
	Name  string
	Fresh bool
}

// ensureWorktree makes the branch's worktree exist: a branch checked out anywhere, the repo root
// included, runs there as is; a created branch without one gets an existingBranch worktree (a
// remote-only one a newBranch tracking it); a not-created branch gets its name, then a newBranch
// worktree from its base. naming says how a not-created branch gets its name.
func (b *TaskBoard) ensureWorktree(ctx context.Context, rec model.CodeRepo, sb model.AdeTaskBranch, naming newBranchName) (worktreeResult, error) {
	if err := b.checkNotArchiving(sb.TaskID); err != nil {
		return worktreeResult{}, err
	}
	mu := b.repoMutex(sb.CodeRepoID)
	mu.Lock()
	defer mu.Unlock()
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return worktreeResult{}, err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return worktreeResult{}, err
	}
	if sb.Name != "" {
		if row, ok := localBranch(inv, sb.Name); ok && row.WorktreePath != "" {
			return worktreeResult{Path: row.WorktreePath, Name: sb.Name}, nil
		}
		path, err := b.worktreeDirFor(sb, rec.Name, "")
		if err != nil {
			return worktreeResult{}, err
		}
		req := gitsession.OpRequest{Kind: "worktreeAdd", Mode: "existingBranch", Path: path, Branch: sb.Name}
		if _, local := localBranch(inv, sb.Name); !local {
			// Remote-only: git cannot resolve the bare short name, so branch it off the tracking ref.
			if rb, ok := remoteBranch(inv, sb.Name); ok {
				req.Mode, req.StartPoint = "newBranch", rb.Remote+"/"+rb.Short
			}
		}
		if err := b.addWorktree(ctx, entry, req); err != nil {
			return worktreeResult{}, err
		}
		return worktreeResult{Path: path, Name: sb.Name, Fresh: true}, nil
	}

	start, err := b.startPoint(ctx, entry, sb)
	if err != nil {
		return worktreeResult{}, err
	}
	name, err := b.claimBranchName(sb, naming, inv)
	if err != nil {
		return worktreeResult{}, err
	}
	path, err := b.worktreeDirFor(sb, rec.Name, "")
	if err == nil {
		req := gitsession.OpRequest{Kind: "worktreeAdd", Mode: "newBranch", Path: path, Branch: name, StartPoint: start}
		err = b.addWorktree(ctx, entry, req)
	}
	if err != nil {
		if rerr := b.deps.Tasks.SetBranchName(sb.ID, ""); rerr != nil {
			slog.Warn("ade: release branch name", "scope", "ade", "branch", sb.ID, "err", rerr)
		}
		return worktreeResult{}, err
	}
	return worktreeResult{Path: path, Name: name, Fresh: true}, nil
}

// newBranchName is how a not-created branch gets its name: wanted as given, else derived from title.
type newBranchName struct{ title, wanted string }

// claimBranchName stores the branch's name before git runs, so the board shows it. An explicit name
// that already exists is refused; a derived one gets -2, -3... until free.
func (b *TaskBoard) claimBranchName(sb model.AdeTaskBranch, n newBranchName, inv []porcelain.InventoryRef) (string, error) {
	if n.wanted != "" {
		if branchExists(inv, n.wanted) {
			return "", invalid("branch %s already exists", n.wanted)
		}
		if err := b.deps.Tasks.SetBranchName(sb.ID, n.wanted); err != nil {
			return "", branchNameErr(err, n.wanted)
		}
		return n.wanted, nil
	}
	base := "feat/" + slug(n.title)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		if branchExists(inv, name) {
			continue
		}
		err := b.deps.Tasks.SetBranchName(sb.ID, name)
		if err == nil {
			return name, nil
		}
		if !errors.Is(err, repos.ErrBranchNameTaken) {
			return "", err
		}
	}
}

func branchNameErr(err error, name string) error {
	if errors.Is(err, repos.ErrBranchNameTaken) {
		return invalid("branch %s is already on a task", name)
	}
	return err
}

// newBranchBase resolves where a new branch of sb starts. full is the ref git takes ("" = the repo's
// default start), display the base's name for the user ("" when it is the repo's main branch). A base that
// is not created yet or no longer exists comes back as refusal text.
func (b *TaskBoard) newBranchBase(ctx context.Context, entry *gitsession.RepoEntry, sb model.AdeTaskBranch) (full, display, refusal string, err error) {
	sc, err := b.baseCtxFor(ctx, entry, sb.CodeRepoID)
	if err != nil {
		return "", "", "", err
	}
	br := sc.resolveBase(sb)
	switch br.reason {
	case baseParentDraft:
		return "", "", fmt.Sprintf("base %s is not created yet: start its task first", cmpNonEmpty(sc.byID[sb.BaseBranchID].Name, "branch")), nil
	case baseMissing:
		return "", "", fmt.Sprintf("base %s no longer exists", br.name), nil
	}
	if !br.ok {
		return "", "", "", nil
	}
	if br.name != sc.mainName {
		display = br.name
	}
	return br.full, display, "", nil
}

// startPoint is the ref a new branch starts from: its resolved base (remote-first), else the repo's
// main. A base that is not created yet or no longer exists refuses the start.
func (b *TaskBoard) startPoint(ctx context.Context, entry *gitsession.RepoEntry, sb model.AdeTaskBranch) (string, error) {
	full, _, refusal, err := b.newBranchBase(ctx, entry, sb)
	if err != nil {
		return "", err
	}
	if refusal != "" {
		return "", invalid("%s", refusal)
	}
	return full, nil
}

func (b *TaskBoard) addWorktree(ctx context.Context, entry *gitsession.RepoEntry, req gitsession.OpRequest) error {
	res, err := entry.RunOp(ctx, b.conn.ID, boardConnLabel, req)
	if err != nil {
		return err
	}
	if !res.OK {
		msg := "git refused"
		if res.Error != nil {
			msg = res.Error.Message
		}
		return invalid("worktree for %s: %s", req.Branch, msg)
	}
	return nil
}

// worktreeOf is the path where the branch is checked out, "" when nowhere.
func (b *TaskBoard) worktreeOf(ctx context.Context, sb model.AdeTaskBranch) (string, error) {
	if sb.Name == "" {
		return "", nil
	}
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return "", err
	}
	inv, err := entry.BranchInventory(ctx)
	if err != nil {
		return "", err
	}
	if row, ok := localBranch(inv, sb.Name); ok {
		return row.WorktreePath, nil
	}
	return "", nil
}

// setupReady is the gate: the branch's setup row is ready, or it has none and its worktree exists.
func (b *TaskBoard) setupReady(sb model.AdeTaskBranch, worktree string, setups map[string]model.AdeWorktreeSetup) bool {
	if s, ok := setups[sb.ID]; ok {
		return s.State == model.AdeSetupReady
	}
	return worktree != ""
}

// startSetup records the setup and, when the repo has a prepare script, runs it in the background.
// onReady is called (off the caller's goroutine) after a successful script; a repo without one is
// ready at once and onReady is not called.
func (b *TaskBoard) startSetup(rec model.CodeRepo, sb model.AdeTaskBranch, name, path string, onReady func(branchID string)) error {
	if err := b.checkNotArchiving(sb.TaskID); err != nil {
		return err
	}
	settings, err := b.deps.GitRepoSettings(rec.RepoID)
	if err != nil {
		return err
	}
	ref := settings.WorktreePrepareRun
	if settings.WorktreePrepareScript == "" && ref == nil {
		if err := b.deps.Logs.Reset(repos.AdeLogSetup, sb.ID, sb.TaskID); err != nil {
			return err
		}
		now := b.deps.Now().UnixMilli()
		sink := b.newLogSink(repos.AdeLogSetup, sb.ID, sb.TaskID)
		zero := 0
		sink.add(logEvent, "no prepare script configured")
		sink.flush()
		defer b.notifyBoard()
		return b.deps.Tasks.UpsertSetup(model.AdeWorktreeSetup{BranchID: sb.ID, State: model.AdeSetupReady, StartedAt: now, FinishedAt: &now, ExitCode: &zero})
	}
	// Claim before the log reset: a busy retry must not wipe the running or just-failed setup's log.
	sctx, endSetup, ok := b.claimSetup(sb.ID)
	if !ok {
		return invalid("a setup is already running for this branch")
	}
	script := setupScript{command: settings.WorktreePrepareScript, dir: path, label: settings.WorktreePrepareScript}
	if ref != nil {
		var err error
		if script, err = b.refSetupScript(sb, path, ref); err != nil {
			endSetup()
			b.recordSetupFailure(sb, err.Error())
			return nil
		}
	}
	if err := b.deps.Logs.Reset(repos.AdeLogSetup, sb.ID, sb.TaskID); err != nil {
		endSetup()
		return err
	}
	if !b.track() {
		endSetup()
		return errBoardClosed
	}
	now := b.deps.Now().UnixMilli()
	sink := b.newLogSink(repos.AdeLogSetup, sb.ID, sb.TaskID)
	if err := b.deps.Tasks.UpsertSetup(model.AdeWorktreeSetup{BranchID: sb.ID, State: model.AdeSetupRunning, StartedAt: now}); err != nil {
		endSetup()
		b.wg.Done()
		return err
	}
	b.notifyBoard()
	timeout, timeoutText := gitsession.ParsePrepareTimeout(settings.WorktreePrepareTimeout)
	go func() {
		defer b.wg.Done()
		defer endSetup()
		b.runSetup(sctx, rec, sb, name, path, script, timeout, timeoutText, sink, now, onReady)
	}()
	return nil
}

// claimSetup registers the branch's running prepare script; ok is false when one already runs.
func (b *TaskBoard) claimSetup(branchID string) (context.Context, func(), bool) {
	b.runMu.Lock()
	defer b.runMu.Unlock()
	if _, busy := b.setupLive[branchID]; busy {
		return nil, nil, false
	}
	ctx, end := b.beginLiveLocked(b.setupLive, branchID)
	return ctx, end, true
}

// setupScript is what a setup runs: the shell text, the folder, the extra env and the mask of a
// referenced Run script's secret values.
type setupScript struct {
	command, dir, label, ref string
	env                      []string
	mask                     *secrets.Masker
}

// refSetupScript resolves the Run script a repo file names as the prepare hook. The error text is
// the failed setup's note.
func (b *TaskBoard) refSetupScript(sb model.AdeTaskBranch, path string, ref *model.ScriptRef) (setupScript, error) {
	fail := func(reason string) (setupScript, error) {
		return setupScript{}, fmt.Errorf("Run script %q: %s", ref.Script, reason)
	}
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return fail(err.Error())
	}
	all, err := b.listScripts()
	if err != nil {
		return fail(err.Error())
	}
	res, err := b.resolveScriptRef(all, repoFileUse, ref.Script, ref.Values(), b.adeVars(tc, sb, path), path)
	if err != nil {
		return fail(err.Error())
	}
	return setupScript{command: res.script.Command, dir: res.dir, label: res.script.Command, env: res.env, mask: secrets.NewMasker(res.secrets...), ref: ref.Script}, nil
}

func (b *TaskBoard) runSetup(ctx context.Context, rec model.CodeRepo, sb model.AdeTaskBranch, name, path string, script setupScript, timeout time.Duration, timeoutText string, sink *logSink, startedAt int64, onReady func(string)) {
	if script.mask != nil {
		sink.setMask(script.mask)
	}
	if script.ref != "" {
		sink.add(logEvent, fmt.Sprintf("running Run script %q", script.ref))
	}
	op := b.startSessionOp(ctx, opSetup, sb.TaskID, "", script.label)
	op.SetCancel(func() bool { return b.cancelLive(b.setupLive, sb.ID, errStopped) != nil })
	commonDir := rec.Root
	if entry, err := b.openRepo(ctx, sb.CodeRepoID); err == nil {
		commonDir = entry.Summary.CommonDir
	}
	shell, login := loginshell.ResolveShell(os.Getenv, loginshell.IsExecutableFile)
	res, err := b.scriptRunner().Run(ctx, gitprepare.Spec{
		Shell: shell, LoginShell: login, Script: script.command, Dir: script.dir, Timeout: timeout,
		Env: append(gitprepare.BuildEnv(os.Environ(), gitprepare.Vars{WorktreePath: path, WorktreeBranch: name, RepoRoot: rec.Root, RepoCommonDir: commonDir}), script.env...),
		OnBatch: func(lines []gitprepare.Line) {
			for _, l := range lines {
				sink.add(l.Stream, l.Text)
			}
		},
	})
	state, exit, note := model.AdeSetupReady, res.ExitCode, ""
	fail := func(text string) {
		state, note = model.AdeSetupFailed, text
		sink.add(logEvent, text)
	}
	switch {
	case res.Cancelled && err == nil && b.ctx.Err() != nil:
		sink.flush()
		op.Finish(oplog.StatusCancelled, "")
		return // app quit: the row stays running (R18)
	case res.Cancelled && err == nil:
		fail("cancelled: " + context.Cause(ctx).Error())
	case err != nil:
		fail(runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndStartErr, Err: err}).Reason)
	case res.TimedOut:
		fail(runoutcome.ForProcess(runoutcome.Process{End: runoutcome.EndTimeout, Timeout: timeoutText}).Reason)
	case res.ExitCode != 0:
		fail(runoutcome.ForProcess(runoutcome.Process{ExitCode: res.ExitCode}).Reason)
	}
	sink.flush()
	switch {
	case res.Cancelled:
		op.Finish(oplog.StatusCancelled, "")
	case state == model.AdeSetupFailed:
		op.Finish(oplog.StatusError, note)
	default:
		op.Finish(oplog.StatusOK, "")
	}
	finished := b.deps.Now().UnixMilli()
	row := model.AdeWorktreeSetup{BranchID: sb.ID, State: state, StartedAt: startedAt, FinishedAt: &finished, ExitCode: &exit, Note: note}
	if err := b.deps.Tasks.UpsertSetup(row); err != nil {
		slog.Warn("ade: record setup", "scope", "ade", "branch", sb.ID, "err", err)
		return
	}
	if state == model.AdeSetupFailed {
		b.setPendingNote(sb.ID, noteSetupFailed)
	}
	b.notifyBoard()
	if state == model.AdeSetupReady && onReady != nil {
		onReady(sb.ID)
	}
}

// recordSetupFailure stores a failed setup that never reached the script (the worktree could not be
// created), so the card, the panel and Needs you show why.
func (b *TaskBoard) recordSetupFailure(sb model.AdeTaskBranch, note string) {
	if err := b.deps.Logs.Reset(repos.AdeLogSetup, sb.ID, sb.TaskID); err != nil {
		slog.Warn("ade: reset setup log", "scope", "ade", "branch", sb.ID, "err", err)
	}
	sink := b.newLogSink(repos.AdeLogSetup, sb.ID, sb.TaskID)
	sink.add(logEvent, note)
	sink.flush()
	now := b.deps.Now().UnixMilli()
	if err := b.deps.Tasks.UpsertSetup(model.AdeWorktreeSetup{BranchID: sb.ID, State: model.AdeSetupFailed, StartedAt: now, FinishedAt: &now, Note: note}); err != nil {
		slog.Warn("ade: record setup failure", "scope", "ade", "branch", sb.ID, "err", err)
		return
	}
	b.setPendingNote(sb.ID, noteSetupFailed)
	b.notifyBoard()
}
