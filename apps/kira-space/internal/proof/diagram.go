package proof

import (
	"strings"
	"unicode/utf8"
)

// Diagram limits. The window renders the text, so these bound its layout cost.
const (
	maxDiagramBytes = 20_000
	maxDiagramLines = 1_000
)

// diagramTypes are the Mermaid header keywords a diagram step may start with. Chart-like types are
// left out (the chart view validates numbers), as are beta, plugin and icon-loading types.
var diagramTypes = []string{
	"flowchart", "graph", "sequenceDiagram", "classDiagram", "stateDiagram", "stateDiagram-v2",
	"erDiagram", "gantt", "journey", "gitGraph", "mindmap", "timeline",
}

type diagramIn struct {
	Source *string `json:"source"`
}

type diagramPayload struct {
	Type   string `json:"type"`
	Source string `json:"source"`
}

func checkDiagram(data []byte) (any, error) {
	var in diagramIn
	if err := decodeStrict(data, &in); err != nil {
		return nil, err
	}
	if in.Source == nil {
		return nil, invalid("data.source is required")
	}
	src := *in.Source
	if !utf8.ValidString(src) {
		return nil, invalid("data.source must be valid UTF-8")
	}
	src = strings.TrimRight(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(src), " \t\n")
	if len(src) < 1 || len(src) > maxDiagramBytes {
		return nil, invalid("data.source must be 1-%d bytes", maxDiagramBytes)
	}
	lines := strings.Split(src, "\n")
	if len(lines) > maxDiagramLines {
		return nil, invalid("data.source holds %d lines; the limit is %d", len(lines), maxDiagramLines)
	}
	for _, r := range src {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return nil, invalid("data.source must not contain control characters")
		}
	}
	first := strings.TrimSpace(lines[0])
	if first == "---" {
		return nil, invalid("data.source: front matter is not allowed; put the title in description")
	}
	typ := strings.FieldsFunc(first, func(r rune) bool { return r == ' ' || r == '\t' || r == ';' })
	if len(typ) == 0 || !contains(diagramTypes, typ[0]) {
		return nil, invalid("data.source must start with one of %s", strings.Join(diagramTypes, ", "))
	}
	if strings.Contains(src, "%%{") {
		return nil, invalid("data.source: %%{...}%% directives are not allowed")
	}
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "click ") {
			return nil, invalid("data.source: click lines are not allowed")
		}
	}
	if low := strings.ToLower(src); strings.Contains(low, "url(") || strings.Contains(low, "@import") {
		return nil, invalid("data.source: CSS url() and @import are not allowed")
	}
	return diagramPayload{Type: typ[0], Source: src}, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
