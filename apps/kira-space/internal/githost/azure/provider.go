package azure

import (
	"context"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

// ProviderID is the setting value and registry key of this provider.
const ProviderID = "azure"

// Options are the `az` seams; a nil field takes the real one.
type Options struct {
	Locator Locator
	Runner  Runner
	Clock   Clock
}

// Provider adapts Client to the githost roles.
type Provider struct {
	client *Client
}

var (
	_ githost.Provider              = (*Provider)(nil)
	_ githost.ChangeRequestSync     = (*Provider)(nil)
	_ githost.ChangeRequestReviews  = (*Provider)(nil)
	_ githost.BranchLinks           = (*Provider)(nil)
	_ githost.ChangeRequestComments = (*Provider)(nil)
	_ githost.CLISetup              = (*Provider)(nil)
)

// New builds the Azure Repos provider over the `az` CLI.
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
	return &Provider{client: NewClient(NewDiscovery(o.Locator, o.Runner, o.Clock), o.Runner)}
}

func (p *Provider) ID() string { return ProviderID }

func (p *Provider) Terms() githost.Terms {
	return githost.Terms{
		Label: "Azure Repos", ChangeRequest: "Pull request", ChangeRequestNoun: "pull request",
		ChangeRequestShort: "PR", CLI: "Azure CLI (az)",
	}
}

func (p *Provider) ParseRemote(remoteURL string) (githost.Repo, bool) { return ParseRemote(remoteURL) }

// Claims is dev.azure.com, the host every Azure Repos remote normalizes to.
func (p *Provider) Claims(_ context.Context, host string) bool { return host == Host }

func (p *Provider) Status(ctx context.Context, r githost.Repo) githost.Status {
	org, _, ok := split(r)
	if !ok {
		return invalidRepo(r)
	}
	return p.client.discovery.Status(ctx, orgURL(org))
}

func (p *Provider) Account(ctx context.Context, r githost.Repo) string {
	return p.Status(ctx, r).Account
}

func (p *Provider) ForCommit(ctx context.Context, r githost.Repo, sha string) ([]githost.ChangeRequest, githost.Status) {
	return p.client.forCommit(ctx, r, sha)
}

func (p *Provider) ForBranch(ctx context.Context, r githost.Repo, branch string) ([]githost.ChangeRequest, githost.Status) {
	return p.client.forBranch(ctx, r, branch)
}

func (p *Provider) Open(ctx context.Context, r githost.Repo) ([]githost.ChangeRequest, githost.Status) {
	return p.client.open(ctx, r)
}

func (p *Provider) Setup() clisetup.Setup {
	return clisetup.Setup{
		DocsURL: "https://learn.microsoft.com/azure/devops/cli/",
		Steps: []clisetup.Step{
			{Purpose: clisetup.PurposeInstall, Command: "brew install azure-cli"},
			{Purpose: clisetup.PurposeExtension, Command: "az extension add --name azure-devops"},
			{Purpose: clisetup.PurposeLogin, Command: "az login"},
			{Purpose: clisetup.PurposeLoginToken, Command: "az devops login --organization https://dev.azure.com/<your-org>"},
			{Purpose: clisetup.PurposeDefaults, Command: "az devops configure --defaults organization=https://dev.azure.com/<your-org> project=<your-project>"},
			{Purpose: clisetup.PurposeCheck, Command: "az devops project list --top 1"},
		},
	}
}

// CLIStatus probes the default organization.
func (p *Provider) CLIStatus(ctx context.Context, refresh bool) githost.Status {
	if refresh {
		p.client.discovery.ForgetDefaults()
	}
	return p.client.discovery.Status(ctx, "")
}
