package github

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// PullReview is the review facts of one pull request.
type PullReview struct {
	Title   string
	State   string // "open" | "draft" | "merged" | "closed"
	HeadSha string
	Author  string // "" when GitHub reports user as null (a deleted account)
}

type rawReviewPull struct {
	Title  string  `json:"title"`
	State  string  `json:"state"`
	Draft  bool    `json:"draft"`
	Merged *string `json:"merged_at"`
	Head   struct {
		Sha string `json:"sha"`
	} `json:"head"`
	User *struct {
		Login string `json:"login"`
	} `json:"user"`
}

// reviewEvents maps the neutral event names onto GitHub's.
var reviewEvents = map[string]string{
	"approve":        "APPROVE",
	"comment":        "COMMENT",
	"requestChanges": "REQUEST_CHANGES",
}

// field is one request field. typed sends a JSON integer (`-F`); otherwise a string (`-f`).
type field struct {
	key, value string
	typed      bool
}

// writeArgv builds a POST `gh api` argv. Fields travel via `-f`. `-F` only for integers Kira
// formats itself: `-F` reads an `@`-prefixed value from a local file, so user text never uses it.
func writeArgv(host, path string, fields []field) []string {
	args := methodArgv(host, "POST", path)
	for _, f := range fields {
		flag := "-f"
		if f.typed {
			flag = "-F"
		}
		args = append(args, flag, f.key+"="+f.value)
	}
	return args
}

// PullForReview reads the facts a review depends on: `repos/{owner}/{repo}/pulls/{n}`.
func (c *Client) PullForReview(ctx context.Context, repo Repo, number int) (PullReview, Status) {
	var raw rawReviewPull
	status := c.get(ctx, repo, fmt.Sprintf("repos/%s/pulls/%d", repo.Path(), number), &raw)
	if !status.OK() {
		return PullReview{}, status
	}
	out := PullReview{Title: raw.Title, State: deriveState(raw.State, raw.Draft, raw.Merged), HeadSha: raw.Head.Sha}
	if raw.User != nil {
		out.Author = raw.User.Login
	}
	return out, status
}

// writeTimeout bounds a mutating `gh api` call. POSTs are slow on large PRs and are never retried.
const (
	writeTimeout       = 30 * time.Second
	writeTimeoutReason = "GitHub did not respond within 30s. The request may have been recorded; open the PR to check before trying again"
)

// SubmitReview creates and submits a review in one call. An empty body is left out of the request.
func (c *Client) SubmitReview(ctx context.Context, repo Repo, number int, event, body, commitID string) Status {
	ghEvent, ok := reviewEvents[event]
	if !ok {
		return Status{Kind: KindForbidden, Host: repo.Host, Reason: "unknown review event " + event}
	}
	fields := []field{{key: "event", value: ghEvent}, {key: "commit_id", value: commitID}}
	if body != "" {
		fields = append(fields, field{key: "body", value: body})
	}
	path := fmt.Sprintf("repos/%s/pulls/%d/reviews", repo.Path(), number)
	return c.callWithin(ctx, repo, writeArgv(repo.Host, path, fields), nil, writeTimeout, true)
}

// CommentOnPull posts a conversation comment: `repos/{owner}/{repo}/issues/{n}/comments`.
func (c *Client) CommentOnPull(ctx context.Context, repo Repo, number int, body string) Status {
	path := fmt.Sprintf("repos/%s/issues/%d/comments", repo.Path(), number)
	return c.callWithin(ctx, repo, writeArgv(repo.Host, path, []field{{key: "body", value: body}}), nil, writeTimeout, true)
}

// CommentOnLines posts a review comment on lines of one file at commit, right side:
// `repos/{owner}/{repo}/pulls/{n}/comments`. line and start_line must be JSON integers.
func (c *Client) CommentOnLines(ctx context.Context, repo Repo, number int, path string, start, end int, commitID, body string) Status {
	fields := []field{
		{key: "body", value: body}, {key: "commit_id", value: commitID},
		{key: "path", value: path}, {key: "side", value: "RIGHT"},
		{key: "line", value: strconv.Itoa(end), typed: true},
	}
	if start < end {
		fields = append(fields,
			field{key: "start_line", value: strconv.Itoa(start), typed: true},
			field{key: "start_side", value: "RIGHT"})
	}
	api := fmt.Sprintf("repos/%s/pulls/%d/comments", repo.Path(), number)
	return c.callWithin(ctx, repo, writeArgv(repo.Host, api, fields), nil, writeTimeout, true)
}
