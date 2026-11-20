package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/kirathecat/kira-studio/internal/textloss"
)

const (
	stateVerbatim = "verbatim"
	partMax       = 12000
	proseFloor    = 400
	lookupCell    = 100
)

const standingUsage = `usage: docshistory doc <units|render|verify|keep|show> [flags] <docs/ARCHITECTURE.md|docs/PACKAGING.md> [slug...]
  units    print segments (slug, kind, chars)
  keep     record the named units as verbatim
  render   write the distilled file
  verify   write the loss and structure report
  show     print a unit's source and distilled lines`

// keepSections are the "##" headings (prefix match) a standing doc carries verbatim, with their subsections.
var keepSections = map[string][]string{
	"ARCHITECTURE.md": {"Known open items"},
	"PACKAGING.md":    {"1. ", "3. ", "4. ", "6. "},
}

// cited are phrases other files cite; a distilled unit holding one in its source must keep it.
var cited = []string{"Go owns the network", "provably read-only", "no Vue reactivity on row data", "WebKit's clipboard gesture heuristics"}

var pilotHeadings = map[string]bool{
	"Stack": true, "Invariants": true, "PostgreSQL / MariaDB / MySQL": true, "Caching": true,
	"Copy and i18n (P265)": true, "Renderer security surface": true,
}

type segKind int

const (
	segHeading segKind = iota
	segVerbatim
	segProse
)

func (k segKind) String() string { return [...]string{"heading", "verbatim", "prose"}[k] }

type segment struct {
	Kind    segKind
	Raw     string
	Why     string
	Slug    string
	Heading string
	Path    string
	Level   int
	Top     string // enclosing "##" heading text
	Body    string
	Lead    string
	Trail   string
}

func (s segment) unitText(file string) string {
	return fmt.Sprintf("# %s: %s\n\nSection: %s.\n\n%s\n", file, s.Path, s.Heading, s.Body)
}

var docHeadingRe = regexp.MustCompile(`^(#{1,6}) +(.*?) *$`)

type element struct {
	kind  string // text, fence, table
	lines []string
}

func ghSlug(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(h) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func isBlank(s string) bool { return strings.TrimSpace(s) == "" }

// groupElements splits a section body into text, fence and table elements, keeping every line.
func groupElements(lines []string) []element {
	var els []element
	add := func(kind, l string) {
		if n := len(els); n > 0 && els[n-1].kind == kind && kind != "fence" {
			els[n-1].lines = append(els[n-1].lines, l)
			return
		}
		els = append(els, element{kind: kind, lines: []string{l}})
	}
	inFence := false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case inFence:
			els[len(els)-1].lines = append(els[len(els)-1].lines, l)
			if strings.HasPrefix(t, "```") {
				inFence = false
			}
		case strings.HasPrefix(t, "```"):
			inFence = true
			els = append(els, element{kind: "fence", lines: []string{l}})
		case strings.HasPrefix(t, "|"):
			add("table", l)
		default:
			add("text", l)
		}
	}
	return els
}

func tableCells(lines []string) (rows [][]string, sepAt int) {
	sepAt = -1
	for i, l := range lines {
		c := splitRow(l)
		if sepAt < 0 && isSeparator(c) {
			sepAt = i
		}
		rows = append(rows, c)
	}
	return rows, sepAt
}

// isLookup reports a table whose mean cell is at most lookupCell chars.
func isLookup(lines []string) bool {
	rows, sepAt := tableCells(lines)
	n, total := 0, 0
	for i, r := range rows {
		if i == sepAt {
			continue
		}
		for _, c := range r {
			n++
			total += len([]rune(c))
		}
	}
	return n == 0 || total/n <= lookupCell
}

type para struct{ raw, text string }

// paragraphs of a text element or a flattened table, each carrying its exact raw lines.
func paragraphs(e element) []para {
	if e.kind == "table" {
		return tableParas(e)
	}
	return textParas(e)
}

