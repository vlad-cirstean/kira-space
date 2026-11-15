package taskfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/proof"
)

// Proof file format limits.
const (
	MaxProofSteps = 100
	maxStepIDLen  = 40
)

var stepIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var proofViews = []string{proof.ViewTable, proof.ViewDocument, proof.ViewKeyValue, proof.ViewChart, proof.ViewDiagram, proof.ViewImage}

// problem is one finding in a proof unit: the file it is in, the line (0 unknown) and the text, which
// ends in the fix.
type problem struct {
	File string
	Line int
	Msg  string
}

func (p problem) text(folder string) string {
	f := folder + p.File
	if p.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f, p.Line, p.Msg)
	}
	return f + ": " + p.Msg
}

// proofStep is one validated step of proof.md.
type proofStep struct {
	ID, Description, View, Query, Language, Source string
	// Payload is the stored payload: validated JSON, or the image metadata JSON.
	Payload []byte
	// File is the side file the step names in data; "" for an inline step.
	File string
	Line int
}

func (s proofStep) hash() string {
	h := sum([]byte(strings.Join([]string{s.Description, s.Query, s.Language, s.Source, s.View, string(s.Payload)}, "\x00")))
	return h
}

// proofDoc is a validated proof.md.
type proofDoc struct {
	Name, Summary string
	Steps         []proofStep
}

func (d proofDoc) head() ProofHead {
	h := ProofHead{Name: d.Name, Summary: d.Summary}
	for _, s := range d.Steps {
		h.Steps = append(h.Steps, Step{ID: s.ID, Hash: s.hash()})
	}
	return h
}

// ProofFolderFiles holds the bytes of the side files of a proof folder, by name.
type proofFiles map[string][]byte

var (
	yamlErrLine = regexp.MustCompile(`line (\d+)`)
	inlineLine  = regexp.MustCompile(`^line (\d+): `)
)

// splitFrontmatter returns the YAML between the --- lines and the number of lines before it.
func splitFrontmatter(raw []byte) (yamlBytes []byte, ok bool) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return nil, false
	}
	rest := text[4:]
	end := -1
	for off := 0; off <= len(rest); {
		nl := strings.IndexByte(rest[off:], '\n')
		line := rest[off:]
		if nl >= 0 {
			line = rest[off : off+nl]
		}
		if strings.TrimRight(line, " \t") == "---" {
			end = off
			break
		}
		if nl < 0 {
			break
		}
		off += nl + 1
	}
	if end < 0 {
		return nil, false
	}
	return []byte(rest[:end]), true
}

// yamlHint maps a yaml.v3 parse error to one line and one fix hint per known class.
func yamlHint(err error, src []byte) (line int, msg string) {
	raw := strings.TrimPrefix(err.Error(), "yaml: ")
	if m := yamlErrLine.FindStringSubmatch(raw); m != nil {
		line, _ = strconv.Atoi(m[1])
	}
	lines := strings.Split(string(src), "\n")
	hasTab := line > 0 && line <= len(lines) && strings.ContainsRune(leadingSpace(lines[line-1]), '\t')
	switch {
	case strings.Contains(raw, "tab character") || hasTab:
		return line, "YAML: found a tab. Indent with spaces."
	case strings.Contains(raw, "mapping values are not allowed"):
		return line, `YAML: mapping values are not allowed here. Quote the value, for example description: "Totals: all rows".`
	case strings.Contains(raw, "did not find expected key") || strings.Contains(raw, "bad indentation") || strings.Contains(raw, "did not find expected '-' indicator"):
		return line, "YAML: bad indentation. Indent the line like its siblings, with spaces."
	case strings.Contains(raw, "did not find expected ',' or ']'") || strings.Contains(raw, "did not find expected ',' or '}'") ||
		strings.Contains(raw, "did not find expected node content") || strings.Contains(raw, "flow"):
		return line, "YAML: a [ or { is not closed. Add the closing ] or }."
	case strings.Contains(raw, "already defined"):
		return line, "YAML: duplicate key. Remove or rename one of them."
	}
	return line, "YAML: " + raw
}

func leadingSpace(s string) string {
	return s[:len(s)-len(strings.TrimLeft(s, " \t"))]
}

