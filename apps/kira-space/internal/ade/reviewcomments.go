package ade

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/gitreview"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

// reviewcomments.go moves a branch row's review comments between the shared review session and the
// row. Sessions are keyed by repo and branch name, not by task, so a task that archives or drops a
// row takes its comments with it (a later task on the same name starts fresh) and Restore puts them
// back.

// storedComment is one review comment as snapshotted on a row.
type storedComment struct {
	Path          string `json:"path"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
	Body          string `json:"body"`
	AnchorSHA     string `json:"anchorSha"`
	AnchorBlobOID string `json:"anchorBlobOid"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Reply         string `json:"reply"`
	CreatedAt     int64  `json:"createdAt"`
	RepliedAt     *int64 `json:"repliedAt"`
	SentAt        *int64 `json:"sentAt"`
	// Origin and AnchorContent keep an uncommitted comment intact; older snapshots lack both and
	// read as commit-origin.
	Origin        string `json:"origin,omitempty"`
	AnchorContent []byte `json:"anchorContent,omitempty"`
}

// reviewSession is the review session key of the row: repo id and the branch spelling reviews use.
func (b *TaskBoard) reviewSession(ctx context.Context, sb model.AdeTaskBranch) (repoID, branch string, err error) {
	entry, err := b.openRepo(ctx, sb.CodeRepoID)
	if err != nil {
		return "", "", err
	}
	branch, _, err = b.reviewSpellings(ctx, entry, sb)
	if err != nil {
		branch = sb.Name
	}
	return entry.Summary.RepoID, branch, nil
}

// snapshotReviewComments stores each named row's comments on the row and clears them from the
// session. Best effort: a row whose snapshot fails keeps its live comments.
func (b *TaskBoard) snapshotReviewComments(ctx context.Context, rows []model.AdeTaskBranch) {
	store, err := b.reviewStore()
	if err != nil {
		return
	}
	for _, sb := range rows {
		if sb.Name == "" {
			continue
		}
		if err := b.snapshotRowComments(ctx, store, sb); err != nil {
			slog.Warn("ade: snapshot review comments", "scope", "ade", "branch", sb.ID, "err", err)
		}
	}
}

func (b *TaskBoard) snapshotRowComments(ctx context.Context, store *gitreview.Store, sb model.AdeTaskBranch) error {
	repoID, branch, err := b.reviewSession(ctx, sb)
	if err != nil {
		return err
	}
	cs, err := store.Comments(ctx, repoID, branch)
	if err != nil {
		return err
	}
	out := make([]storedComment, 0, len(cs))
	for _, c := range cs {
		sc := storedComment{
			Path: c.Path, Start: c.Range.Start, End: c.Range.End, Body: c.Body, AnchorSHA: c.AnchorSHA,
			AnchorBlobOID: c.AnchorBlobOID, Kind: c.Kind, State: c.State, Reply: c.Reply, CreatedAt: c.CreatedAt.UnixMilli(),
			Origin: c.Origin, AnchorContent: c.AnchorContent,
		}
		if c.RepliedAt != nil {
			v := c.RepliedAt.UnixMilli()
			sc.RepliedAt = &v
		}
		if c.SentAt != nil {
			v := c.SentAt.UnixMilli()
			sc.SentAt = &v
		}
		out = append(out, sc)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if err := b.deps.Tasks.SetBranchReviewComments(sb.ID, string(raw)); err != nil {
		return err
	}
	if len(cs) == 0 {
		return nil
	}
	_, err = store.ClearComments(ctx, repoID, branch)
	return err
}

// storedComments decodes a row's snapshot; ok is false when the row has none.
func storedComments(sb model.AdeTaskBranch) (cs []storedComment, ok bool) {
	if sb.ReviewCommentsJSON == "" {
		return nil, false
	}
	if err := json.Unmarshal([]byte(sb.ReviewCommentsJSON), &cs); err != nil {
		slog.Warn("ade: decode review comments", "scope", "ade", "branch", sb.ID, "err", err)
		return nil, false
	}
	return cs, true
}

// restoreReviewComments moves the rows' snapshots back into their review sessions and drops the
// snapshots. A row that fails keeps its snapshot.
func (b *TaskBoard) restoreReviewComments(ctx context.Context, rows []model.AdeTaskBranch) {
	store, err := b.reviewStore()
	if err != nil {
		return
	}
	for _, sb := range rows {
		cs, ok := storedComments(sb)
		if !ok {
			continue
		}
		if err := b.restoreRowComments(ctx, store, sb, cs); err != nil {
			slog.Warn("ade: restore review comments", "scope", "ade", "branch", sb.ID, "err", err)
		}
	}
}

func (b *TaskBoard) restoreRowComments(ctx context.Context, store *gitreview.Store, sb model.AdeTaskBranch, cs []storedComment) error {
	repoID, branch, err := b.reviewSession(ctx, sb)
	if err != nil {
		return err
	}
	// Idempotent: a retry after a partial restore starts from an empty session.
	if _, err := store.ClearComments(ctx, repoID, branch); err != nil {
		return err
	}
	for _, sc := range cs {
		added, err := store.AddComment(ctx, repoID, branch, gitreview.Comment{
			Path: sc.Path, Range: gitreview.LineRange{Start: sc.Start, End: sc.End}, Body: sc.Body,
			AnchorSHA: sc.AnchorSHA, AnchorBlobOID: sc.AnchorBlobOID, CreatedAt: time.UnixMilli(sc.CreatedAt), Kind: sc.Kind,
			Origin: sc.Origin, AnchorContent: sc.AnchorContent,
		})
		if err != nil {
			return err
		}
		resolved := sc.State == gitreview.CommentStateAddressed
		switch {
		case sc.Reply != "" && sc.RepliedAt != nil:
			_, err = store.ReplyComment(ctx, repoID, branch, added.ID, sc.Reply, resolved, time.UnixMilli(*sc.RepliedAt))
		case resolved:
			_, err = store.SetCommentState(ctx, repoID, branch, added.ID, sc.State)
		}
		if err != nil {
			return err
		}
		if sc.SentAt != nil {
			if err := store.MarkSent(ctx, repoID, branch, []string{added.ID}, time.UnixMilli(*sc.SentAt)); err != nil {
				return err
			}
		}
	}
	return b.deps.Tasks.SetBranchReviewComments(sb.ID, "")
}