func flattenRow(row, header []string) string {
	var b strings.Builder
	for j, c := range row {
		switch {
		case c == "":
		case j == 0 && strings.HasPrefix(c, "**"):
			b.WriteString(c)
		case j == 0:
			b.WriteString("**" + c + "**")
		case j < len(header) && header[j] != "":
			b.WriteString("\n" + header[j] + ": " + c)
		default:
			b.WriteString("\n" + c)
		}
	}
	return b.String()
}

func tableParas(e element) []para {
	rows, sepAt := tableCells(e.lines)
	var header []string
	if sepAt > 0 {
		header = rows[sepAt-1]
	}
	var out []para
	prefix := ""
	for i, l := range e.lines {
		if i <= sepAt || (sepAt < 0 && i == 0) {
			prefix += l
			continue
		}
		out = append(out, para{raw: prefix + l, text: flattenRow(rows[i], header)})
		prefix = ""
	}
	if prefix != "" && len(out) > 0 {
		out[len(out)-1].raw += prefix
	}
	return out
}

func textParas(e element) []para {
	var out []para
	var raw, txt []string
	inBlank := false
	flush := func() {
		if len(raw) > 0 {
			out = append(out, para{raw: strings.Join(raw, ""), text: strings.TrimSpace(strings.Join(txt, "\n"))})
		}
		raw, txt, inBlank = nil, nil, false
	}
	for _, l := range e.lines {
		if isBlank(l) {
			raw = append(raw, l)
			inBlank = true
			continue
		}
		if inBlank && len(txt) > 0 {
			flush()
		}
		inBlank = false
		raw = append(raw, l)
		txt = append(txt, strings.TrimRight(l, "\r\n"))
	}
	flush()
	return out
}

func trailingBlanks(s string) (core, trail string) {
	lines := strings.SplitAfter(s, "\n")
	i := len(lines)
	for i > 0 && isBlank(lines[i-1]) {
		i--
	}
	return strings.Join(lines[:i], ""), strings.Join(lines[i:], "")
}

func leadingBlanks(s string) (lead, core string) {
	lines := strings.SplitAfter(s, "\n")
	i := 0
	for i < len(lines) && isBlank(lines[i]) {
		i++
	}
	return strings.Join(lines[:i], ""), strings.Join(lines[i:], "")
}

func keepHeading(file, h string) bool {
	for _, k := range keepSections[file] {
		if strings.HasPrefix(h, k) {
			return true
		}
	}
	return false
}

type sect struct {
	head    string
	level   int
	path    string
	top     string
	body    []string
	hasHead bool
}

// sections cuts the file at headings outside fences; the first section may have no heading.
func sections(text string) []sect {
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	var sects []sect
	cur := sect{}
	var stack []string
	top := ""
	inFence := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
		}
		m := docHeadingRe.FindStringSubmatch(strings.TrimRight(l, "\r\n"))
		if inFence || m == nil {
			cur.body = append(cur.body, l)
			continue
		}
		sects = append(sects, cur)
		lvl := len(m[1])
		stack = append(stack[:min(lvl-1, len(stack))], m[2])
		switch lvl {
		case 1:
			top = ""
		case 2:
			top = m[2]
		}
		cur = sect{head: m[2], level: lvl, path: strings.Join(nonEmpty(stack), " > "), top: top, body: []string{l}, hasHead: true}
	}
	return append(sects, cur)
}

// docSplitter accumulates the segments of one file.
type docSplitter struct {
	file   string
	segs   []segment
	seen   map[string]int
	counts map[string]int
}

func (d *docSplitter) verbatim(s sect, raw, why string) {
	if raw != "" {
		d.segs = append(d.segs, segment{Kind: segVerbatim, Raw: raw, Why: why, Heading: s.head, Path: s.path, Top: s.top})
	}
}

