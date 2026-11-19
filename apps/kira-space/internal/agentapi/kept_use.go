package agentapi

import "github.com/kirathecat/kira-studio/apps/kira-space/internal/keptvars"

// NewKeptUse is the approval view of a kept entry used at sites of a request: metadata and a preview, never
// the value of a secret. Other MCPs (the SQL params of dbmcp) build their uses with it.
func NewKeptUse(e keptvars.Entry, sites []KeptSite) KeptUse {
	use := KeptUse{
		ID: e.ID, Name: e.Name, Type: e.Type, Items: e.Items, Bytes: e.Bytes, Secret: e.Secret,
		Source: e.Source, Origin: e.Origin, CreatedAt: e.CreatedAt, ExpiresAt: e.ExpiresAt, Sites: sites,
	}
	if !e.Secret {
		use.Preview, use.PreviewTruncated = keptvars.Preview(e.Value, keptvars.PreviewBytes)
	}
	return use
}
