package ade

import (
	"context"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// links.go files and edits the free-form lines of a task's Details links.

// Where a link landed (LinkResult.Filed).
const (
	linkFiledJira   = "jira"
	linkFiledHost   = "host"
	linkFiledStored = "stored"
	linkFiledKnown  = "known"
)

// Slots EditLink addresses.
const (
	linkSlotJira   = "jira"
	linkSlotHost   = "host"
	linkSlotStored = "stored"
)

var linkRefusals = map[string]string{
	linkErrNotLink:     "backend.space.ade.linkNotLink.body",
	linkErrOneLink:     "backend.space.ade.linkOneLink.body",
	linkErrAlreadyUsed: "backend.space.ade.linkAlreadyLinked.body",
}

func linkRefusal(code string) error {
	return invalid("%s", i18n.T(linkRefusals[code], nil))
}

func sameLinkURL(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

// autoLinkTitle names a link the user gave no title.
func (b *TaskBoard) autoLinkTitle(c linkClass) string {
	switch {
	case c.Origin == linkOriginTracker && c.Ref != "":
		return c.Ref
	case c.Kind == artifactBranch && c.Ref != "":
		return c.Ref
	}
	return b.pastedArtifactTitle(c.URL)
}

// parseAndClassify reads one line. The task row only feeds the known-tracker-host rule.
func (b *TaskBoard) parseAndClassify(ctx context.Context, taskID, text string) (parsedLink, linkClass, error) {
	p, code := parseLinkText(text)
	if code != "" {
		return p, linkClass{}, linkRefusal(code)
	}
	task, err := b.deps.Tasks.GetTask(taskID)
	if err != nil {
		return p, linkClass{}, err
	}
	c, code := b.classifyLink(ctx, task, p)
	if code != "" {
		return p, c, linkRefusal(code)
	}
	return p, c, nil
}

// AddLink files one line of text under the right category of the task's links.
func (b *TaskBoard) AddLink(ctx context.Context, args adewire.AddLinkArgs) (adewire.LinkResult, error) {
	_, c, err := b.parseAndClassify(ctx, args.TaskID, args.Text)
	if err != nil {
		return adewire.LinkResult{}, err
	}
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return adewire.LinkResult{}, err
	}
	return b.fileLinkLocked(tc, c)
}

// fileLinkLocked files a classified link: an empty task slot first, else a stored row. The caller holds
// the task lock.
func (b *TaskBoard) fileLinkLocked(tc *taskCtx, c linkClass) (adewire.LinkResult, error) {
	t := tc.task
	switch c.Origin {
	case linkOriginTracker:
		if t.JiraKey == c.Ref {
			return adewire.LinkResult{Filed: linkFiledKnown, Kind: c.Kind}, nil
		}
		if t.JiraKey == "" {
			return b.setSlotLocked(t.ID, linkFiledJira, c)
		}
	case linkOriginHost:
		if t.HostURL != "" && sameLinkURL(t.HostURL, c.URL) {
			return adewire.LinkResult{Filed: linkFiledKnown, Kind: c.Kind}, nil
		}
		if t.HostURL == "" {
			return b.setSlotLocked(t.ID, linkFiledHost, c)
		}
	}
	if c.Origin == linkOriginTracker && c.Ref != "" {
		others, err := b.deps.Tasks.Artifacts(t.ID)
		if err != nil {
			return adewire.LinkResult{}, err
		}
		for _, o := range others {
			if o.Kind == c.Kind && o.Ref == c.Ref {
				return adewire.LinkResult{Filed: linkFiledKnown, Kind: c.Kind}, nil
			}
		}
	}
	title := c.Title
	if title == "" {
		title = b.autoLinkTitle(c)
	}
	clean, msg := agentmcp.CleanArtifactAs(agentmcp.ArtifactInput{Kind: c.Kind, Title: title, URL: c.URL, Ref: c.Ref}, userArtifactKind)
	if msg != "" {
		return adewire.LinkResult{}, invalid("%s", msg)
	}
	rows, err := b.storeCleanLocked(tc, model.AdeArtifactSourceUser, "", "", "", []agentmcp.ArtifactInput{clean})
	if err != nil {
		return adewire.LinkResult{}, err
	}
	a := wireStoredArtifact(rows[0])
	return adewire.LinkResult{Filed: linkFiledStored, Kind: a.Kind, Artifact: &a}, nil
}

// setSlotLocked writes the task's Jira or host link.
func (b *TaskBoard) setSlotLocked(taskID, slot string, c linkClass) (adewire.LinkResult, error) {
	var patch model.AdeTaskPatch
	if slot == linkFiledJira {
		patch.JiraKey, patch.JiraURL = &c.Ref, &c.URL
	} else {
		patch.HostURL = &c.URL
	}
	if _, err := b.deps.Tasks.UpdateTask(taskID, patch); err != nil {
		return adewire.LinkResult{}, err
	}
	b.notifyBoard()
	return adewire.LinkResult{Filed: slot, Kind: c.Kind}, nil
}

func (b *TaskBoard) clearSlotLocked(taskID, slot string) error {
	empty := ""
	var patch model.AdeTaskPatch
	if slot == linkSlotJira {
		patch.JiraKey, patch.JiraURL = &empty, &empty
	} else {
		patch.HostURL = &empty
	}
	if _, err := b.deps.Tasks.UpdateTask(taskID, patch); err != nil {
		return err
	}
	b.notifyBoard()
	return nil
}

// EditLink re-files a link row from new text, or recategorises a stored row. A slot row whose new text
// no longer fits the slot moves to the right place; the slot clears only after the add succeeded.
func (b *TaskBoard) EditLink(ctx context.Context, args adewire.EditLinkArgs) (adewire.LinkResult, error) {
	var (
		p parsedLink
		c linkClass
	)
	if args.Kind != "" && args.Slot != linkSlotStored {
		return adewire.LinkResult{}, invalid("only a stored link has a kind")
	}
	if args.Kind == "" {
		var err error
		if p, c, err = b.parseAndClassifyEdit(ctx, args); err != nil {
			return adewire.LinkResult{}, err
		}
	}
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	tc, err := b.loadTaskCtx(args.TaskID)
	if err != nil {
		return adewire.LinkResult{}, err
	}
	switch args.Slot {
	case linkSlotStored:
		return b.editStoredLocked(tc, args, p, c)
	case linkSlotJira:
		if c.Origin == linkOriginTracker {
			return b.setSlotLocked(tc.task.ID, linkFiledJira, c)
		}
	case linkSlotHost:
		if c.Origin == linkOriginHost {
			return b.setSlotLocked(tc.task.ID, linkFiledHost, c)
		}
	}
	res, err := b.fileLinkLocked(tc, c)
	if err != nil {
		return adewire.LinkResult{}, err
	}
	if err := b.clearSlotLocked(tc.task.ID, args.Slot); err != nil {
		return adewire.LinkResult{}, err
	}
	return res, nil
}

// parseAndClassifyEdit is parseAndClassify, except a words-only line on a stored row is a rename and
// needs no classification.
func (b *TaskBoard) parseAndClassifyEdit(ctx context.Context, args adewire.EditLinkArgs) (parsedLink, linkClass, error) {
	p, code := parseLinkText(args.Text)
	if code != "" {
		return p, linkClass{}, linkRefusal(code)
	}
	if args.Slot == linkSlotStored && p.URL == "" {
		if _, c, err := b.parseAndClassify(ctx, args.TaskID, args.Text); err == nil {
			return p, c, nil
		}
		return p, linkClass{}, nil
	}
	return b.parseAndClassify(ctx, args.TaskID, args.Text)
}

func (b *TaskBoard) editStoredLocked(tc *taskCtx, args adewire.EditLinkArgs, p parsedLink, c linkClass) (adewire.LinkResult, error) {
	row, err := b.deps.Tasks.GetArtifact(args.ID)
	if err != nil {
		return adewire.LinkResult{}, err
	}
	if row == nil || row.TaskID != args.TaskID {
		return adewire.LinkResult{}, invalid("artifact %s is not stored for this task", args.ID)
	}
	kind, title, url, ref := row.Kind, row.Title, row.URL, row.Ref
	switch {
	case args.Kind != "":
		kind = args.Kind
	case p.URL == "" && c.Origin == linkOriginTracker:
		if err := b.refuseTakenLink(tc, row.ID, c, ""); err != nil {
			return adewire.LinkResult{}, err
		}
		kind, ref, url = c.Kind, c.Ref, ""
	case p.URL == "":
		title = p.Words
	default:
		if err := b.refuseTakenLink(tc, row.ID, c, p.URL); err != nil {
			return adewire.LinkResult{}, err
		}
		autoTitle := row.Title == b.pastedArtifactTitle(row.URL) || row.Title == row.Ref
		kind, url, ref = c.Kind, c.URL, c.Ref
		switch {
		case c.Title != "":
			title = c.Title
		case autoTitle:
			title = b.autoLinkTitle(c)
		}
	}
	clean, msg := agentmcp.CleanArtifactAs(agentmcp.ArtifactInput{Kind: kind, Title: title, URL: url, Ref: ref}, userArtifactKind)
	if msg != "" {
		return adewire.LinkResult{}, invalid("%s", msg)
	}
	updated, err := b.deps.Tasks.UpdateArtifact(args.TaskID, args.ID, clean.Kind, clean.Title, clean.URL, clean.Ref, b.deps.Now().UnixMilli())
	if err != nil {
		return adewire.LinkResult{}, err
	}
	b.notifyBoard()
	a := wireStoredArtifact(updated)
	return adewire.LinkResult{Filed: linkFiledStored, Kind: a.Kind, Artifact: &a}, nil
}

// refuseTakenLink refuses an edit whose target the task already shows elsewhere: another stored row
// (same URL, or same tracker ref when url is empty) or the Jira or host slot, which the Details tab
// would otherwise hide the edited row behind.
func (b *TaskBoard) refuseTakenLink(tc *taskCtx, rowID string, c linkClass, url string) error {
	t := tc.task
	if c.Origin == linkOriginTracker && c.Ref != "" && t.JiraKey == c.Ref {
		return linkRefusal(linkErrAlreadyUsed)
	}
	if url != "" && (t.HostURL != "" && sameLinkURL(t.HostURL, url) || t.JiraURL != "" && sameLinkURL(t.JiraURL, url)) {
		return linkRefusal(linkErrAlreadyUsed)
	}
	others, err := b.deps.Tasks.Artifacts(tc.task.ID)
	if err != nil {
		return err
	}
	for _, o := range others {
		if o.ID == rowID {
			continue
		}
		if url != "" && o.URL != "" && sameLinkURL(o.URL, url) {
			return linkRefusal(linkErrAlreadyUsed)
		}
		if url == "" && o.Kind == c.Kind && o.Ref == c.Ref {
			return linkRefusal(linkErrAlreadyUsed)
		}
	}
	return nil
}
