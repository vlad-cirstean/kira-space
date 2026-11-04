package configfs

import (
	"errors"
	"strings"

	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// Edit is one form field change, in the file's own key vocabulary.
type Edit struct {
	Path  []string `json:"path"`
	Value any      `json:"value"`
	Unset bool     `json:"unset"`
}

// FormResult is the draft after the edits and its decoded value. Value is nil when Problem is set.
type FormResult struct {
	Text    string   `json:"text"`
	Value   any      `json:"value"`
	Problem *Problem `json:"problem"`
}

// Form applies edits to text as YAML node edits, then decodes the result with the module's own
// DecodeFunc. With no edits the text is unchanged. A file that does not parse comes back as a
// Problem with its line, never an error. References (${secret:...}, ${NAME}) stay verbatim: Form
// never resolves them. Callers enforce their file size cap on input and output.
func Form[T any](decode DecodeFunc[T], name string, text []byte, edits []Edit) FormResult {
	out := text
	if len(edits) > 0 {
		sets := make([]NodeSet, len(edits))
		for i, e := range edits {
			sets[i] = NodeSet{Path: e.Path, Value: e.Value, Unset: e.Unset}
		}
		src := text
		if strings.TrimSpace(string(src)) == "" {
			src = []byte("{}\n")
		}
		edited, err := EditYAML(src, sets)
		if err != nil {
			if _, p := decode(name, text); p != nil {
				return FormResult{Text: string(text), Problem: p}
			}
			return FormResult{Text: string(text), Problem: editProblem(err, edits)}
		}
		out = edited
	}
	v, p := decode(name, out)
	if p != nil {
		return FormResult{Text: string(out), Problem: p}
	}
	return FormResult{Text: string(out), Value: v}
}

func editProblem(err error, edits []Edit) *Problem {
	if errors.Is(err, ErrNotMapping) {
		field := ""
		if len(edits) > 0 {
			field = strings.Join(edits[0].Path, ".")
		}
		return &Problem{Message: i18n.T("backend.core.config.type.body", map[string]string{"field": field, "want": "mapping"})}
	}
	return &Problem{Message: i18n.T("backend.core.config.syntax.body", map[string]string{"detail": err.Error()})}
}
