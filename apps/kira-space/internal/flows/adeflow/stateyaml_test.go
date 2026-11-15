package adeflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/internal/testx"
)

// stateYAML waits until the task's state.yaml parses and ok accepts it, then returns it.
func stateYAML(t *testing.T, app *flowharness.App, taskID string, ok func(map[string]any) bool) map[string]any {
	t.Helper()
	folder := dfFolder(t, app, taskID)
	var doc map[string]any
	testx.WaitUntil(t, waitFor, func() bool {
		raw, err := os.ReadFile(filepath.Join(folder, "state.yaml"))
		if err != nil {
			return false
		}
		var d map[string]any
		if yaml.Unmarshal(raw, &d) != nil {
			return false
		}
		doc = d
		return ok == nil || ok(d)
	})
	return doc
}

// stateChildren is the chain.children list of a parsed state.yaml.
func stateChildren(doc map[string]any) []map[string]any {
	chain, _ := doc["chain"].(map[string]any)
	list, _ := chain["children"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		if m, ok := c.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