func (d *docSplitter) flushRun(s sect, anchor string, run []para) {
	for _, part := range partsOf(run) {
		raw := ""
		txt := make([]string, 0, len(part))
		for _, p := range part {
			raw += p.raw
			if p.text != "" {
				txt = append(txt, p.text)
			}
		}
		body := strings.Join(txt, "\n\n")
		if len([]rune(body)) < proseFloor {
			d.verbatim(s, raw, "short")
			continue
		}
		core, trail := trailingBlanks(raw)
		lead, _ := leadingBlanks(core)
		d.counts[anchor]++
		d.segs = append(d.segs, segment{Kind: segProse, Raw: raw, Body: body, Lead: lead, Trail: trail, Heading: s.head, Path: s.path, Top: s.top,
			Slug: fmt.Sprintf("%s/%s/%02d", strings.ToLower(strings.TrimSuffix(d.file, ".md")), anchor, d.counts[anchor])})
	}
}

func (d *docSplitter) section(s sect) {
	body := s.body
	if s.hasHead {
		d.segs = append(d.segs, segment{Kind: segHeading, Raw: body[0], Heading: s.head, Path: s.path, Level: s.level, Top: s.top})
		body = body[1:]
	}
	if len(body) == 0 {
		return
	}
	switch {
	case !s.hasHead || s.level == 1:
		d.verbatim(s, strings.Join(body, ""), "intro")
		return
	case s.top != "" && keepHeading(d.file, s.top):
		d.verbatim(s, strings.Join(body, ""), "keep")
		return
	}
	anchor := ghSlug(s.head)
	if n := d.seen[anchor]; n > 0 {
		anchor = fmt.Sprintf("%s-%d", anchor, n)
	}
	d.seen[ghSlug(s.head)]++
	var run []para
	for _, e := range groupElements(body) {
		raw := strings.Join(e.lines, "")
		switch {
		case e.kind == "fence" || (e.kind == "table" && isLookup(e.lines)):
			d.flushRun(s, anchor, run)
			run = run[:0]
			d.verbatim(s, raw, map[string]string{"fence": "fence", "table": "lookup"}[e.kind])
		case e.kind == "text" && allBlank(e.lines) && len(run) == 0:
			d.verbatim(s, raw, "gap")
		case e.kind == "text" && allBlank(e.lines):
			run[len(run)-1].raw += raw
		default:
			run = append(run, paragraphs(e)...)
		}
	}
	d.flushRun(s, anchor, run)
}

