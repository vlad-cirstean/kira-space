package ids

import "regexp"

// Prefix names the feature a nanoid belongs to.
type Prefix string

// Registered prefixes. This file and packages/shared/ids.ts stay equal (check-conventions N13).
const (
	// Worktree is an ADE worktree folder name.
	Worktree Prefix = "wt"
	// Fact is a memory fact id.
	Fact Prefix = "km"
	// Group is a memory group id.
	Group Prefix = "kg"
	// Shot is a proof shot folder handle.
	Shot Prefix = "shot"
	// TUI is a TUI launch file stem.
	TUI Prefix = "tui"
	// Sandbox is a sandbox scratch dir name.
	Sandbox Prefix = "sbx"
	// Temp is a partial download file suffix.
	Temp Prefix = "tmp"
	// DockerTemp is a Docker temp container name suffix.
	DockerTemp Prefix = "dtc"
	// Distill is a distilled memory file name suffix.
	Distill Prefix = "dfl"
	// FlowRun is a flow test run tag used in Docker names.
	FlowRun Prefix = "frn"
	// Step is a workflow step id in user-edited YAML.
	Step Prefix = "stp"
	// Exemption is a DB MCP limit exemption id, minted on a limit refusal.
	Exemption Prefix = "exm"
)

var all = []Prefix{Worktree, Fact, Group, Shot, TUI, Sandbox, Temp, DockerTemp, Distill, FlowRun, Step, Exemption}

var sizes = map[Prefix]int{
	Worktree: 10, Fact: 12, Group: 12, Shot: 12, TUI: 12, Sandbox: 12,
	Temp: 12, DockerTemp: 12, Distill: 12, FlowRun: 12, Step: 12, Exemption: 12,
}

func (p Prefix) size() int { return sizes[p] }

func init() {
	prefixRe := regexp.MustCompile(`^[a-z]{2,4}$`)
	seen := map[Prefix]bool{}
	for _, p := range all {
		n, ok := sizes[p]
		if !ok || !prefixRe.MatchString(string(p)) || n < 10 || n > 21 || seen[p] {
			panic("ids: bad registry entry " + string(p))
		}
		seen[p] = true
	}
}
