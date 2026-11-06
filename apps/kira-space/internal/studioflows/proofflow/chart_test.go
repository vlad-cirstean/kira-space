package proofflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

func chartPayload(t *testing.T, f *fixture, id string, rev, ordinal int) proof.StepDetail {
	t.Helper()
	st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: rev, Ordinal: ordinal})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestProofCharts(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Deploy health")
	steps := []map[string]any{lineChartStep(), stackedBarStep(6), histogramStep(), areaStep(), scatterStep(), pieStep()}
	for _, s := range steps {
		f.addStep(t, d, s)
	}
	f.closeDraft(t, d)
	id := sessionID(t, f.app, "Deploy health")

	list, err := f.app.W.Proof.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "proof-chart", "ProofService.Sessions", list, maskIDs)
	det := session(t, f.app, id)
	f.app.Contract(t, "proof-chart", "ProofService.Session", det, maskIDs)
	rev := det.Current.Rev

	keys := []string{"line", "bar", "histogram", "area", "scatter", "pie"}
	for i, key := range keys {
		st := chartPayload(t, f, id, rev, i+1)
		if st.View != "chart" {
			t.Fatalf("step %d view = %q, want chart", i+1, st.View)
		}
		f.app.Contract(t, "proof-chart", "ProofService.Step#"+key, st, maskIDs)
	}

	var line struct {
		Series []struct {
			Name   string  `json:"name"`
			Points [][]any `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(chartPayload(t, f, id, rev, 1).Data, &line); err != nil {
		t.Fatal(err)
	}
	before, after := line.Series[0], line.Series[1]
	var xs []float64
	for _, p := range before.Points {
		xs = append(xs, p[0].(float64))
	}
	if xs[0] != 1768478400000 || xs[1] != 1768478460000 || xs[2] != 1768478520000 {
		t.Fatalf("before x = %v, want ascending epoch ms", xs)
	}
	if after.Name != "after <b>fix</b>" || after.Points[1][1] != nil || after.Points[0][0].(float64) != 1768478580000 {
		t.Fatalf("after = %+v, want literal name, kept gap, offset time as UTC ms", after)
	}
	var bar struct {
		Series []struct {
			Points [][]any `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(chartPayload(t, f, id, rev, 2).Data, &bar); err != nil {
		t.Fatal(err)
	}
	if bar.Series[1].Points[0][0] != "web" || bar.Series[1].Points[1][0] != "api" {
		t.Fatalf("5xx points = %v, want given order", bar.Series[1].Points)
	}

	args := map[string]any{"description": "d", "view": "chart", "data": map[string]any{
		"type": "bar", "x": map[string]any{"type": "category"},
		"series": manySeries(21),
	}}
	if msg := f.fail(t, "proof_add_step", withDraft(args, f.start(t, "Too many series"))); !strings.Contains(msg, "20") {
		t.Fatalf("message = %q, want the series limit", msg)
	}

	set := func(ordinal int, c proof.ChartViewState) error {
		return f.app.W.Proof.SetViewState(bridge.ProofSetViewStateArgs{SessionID: id, Rev: rev, Ordinal: ordinal, State: proof.ViewState{Chart: &c}})
	}
	if err := set(2, proof.ChartViewState{Hidden: []string{"5xx"}, Selected: &proof.ChartPoint{Series: 0, Index: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := set(1, proof.ChartViewState{Hidden: []string{}, Zoom: &proof.ChartZoom{Start: 25, End: 75}}); err != nil {
		t.Fatal(err)
	}
	if err := set(1, proof.ChartViewState{Zoom: &proof.ChartZoom{Start: 80, End: 20}}); err == nil {
		t.Fatal("zoom start after end accepted")
	}
	vs, err := f.app.W.Proof.ViewState(bridge.ProofRevArgs{SessionID: id, Rev: rev})
	if err != nil || len(vs) != 2 {
		t.Fatalf("ViewState = %+v, %v; want two entries", vs, err)
	}
	f.app.Contract(t, "proof-chart", "ProofService.ViewState#chart", vs, maskIDs)

	var ed proof.StartResult
	f.call(t, "proof_edit_session", map[string]any{"name": "Deploy health", "reason": "Recount after replay"}, &ed)
	f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": 1})
	replaced := stackedBarStep(9)
	replaced["replacesOrdinal"] = 2
	f.addStep(t, ed.DraftID, replaced)
	for o := 3; o <= 6; o++ {
		f.addStep(t, ed.DraftID, map[string]any{"keepOrdinal": o})
	}
	f.closeDraft(t, ed.DraftID)
	det = session(t, f.app, id)
	if det.Pending == nil {
		t.Fatal("no pending revision")
	}
	f.app.Contract(t, "proof-chart", "ProofService.Session#pending", det, maskIDs)
	f.app.Contract(t, "proof-chart", "ProofService.Step#bar-new", chartPayload(t, f, id, det.Pending.Rev, 2), maskIDs)

	if err := f.app.W.Proof.AcceptEdit(bridge.ProofSessionArgs{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	det = session(t, f.app, id)
	vs, err = f.app.W.Proof.ViewState(bridge.ProofRevArgs{SessionID: id, Rev: det.Current.Rev})
	if err != nil || len(vs) != 1 || vs[0].Ordinal != 1 || vs[0].State.Chart == nil || vs[0].State.Chart.Zoom == nil || vs[0].State.Chart.Zoom.Start != 25 {
		t.Fatalf("view state after accept = %+v, %v; want step 1's chart zoom only", vs, err)
	}
}

func manySeries(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = map[string]any{"name": "s" + string(rune('a'+i)), "points": []any{[]any{"api", 1}}}
	}
	return out
}
