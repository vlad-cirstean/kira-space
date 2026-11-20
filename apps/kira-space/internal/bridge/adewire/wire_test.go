package adewire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/githost"
)

const fixtureDir = "../../../tests/fixtures/ade-v2"

func TestFixturesMatchWireTypes(t *testing.T) {
	table := map[string]func() any{
		"board.json":               func() any { return new(Board) },
		"change-requests.json":     func() any { return new(ChangeRequestsResult) },
		"host-providers.json":      func() any { return new(githost.Providers) },
		"workflows.json":           func() any { return new(WorkflowsResult) },
		"workflow-yaml.json":       func() any { return new(WorkflowYaml) },
		"workflow-validation.json": func() any { return new(WorkflowValidation) },
		"repos.json":               func() any { return new(ReposResult) },
		"folder-import.json":       func() any { return new(FolderImportResult) },
		"candidates.json":          func() any { return new(CandidateBranchesResult) },
		"add-existing-branch.json": func() any { return new(AddExistingBranchResult) },
		"task.json":                func() any { return new(Task) },
		"branch.json":              func() any { return new(Branch) },
		"refresh.json":             func() any { return new(RefreshResult) },
		"force-push.json":          func() any { return new(ForcePushResult) },
		"archive-risk.json":        func() any { return new(ArchiveRisk) },
		"start-run.json":           func() any { return new(StartRunResult) },
		"log-page.json":            func() any { return new(LogPage) },
		"sessions.json":            func() any { return new(SessionsResult) },
		"launch.json":              func() any { return new(Launch) },
		"event-runs.json":          func() any { return new(RunsChangedEvent) },
		"event-log.json":           func() any { return new(LogEvent) },
		"event-open-session.json":  func() any { return new(OpenSessionEvent) },
		"review-target.json":       func() any { return new(ReviewWindowTarget) },
		"review-agent.json":        func() any { return new(ReviewAgentState) },
		"review-agent-launch.json": func() any { return new(ReviewAgentLaunch) },
	}

	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk []string
	for _, e := range entries {
		onDisk = append(onDisk, e.Name())
	}
	var inTable []string
	for name := range table {
		inTable = append(inTable, name)
	}
	sort.Strings(onDisk)
	sort.Strings(inTable)
	if !reflect.DeepEqual(onDisk, inTable) {
		t.Fatalf("fixture files %v differ from table %v", onDisk, inTable)
	}

	for name, newT := range table {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(fixtureDir, name))
			if err != nil {
				t.Fatal(err)
			}
			v := newT()
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(v); err != nil {
				t.Fatalf("decode: %v", err)
			}
			again, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(again, &got); err != nil {
				t.Fatal(err)
			}
			if path, a, b := firstDiff("$", want, got); path != "" {
				t.Fatalf("round trip differs at %s: fixture %v, re-encoded %v", path, a, b)
			}
		})
	}
}

func firstDiff(path string, a, b any) (string, any, any) {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return path, a, b
		}
		keys := map[string]bool{}
		for k := range av {
			keys[k] = true
		}
		for k := range bv {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			x, xok := av[k]
			y, yok := bv[k]
			if !xok || !yok {
				return path + "." + k, x, y
			}
			if p, x2, y2 := firstDiff(path+"."+k, x, y); p != "" {
				return p, x2, y2
			}
		}
		return "", nil, nil
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return path, a, b
		}
		for i := range av {
			if p, x, y := firstDiff(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i]); p != "" {
				return p, x, y
			}
		}
		return "", nil, nil
	default:
		if !reflect.DeepEqual(a, b) {
			return path, a, b
		}
		return "", nil, nil
	}
}
