package repocfg

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/model"
)

//go:embed repo.schema.json
var baseSchema []byte

//go:embed repos_intro.md
var intro string

// providerPattern is appsettings.ValidProviderID's syntax plus the two fixed values.
const providerPattern = `^(auto|off|[a-z][a-z0-9-]{0,31})$`

// assembleSchema fills the model-derived bounds, enums and defaults into the base schema, so the
// options in AGENTS.md cannot drift from the code. hosts are the registered Git host ids.
func assembleSchema(hosts []string) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(baseSchema, &root); err != nil {
		return nil, fmt.Errorf("repocfg: schema: %w", err)
	}
	d := model.DefaultGitRepoSettings()
	leaf := func(section, key string) map[string]any {
		return root["properties"].(map[string]any)[section].(map[string]any)["properties"].(map[string]any)[key].(map[string]any)
	}
	page := leaf("graph", "page_size")
	page["minimum"], page["maximum"], page["default"] = model.GraphPageSizeMin, model.GraphPageSizeMax, d.GraphPageSize
	scope := leaf("graph", "scope")
	scope["enum"], scope["default"] = model.GraphScopes, d.GraphScope
	strategy := leaf("pull", "strategy")
	strategy["enum"], strategy["default"] = model.PullStrategies, d.PullStrategy
	leaf("stash", "show_in_graph")["default"] = d.StashShowInGraph
	leaf("stash", "include_untracked")["default"] = d.StashIncludeUntracked
	leaf("review", "base_candidates")["default"] = d.ReviewBaseCandidates
	leaf("checkout", "auto_stash")["default"] = d.CheckoutAutoStash
	timeout := leaf("worktree", "prepare_timeout")
	timeout["default"] = d.WorktreePrepareTimeout
	leaf("worktree", "prepare_script")["maxLength"] = MaxPrepareBytes
	provider := leaf("git_host", "provider")
	provider["pattern"], provider["default"] = providerPattern, d.GitHostProvider
	if len(hosts) > 0 {
		provider["description"] = provider["description"].(string) + " Registered providers: " + strings.Join(hosts, ", ") + "."
	}
	props := root["properties"].(map[string]any)
	props["integration_branches"].(map[string]any)["maxItems"] = MaxIntegration
	envs := props["environments"].(map[string]any)
	envs["maxItems"] = MaxEnvironments
	envs["items"].(map[string]any)["properties"].(map[string]any)["deployed_sha_script"].(map[string]any)["maxLength"] = MaxEnvScriptBytes
	return json.Marshal(root)
}