// splitDoc walks the file and emits contiguous segments whose Raw concatenates to the input.
func splitDoc(file, text string) []segment {
	d := &docSplitter{file: file, seen: map[string]int{}, counts: map[string]int{}}
	for _, s := range sections(text) {
		d.section(s)
	}
	return d.segs
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func allBlank(lines []string) bool {
	for _, l := range lines {
		if !isBlank(l) {
			return false
		}
	}
	return true
}

// partsOf greedily packs paragraphs into parts of at most partMax chars; a paragraph never splits.
func partsOf(run []para) [][]para {
	var parts [][]para
	cur := make([]para, 0, len(run))
	size := 0
	for _, p := range run {
		n := len([]rune(p.text)) + 2
		if len(cur) > 0 && size+n > partMax {
			parts = append(parts, cur)
			cur, size = nil, 0
		}
		cur = append(cur, p)
		size += n
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts
}

// ---- source and records ----

type standing struct {
	path, file, runFile string
	runs                string
}

func newStanding(path, runs string) (standing, error) {
	file := filepath.Base(path)
	if _, ok := keepSections[file]; !ok {
		return standing{}, fmt.Errorf("%s: not a standing doc (ARCHITECTURE.md, PACKAGING.md)", path)
	}
	return standing{path: path, file: file, runs: runs, runFile: filepath.Join(runs, strings.TrimSuffix(file, ".md")+".jsonl")}, nil
}

// pin returns the recorded source commit, else the last commit touching a clean file.
func (st standing) pin(recs map[string]Record) (string, error) {
	for _, r := range recs {
		if r.SourceCommit != "" {
			return r.SourceCommit, nil
		}
	}
	out, err := git("status", "--porcelain", "--", st.path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) != "" {
		return "", fmt.Errorf("uncommitted changes in %s", st.path)
	}
	c, err := git("log", "-1", "--format=%H", "--", st.path)
	if err != nil || strings.TrimSpace(c) == "" {
		return "", fmt.Errorf("no commit touches %s: %v", st.path, err)
	}
	return strings.TrimSpace(c), nil
}

func (st standing) load() (segs []segment, recs map[string]Record, commit, src string, err error) {
	recs, err = loadRecords(st.runFile)
	if err != nil {
		return
	}
	if commit, err = st.pin(recs); err != nil {
		return
	}
	if src, err = git("show", commit+":"+st.path); err != nil {
		return
	}
	segs = splitDoc(st.file, src)
	return
}

func proseSegs(segs []segment) []segment {
	var out []segment
	for _, s := range segs {
		if s.Kind == segProse {
			out = append(out, s)
		}
	}
	return out
}

func renderDoc(file, commit string, segs []segment, recs map[string]Record) (string, error) {
	var b strings.Builder
	var missing []string
	distilled := false
	for _, s := range segs {
		distilled = distilled || (s.Kind == segProse && recs[s.Slug].State == stateOK)
	}
	for i, s := range segs {
		switch s.Kind {
		case segHeading, segVerbatim:
			b.WriteString(s.Raw)
			if i == 0 && distilled && s.Kind == segHeading && s.Level == 1 {
				fmt.Fprintf(&b, "\nDistilled in P303, one fact per line. Full earlier text: git show %s:docs/%s.\n", commit[:12], file)
			}
		case segProse:
			r, ok := recs[s.Slug]
			switch {
			case ok && r.State == stateVerbatim && r.InputSha256 == sha(s.unitText(file)):
				b.WriteString(s.Raw)
			case ok && r.State == stateOK && r.InputSha256 == sha(s.unitText(file)):
				b.WriteString(s.Lead)
				for _, l := range r.Lines {
					b.WriteString("- " + l + "\n")
				}
				b.WriteString(s.Trail)
			default:
				missing = append(missing, s.Slug)
			}
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("%d prose units without an ok or verbatim record: %s", len(missing), strings.Join(missing, ", "))
	}
	return b.String(), nil
}

// ---- subcommands ----

func standingMain(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", standingUsage)
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if err := os.Chdir(strings.TrimSpace(top)); err != nil {
		return err
	}
	cmd := args[0]
	fs := flag.NewFlagSet("doc "+cmd, flag.ContinueOnError)
	runs := fs.String("runs", "docs/history-runs", "runs directory")
	out := fs.String("out", "-", "verify report path, - for stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("name a standing doc\n%s", standingUsage)
	}
	st, err := newStanding(rest[0], *runs)
	if err != nil {
		return err
	}
	switch cmd {
	case "units":
		return st.cmdUnits()
	case "keep":
		return st.cmdKeep(rest[1:])
	case "render":
		return st.cmdRender()
	case "verify":
		return st.cmdVerify(*out)
	case "show":
		return st.cmdShow(rest[1:])
	}
	return fmt.Errorf("unknown doc command %q\n%s", cmd, standingUsage)
}

func (st standing) cmdUnits() error {
	segs := splitDoc(st.file, mustShow(st))
	var nProse, proseChars, verbChars, heads int
	for _, s := range segs {
		switch s.Kind {
		case segHeading:
			heads++
			fmt.Printf("%s\theading\t%s\n", "-", s.Raw[:len(s.Raw)-1])
		case segVerbatim:
			fmt.Printf("%s\tverbatim:%s\t%d\n", "-", s.Why, len([]rune(s.Raw)))
			verbChars += len([]rune(s.Raw))
		case segProse:
			fmt.Printf("%s\tprose\t%d\n", s.Slug, len([]rune(s.Body)))
			nProse++
			proseChars += len([]rune(s.Body))
		}
	}
	fmt.Printf("# %s: %d headings, %d prose units, %d prose chars, %d verbatim chars\n", st.path, heads, nProse, proseChars, verbChars)
	return nil
}

func mustShow(st standing) string {
	recs, _ := loadRecords(st.runFile)
	commit, err := st.pin(recs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docshistory:", err)
		os.Exit(1)
	}
	src, err := git("show", commit+":"+st.path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docshistory:", err)
		os.Exit(1)
	}
	return src
}

func (st standing) cmdKeep(slugs []string) error {
	segs, _, commit, _, err := st.load()
	if err != nil {
		return err
	}
	byslug := map[string]segment{}
	for _, s := range proseSegs(segs) {
		byslug[s.Slug] = s
	}
	for _, sl := range slugs {
		s, ok := byslug[sl]
		if !ok {
			return fmt.Errorf("no prose unit %q", sl)
		}
		text := s.unitText(st.file)
		rec := Record{Slug: sl, ID: sl, Title: s.Path, InputSha256: sha(text), InChars: len([]rune(text)), State: stateVerbatim, SourceCommit: commit, CostUSD: 0}
		if err := appendLine(st.runFile, rec); err != nil {
			return err
		}
		fmt.Printf("kept verbatim: %s\n", sl)
	}
	return nil
}

func (st standing) cmdRender() error {
	segs, recs, commit, _, err := st.load()
	if err != nil {
		return err
	}
	text, err := renderDoc(st.file, commit, segs, recs)
	if err != nil {
		return err
	}
	if err := os.WriteFile(st.path, []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: wrote %d bytes\n", st.path, len(text))
	return nil
}

func (st standing) cmdShow(slugs []string) error {
	segs, recs, _, _, err := st.load()
	if err != nil {
		return err
	}
	for _, s := range proseSegs(segs) {
		for _, sl := range slugs {
			if s.Slug != sl {
				continue
			}
			fmt.Printf("===== %s (%s)\n----- source\n%s\n----- distilled (%s)\n", s.Slug, s.Path, s.Body, recs[s.Slug].State)
			for _, l := range recs[s.Slug].Lines {
				fmt.Printf("- %s\n", l)
			}
		}
	}
	return nil
}

// ---- verify ----

var (
	errCodeRe = regexp.MustCompile(`\bE_[A-Z0-9_]+\b`)
	cmdSpanRe = regexp.MustCompile("`([^`]+)`")
	checkIDRe = regexp.MustCompile(`\b[SAN][0-9]\b`)
	numRe     = regexp.MustCompile(`^\d+(\.\d+)?$`)
	cmdStarts = []string{"bun", "wails3", "go", "git", "gh", "scripts/", "codesign", "spctl", "xattr", "hdiutil", "open", "curl", "task"}
)

type lossCount struct{ num, ident, path, quoted int }

func (l lossCount) hard() int { return l.num + l.ident + l.path + l.quoted }

func (l *lossCount) add(o lossCount) {
	l.num += o.num
	l.ident += o.ident
	l.path += o.path
	l.quoted += o.quoted
}

func classify(tokens []string) (c lossCount, neg int, hard []string) {
	seen := map[string]bool{}
	for _, t := range tokens {
		if seen[t] {
			continue
		}
		seen[t] = true
		switch {
		case strings.HasPrefix(t, "negation "):
			neg++
		case strings.HasPrefix(t, `"`):
			c.quoted++
			hard = append(hard, t)
		case numRe.MatchString(t):
			c.num++
			hard = append(hard, t)
		case strings.Contains(t, "/"):
			c.path++
			hard = append(hard, t)
		case strings.ContainsAny(t, "_.") || regexp.MustCompile(`^[a-z]+[A-Z]`).MatchString(t):
			c.ident++
			hard = append(hard, t)
		}
	}
	return c, neg, hard
}

type unitLoss struct {
	slug         string
	in, out      int
	cost         float64
	total, lost  lossCount
	neg          int
	lostTokens   []string
	flags        []string
	state        string
	top, heading string
}

func normText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(s, "`", "")), " "))
}

