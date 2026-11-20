package llmprompts

import (
	"strings"
	"testing"
)

func problemsFor(t *testing.T, id ID, text string) []Problem {
	t.Helper()
	return validateText(id, text, defaultText)
}

func TestValidateText(t *testing.T) {
	longText := strings.Repeat("a", MaxTextBytes+1)
	cases := []struct {
		name string
		id   ID
		text string
		line int
		ok   bool
	}{
		{name: "default is valid", id: "ade.rebase.prompt", text: defaultText("ade.rebase.prompt"), ok: true},
		{name: "empty", id: "ade.sendBack.line", text: ""},
		{name: "too long", id: "dbmcp.instructions", text: longText},
		{name: "bad utf8", id: "dbmcp.instructions", text: "a\xffb"},
		{name: "parse error with line", id: "ade.sendBack.line", text: "ok\n{{if .From}}", line: 2},
		{name: "undeclared field", id: "ade.sendBack.line", text: "{{.From}}{{.Branch}}{{.Reason}}\n{{.Nope}}", line: 2},
		{name: "missing required var", id: "ade.rebase.prompt", text: "{{if .Stack}}rebase {{range .Stack}}{{.Name}}{{end}}{{end}}"},
		{name: "literal accepts braces", id: "dbmcp.instructions", text: "use {{ braces }}", ok: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ps := problemsFor(t, c.id, c.text)
			if c.ok {
				if len(ps) != 0 {
					t.Fatalf("want valid, got %+v", ps)
				}
				return
			}
			if len(ps) == 0 {
				t.Fatal("want a problem, got none")
			}
			if c.line != 0 && ps[0].Line != c.line {
				t.Fatalf("line = %d, want %d (%+v)", ps[0].Line, c.line, ps[0])
			}
		})
	}
}

func TestValidateChangesFragmentBreaksIncluder(t *testing.T) {
	ps := validateChanges(map[ID]string{"ade.repoLine": "{{.Base}}x"}, defaultText)
	if len(ps) != 1 || ps[0].ID != "ade.repoLine" || !strings.Contains(ps[0].Message, "ade.start.message") {
		t.Fatalf("want one finding on ade.repoLine naming ade.start.message, got %+v", ps)
	}
	if ps := validateChanges(map[ID]string{"ade.repoLine": "{{.Repo}}"}, defaultText); len(ps) != 0 {
		t.Fatalf("compatible edit refused: %+v", ps)
	}
}
