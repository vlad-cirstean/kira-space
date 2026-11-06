package proofflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
)

func diagramStep(desc string, source string) map[string]any {
	return map[string]any{"description": desc, "view": "diagram", "source": "mermaid", "data": map[string]any{"source": source}}
}

func TestProofDiagram(t *testing.T) {
	f := newFixture(t)
	d := f.start(t, "Checkout flow")

	sources := []string{
		"flowchart TD\r\n  Cart --> Checkout\r\n  Checkout --> Paid\r\n\r\n",
		"sequenceDiagram\n  Browser->>API: POST /pay\n  API->>DB: insert\n",
		"flowchart LR\n  I@{ img: \"https://example.invalid/x.png\" }\n  I --> J",
		"flowchart TD\n  A -->",
	}
	for i, src := range sources {
		f.addStep(t, d, diagramStep("Diagram "+string(rune('1'+i)), src))
	}

	refused := []struct{ name, source, want string }{
		{"unknown header", "pie\n  \"a\": 1", "must start with one of"},
		{"front matter", "---\ntitle: x\n---\nflowchart TD\n  A --> B", "front matter"},
		{"directive", "%%{init: {}}%%\nflowchart TD\n  A --> B", "must start with one of"},
		{"directive later", "flowchart TD\n  %%{init: {}}%%\n  A --> B", "directives"},
		{"click", "flowchart TD\n  A --> B\n  click A cb", "click lines"},
		{"url", "flowchart TD\n  A --> B\n  style A fill:url(#x)", "url()"},
		{"import", "flowchart TD\n  A --> B\n  %% @IMPORT x", "@import"},
		{"empty", "  \n", "must be 1-20000 bytes"},
		{"too many bytes", "flowchart TD\n" + strings.Repeat("a", 20001), "20000 bytes"},
		{"too many lines", "flowchart TD" + strings.Repeat("\n A --> B", 1000), "1001 lines"},
		{"control", "flowchart TD\n  A --> B\x00", "control characters"},
	}
	for _, c := range refused {
		msg := f.fail(t, "proof_add_step", withDraft(diagramStep("bad", c.source), d))
		if !strings.Contains(msg, c.want) {
			t.Errorf("%s: message = %q, want %q", c.name, msg, c.want)
		}
	}
	extra := diagramStep("bad", "flowchart TD\n  A --> B")
	extra["data"] = map[string]any{"source": "flowchart TD\n  A --> B", "theme": "dark"}
	if msg := f.fail(t, "proof_add_step", withDraft(extra, d)); !strings.Contains(msg, "theme") {
		t.Errorf("unknown field: message = %q, want it named", msg)
	}

	f.closeDraft(t, d)
	id := sessionID(t, f.app, "Checkout flow")
	list, err := f.app.W.Proof.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	f.app.Contract(t, "proof-diagram", "ProofService.Sessions", list, maskIDs)
	det := session(t, f.app, id)
	f.app.Contract(t, "proof-diagram", "ProofService.Session", det, maskIDs)
	if len(det.Current.Steps) != 4 {
		t.Fatalf("steps = %d, want 4", len(det.Current.Steps))
	}
	for i, key := range []string{"flowchart", "sequence", "image", "broken"} {
		st, err := f.app.W.Proof.Step(bridge.ProofStepArgs{SessionID: id, Rev: det.Current.Rev, Ordinal: i + 1})
		if err != nil {
			t.Fatal(err)
		}
		if st.View != "diagram" {
			t.Fatalf("step %d view = %q, want diagram", i+1, st.View)
		}
		var p struct{ Type, Source string }
		if err := json.Unmarshal(st.Data, &p); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(p.Source, "\r") || strings.HasSuffix(p.Source, "\n") {
			t.Fatalf("step %d source not normalized: %q", i+1, p.Source)
		}
		wantType := strings.Fields(sources[i])[0]
		if p.Type != wantType {
			t.Fatalf("step %d type = %q, want %q", i+1, p.Type, wantType)
		}
		f.app.Contract(t, "proof-diagram", "ProofService.Step#"+key, st, maskIDs)
	}
}