func measure(file string, s segment, r Record) unitLoss {
	src := stripMarkup(s.Body)
	cur := stripMarkup(strings.Join(r.Lines, " "))
	u := unitLoss{slug: s.Slug, in: len([]rune(s.Body)), cost: r.CostUSD, state: r.State, top: s.Top, heading: s.Heading}
	if r.State == stateVerbatim {
		u.out = u.in
		return u
	}
	u.out = len([]rune(strings.Join(r.Lines, "\n")))
	u.total, _, _ = classify(textloss.LostText(src, ""))
	var hard []string
	u.lost, u.neg, hard = classify(textloss.LostText(src, cur))
	sort.Strings(hard)
	u.lostTokens = hard
	if u.total.hard() >= 20 && u.lost.hard()*2 > u.total.hard() {
		u.flags = append(u.flags, fmt.Sprintf("hard loss %d/%d over 50%%", u.lost.hard(), u.total.hard()))
	}
	for _, code := range errCodeRe.FindAllString(s.Body, -1) {
		if !strings.Contains(cur, code) {
			u.flags = append(u.flags, "missing "+code)
		}
	}
	nb, nc := normText(s.Body), normText(strings.Join(r.Lines, " "))
	for _, p := range cited {
		if lp := strings.ToLower(p); strings.Contains(nb, lp) && !strings.Contains(nc, lp) {
			u.flags = append(u.flags, "missing phrase "+p)
		}
	}
	return u
}

