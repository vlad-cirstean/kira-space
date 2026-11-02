package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func text(s string, marks ...string) string {
	m := ""
	for _, t := range marks {
		m += `{"type":"` + t + `"},`
	}
	b, _ := json.Marshal(s)
	if m != "" {
		return `{"type":"text","text":` + string(b) + `,"marks":[` + strings.TrimSuffix(m, ",") + `]}`
	}
	return `{"type":"text","text":` + string(b) + `}`
}

func para(inner ...string) string {
	return `{"type":"paragraph","content":[` + strings.Join(inner, ",") + `]}`
}

func doc(blocks ...string) string {
	return `{"type":"doc","version":1,"content":[` + strings.Join(blocks, ",") + `]}`
}

// testMedia resolves a.png only.
func testMedia(name string) string {
	if name == "a.png" {
		return "https://x.test/a b.png"
	}
	return ""
}

func TestAdfMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"escapes markup and html", doc(para(text("<script>x</script> *a* _b_ `c`"))), `\<script\>x\</script\> \*a\* \_b\_ \` + "`c\\`"},
		{"escapes heading-like text", doc(para(text("# x"))), `\# x`},
		{"escapes leading list number", doc(para(text("1. x"))), `1\. x`},
		{"escapes table pipe in text", doc(para(text("a|b"))), `a\|b`},
		{"heading", doc(`{"type":"heading","attrs":{"level":3},"content":[` + text("T") + `]}`), `### T`},
		{"blocks join with blank line", doc(para(text("a")), para(text("b"))), "a\n\nb"},
		{"marks nest", doc(para(text("x", "strong", "em"))), `_**x**_`},
		{"strike", doc(para(text("x", "strike"))), `~~x~~`},
		{"ignored marks", doc(para(text("x", "underline", "textColor"))), `x`},
		{"edge spaces stay outside mark", doc(para(text("a"), text(" x ", "strong"), text("b"))), `a **x** b`},
		{"code mark not escaped", doc(para(text("a*b", "code"))), "`a*b`"},
		{"code with backtick", doc(para(text("a`b", "code"))), "``a`b``"},
		{"link", doc(para(`{"type":"text","text":"go","marks":[{"type":"link","attrs":{"href":"https://x.test/a b(c)"}}]}`)), `[go](https://x.test/a%20b%28c%29)`},
		{"mailto link", doc(para(`{"type":"text","text":"me","marks":[{"type":"link","attrs":{"href":"mailto:a@b.c"}}]}`)), `[me](mailto:a@b.c)`},
		{"javascript link dropped", doc(para(`{"type":"text","text":"bad","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}`)), `bad`},
		{"hard break", doc(para(text("a"), `{"type":"hardBreak"}`, text("b"))), "a\\\nb"},
		{"nested bullets", doc(`{"type":"bulletList","content":[{"type":"listItem","content":[` + para(text("a")) + `,{"type":"bulletList","content":[{"type":"listItem","content":[` + para(text("b")) + `]}]}]},{"type":"listItem","content":[` + para(text("c")) + `]}]}`), "- a\n  - b\n- c"},
		{"ordered start", doc(`{"type":"orderedList","attrs":{"order":3},"content":[{"type":"listItem","content":[` + para(text("a")) + `]},{"type":"listItem","content":[` + para(text("b")) + `]}]}`), "3. a\n4. b"},
		{"task list", doc(`{"type":"taskList","content":[{"type":"taskItem","attrs":{"state":"DONE"},"content":[` + text("a") + `]},{"type":"taskItem","attrs":{"state":"TODO"},"content":[` + text("b") + `]}]}`), `- \[x\] a` + "\n" + `- \[ \] b`},
		{"code fence holds backticks", bt(doc(`{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"a ~~~ b"}]}`)), "````go\na ``` b\n````"},
		{"code fence bad language", doc(`{"type":"codeBlock","attrs":{"language":"x y"},"content":[{"type":"text","text":"a"}]}`), "```\na\n```"},
		{"blockquote", doc(`{"type":"blockquote","content":[` + para(text("a")) + `,` + para(text("b")) + `]}`), "> a\n>\n> b"},
		{"panel", doc(`{"type":"panel","attrs":{"panelType":"info"},"content":[` + para(text("a")) + `]}`), "> [!NOTE]\n> a"},
		{"panel warning", doc(`{"type":"panel","attrs":{"panelType":"warning"},"content":[` + para(text("a")) + `]}`), "> [!WARNING]\n> a"},
		{"panel unknown kind", doc(`{"type":"panel","attrs":{"panelType":"custom"},"content":[` + para(text("a")) + `]}`), "> [!NOTE]\n> a"},
		{"rule", doc(`{"type":"rule"}`), `---`},
		{"table with pipe", doc(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[` + para(text("h")) + `]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[` + para(text("a"), `{"type":"hardBreak"}`, text("b|c")) + `]}]}]}`), "| h |\n| --- |\n| a b\\|c |"},
		{"mention", doc(para(`{"type":"mention","attrs":{"text":"@Dana *D*"}}`)), `[@Dana \*D\*](#kira-mention)`},
		{"emoji", doc(para(`{"type":"emoji","attrs":{"shortName":":smile:","text":"😄"}}`, `{"type":"emoji","attrs":{"shortName":":x:"}}`)), `😄:x:`},
		{"inline card", doc(para(`{"type":"inlineCard","attrs":{"url":"https://x.test/p"}}`)), `[https://x.test/p](https://x.test/p)`},
		{"card with bad scheme", doc(para(`{"type":"inlineCard","attrs":{"url":"javascript:1"}}`)), ``},
		{"block card", doc(`{"type":"blockCard","attrs":{"url":"https://x.test/p"}}`), `[https://x.test/p](https://x.test/p)`},
		{"media resolved", doc(`{"type":"mediaSingle","content":[{"type":"media","attrs":{"alt":"a.png"}}]}`), `![a.png](https://x.test/a%20b.png)`},
		{"media unresolved", doc(`{"type":"mediaSingle","content":[{"type":"media","attrs":{"alt":"b.png"}}]}`), `![b.png](#kira-attachment)`},
		{"media external", doc(`{"type":"mediaSingle","content":[{"type":"media","attrs":{"type":"external","url":"https://x.test/e.png","alt":"e"}}]}`), `![e](https://x.test/e.png)`},
		{"media file name and default", doc(`{"type":"mediaGroup","content":[{"type":"media","attrs":{"__fileName":"f.txt"}},{"type":"media","attrs":{}}]}`), `![f.txt](#kira-attachment) ![file](#kira-attachment)`},
		{"date", doc(para(`{"type":"date","attrs":{"timestamp":"1704153600000"}}`)), `2024-01-02`},
		{"status", doc(para(`{"type":"status","attrs":{"text":"DONE"}}`)), `[DONE](#kira-status-neutral)`},
		{"status green", doc(para(`{"type":"status","attrs":{"text":"DONE","color":"Green"}}`)), `[DONE](#kira-status-green)`},
		{"status odd color", doc(para(`{"type":"status","attrs":{"text":"X","color":"pink)"}}`)), `[X](#kira-status-neutral)`},
		{"expand", doc(`{"type":"expand","attrs":{"title":"More"},"content":[` + para(text("a")) + `]}`), "**More**\n\na"},
		{"bodied extension", doc(`{"type":"bodiedExtension","content":[` + para(text("a")) + `]}`), `a`},
		{"extension dropped", doc(`{"type":"extension","attrs":{}}`, para(text("a"))), `a`},
		{"unknown node with content", doc(`{"type":"mystery","content":[` + para(text("a")) + `]}`), `a`},
		{"unknown node with text", doc(`{"type":"mystery","text":"*a*"}`), `\*a\*`},
		{"not a document", `"x"`, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := adfMarkdown(json.RawMessage(c.in), testMedia); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestAdfDepthCap(t *testing.T) {
	in := para(text("deep"))
	for range adfMaxDepth + 10 {
		in = `{"type":"blockquote","content":[` + in + `]}`
	}
	if got := adfMarkdown(json.RawMessage(doc(in)), nil); strings.Contains(got, "deep") {
		t.Fatal("content past the depth cap survived")
	}
	shallow := para(text("ok"))
	for range 5 {
		shallow = `{"type":"blockquote","content":[` + shallow + `]}`
	}
	if got := adfMarkdown(json.RawMessage(doc(shallow)), nil); !strings.Contains(got, "ok") {
		t.Fatalf("shallow nesting lost: %q", got)
	}
}

func TestPlainMarkdown(t *testing.T) {
	got := plainMarkdown("h1. T <b>\r\nline\r\n\r\n* x")
	want := "h1. T \\<b\\>\\\nline\n\n\\* x"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func bt(s string) string { return strings.ReplaceAll(s, "~~~", "```") }
