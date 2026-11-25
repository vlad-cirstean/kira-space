package ade

import (
	"context"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
)

// foundlinks.go lists links found in a task's notes and handovers (P731). They derive on read, so an
// edit or delete of the text takes its links along; they never replace or promote a main link.

// Limits; over a cap the newest win and the rest drop silently (a gap means a failed source).
const (
	maxFoundRows    = 200
	maxFoundPerRow  = 50
	maxFoundLinks   = 100
	maxFoundSources = 5
)

type notesSource struct{ b *TaskBoard }

func (notesSource) Name() string { return artifactSrcNotes }

func (s notesSource) Artifacts(_ context.Context, sc ArtifactScope) ([]adewire.Artifact, []adewire.ArtifactGap, error) {
	rows, err := s.b.deps.Tasks.Handovers(sc.Task.ID) // newest first
	if err != nil {
		return nil, nil, err
	}
	if len(rows) > maxFoundRows {
		rows = rows[:maxFoundRows]
	}
	dismissed, err := s.b.deps.Tasks.FoundLinkDismissals(sc.Task.ID)
	if err != nil {
		return nil, nil, err
	}
	own := map[string]bool{}
	for _, raw := range []string{sc.Task.HostURL, sc.Task.JiraURL} {
		if k, ok := normalizeLinkURL(raw); ok {
			own[k] = true
		}
	}
	var order []string
	acc := map[string]*adewire.Artifact{}
	titled := map[string]bool{}
	for _, row := range rows {
		links := extractLinks(row.Body)
		if len(links) > maxFoundPerRow {
			links = links[:maxFoundPerRow]
		}
		for _, l := range links {
			key, ok := normalizeLinkURL(l.URL)
			if !ok || dismissed[key] || own[key] {
				continue
			}
			a := acc[key]
			if a == nil {
				if len(order) == maxFoundLinks {
					continue
				}
				a = s.b.foundArtifact(sc.Task, key, row.CreatedAt)
				acc[key] = a
				order = append(order, key)
			}
			if t := clipTitle(l.Title); t != "" && !titled[key] {
				a.Title, titled[key] = t, true
			}
			if len(a.FoundIn) < maxFoundSources {
				a.FoundIn = append(a.FoundIn, adewire.ArtifactFound{
					HandoverID: row.ID, State: row.State, FromStep: row.FromStepName, ToStep: row.ToStepName,
					FromTask: row.FromTaskTitle, CreatedAt: row.CreatedAt,
				})
			}
		}
	}
	out := make([]adewire.Artifact, len(order))
	for i, k := range order {
		out[i] = *acc[k]
	}
	return out, nil, nil
}

// foundArtifact builds the detected artifact of one normalized URL. No network: kind comes from the
// pure recognition rules only.
func (b *TaskBoard) foundArtifact(task model.AdeTask, key string, createdAt int64) *adewire.Artifact {
	u, _ := httpURL(key)
	c, ok := b.localLinkKind(task, key, u)
	if !ok {
		c = linkClass{Kind: model.AdeArtifactLink}
		if isDocURL(u, false) {
			c.Kind = model.AdeArtifactDoc
		}
	}
	c.URL = key
	return &adewire.Artifact{
		Kind: c.Kind, Title: b.autoLinkTitle(c), URL: key, Ref: c.Ref, Source: artifactSrcNotes, CreatedAt: createdAt,
	}
}

// normalizeLinkURL is the stored-row form of a URL, so keys match AddLink and links.yaml rows. The
// kind and title are placeholders: only the URL rules matter here.
func normalizeLinkURL(raw string) (string, bool) {
	in, refusal := agentmcp.CleanArtifactAs(agentmcp.ArtifactInput{Kind: model.AdeArtifactLink, Title: "link", URL: raw},
		func(string) bool { return true })
	return in.URL, refusal == "" && in.URL != ""
}

func clipTitle(t string) string {
	t = strings.TrimSpace(t)
	if len(t) > agentmcp.MaxArtifactTitle {
		t = strings.ToValidUTF8(t[:agentmcp.MaxArtifactTitle], "")
	}
	return t
}

// DismissFoundLink hides a found link of a live task. Main links are unaffected: the filter applies to
// links found in text only.
func (b *TaskBoard) DismissFoundLink(_ context.Context, args adewire.DismissFoundLinkArgs) error {
	mu := b.taskMu(args.TaskID)
	mu.Lock()
	defer mu.Unlock()
	if _, err := b.loadTaskCtx(args.TaskID); err != nil {
		return err
	}
	key, ok := normalizeLinkURL(args.URL)
	if !ok {
		return invalid("url must be an absolute http or https address")
	}
	if err := b.deps.Tasks.DismissFoundLink(args.TaskID, key, b.deps.Now().UnixMilli()); err != nil {
		return err
	}
	b.notifyBoardWith(taskChanged(args.TaskID))
	return nil
}
