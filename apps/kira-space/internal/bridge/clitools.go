package bridge

import "github.com/kirathecat/kira-studio/apps/kira-space/internal/clisetup"

// ToolsArgs is GitHostService.Tools' request. Refresh drops each CLI's cached probe first.
type ToolsArgs struct {
	Refresh bool `json:"refresh"`
}

// Tool kinds.
const (
	ToolKindHost = "host"
)

// Tool is one local CLI Settings explains: a code host's.
type Tool struct {
	Kind     string         `json:"kind"` // host
	Provider string         `json:"provider"`
	Label    string         `json:"label"` // GitHub, Azure Repos
	CLI      string         `json:"cli"`   // GitHub CLI (gh)
	Setup    clisetup.Setup `json:"setup"`
	Status   ToolStatus     `json:"status"`
}

// ToolStatus is a CLI's install and sign-in state.
type ToolStatus struct {
	Kind    string   `json:"kind"` // ok | notFound | unauthenticated | forbidden
	Path    string   `json:"path"`
	Version string   `json:"version"`
	Account string   `json:"account"`
	Site    string   `json:"site"`
	Reason  string   `json:"reason"`
	Probed  []string `json:"probed"`
}

// ToolsResult lists the code host CLIs in registration order.
type ToolsResult struct {
	Tools []Tool `json:"tools"`
}
