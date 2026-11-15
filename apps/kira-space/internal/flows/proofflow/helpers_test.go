package proofflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge/adewire"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/flowharness"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/taskfiles"
	"github.com/kirathecat/kira-studio/internal/agentmcp"
	"github.com/kirathecat/kira-studio/internal/approvals"
	"github.com/kirathecat/kira-studio/internal/prompts"
)

var ctx = context.Background()

// fixture is an app with one task whose folder holds the proofs.
type fixture struct {
	app    *flowharness.App
	taskID string
	folder string
	// last holds the steps each proof was written with, so an edit can keep or replace one by ordinal.
	last map[string][]map[string]any
	// summary is written to the next proof; slug, when set, names the folder of the next write once.
	summary, slug string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	app := flowharness.New(t)
	task, err := app.W.AdeTask.CreateTask(ctx, adewire.CreateTaskArgs{Title: "Login form"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	dir, err := app.W.AdeTask.DetailsFolder(ctx, adewire.DiskChangesArgs{TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{app: app, taskID: task.ID, folder: dir.Path, last: map[string][]map[string]any{}}
}

// saved is what a submit of one proof answers.
type saved struct {
	ID, Slug, Name string
	// Reply is the whole submit reply; Pending is true when the edit waits for review.
	Reply   string
	Pending bool
}

// slugOf names the folder of a proof.
func slugOf(name string) string { return taskfiles.SuggestSlug(name) }

// write renders the proof folder: proof.md from the step maps and a side file per image or data file.
// A step holds description, view, query, language, source and data (inline YAML); "image" is raw
// bytes written as a file and "file" a name for a data file; "id" names it (default s<position>);
// keepOrdinal and replacesOrdinal reuse the id of the previous write's step at that ordinal.
func (f *fixture) write(t *testing.T, name string, steps []map[string]any) string {
	t.Helper()
	slug := slugOf(name)
	if f.slug != "" {
		slug, f.slug = f.slug, ""
	}
	dir := filepath.Join(f.folder, "proofs", slug)
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := f.last[name]
	var out []any
	var written []map[string]any
	fresh := 0
	for i, st := range steps {
		if k, ok := st["keepOrdinal"].(int); ok && k >= 1 && k <= len(prev) {
			st = prev[k-1]
		} else if r, ok := st["replacesOrdinal"].(int); ok && r >= 1 && r <= len(prev) {
			cp := map[string]any{}
			for key, v := range st {
				if key != "replacesOrdinal" {
					cp[key] = v
				}
			}
			cp["id"] = prev[r-1]["id"]
			st = cp
		}
		id, _ := st["id"].(string)
		if id == "" {
			id = fmt.Sprintf("s%d", i+1)
			if len(prev) > 0 {
				fresh++
				id = fmt.Sprintf("n%d", fresh)
			}
		}
		step := map[string]any{"id": id}
		keep := map[string]any{"id": id}
		for k, v := range st {
			keep[k] = v
			switch k {
			case "id", "image", "file", "data", "keepOrdinal", "replacesOrdinal":
			default:
				step[k] = v
			}
		}
		switch {
		case st["image"] != nil:
			raw := st["image"].([]byte)
			ext, _ := st["ext"].(string)
			if ext == "" {
				ext = "png"
			}
			file := id + "." + ext
			if err := os.WriteFile(filepath.Join(dir, file), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			step["data"] = file
		case st["file"] != nil:
			file := st["file"].(string)
			raw, err := json.Marshal(st["data"])
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, file), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			step["data"] = file
		case st["view"] == "diagram":
			if d, ok := st["data"].(map[string]any); ok {
				step["inline"] = d["source"]
			}
		default:
			step["inline"] = st["data"]
		}
		delete(step, "ext")
		out = append(out, step)
		written = append(written, keep)
	}
	f.last[name] = written
	body, err := yaml.Marshal(map[string]any{"name": name, "summary": f.summary, "steps": out})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proof.md"), append(append([]byte("---\n"), body...), []byte("---\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug
}

// submit runs the agent's submit for the task and returns the reply.
func (f *fixture) submit(t *testing.T, reason string) string {
	t.Helper()
	reply, err := f.app.W.AdeBoard.Submit(ctx, agentmcp.SpaceCall{TaskID: f.taskID, SessionID: "session-1"}, agentmcp.SubmitArgs{Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

// save writes a proof and submits it; the proof must be accepted.
func (f *fixture) save(t *testing.T, name string, steps ...map[string]any) saved {
	t.Helper()
	slug := f.write(t, name, steps)
	reply := f.submit(t, "")
	if !strings.Contains(reply, "accepted") || strings.Contains(reply, "invalid 1") {
		t.Fatalf("submit of %q = %q, want accepted", name, reply)
	}
	return saved{ID: f.idOf(t, name), Slug: slug, Name: name, Reply: reply}
}

// saveFail writes a proof that must be refused and returns the reply; no proof of that name remains.
func (f *fixture) saveFail(t *testing.T, name string, steps ...map[string]any) string {
	t.Helper()
	slug := f.write(t, name, steps)
	reply := f.submit(t, "")
	if !strings.Contains(reply, "invalid") || strings.Contains(reply, "invalid 0") {
		t.Fatalf("submit of %q = %q, want invalid", name, reply)
	}
	// An invalid folder stays on disk and in the next submit's reply: clear it.
	if err := os.RemoveAll(filepath.Join(f.folder, "proofs", slug)); err != nil {
		t.Fatal(err)
	}
	f.submit(t, "")
	delete(f.last, name)
	return reply
}

// edit rewrites an accepted proof and submits with a reason; the edit waits for review.
func (f *fixture) edit(t *testing.T, name, reason string, steps ...map[string]any) saved {
	t.Helper()
	slug := f.write(t, name, steps)
	reply := f.submit(t, reason)
	return saved{ID: f.idOf(t, name), Slug: slug, Name: name, Reply: reply, Pending: strings.Contains(reply, "pending user 1")}
}

// proofs lists the task's proofs.
func (f *fixture) proofs(t *testing.T) []adewire.TaskProof {
	t.Helper()
	res, err := f.app.W.AdeTask.TaskProofs(ctx, adewire.TaskArgs{TaskID: f.taskID})
	if err != nil {
		t.Fatal(err)
	}
	return res.Proofs
}

func (f *fixture) idOf(t *testing.T, name string) string {
	t.Helper()
	for _, p := range f.proofs(t) {
		if p.Name == name {
			return p.ProofID
		}
	}
	t.Fatalf("no proof named %q in %+v", name, f.proofs(t))
	return ""
}

func tableStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "table", "query": "select id, name, tags from orders", "language": "sql", "source": "postgres",
		"data": map[string]any{
			"columns": []any{map[string]any{"name": "id"}, map[string]any{"name": "name"}, map[string]any{"name": "tags", "type": "json"}},
			"rows":    []any{[]any{1, "alpha", []any{"a", "b"}}, []any{2, nil, nil}, []any{9007199254740993, "<b>bold</b>", map[string]any{"k": 1}}},
		},
	}
}

func documentStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "document", "source": "elasticsearch", "language": "json",
		"query": `{"query":{"match_all":{}}}`,
		"data": map[string]any{"documents": []any{
			map[string]any{"body": map[string]any{"z": 1, "a": map[string]any{"nested": true}}},
			map[string]any{"id": "doc-b", "body": map[string]any{"title": "Second"}},
		}},
	}
}

func keyValueStep(desc string) map[string]any {
	return map[string]any{
		"description": desc, "view": "keyvalue",
		"data": map[string]any{"entries": []any{
			map[string]any{"field": "status", "value": "ok"},
			map[string]any{"field": "latencyMs", "value": 12.5},
		}},
	}
}

func proofPrompts(t *testing.T, app *flowharness.App) []prompts.Routed {
	t.Helper()
	all, err := app.W.PromptsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []prompts.Routed
	for _, p := range all {
		if p.Kind == prompts.KindProof {
			out = append(out, p)
		}
	}
	return out
}

func reviewItems(t *testing.T, app *flowharness.App) []approvals.Item {
	t.Helper()
	all, err := app.W.ApprovalsSvc.List()
	if err != nil {
		t.Fatal(err)
	}
	var out []approvals.Item
	for _, it := range all {
		if it.Kind == approvals.KindProofReview {
			out = append(out, it)
		}
	}
	return out
}

func session(t *testing.T, app *flowharness.App, id string) proof.Detail {
	t.Helper()
	d, err := app.W.Proof.Session(bridge.ProofSessionArgs{SessionID: id})
	if err != nil {
		t.Fatalf("Session(%s): %v", id, err)
	}
	return d
}

func chartStep(desc, source string, data map[string]any) map[string]any {
	return map[string]any{"description": desc, "view": "chart", "source": source, "data": data}
}

func lineChartStep() map[string]any {
	step := chartStep("Errors per minute around the deploy", "elasticsearch", map[string]any{
		"type": "line",
		"x":    map[string]any{"type": "time", "name": "time"},
		"y":    map[string]any{"name": "errors", "unit": "req"},
		"series": []any{
			map[string]any{"name": "before", "points": []any{
				[]any{"2026-01-15T12:02:00Z", 4}, []any{"2026-01-15T12:00:00Z", 7}, []any{1768478460000, 5},
			}},
			map[string]any{"name": "after <b>fix</b>", "points": []any{
				[]any{"2026-01-15T13:03:00+01:00", 1}, []any{"2026-01-15T12:04:00.500Z", nil}, []any{"2026-01-15T12:05:00Z", 0},
			}},
		},
	})
	step["language"] = "json"
	step["query"] = `{"aggs":{"per_minute":{"date_histogram":{"field":"@timestamp","fixed_interval":"1m"}}}}`
	return step
}

func stackedBarStep(fiveXXWeb int) map[string]any {
	return chartStep("Responses by service", "elasticsearch", map[string]any{
		"type": "bar", "stacked": true,
		"x": map[string]any{"type": "category", "name": "service"},
		"series": []any{
			map[string]any{"name": "2xx", "points": []any{[]any{"api", 120}, []any{"web", 80}, []any{"worker", 30}}},
			map[string]any{"name": "5xx", "points": []any{[]any{"web", fiveXXWeb}, []any{"api", 2}}},
		},
	})
}

func histogramStep() map[string]any {
	return chartStep("Response time histogram", "elasticsearch", map[string]any{
		"type": "bar",
		"x":    map[string]any{"type": "value", "name": "ms"},
		"series": []any{
			map[string]any{"name": "requests", "points": []any{[]any{0, 12}, []any{50, 40}, []any{100, 22}, []any{150, 5}}},
		},
	})
}

func areaStep() map[string]any {
	minute := func(i, v int) []any { return []any{1768478400000 + i*60000, v} }
	return chartStep("Requests by region", "elasticsearch", map[string]any{
		"type": "area", "stacked": true,
		"x": map[string]any{"type": "time", "name": "time"},
		"series": []any{
			map[string]any{"name": "eu", "points": []any{minute(0, 10), minute(1, 14), minute(2, 9), minute(3, 12), minute(4, 11)}},
			map[string]any{"name": "us", "points": []any{minute(0, 20), minute(1, 18), minute(2, 25), minute(3, 22), minute(4, 19)}},
		},
	})
}

func scatterStep() map[string]any {
	return chartStep("Latency vs payload size", "elasticsearch", map[string]any{
		"type": "scatter",
		"x":    map[string]any{"type": "value", "name": "bytes"},
		"y":    map[string]any{"name": "ms", "unit": "ms"},
		"series": []any{
			map[string]any{"name": "requests", "points": []any{
				[]any{120, 8}, []any{340, 11}, []any{800, 15}, []any{1500, 22},
				[]any{2400, 31}, []any{3900, 38}, []any{6100, 60}, []any{9000, 95},
			}},
		},
	})
}

func pieStep() map[string]any {
	return chartStep("Share of 5xx by endpoint", "elasticsearch", map[string]any{
		"type": "pie", "unit": "req",
		"slices": []any{
			map[string]any{"name": "/checkout", "value": 5},
			map[string]any{"name": "/login", "value": 2},
			map[string]any{"name": "/search", "value": 1},
		},
	})
}
