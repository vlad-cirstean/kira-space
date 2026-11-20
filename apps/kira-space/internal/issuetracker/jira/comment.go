package jira

import (
	"context"
	"errors"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
	"github.com/kirathecat/kira-studio/internal/fileservice"
)

var _ issuetracker.IssueComments = (*Provider)(nil)

// writeTimeout bounds `acli jira workitem comment create`.
const writeTimeout = 30 * time.Second

// PostIssueComment posts body as an ADF comment. The body travels in a private temp file, never argv,
// so the op log row holds no comment text.
func (p *Provider) PostIssueComment(ctx context.Context, ref issuetracker.Ref, body string) error {
	return p.client.postComment(ctx, ref, body)
}

func (c *client) postComment(ctx context.Context, ref issuetracker.Ref, body string) error {
	st, err := c.ready(ctx, ref)
	if err != nil {
		return err
	}
	adf, err := markdownADF(body)
	if err != nil {
		return errors.New("jira: comment could not be converted")
	}
	bodyPath, cleanup, err := fileservice.Host().TempFile("", "kira-acli-comment-*.json", adf)
	if err != nil {
		return errors.New("jira: could not write the comment body")
	}
	defer cleanup()
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err = c.runner.Run(wctx, st.Path, []string{"jira", "workitem", "comment", "create", "--key", ref.Key, "--body-file", bodyPath})
	if err != nil {
		return classify(err, wctx, "workitem comment create")
	}
	if ref.Site == "" {
		ref.Site = st.Site
	}
	c.forgetDetail(cacheKey(ref))
	c.forget(cacheKey(ref))
	return nil
}