// commandGate returns the commands and check ids of s's source missing from its distilled lines.
func commandGate(s segment, r Record) []string {
	if r.State != stateOK {
		return nil
	}
	cur := strings.Join(strings.Fields(strings.ReplaceAll(strings.Join(r.Lines, " "), "`", "")), " ")
	var miss []string
	for _, m := range cmdSpanRe.FindAllStringSubmatch(s.Body, -1) {
		span := strings.Join(strings.Fields(m[1]), " ")
		for _, p := range cmdStarts {
			if strings.HasPrefix(span, p) && (strings.HasSuffix(p, "/") || strings.HasPrefix(span, p+" ") || span == p) {
				if !strings.Contains(cur, span) {
					miss = append(miss, span)
				}
				break
			}
		}
	}
	for _, id := range checkIDRe.FindAllString(s.Body, -1) {
		if !strings.Contains(cur, id) {
			miss = append(miss, id)
		}
	}
	return miss
}

func sectionOf(text, prefix string) string {
	lines := strings.SplitAfter(text, "\n")
	var out []string
	in, fence := false, false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && strings.HasPrefix(l, "## ") {
			in = strings.HasPrefix(strings.TrimPrefix(l, "## "), prefix)
		}
		if in {
			out = append(out, l)
		}
	}
	return strings.Join(out, "")
}

// verifyRun holds what the report sections share.
type verifyRun struct {
	st       standing
	segs     []segment
	recs     map[string]Record
	commit   string
	src      string
	rendered string
	b        strings.Builder
	fail     bool
}

func (v *verifyRun) f(format string, a ...any) { fmt.Fprintf(&v.b, format, a...) }

func (v *verifyRun) structure(rerr error) {
	v.f("## V1 structure\n\n")
	if rerr != nil {
		v.f("- render: FAIL: %v\n", rerr)
		v.fail = true
		return
	}
	sh, rh := headingsOf(v.src), headingsOf(v.rendered)
	same := strings.Join(sh, "\n") == strings.Join(rh, "\n")
	v.f("- headings: %d source, %d rendered, identical %v\n", len(sh), len(rh), same)
	v.fail = v.fail || !same
	nv, bad := 0, 0
	for _, s := range v.segs {
		if s.Kind == segProse && v.recs[s.Slug].State != stateVerbatim {
			continue
		}
		nv++
		if !strings.Contains(v.rendered, s.Raw) {
			bad++
		}
	}
	v.f("- verbatim segments (incl. kept units): %d, byte-identical in render %d\n", nv, nv-bad)
	v.fail = v.fail || bad > 0
	for _, k := range keepSections[v.st.file] {
		a, c := sectionOf(v.src, k), sectionOf(v.rendered, k)
		v.f("- section %q: %d bytes, %d bullet lines in source, identical %v\n", k, len(a), strings.Count(a, "\n- "), a == c && a != "")
		v.fail = v.fail || a != c || a == ""
	}
}

