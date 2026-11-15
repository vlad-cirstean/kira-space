package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Types and tool texts of the v0.3.13 tools the benchmark's frozen arm T and the P643 spike still
// serve. agentmcp no longer has them; testdata/details-bench/tools-baseline.json holds their schemas.

type frozenListTasksArgs struct {
	State string `json:"state,omitempty"`
}

type frozenChildTaskInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Kind         string `json:"kind"`
	Workflow     string `json:"workflow"`
	Status       string `json:"status"`
	Step         string `json:"step"`
	Finished     bool   `json:"finished"`
	Archived     bool   `json:"archived"`
	CreatedAt    string `json:"createdAt"`
	LastHandover string `json:"lastHandover"`
}

type frozenListTasksResult struct {
	Tasks []frozenChildTaskInfo `json:"tasks"`
	More  bool                  `json:"more,omitempty"`
}

type frozenTaskChainArgs struct {
	TaskID string `json:"taskId,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Before string `json:"before,omitempty"`
}

type frozenChainRow struct {
	ID         string `json:"id"`
	From       string `json:"from"`
	To         string `json:"to"`
	Source     string `json:"source"`
	State      string `json:"state"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	FromTaskID string `json:"fromTaskId,omitempty"`
}

type frozenTaskChainResult struct {
	Rows []frozenChainRow `json:"rows"`
	More bool             `json:"more,omitempty"`
	Next string           `json:"next,omitempty"`
}

// frozenToolText is the v0.3.13 description of a kira-space tool, from the baseline fixture.
func frozenToolText(name string) string {
	raw, err := os.ReadFile(filepath.Join(benchFixtures, "tools-baseline.json"))
	if err != nil {
		panic(err)
	}
	var b benchBaseline
	if err := json.Unmarshal(raw, &b); err != nil {
		panic(err)
	}
	for _, servers := range b.Roles {
		for _, tools := range servers {
			for _, tl := range tools {
				if tl.Name == name {
					return tl.Description
				}
			}
		}
	}
	panic("baseline has no tool " + name)
}

type frozenOutcomeQuery struct {
	RunID  string
	Branch string
	Kind   string
	Limit  int
}

type frozenRunOutcomeEntry struct {
	RunID      string `json:"runId"`
	Kind       string `json:"kind"`
	Step       string `json:"step"`
	Repo       string `json:"repo"`
	Branch     string `json:"branch"`
	State      string `json:"state"`
	FinishedAt *int64 `json:"finishedAt"`
	Outcome    any    `json:"outcome"`
}