// parseProofMD validates proof.md against the side files and returns the document, or every problem found.
func parseProofMD(raw []byte, files proofFiles, used map[string]bool) (proofDoc, []problem) {
	const f = "proof.md"
	fm, ok := splitFrontmatter(raw)
	if !ok {
		return proofDoc{}, []problem{{f, 1, "no frontmatter. Start the file with --- and end the YAML with ---."}}
	}
	if !utf8.Valid(fm) {
		return proofDoc{}, []problem{{f, 0, "the file is not valid UTF-8. Save it as UTF-8."}}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil {
		line, msg := yamlHint(err, fm)
		if line > 0 {
			line++
		}
		return proofDoc{}, []problem{{f, line, msg}}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return proofDoc{}, []problem{{f, 2, "the frontmatter must be a mapping with name, summary and steps."}}
	}
	return (&proofParser{files: files, used: used}).run(doc.Content[0])
}

type proofParser struct {
	files proofFiles
	used  map[string]bool
	probs []problem
	out   proofDoc
}

// add records a problem at a frontmatter line (file line = yaml line + 1).
func (p *proofParser) add(line int, format string, args ...any) {
	if line > 0 {
		line++
	}
	p.probs = append(p.probs, problem{"proof.md", line, fmt.Sprintf(format, args...)})
}

func scalarOf(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

func (p *proofParser) run(m *yaml.Node) (proofDoc, []problem) {
	seen := map[string]bool{}
	var stepsNode *yaml.Node
	var sawName bool
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if seen[k.Value] {
			p.add(k.Line, "duplicate key %q. Remove one of them.", k.Value)
			continue
		}
		seen[k.Value] = true
		switch k.Value {
		case "kira":
			if s, _ := scalarOf(v); s != "proof/1" {
				p.add(v.Line, "kira must be proof/1. Write kira: proof/1.")
			}
		case "name":
			sawName = true
			s, ok := scalarOf(v)
			if !ok {
				p.add(v.Line, "name must be text. Write name: Orders match ledger.")
				break
			}
			clean, _, err := proof.NormalizeName(s)
			if err != nil {
				p.add(v.Line, "name: %s.", err.Error())
				break
			}
			p.out.Name = clean
		case "summary":
			s, ok := scalarOf(v)
			if !ok {
				p.add(v.Line, "summary must be text.")
				break
			}
			if err := proof.CheckText("summary", strings.TrimSpace(s), proof.MaxSummaryRunes); err != nil {
				p.add(v.Line, "%s.", err.Error())
				break
			}
			p.out.Summary = strings.TrimSpace(s)
		case "steps":
			stepsNode = v
		default:
			p.add(k.Line, "unknown field %q. Allowed: kira, name, summary, steps.", k.Value)
		}
	}
	if !sawName {
		p.add(0, "name is missing. Add name: <1-%d characters>.", proof.MaxNameRunes)
	}
	switch {
	case stepsNode == nil:
		p.add(0, "steps is missing. Add steps: with 1-%d steps.", MaxProofSteps)
	case stepsNode.Kind != yaml.SequenceNode:
		p.add(stepsNode.Line, "steps must be a list of steps, each starting with - id: ...")
	case len(stepsNode.Content) < 1 || len(stepsNode.Content) > MaxProofSteps:
		p.add(stepsNode.Line, "steps holds %d steps; it must hold 1-%d.", len(stepsNode.Content), MaxProofSteps)
	default:
		ids := map[string]bool{}
		for _, sn := range stepsNode.Content {
			if st, ok := p.step(sn, ids); ok {
				p.out.Steps = append(p.out.Steps, st)
			}
		}
	}
	if len(p.probs) > 0 {
		return proofDoc{}, p.probs
	}
	return p.out, nil
}

var stepFields = map[string]bool{"id": true, "description": true, "view": true, "inline": true, "data": true, "query": true, "language": true, "source": true}

func (p *proofParser) step(n *yaml.Node, ids map[string]bool) (proofStep, bool) {
	if n.Kind != yaml.MappingNode {
		p.add(n.Line, "a step must be a mapping: - id: totals, description: ..., view: table, inline: ...")
		return proofStep{}, false
	}
	start := len(p.probs)
	st, inline, data := p.stepFields(n)
	id := st.ID
	label := "steps[" + id + "]"
	if id == "" {
		label = "steps[?]"
	}
	switch {
	case id == "":
		p.add(n.Line, "step has no id. Add id: totals (a-z, 0-9 and -, at most %d characters).", maxStepIDLen)
		return st, false
	case len(id) > maxStepIDLen || !stepIDRe.MatchString(id):
		p.add(n.Line, "%s: id must be a-z, 0-9 and -, 1-%d characters, starting with a letter or digit.", label, maxStepIDLen)
		return st, false
	case ids[id]:
		p.add(n.Line, "%s: the id is used twice. Give each step its own id.", label)
		return st, false
	}
	ids[id] = true
	if r := utf8.RuneCountInString(st.Description); r < 1 || r > proof.MaxDescRunes || strings.ContainsAny(st.Description, "\n\r") {
		p.add(n.Line, "%s.description: write one line of 1-%d characters.", label, proof.MaxDescRunes)
	}
	if !contains(proofViews, st.View) {
		p.add(n.Line, "%s.view: must be one of %s.", label, strings.Join(proofViews, ", "))
		return st, false
	}
	if !proof.ValidLanguage(st.Language) {
		p.add(n.Line, "%s.language: must be one of sql, json, shell, text.", label)
	}
	if len(st.Query) > proof.MaxQueryBytes {
		p.add(n.Line, "%s.query is %d KiB; the limit is %d KiB.", label, len(st.Query)>>10, proof.MaxQueryBytes>>10)
	}
	if utf8.RuneCountInString(st.Source) > proof.MaxSourceRunes {
		p.add(n.Line, "%s.source: at most %d characters.", label, proof.MaxSourceRunes)
	}
	if (inline == nil) == (data == nil) {
		p.add(n.Line, "%s: give exactly one of inline (the data in YAML) or data (a file in this folder).", label)
		return st, false
	}
	if len(p.probs) > start {
		return st, false
	}
	switch {
	case inline != nil:
		p.inline(&st, label, inline)
	case st.View == proof.ViewImage:
		p.image(&st, label, data)
	default:
		p.dataFile(&st, label, data)
	}
	return st, len(p.probs) == start
}

// stepFields reads the keys of one step mapping; inline and data are the nodes of those keys.
func (p *proofParser) stepFields(n *yaml.Node) (st proofStep, inline, data *yaml.Node) {
	st = proofStep{Line: n.Line, Language: "text"}
	seen := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if seen[k.Value] {
			p.add(k.Line, "duplicate key %q. Remove one of them.", k.Value)
			continue
		}
		seen[k.Value] = true
		if !stepFields[k.Value] {
			p.add(k.Line, "unknown step field %q. Allowed: id, description, view, inline, data, query, language, source.", k.Value)
			continue
		}
		s, isScalar := scalarOf(v)
		switch k.Value {
		case "inline":
			inline = v
		case "data":
			data = v
			if !isScalar {
				p.add(v.Line, "data must be a file name in this folder, like data: shot.png. Put structured data under inline.")
			}
			st.File = s
		default:
			if !isScalar {
				p.add(v.Line, "%s must be text.", k.Value)
				continue
			}
			setStepText(&st, k.Value, s)
		}
	}
	return st, inline, data
}