func (v *verifyRun) hardOf(text string) lossCount {
	c, _, _ := classify(textloss.LostText(stripMarkup(text), ""))
	return c
}

func (v *verifyRun) loss() []unitLoss {
	var all []unitLoss
	var distTot, distLost, docTot lossCount
	var cost float64
	var inSum, outSum, nOK, nVerb, nFall int
	for _, s := range v.segs {
		if s.Kind != segProse {
			if s.Kind == segVerbatim {
				docTot.add(v.hardOf(s.Raw))
			}
			continue
		}
		r, ok := v.recs[s.Slug]
		if !ok {
			continue
		}
		u := measure(v.st.file, s, r)
		all = append(all, u)
		inSum += u.in
		outSum += u.out
		cost += u.cost
		switch r.State {
		case stateOK:
			nOK++
			distTot.add(u.total)
			distLost.add(u.lost)
			docTot.add(u.total)
		case stateVerbatim:
			nVerb++
			docTot.add(v.hardOf(s.Body))
		default:
			nFall++
		}
	}
	v.f("\n## V2 loss\n\n- units: ok %d, kept verbatim %d, fallback %d of %d prose units\n", nOK, nVerb, nFall, nOK+nVerb+nFall)
	if distTot.hard() > 0 {
		pct := 100 * float64(distLost.hard()) / float64(distTot.hard())
		v.f("- hard tokens in distilled units: lost %d of %d (%.1f%%): numbers %d/%d, identifiers %d/%d, paths %d/%d, quoted %d/%d\n",
			distLost.hard(), distTot.hard(), pct, distLost.num, distTot.num, distLost.ident, distTot.ident, distLost.path, distTot.path, distLost.quoted, distTot.quoted)
		v.f("- whole-doc hard loss (verbatim segments and kept units count as kept): %d of %d (%.1f%%)\n", distLost.hard(), docTot.hard(), 100*float64(distLost.hard())/float64(max(docTot.hard(), 1)))
		if v.st.file == "ARCHITECTURE.md" && pct > 45 {
			v.f("- DOC GATE FAIL: distilled-unit hard loss %.1f%% over 45%%: stop before render\n", pct)
			v.fail = true
		}
	}
	v.errCodes()
	v.f("- prose chars in %d, out %d, ok-unit cost %.4f USD\n", inSum, outSum, cost)
	if v.rendered != "" {
		v.f("- bytes: source %d, rendered %d; lines: source %d, rendered %d\n", len(v.src), len(v.rendered), strings.Count(v.src, "\n"), strings.Count(v.rendered, "\n"))
	}
	return all
}

func (v *verifyRun) errCodes() {
	codes := map[string]bool{}
	for _, c := range errCodeRe.FindAllString(v.src, -1) {
		codes[c] = true
	}
	kept := 0
	for c := range codes {
		if strings.Contains(v.rendered, c) {
			kept++
		}
	}
	v.f("- E_* codes: %d in source, %d kept in render\n", len(codes), kept)
}

