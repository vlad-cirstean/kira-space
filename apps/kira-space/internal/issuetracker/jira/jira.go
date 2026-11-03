// Package jira is the issue tracker provider over Atlassian's `acli`. It reads title, status and the
// Development summary of one work item.
// acli keeps its own login; Kira holds no Jira credential.
package jira

import (
	"context"
	"time"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/issuetracker"
)

// ProviderID is the registry key of this provider.
const ProviderID = "jira"

// ProviderLabel is the name shown to the user and the source of its op log rows.
const ProviderLabel = "Jira"

const docsURL = "https://developer.atlassian.com/cloud/acli/guides/introduction/"

// Clock is the time seam of the caches.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Options are the acli seams; a nil field takes the real one.
type Options struct {
	Locator Locator
	Runner  Runner
	Clock   Clock
}

// Provider is the Jira issue tracker.
type Provider struct {
	disc   *discovery
	client *client
}

var _ issuetracker.Provider = (*Provider)(nil)

// New builds the Jira provider over acli.
func New(o Options) *Provider {
	if o.Locator == nil {
		o.Locator = NewPlatformLocator()
	}
	if o.Runner == nil {
		o.Runner = NewExecRunner()
	}
	if o.Clock == nil {
		o.Clock = realClock{}
	}
	d := newDiscovery(o.Locator, o.Runner, o.Clock)
	return &Provider{disc: d, client: newClient(d, o.Runner, o.Clock)}
}

func (p *Provider) ID() string { return ProviderID }

func (p *Provider) Terms() issuetracker.Terms {
	return issuetracker.Terms{Label: ProviderLabel, Issue: "Work item", CLI: "Atlassian CLI (acli)"}
}

func (p *Provider) Setup() clisetup.Setup {
	return clisetup.Setup{
		DocsURL: docsURL,
		Steps: []clisetup.Step{
			{Purpose: clisetup.PurposeInstall, Command: "brew tap atlassian/homebrew-acli && brew trust --formula atlassian/acli/acli && brew install acli"},
			{Purpose: clisetup.PurposeLogin, Command: "acli jira auth login --web"},
			{Purpose: clisetup.PurposeLoginToken, Command: "acli jira auth login --site <your-site>.atlassian.net --email <you@example.com> --token < token.txt"},
			{Purpose: clisetup.PurposeCheck, Command: "acli jira auth status"},
		},
	}
}

func (p *Provider) ParseRef(raw string) (issuetracker.Ref, bool) { return parseRef(raw) }

func (p *Provider) Status(ctx context.Context, refresh bool) issuetracker.Status {
	return p.disc.status(ctx, refresh)
}

func (p *Provider) Issue(ctx context.Context, ref issuetracker.Ref) (issuetracker.Issue, error) {
	return p.client.issue(ctx, ref)
}