func setStepText(st *proofStep, key, s string) {
	switch key {
	case "id":
		st.ID = s
	case "description":
		st.Description = strings.TrimSpace(s)
	case "view":
		st.View = s
	case "query":
		st.Query = s
	case "language":
		st.Language = s
	case "source":
		st.Source = s
	}
}

func (p *proofParser) inline(st *proofStep, label string, n *yaml.Node) {
	if st.View == proof.ViewImage {
		p.add(n.Line, "%s.inline: an image step takes a file. Write the image into this folder and set data: <file>.", label)
		return
	}
	var raw json.RawMessage
	if st.View == proof.ViewDiagram {
		s, ok := scalarOf(n)
		if !ok {
			p.add(n.Line, "%s.inline: a diagram is one text block. Use inline: | and indent the Mermaid source.", label)
			return
		}
		b, _ := json.Marshal(map[string]string{"source": s})
		raw = b
	} else {
		var err error
		if raw, err = proof.InlineToJSON(n); err != nil {
			line, msg := n.Line, err.Error()
			if m := inlineLine.FindStringSubmatch(msg); m != nil {
				line, _ = strconv.Atoi(m[1])
				msg = msg[len(m[0]):]
			}
			p.add(line, "%s.inline: %s.", label, strings.TrimSuffix(msg, "."))
			return
		}
	}
	if len(raw) > InlineStepMaxBytes {
		p.add(n.Line, "%s.inline is %d KiB; the limit is %d KiB. Move it to <this folder>/%s.json and set data: %s.json.", label, len(raw)>>10, InlineStepMaxBytes>>10, st.ID, st.ID)
		return
	}
	payload, err := proof.ValidateStep(st.View, raw)
	if err != nil {
		p.add(n.Line, "%s.inline: %s.", label, strings.TrimSuffix(err.Error(), "."))
		return
	}
	st.Payload = payload
}

func (p *proofParser) dataFile(st *proofStep, label string, n *yaml.Node) {
	raw, ok := p.files[st.File]
	if !ok {
		p.add(n.Line, "%s.data: %s not found in this folder. Write the file there first.", label, st.File)
		return
	}
	if !dataRe.MatchString(st.File) {
		p.add(n.Line, "%s.data: %s must be a .json file named a-z, 0-9 and - (at most %d characters before .json).", label, st.File, 40)
		return
	}
	p.used[st.File] = true
	payload, err := proof.ValidateStep(st.View, raw)
	if err != nil {
		p.add(n.Line, "%s.data: %s: %s.", label, st.File, strings.TrimSuffix(err.Error(), "."))
		return
	}
	st.Payload = payload
}

func (p *proofParser) image(st *proofStep, label string, n *yaml.Node) {
	raw, ok := p.files[st.File]
	if !ok {
		p.add(n.Line, "%s.data: %s not found in this folder. Write the image there first.", label, st.File)
		return
	}
	p.used[st.File] = true
	info, err := proof.CheckImage(st.File, raw)
	if err != nil {
		p.add(n.Line, "%s.data: %s: %s.", label, st.File, strings.TrimSuffix(err.Error(), "."))
		return
	}
	meta, _ := json.Marshal(proof.ImageMeta{Format: info.Format, Width: info.Width, Height: info.Height, Bytes: info.Bytes, File: st.File, SHA256: sum(raw)})
	st.Payload = meta
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
