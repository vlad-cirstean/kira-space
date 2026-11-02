package jira

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
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
	f, err := os.CreateTemp("", "kira-acli-comment-*.json")
	if err != nil {
		return errors.New("jira: could not write the comment body")
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return errors.New("jira: could not write the comment body")
	}
	_, werr := f.Write(adf)
	if cerr := f.Close(); werr != nil || cerr != nil {
		return errors.New("jira: could not write the comment body")
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err = c.runner.Run(wctx, st.Path, []string{"jira", "workitem", "comment", "create", "--key", ref.Key, "--body-file", f.Name()})
	if err != nil {
		return classify(err, wctx, "workitem comment create")
	}
	c.forget(cacheKey(ref))
	if ref.Site == "" {
		ref.Site = st.Site
	}
	c.forgetDetail(cacheKey(ref))
	return nil
}
