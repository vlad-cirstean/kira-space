package mobileweb

import (
	"encoding/json"
	"net/http"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/storage/repos"
)

// phoneRunLimit is how many recent script runs the phone list holds.
const phoneRunLimit = 30

var (
	phoneRunKeys = []string{
		"id", "scriptName", "color", "kind", "trigger", "state", "createdAt", "startedAt", "finishedAt",
		"taskId", "taskTitle", "branchId", "branchLabel", "resumable",
	}
	phoneOutcomeKeys = []string{"status", "reason", "exitCode"}
)

// phoneRun projects one script run to a keep-list: cwd, command, prompt, params, tools, terminal
// id, output tail and last error never reach a phone, and a field added to the run stays private
// until it is listed here.
func phoneRun(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var run map[string]any
	if err := json.Unmarshal(raw, &run); err != nil {
		return nil, err
	}
	out := keep(run, phoneRunKeys)
	if outcome, ok := run["outcome"].(map[string]any); ok {
		out["outcome"] = keep(outcome, phoneOutcomeKeys)
	} else {
		out["outcome"] = nil
	}
	return out, nil
}

func keep(m map[string]any, keys []string) map[string]any {
	out := make(map[string]any, len(keys)+1)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}

// handleRuns serves the newest script runs, projected. Without a seam it answers an empty list.
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
	runs := []any{}
	if s.cfg.ScriptRuns != nil {
		got, err := s.cfg.ScriptRuns(r.Context(), phoneRunLimit)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		for _, run := range got {
			p, err := phoneRun(run)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			runs = append(runs, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// Computer is the status of the machine around the app that a phone may see.
type Computer struct {
	Docker *DockerSummary `json:"docker"`
}

// DockerSummary is the engine's counts only: endpoint, message and version stay on the desktop.
type DockerSummary struct {
	State   string `json:"state"`
	Reason  string `json:"reason"`
	Running int    `json:"running"`
	Stopped int    `json:"stopped"`
	Total   int    `json:"total"`
}

// handleComputer serves the computer summary. Without a seam Docker is null.
func (s *Server) handleComputer(w http.ResponseWriter, r *http.Request, _ repos.MobileDeviceRow) {
	out := Computer{}
	if s.cfg.Computer != nil {
		var err error
		if out, err = s.cfg.Computer(r.Context()); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}