func (v *verifyRun) commandGates() {
	v.f("\n## PACKAGING command gate\n\n")
	failedTop := map[string][]string{}
	for _, s := range proseSegs(v.segs) {
		if miss := commandGate(s, v.recs[s.Slug]); len(miss) > 0 {
			failedTop[s.Top] = append(failedTop[s.Top], s.Slug)
			v.f("- FAIL %s missing: %s\n", s.Slug, strings.Join(miss, " | "))
		}
	}
	if len(failedTop) == 0 {
		v.f("- all commands and check ids in distilled units survive\n")
	}
	for top, sl := range failedTop {
		v.f("- section %q reverts: keep %s\n", top, strings.Join(sl, " "))
	}
}

func (v *verifyRun) flagged(all []unitLoss) {
	v.f("\n## Flagged units\n\n")
	n := 0
	for _, u := range all {
		if len(u.flags) > 0 {
			n++
			v.f("- %s: %s\n", u.slug, strings.Join(u.flags, "; "))
		}
	}
	if n == 0 {
		v.f("none\n")
	}
}

func (v *verifyRun) handRead() {
	v.f("\n## Hand-read sample\n\n")
	picks := proseSegs(v.segs)
	if v.st.file == "ARCHITECTURE.md" {
		picks = pickSample(v.segs, v.recs, 12)
	}
	for _, s := range picks {
		if v.recs[s.Slug].State == stateOK {
			v.f("- %s (%s)\n", s.Slug, s.Path)
		}
	}
}

func (v *verifyRun) perUnit(all []unitLoss) {
	v.f("\n## Per unit\n\n| slug | state | in | out | cost | hard lost/total | num | ident | path | quoted | neg drops | flags |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, u := range all {
		v.f("| %s | %s | %d | %d | %.4f | %d/%d | %d/%d | %d/%d | %d/%d | %d/%d | %d | %s |\n", u.slug, u.state, u.in, u.out, u.cost,
			u.lost.hard(), u.total.hard(), u.lost.num, u.total.num, u.lost.ident, u.total.ident, u.lost.path, u.total.path, u.lost.quoted, u.total.quoted, u.neg, strings.Join(u.flags, "; "))
	}
	v.f("\n## Lost hard tokens (clipped to 30)\n\n")
	for _, u := range all {
		if len(u.lostTokens) > 0 {
			v.f("- %s: %s\n", u.slug, strings.Join(u.lostTokens[:min(30, len(u.lostTokens))], " "))
		}
	}
}

func (st standing) cmdVerify(outPath string) error {
	segs, recs, commit, src, err := st.load()
	if err != nil {
		return err
	}
	v := &verifyRun{st: st, segs: segs, recs: recs, commit: commit, src: src}
	var rerr error
	v.rendered, rerr = renderDoc(st.file, commit, segs, recs)
	v.f("# P303 verify: %s\n\nSource commit %s. Records %s.\n\n", st.path, commit[:12], st.runFile)
	v.structure(rerr)
	all := v.loss()
	if st.file == "PACKAGING.md" {
		v.commandGates()
	}
	v.flagged(all)
	v.handRead()
	v.perUnit(all)
	if outPath == "-" {
		fmt.Print(v.b.String())
	} else if err := os.WriteFile(outPath, []byte(v.b.String()), 0o644); err != nil {
		return err
	}
	if v.fail {
		return fmt.Errorf("verify: a V1 or doc gate check failed")
	}
	return nil
}

func headingsOf(text string) []string {
	var out []string
	fence := false
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
		}
		if !fence && docHeadingRe.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

// sample picks n distilled units outside the pilot, at most one per "##", ordered by sha1 of the slug.
func pickSample(segs []segment, recs map[string]Record, n int) []segment {
	best := map[string]segment{}
	key := func(s segment) string { return fmt.Sprintf("%x", sha1.Sum([]byte(s.Slug))) }
	for _, s := range proseSegs(segs) {
		if recs[s.Slug].State != stateOK || pilotHeadings[s.Heading] || pilotHeadings[s.Top] {
			continue
		}
		if b, ok := best[s.Top]; !ok || key(s) < key(b) {
			best[s.Top] = s
		}
	}
	out := make([]segment, 0, len(best))
	for _, s := range best {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out[:min(n, len(out))]
}
