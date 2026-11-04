package ade

import (
	"context"
	"log/slog"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// reviewStepID is the id of the manual review step; ▶ Review there opens the review agent.
const reviewStepID = "review"

// launchReviewAgent is the one launcher of a task's review agent: a new conversation, or the
// stopped one resumed with fresh --add-dir directories. message is the user's first message ("" =
// none on a resume, the composed review message on a new launch). step, when set, adds its prompt
// to the composed message. The task mutex is held.
func (b *TaskBoard) launchReviewAgent(ctx context.Context, tc *taskCtx, tr *Tracker, message string, step *adewire.WorkflowStep) (adewire.Launch, bool, string, error) {
	stepID := ""
	if step != nil {
		stepID = step.ID
	}
	var settings map[string]map[string]string
	provider := stepAgentOf(tc, reviewStepID)
	if step != nil {
		settings, provider = step.Settings, step.Agent
	}
	row, err := b.deps.Sessions.ReviewAgent(tc.task.ID)
	if err != nil {
		return adewire.Launch{}, false, "", err
	}
	if row != nil && row.State == model.AdeSessionStateRunning {
		return adewire.Launch{}, false, "", invalid("a review agent is already running")
	}
	lines, cwd, extra, err := b.launchDirs(ctx, tc)
	if err != nil {
		return adewire.Launch{}, false, "", err
	}
	note := ""
	if row != nil {
		if info, serr := os.Stat(row.Cwd); serr == nil && info.IsDir() {
			l, err := b.prepareWithGrant(ctx, tr, tc, PrepareArgs{TaskID: tc.task.ID, Resume: row.ID, Message: message, AddDirs: extra, Provider: row.Provider, Settings: settings}, opReviewAgent, "", stepID, nil)
			if err != nil {
				return adewire.Launch{}, false, "", err
			}
			return l, true, "", nil
		}
		if err := b.deps.Sessions.ClearPurpose(row.ID); err != nil {
			return adewire.Launch{}, false, "", err
		}
		note = i18n.T("backend.space.ade.reviewConversationReset.body", map[string]string{"path": row.Cwd})
	}
	if message == "" {
		title := taskTitle(tc.task, tc.branches)
		for i, l := range lines {
			if sb, ok := tc.branch(l.BranchID); ok {
				lines[i].ChangeRequestURL = b.changeRequestURL(ctx, sb, tc.task.HostURL)
			}
		}
		message = composeReviewMessage(b.prompts(), title, tc.task.JiraKey, tc.task.JiraURL, tc.task.Notes, lines, b.handoverFor(tc, cmpNonEmpty(stepID, b.currentStepID(tc))))
		if step != nil && step.Prompt != "" {
			message += "\n" + substitute(step.Prompt, repoVars(title, tc.task.JiraKey, lines), false)
		}
	}
	l, err := b.prepareWithGrant(ctx, tr, tc, PrepareArgs{
		TaskID: tc.task.ID, Cwd: cwd, Message: message, AddDirs: extra, Purpose: model.AdeSessionPurposeReview, Settings: settings, Provider: provider,
	}, opReviewAgent, "", stepID, nil)
	if err != nil {
		return adewire.Launch{}, false, "", err
	}
	return l, false, note, nil
}

// LaunchReviewAgent starts or resumes the task's review agent (the review window's button).
func (b *TaskBoard) LaunchReviewAgent(ctx context.Context, taskID string) (adewire.ReviewAgentLaunch, error) {
	tr, err := b.tracker()
	if err != nil {
		return adewire.ReviewAgentLaunch{}, err
	}
	mu := b.taskMu(taskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(taskID)
	if err != nil {
		return adewire.ReviewAgentLaunch{}, err
	}
	l, resumed, note, err := b.launchReviewAgent(ctx, tc, tr, "", nil)
	if err != nil {
		return adewire.ReviewAgentLaunch{}, err
	}
	return adewire.ReviewAgentLaunch{Launch: l, Resumed: resumed, Note: note}, nil
}

// ReviewAgent returns the task's review agent row as a wire session, nil when it has none.
func (b *TaskBoard) ReviewAgent(taskID string) (*adewire.Session, error) {
	row, err := b.deps.Sessions.ReviewAgent(taskID)
	if err != nil || row == nil {
		return nil, err
	}
	w := toWireSession(*row)
	return &w, nil
}

// forwardMessage composes the forward message for the branch's open comments; the task mutex is held.
func (b *TaskBoard) forwardMessage(ctx context.Context, tc *taskCtx, sb model.AdeTaskBranch) (string, []gitreview.Comment, adewire.ReviewWindowTarget, *gitreview.Store, error) {
	none := adewire.ReviewWindowTarget{}
	if err := b.reviewableBranch(tc, sb); err != nil {
		return "", nil, none, nil, err
	}
	store, err := b.reviewStore()
	if err != nil {
		return "", nil, none, nil, err
	}
	tgt, err := b.reviewTarget(ctx, tc, sb)
	if err != nil {
		return "", nil, none, nil, err
	}
	if err := b.promoteBranchReview(ctx, sb); err != nil {
		return "", nil, none, nil, err
	}
	open, err := store.OpenComments(ctx, tgt.GitRepoID, tgt.Branch)
	if err != nil {
		return "", nil, none, nil, err
	}
	if len(open) == 0 {
		return "", nil, none, nil, invalid("%s", i18n.T("backend.space.ade.reviewStep.noOpen.body", nil))
	}
	message := llmprompts.AdeReviewForwardMessage.Text(b.prompts(), llmprompts.ReviewRoundVars{
		Title: taskTitle(tc.task, tc.branches), JiraKey: tc.task.JiraKey, JiraURL: tc.task.JiraURL,
		Repo: tc.nick[sb.CodeRepoID], Branch: sb.Name, Base: tgt.Base, Worktree: tgt.Worktree,
		Comments: commentVars(open),
	})
	return message, open, tgt, store, nil
}

// ReviewForwardText returns the message ForwardReviewComments would send, for copying. It sends
// nothing and stamps no comment.
func (b *TaskBoard) ReviewForwardText(ctx context.Context, branchID string) (string, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return "", err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return "", err
	}
	message, _, _, _, err := b.forwardMessage(ctx, tc, sb)
	return message, err
}

// ForwardReviewComments pastes the branch's open review comments into the running review agent and
// removes them. It returns how many it forwarded.
func (b *TaskBoard) ForwardReviewComments(ctx context.Context, branchID string) (int, error) {
	sb, err := b.deps.Tasks.GetBranch(branchID)
	if err != nil {
		return 0, err
	}
	mu := b.taskMu(sb.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(sb.TaskID)
	if err != nil {
		return 0, err
	}
	if err := b.reviewableBranch(tc, sb); err != nil {
		return 0, err
	}
	tr, err := b.tracker()
	if err != nil {
		return 0, err
	}
	row, err := b.deps.Sessions.ReviewAgent(sb.TaskID)
	if err != nil {
		return 0, err
	}
	if row == nil || row.State != model.AdeSessionStateRunning {
		return 0, invalid("%s", i18n.T("backend.space.ade.reviewForward.notRunning.body", nil))
	}
	message, open, tgt, store, err := b.forwardMessage(ctx, tc, sb)
	if err != nil {
		return 0, err
	}
	if err := tr.Send(row.ID, message); err != nil {
		return 0, err
	}
	ids := make([]int64, len(open))
	for i, c := range open {
		ids[i] = c.ID
	}
	// The agent already has the comments: failing here would invite a second forward that pastes
	// them again.
	if _, err := store.RemoveComments(ctx, tgt.GitRepoID, tgt.Branch, ids); err != nil {
		slog.Warn("ade: remove forwarded review comments", "scope", "ade", "branch", sb.ID, "err", err)
	}
	b.emitReview(sb.TaskID, sb.ID)
	return len(open), nil
}
