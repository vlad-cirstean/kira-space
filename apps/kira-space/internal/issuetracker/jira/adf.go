package jira

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Hand-rolled: no Go ADF converter escapes text and filters link schemes. The output must be Markdown
// that the Space renderer takes as-is, so no run of ADF text may become markup or raw HTML.

const adfMaxDepth = 64

type adfNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Content []adfNode      `json:"content"`
	Marks   []adfMark      `json:"marks"`
}

type adfMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

func (n adfNode) attr(k string) string {
	switch v := n.Attrs[k].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

// mediaURL resolves an attachment file name to its content URL; "" when unknown. Nil resolves nothing.
type mediaURL func(name string) string

// adfMarkdown converts an ADF document to Markdown; unreadable input gives "".
func adfMarkdown(raw json.RawMessage, m mediaURL) string {
	var doc adfNode
	if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return strings.TrimSpace(strings.Join(adfBlocks(doc.Content, 0, m), "\n\n"))
}

var inlineTypes = map[string]bool{
	"text": true, "hardBreak": true, "mention": true, "emoji": true, "inlineCard": true,
	"date": true, "status": true, "mediaInline": true, "inlineExtension": true,
}

// adfBlocks renders sibling nodes, joining a run of inline nodes into one paragraph.
func adfBlocks(nodes []adfNode, depth int, m mediaURL) []string {
	if depth > adfMaxDepth {
		return nil
	}
	var out []string
	var run []adfNode
	flush := func() {
		if len(run) == 0 {
			return
		}
		if s := adfInline(run, depth, m); strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
		run = nil
	}
	for _, n := range nodes {
		if inlineTypes[n.Type] {
			run = append(run, n)
			continue
		}
		flush()
		if s := adfBlock(n, depth+1, m); strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	flush()
	return out
}

func adfBlock(n adfNode, depth int, m mediaURL) string {
	if depth > adfMaxDepth {
		return ""
	}
	switch n.Type {
	case "paragraph":
		return adfInline(n.Content, depth, m)
	case "heading":
		lvl, _ := strconv.Atoi(n.attr("level"))
		lvl = min(max(lvl, 1), 6)
		return strings.Repeat("#", lvl) + " " + oneLine(adfInline(n.Content, depth, m))
	case "bulletList":
		return adfList(n, depth, false, m)
	case "orderedList":
		return adfList(n, depth, true, m)
	case "taskList", "decisionList":
		return adfList(n, depth, false, m)
	case "codeBlock":
		return adfCodeBlock(n)
	case "blockquote":
		return adfQuote(strings.Join(adfBlocks(n.Content, depth, m), "\n\n"))
	case "panel":
		return adfQuote("[!" + panelKind(n.attr("panelType")) + "]\n" + strings.Join(adfBlocks(n.Content, depth, m), "\n\n"))
	case "rule":
		return "---"
	case "table":
		return adfTable(n, depth, m)
	case "mediaSingle", "mediaGroup":
		var parts []string
		for _, c := range n.Content {
			if s := adfInlineNode(c, depth, m); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	case "blockCard", "embedCard":
		return adfCard(n)
	case "expand", "nestedExpand":
		blocks := adfBlocks(n.Content, depth, m)
		if t := n.attr("title"); t != "" {
			blocks = append([]string{wrapMark(escapeText(oneLine(t)), "**")}, blocks...)
		}
		return strings.Join(blocks, "\n\n")
	case "bodiedExtension":
		return strings.Join(adfBlocks(n.Content, depth, m), "\n\n")
	case "extension", "inlineExtension":
		return ""
	}
	if len(n.Content) > 0 {
		return strings.Join(adfBlocks(n.Content, depth, m), "\n\n")
	}
	return escapeText(n.Text)
}

func adfList(n adfNode, depth int, ordered bool, m mediaURL) string {
	start := 1
	if ordered {
		if v, err := strconv.Atoi(n.attr("order")); err == nil {
			start = v
		}
	}
	var items []string
	for i, it := range n.Content {
		if it.Type != "listItem" && it.Type != "taskItem" && it.Type != "decisionItem" {
			continue
		}
		marker := "- "
		switch {
		case ordered:
			marker = fmt.Sprintf("%d. ", start+i)
		case it.Type == "taskItem":
			marker = `- \[ \] `
			if it.attr("state") == "DONE" {
				marker = `- \[x\] `
			}
		}
		body := strings.Join(adfBlocks(it.Content, depth, m), "\n")
		items = append(items, prefixLines(body, marker, strings.Repeat(" ", visibleWidth(marker))))
	}
	return strings.Join(items, "\n")
}

// visibleWidth is the marker width a nested block must indent by; escapes count as one char.
func visibleWidth(marker string) int {
	return len(strings.ReplaceAll(marker, `\`, ""))
}

func prefixLines(s, first, rest string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		switch {
		case i == 0:
			lines[i] = first + l
		case l != "":
			lines[i] = rest + l
		}
	}
	return strings.Join(lines, "\n")
}

func adfQuote(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

var langRe = regexp.MustCompile(`^[A-Za-z0-9_+#.-]{1,32}$`)

func adfCodeBlock(n adfNode) string {
	var sb strings.Builder
	for _, c := range n.Content {
		sb.WriteString(c.Text)
	}
	body := strings.TrimRight(strings.ReplaceAll(sb.String(), "\r\n", "\n"), "\n")
	fence := strings.Repeat("`", max(3, longestRun(body, '`')+1))
	lang := n.attr("language")
	if !langRe.MatchString(lang) {
		lang = ""
	}
	return fence + lang + "\n" + body + "\n" + fence
}

func longestRun(s string, c rune) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == c {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

func adfTable(n adfNode, depth int, m mediaURL) string {
	var rows [][]string
	cols := 0
	for _, r := range n.Content {
		if r.Type != "tableRow" {
			continue
		}
		var cells []string
		for _, c := range r.Content {
			cell := strings.Join(adfBlocks(c.Content, depth, m), " ")
			cell = oneLine(cell)
			cells = append(cells, escapePipes(cell))
		}
		cols = max(cols, len(cells))
		rows = append(rows, cells)
	}
	if len(rows) == 0 || cols == 0 {
		return ""
	}
	line := func(cells []string) string {
		for len(cells) < cols {
			cells = append(cells, "")
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	sep := make([]string, cols)
	for i := range sep {
		sep[i] = "---"
	}
	out := []string{line(rows[0]), line(sep)}
	for _, r := range rows[1:] {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\\\n", " ")), " ")
}

func adfCard(n adfNode) string {
	u := n.attr("url")
	if u == "" {
		if d, ok := n.Attrs["data"].(map[string]any); ok {
			u, _ = d["url"].(string)
		}
	}
	if !isHTTP(u) {
		return ""
	}
	return "[" + escapeText(u) + "](" + linkDest(u) + ")"
}

func isHTTP(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://")
}

func isLinkable(u string) bool {
	return isHTTP(u) || strings.HasPrefix(strings.ToLower(u), "mailto:")
}

// linkDest percent-encodes what would end a Markdown link destination early.
func linkDest(u string) string {
	var sb strings.Builder
	for _, r := range u {
		switch r {
		case ' ', '(', ')', '<', '>', '\\', '\n', '\r', '\t':
			fmt.Fprintf(&sb, "%%%02X", r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func adfInline(nodes []adfNode, depth int, m mediaURL) string {
	var sb strings.Builder
	for _, n := range nodes {
		sb.WriteString(adfInlineNode(n, depth, m))
	}
	return sb.String()
}

func adfInlineNode(n adfNode, depth int, m mediaURL) string {
	if depth > adfMaxDepth {
		return ""
	}
	switch n.Type {
	case "text":
		return adfText(n)
	case "hardBreak":
		return "\\\n"
	case "mention":
		name := strings.TrimPrefix(n.attr("text"), "@")
		if name == "" {
			name = "user"
		}
		return "[" + escapeText("@"+oneLine(name)) + "](#kira-mention)"
	case "emoji":
		t := n.attr("text")
		if t == "" {
			t = n.attr("shortName")
		}
		return escapeText(t)
	case "inlineCard":
		return adfCard(n)
	case "date":
		ms, err := strconv.ParseInt(n.attr("timestamp"), 10, 64)
		if err != nil {
			return ""
		}
		return time.UnixMilli(ms).UTC().Format("2006-01-02")
	case "status":
		return "[" + escapeText(oneLine(n.attr("text"))) + "](#kira-status-" + statusColor(n.attr("color")) + ")"
	case "media", "mediaInline":
		name := n.attr("alt")
		if name == "" {
			name = n.attr("__fileName")
		}
		if name == "" {
			name = "file"
		}
		name = oneLine(name)
		dest := "#kira-attachment"
		if u := n.attr("url"); n.attr("type") == "external" && isHTTP(u) {
			dest = linkDest(u)
		} else if m != nil {
			if u := m(name); u != "" {
				dest = linkDest(u)
			}
		}
		return "![" + escapeText(name) + "](" + dest + ")"
	case "inlineExtension", "extension":
		return ""
	}
	if len(n.Content) > 0 {
		return adfInline(n.Content, depth+1, m)
	}
	return escapeText(n.Text)
}

func adfText(n adfNode) string {
	var code, link bool
	var href string
	for _, m := range n.Marks {
		switch m.Type {
		case "code":
			code = true
		case "link":
			if h, _ := m.Attrs["href"].(string); isLinkable(h) {
				link, href = true, h
			}
		}
	}
	var s string
	if code {
		s = codeSpan(n.Text)
	} else {
		s = escapeText(n.Text)
		s = strings.ReplaceAll(s, "\n", "\\\n")
	}
	for _, m := range n.Marks {
		switch m.Type {
		case "strong":
			s = wrapMark(s, "**")
		case "em":
			s = wrapMark(s, "_")
		case "strike":
			s = wrapMark(s, "~~")
		}
	}
	if link {
		if strings.TrimSpace(s) == "" {
			s = escapeText(href)
		}
		return "[" + s + "](" + linkDest(href) + ")"
	}
	return s
}

func codeSpan(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", " ")
	if s == "" {
		return ""
	}
	fence := strings.Repeat("`", longestRun(s, '`')+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// wrapMark puts delim around s but outside its edge spaces, which Markdown does not allow inside.
func wrapMark(s, delim string) string {
	core := strings.TrimSpace(s)
	if core == "" {
		return s
	}
	i := strings.Index(s, core)
	return s[:i] + delim + core + delim + s[i+len(core):]
}

var (
	escapeChars = "\\`*_[]<>#+-!|~{}=&"
	leadNumRe   = regexp.MustCompile(`^(\s*\d+)([.)])`)
)

// escapeText backslash-escapes every Markdown-significant character of s, per line.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		var sb strings.Builder
		for _, r := range l {
			if r < 128 && strings.ContainsRune(escapeChars, r) {
				sb.WriteByte('\\')
			}
			sb.WriteRune(r)
		}
		lines[i] = leadNumRe.ReplaceAllString(sb.String(), `$1\$2`)
	}
	return strings.Join(lines, "\n")
}

// plainMarkdown renders a plain-text body (Jira wiki markup shows literally) as Markdown.
func plainMarkdown(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(escapeText(strings.TrimSpace(s)), "\n")
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			if l != "" && lines[i-1] != "" {
				sb.WriteString("\\\n")
			} else {
				sb.WriteString("\n")
			}
		}
		sb.WriteString(l)
	}
	return sb.String()
}

// escapePipes escapes the pipes escapeText did not, i.e. those inside code spans.
func escapePipes(s string) string {
	var sb strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case esc:
			esc = false
		case r == '\\':
			esc = true
		case r == '|':
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// panelKind maps an ADF panel type to the GitHub alert kind the Markdown renderer draws.
func panelKind(t string) string {
	switch t {
	case "note":
		return "IMPORTANT"
	case "success":
		return "TIP"
	case "warning":
		return "WARNING"
	case "error":
		return "CAUTION"
	}
	return "NOTE"
}

// statusColor limits an ADF status color to the set the renderer styles.
func statusColor(c string) string {
	switch c = strings.ToLower(c); c {
	case "neutral", "purple", "blue", "red", "yellow", "green":
		return c
	}
	return "neutral"
}
