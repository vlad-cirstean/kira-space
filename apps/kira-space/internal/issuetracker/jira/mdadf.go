package jira

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	mdtext "github.com/yuin/goldmark/text"

	"github.com/kirathecat/kira-studio/internal/ids"
)

// Hand-written walk over goldmark's tree: no Go Markdown-to-ADF converter escapes text and filters
// link schemes to the bar adf.go sets for the opposite direction.

type adfDoc = map[string]any

var mdParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

var errEmptyComment = errors.New("jira: empty comment")

// markdownADF converts Markdown to an ADF document. Raw HTML stays literal text, never markup.
func markdownADF(md string) (json.RawMessage, error) {
	src := []byte(md)
	root := mdParser.Parse(mdtext.NewReader(src))
	content := mdBlockNodes(root, src)
	if len(content) == 0 {
		return nil, errEmptyComment
	}
	return json.Marshal(adfDoc{"version": 1, "type": "doc", "content": content})
}

func mdNode(typ string, content []adfDoc) adfDoc {
	n := adfDoc{"type": typ}
	if content == nil {
		content = []adfDoc{}
	}
	n["content"] = content
	return n
}

// mdBlockNodes converts n's block children.
func mdBlockNodes(n ast.Node, src []byte) []adfDoc {
	var out []adfDoc
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, mdBlock(c, src)...)
	}
	return out
}

func mdBlock(n ast.Node, src []byte) []adfDoc {
	switch v := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		in := mdInlines(n, src, nil)
		if len(in) == 0 {
			return nil
		}
		return []adfDoc{mdNode("paragraph", in)}
	case *ast.Heading:
		h := mdNode("heading", mdInlines(n, src, nil))
		h["attrs"] = adfDoc{"level": v.Level}
		return []adfDoc{h}
	case *ast.ThematicBreak:
		return []adfDoc{{"type": "rule"}}
	case *ast.Blockquote:
		c := mdBlockNodes(n, src)
		if len(c) == 0 {
			return nil
		}
		return []adfDoc{mdNode("blockquote", c)}
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return []adfDoc{mdCodeBlock(n, src)}
	case *ast.HTMLBlock:
		return mdHTMLBlock(v, src)
	case *ast.List:
		return []adfDoc{mdList(v, src, false)}
	case *extast.Table:
		return []adfDoc{mdTable(v, src)}
	}
	return mdBlockNodes(n, src)
}

func mdCodeBlock(n ast.Node, src []byte) adfDoc {
	var b bytes.Buffer
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	code := strings.TrimSuffix(b.String(), "\n")
	var content []adfDoc
	if code != "" {
		content = []adfDoc{{"type": "text", "text": code}}
	}
	cb := mdNode("codeBlock", content)
	if f, ok := n.(*ast.FencedCodeBlock); ok {
		if lang := f.Language(src); len(lang) > 0 {
			cb["attrs"] = adfDoc{"language": string(lang)}
		}
	}
	return cb
}

func mdHTMLBlock(n *ast.HTMLBlock, src []byte) []adfDoc {
	var content []adfDoc
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		line := strings.TrimRight(string(seg.Value(src)), "\r\n")
		if len(content) > 0 {
			content = append(content, adfDoc{"type": "hardBreak"})
		}
		if line != "" {
			content = append(content, adfDoc{"type": "text", "text": line})
		}
	}
	if len(content) == 0 {
		return nil
	}
	return []adfDoc{mdNode("paragraph", content)}
}

func listItemHasCheckbox(item ast.Node) bool {
	first := item.FirstChild()
	if first == nil {
		return false
	}
	_, ok := first.FirstChild().(*extast.TaskCheckBox)
	return ok
}

func mdList(l *ast.List, src []byte, task bool) adfDoc {
	if l.FirstChild() != nil && listItemHasCheckbox(l.FirstChild()) {
		task = true
	}
	var items []adfDoc
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		if task {
			items = append(items, mdTaskItems(item, src)...)
		} else {
			items = append(items, mdListItem(item, src))
		}
	}
	switch {
	case task:
		tl := mdNode("taskList", items)
		tl["attrs"] = adfDoc{"localId": ids.NewID()}
		return tl
	case l.IsOrdered():
		ol := mdNode("orderedList", items)
		ol["attrs"] = adfDoc{"order": l.Start}
		return ol
	}
	return mdNode("bulletList", items)
}

func mdListItem(item ast.Node, src []byte) adfDoc {
	content := mdBlockNodes(item, src)
	if len(content) == 0 {
		content = []adfDoc{mdNode("paragraph", nil)}
	}
	return mdNode("listItem", content)
}

// mdTaskItems makes one taskItem from the item's first text block; nested lists follow as a nested
// taskList (ADF forbids other nesting inside a task list).
func mdTaskItems(item ast.Node, src []byte) []adfDoc {
	state := "TODO"
	var inline []adfDoc
	var nested []adfDoc
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			if cb, ok := c.FirstChild().(*extast.TaskCheckBox); ok && cb.IsChecked {
				state = "DONE"
			}
			inline = append(inline, mdInlines(c, src, nil)...)
		case *ast.List:
			nested = append(nested, mdList(v, src, true))
		}
	}
	ti := mdNode("taskItem", inline)
	ti["attrs"] = adfDoc{"localId": ids.NewID(), "state": state}
	return append([]adfDoc{ti}, nested...)
}

func mdTable(t *extast.Table, src []byte) adfDoc {
	var rows []adfDoc
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		cellType := "tableCell"
		if _, ok := r.(*extast.TableHeader); ok {
			cellType = "tableHeader"
		}
		var cells []adfDoc
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			cells = append(cells, mdNode(cellType, []adfDoc{mdNode("paragraph", mdInlines(c, src, nil))}))
		}
		rows = append(rows, mdNode("tableRow", cells))
	}
	return mdNode("table", rows)
}

func mdText(s string, marks []adfDoc) adfDoc {
	n := adfDoc{"type": "text", "text": s}
	if len(marks) > 0 {
		n["marks"] = append([]adfDoc{}, marks...)
	}
	return n
}

func safeHref(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") || strings.HasPrefix(h, "mailto:")
}

func linkMark(href string) adfDoc {
	return adfDoc{"type": "link", "attrs": adfDoc{"href": strings.TrimSpace(href)}}
}

// mdInlines converts n's inline children; marks nest outer to inner.
func mdInlines(n ast.Node, src []byte, marks []adfDoc) []adfDoc {
	var out []adfDoc
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, mdInline(c, src, marks)...)
	}
	return out
}

func mdInline(n ast.Node, src []byte, marks []adfDoc) []adfDoc {
	with := func(m adfDoc) []adfDoc { return append(append([]adfDoc{}, marks...), m) }
	switch v := n.(type) {
	case *ast.Text:
		return mdTextNode(v, src, marks)
	case *ast.String:
		if len(v.Value) == 0 {
			return nil
		}
		return []adfDoc{mdText(string(v.Value), marks)}
	case *ast.Emphasis:
		typ := "em"
		if v.Level >= 2 {
			typ = "strong"
		}
		return mdInlines(n, src, with(adfDoc{"type": typ}))
	case *extast.Strikethrough:
		return mdInlines(n, src, with(adfDoc{"type": "strike"}))
	case *ast.CodeSpan:
		return mdCodeSpan(n, src)
	case *ast.Link:
		if safeHref(string(v.Destination)) {
			return mdInlines(n, src, with(linkMark(string(v.Destination))))
		}
		return mdInlines(n, src, marks)
	case *ast.AutoLink:
		return mdLinked(string(v.Label(src)), string(v.URL(src)), marks)
	case *ast.Image:
		alt := strings.TrimSpace(string(v.Text(src))) //nolint:staticcheck // plain alt text of the children
		if alt == "" {
			alt = string(v.Destination)
		}
		return mdLinked(alt, string(v.Destination), marks)
	case *ast.RawHTML:
		var b bytes.Buffer
		for i := range v.Segments.Len() {
			seg := v.Segments.At(i)
			b.Write(seg.Value(src))
		}
		if b.Len() == 0 {
			return nil
		}
		return []adfDoc{mdText(b.String(), marks)}
	case *extast.TaskCheckBox:
		return nil
	}
	return mdInlines(n, src, marks)
}

func mdTextNode(v *ast.Text, src []byte, marks []adfDoc) []adfDoc {
	var out []adfDoc
	if s := string(v.Segment.Value(src)); s != "" {
		out = append(out, mdText(s, marks))
	}
	switch {
	case v.HardLineBreak():
		out = append(out, adfDoc{"type": "hardBreak"})
	case v.SoftLineBreak():
		out = append(out, mdText(" ", marks))
	}
	return out
}

func mdCodeSpan(n ast.Node, src []byte) []adfDoc {
	var b bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(src))
		}
	}
	if b.Len() == 0 {
		return nil
	}
	return []adfDoc{mdText(b.String(), []adfDoc{{"type": "code"}})}
}

// mdLinked emits label as text, linked only when href passes safeHref.
func mdLinked(label, href string, marks []adfDoc) []adfDoc {
	if label == "" {
		return nil
	}
	if safeHref(href) {
		return []adfDoc{mdText(label, append(append([]adfDoc{}, marks...), linkMark(href)))}
	}
	return []adfDoc{mdText(label, marks)}
}
