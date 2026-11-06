package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ctxBefore   = 5
	ctxAfter    = 25
	ctxMaxRunes = 6000
	maxLineRuns = 1 << 26
)

// rawRange is one comment as UTF-8 byte offsets, end exclusive. Kind is line, block or template.
type rawRange struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
}

// group is one candidate unit: an ast.CommentGroup, a block comment or a run of // lines.
type group struct {
	start, end int
	cgo        bool
}

var (
	goDirectiveRe = regexp.MustCompile(`(?m)^[ \t]*(//(go:|nolint|lint:|export |line )|// \+build|// (Output|Unordered output):)|Code generated .* DO NOT EDIT`)
	tsDirectiveRe = regexp.MustCompile(`biome-ignore|eslint-|@ts-expect-error|@ts-ignore|@ts-nocheck|@ts-check|/// <reference|#__PURE__|@__PURE__|@vite-ignore|webpackChunkName|istanbul|prettier-ignore|@vitest-environment|@license|@jsxImportSource`)
	licenseRe     = regexp.MustCompile(`Copyright|SPDX-License-Identifier`)
)

// scopeStatus reports why a tracked file is out of scope, or "".
func scopeStatus(p string) string {
	switch {
	case strings.HasPrefix(p, "apps/kira-space/internal/httpclient/cookiejar/"),
		strings.HasPrefix(p, "testdata/"), strings.Contains(p, "/testdata/"),
		strings.HasPrefix(p, "docs/"), strings.HasPrefix(p, ".claude/"), strings.Contains(p, "node_modules/"),
		p == "packages/shared/protocol/wire.ts", strings.HasPrefix(p, "packages/shared/protocol/wire/"),
		strings.HasPrefix(p, "packages/git-ipc/src/generated/"), strings.HasSuffix(p, ".d.ts"):
		return stExcluded
	}
	if parts := strings.Split(p, "/"); len(parts) > 3 && parts[0] == "apps" && parts[2] == "tests" && parts[3] == "contract" {
		return stExcluded
	}
	return ""
}

func langOf(p string) string {
	switch path.Ext(p) {
	case ".go":
		return "go"
	case ".ts":
		return "ts"
	case ".vue":
		return "vue"
	}
	return ""
}

func isTestFile(p string) bool {
	return strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, ".spec.ts") || strings.HasSuffix(p, ".test.ts") ||
		strings.Contains(p, "/tests/") || strings.Contains(p, "/test/")
}

// lineIndex maps byte offsets to 0-based lines.
type lineIndex []int

func newLineIndex(src []byte) lineIndex {
	idx := lineIndex{0}
	for i, b := range src {
		if b == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

func (l lineIndex) line(off int) int {
	return sort.Search(len(l), func(i int) bool { return l[i] > off }) - 1
}

func onlyWS(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 }

// goGroups parses src and returns one group per comment group. generated reports a generated-code header.
func goGroups(name string, src []byte) (gs []group, generated bool, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	cgo := map[int]bool{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		for _, s := range gd.Specs {
			if is, ok := s.(*ast.ImportSpec); ok && is.Path.Value == `"C"` {
				if is.Doc != nil {
					cgo[fset.Position(is.Doc.Pos()).Offset] = true
				}
				if gd.Doc != nil {
					cgo[fset.Position(gd.Doc.Pos()).Offset] = true
				}
			}
		}
	}
	for _, g := range f.Comments {
		s := fset.Position(g.Pos()).Offset
		gs = append(gs, group{start: s, end: fset.Position(g.End()).Offset, cgo: cgo[s]})
	}
	return gs, ast.IsGenerated(f), nil
}

// tsGroups turns raw comment ranges into groups: a block or template comment alone, consecutive whole-line
// // comments at one column as a run.
func tsGroups(src []byte, ranges []rawRange) []group {
	idx := newLineIndex(src)
	var out []group
	var runLine, runCol = -2, -1
	for _, r := range ranges {
		if r.Kind != "line" {
			out = append(out, group{start: r.Start, end: r.End})
			runLine = -2
			continue
		}
		ln := idx.line(r.Start)
		col := r.Start - idx[ln]
		whole := onlyWS(src[idx[ln]:r.Start])
		if whole && runLine == ln-1 && runCol == col && len(out) > 0 {
			out[len(out)-1].end = r.End
		} else {
			out = append(out, group{start: r.Start, end: r.End})
		}
		if whole {
			runLine, runCol = ln, col
		} else {
			runLine = -2
		}
	}
	return out
}

// classify turns groups of one file into manifest records and units. A file-level status wins over all.
func classify(file, lang string, src []byte, gs []group, fileStatus, fileReason string, minChars, maxChars int) ([]Comment, []Unit) {
	idx := newLineIndex(src)
	fsha := sha(src)
	test := isTestFile(file)
	dirRe := tsDirectiveRe
	if lang == "go" {
		dirRe = goDirectiveRe
	}
	var cs []Comment
	var us []Unit
	for _, g := range gs {
		ln := idx.line(g.start)
		ls := idx[ln]
		le := len(src)
		if endLine := idx.line(g.end - 1); endLine+1 < len(idx) {
			le = idx[endLine+1] - 1
		}
		whole := onlyWS(src[ls:g.start]) && (g.end > le || onlyWS(src[g.end:le]))
		start := g.start
		if whole {
			start = ls
		}
		text := string(src[start:g.end])
		c := Comment{File: file, Line: ln + 1, Lang: lang, Chars: utf8.RuneCountInString(text), First: firstLine(text), Test: test}
		switch {
		case fileStatus != "":
			c.Status, c.Reason = fileStatus, fileReason
		case g.cgo:
			c.Status, c.Reason = stDirective, "cgo preamble"
		case licenseRe.MatchString(text):
			c.Status, c.Reason = stLicense, licenseRe.FindString(text)
		case dirRe.MatchString(text):
			c.Status, c.Reason = stDirective, strings.TrimSpace(dirRe.FindString(text))
		case lang != "go" && strings.HasPrefix(strings.TrimSpace(text), "/*!"):
			c.Status, c.Reason = stLicense, "/*!"
		case c.Chars < minChars:
			c.Status = stBelowMin
		case c.Chars > maxChars:
			c.Status = stOverMax
		case strings.Contains(text, "\r"):
			c.Status = stCRLF
		case !whole:
			c.Status = stNotWholeLine
		default:
			c.Status = stEligible
		}
		textSha := sha([]byte(text))
		c.ID = sha([]byte(fmt.Sprintf("%s:%d:%d:%s", file, start, g.end, textSha)))[:16]
		cs = append(cs, c)
		if c.Status != stEligible {
			continue
		}
		ctx := contextOf(src, idx, ln, idx.line(g.end-1))
		us = append(us, Unit{ID: c.ID, File: file, Lang: lang, Line: c.Line, Start: start, End: g.end, Text: text, Context: ctx,
			TextSha: textSha, FileSha: fsha, InputKey: sha([]byte(text + "\x00" + ctx)), Test: test})
	}
	return cs, us
}

// contextOf is the 5 lines before and the 25 lines after the unit, verbatim, cut at a line boundary to
// ctxMaxRunes by dropping the farthest lines first.
func contextOf(src []byte, idx lineIndex, first, last int) string {
	line := func(i int) string {
		end := len(src)
		if i+1 < len(idx) {
			end = idx[i+1] - 1
		}
		return string(src[idx[i]:max(end, idx[i])])
	}
	var before, after []string
	for i := max(0, first-ctxBefore); i < first; i++ {
		before = append(before, line(i))
	}
	for i := last + 1; i < min(len(idx), last+1+ctxAfter); i++ {
		after = append(after, line(i))
	}
	size := func() int {
		n := 0
		for _, l := range before {
			n += utf8.RuneCountInString(l) + 1
		}
		for _, l := range after {
			n += utf8.RuneCountInString(l) + 1
		}
		return n
	}
	for size() > ctxMaxRunes && (len(before) > 0 || len(after) > 0) {
		if len(after) > 0 {
			after = after[:len(after)-1]
		} else {
			before = before[1:]
		}
	}
	return strings.Join(append(before, after...), "\n")
}

// trackedFiles lists git-tracked Go, TS and Vue files.
func trackedFiles() ([]string, error) {
	out, err := exec.Command("git", "ls-files", "-z", "--", "*.go", "*.ts", "*.vue").Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" && langOf(f) != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// scanTS runs the Bun helper over files and returns their comment ranges, or an error string per file.
func scanTS(files []string) (map[string][]rawRange, map[string]string, error) {
	cmd := exec.Command("bun", "scripts/commentsweep/scan.ts", "scan")
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n") + "\n")
	cmd.Stderr = os.Stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	ranges := map[string][]rawRange{}
	errs := map[string]string{}
	sc := bufio.NewScanner(pipe)
	sc.Buffer(make([]byte, 1<<20), maxLineRuns)
	for sc.Scan() {
		var r struct {
			File   string     `json:"file"`
			Error  string     `json:"error"`
			Ranges []rawRange `json:"ranges"`
		}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, nil, fmt.Errorf("scan.ts output: %w", err)
		}
		if r.Error != "" {
			errs[r.File] = r.Error
		}
		ranges[r.File] = r.Ranges
	}
	if err := sc.Err(); err != nil {
		return nil, nil, err
	}
	return ranges, errs, cmd.Wait()
}

// extractAll scans every tracked file and returns manifest records and eligible units.
func extractAll(files []string, minChars, maxChars int) ([]Comment, []Unit, error) {
	var tsFiles []string
	for _, f := range files {
		if langOf(f) != "go" {
			tsFiles = append(tsFiles, f)
		}
	}
	ranges, tsErrs, err := scanTS(tsFiles)
	if err != nil {
		return nil, nil, err
	}
	var cs []Comment
	var us []Unit
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		lang := langOf(f)
		scope := scopeStatus(f)
		var gs []group
		fileStatus, reason := scope, ""
		if lang == "go" {
			g, generated, perr := goGroups(f, src)
			switch {
			case perr != nil:
				cs = append(cs, Comment{ID: sha([]byte(f))[:16], File: f, Line: 1, Lang: lang, Status: stParseError, Reason: clip(perr.Error(), 160), Test: isTestFile(f)})
				continue
			case generated && fileStatus == "":
				fileStatus = stGenerated
			}
			gs = g
		} else {
			if e, bad := tsErrs[f]; bad {
				cs = append(cs, Comment{ID: sha([]byte(f))[:16], File: f, Line: 1, Lang: lang, Status: stParseError, Reason: clip(e, 160), Test: isTestFile(f)})
				continue
			}
			gs = tsGroups(src, ranges[f])
			if fileStatus == "" && (bytes.Contains(headOf(src), []byte("Code generated")) || bytes.Contains(headOf(src), []byte("@generated"))) {
				fileStatus = stGenerated
			}
		}
		c, u := classify(f, lang, src, gs, fileStatus, reason, minChars, maxChars)
		cs = append(cs, c...)
		us = append(us, u...)
	}
	sortUnits(us)
	return cs, us, nil
}

func headOf(src []byte) []byte { return src[:min(len(src), 600)] }

func clip(s string, n int) string {
	s = strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func projectCost(chars int) float64 { return 0.0118 + 0.0000104*float64(chars) }

type countRow struct{ eligible, test, over, below, directive, whole, excluded, other int }

// scanReport prints per-language counts against the plan's >=300 row and the cost projection.
func scanReport(cs []Comment, us []Unit) {
	rows := map[string]*countRow{"go": {}, "ts": {}, "vue": {}}
	for _, c := range cs {
		r := rows[c.Lang]
		switch c.Status {
		case stEligible:
			r.eligible++
			if c.Test {
				r.test++
			}
		case stOverMax:
			r.over++
		case stBelowMin:
			r.below++
		case stDirective, stLicense:
			r.directive++
		case stNotWholeLine:
			r.whole++
		case stExcluded, stGenerated:
			r.excluded++
		default:
			r.other++
		}
	}
	plan := map[string]int{"go": 2912, "ts": 1965, "vue": 476}
	var sum float64
	for _, u := range us {
		sum += projectCost(len([]rune(u.Text)))
	}
	fmt.Printf("%-4s %9s %6s %8s %9s %9s %10s %9s %6s %s\n", "lang", "eligible", "test", "over-max", "below-min", "directive", "not-whole", "excl/gen", "other", "vs plan")
	for _, l := range []string{"go", "ts", "vue"} {
		r := rows[l]
		fmt.Printf("%-4s %9d %6d %8d %9d %9d %10d %9d %6d %+.1f%% (plan %d)\n", l, r.eligible, r.test, r.over, r.below, r.directive, r.whole, r.excluded, r.other,
			100*float64(r.eligible-plan[l])/float64(plan[l]), plan[l])
	}
	fmt.Printf("projected first pass USD: low %.1f likely %.1f high %.1f (%d calls)\n", sum*0.9, sum, sum*1.35, len(us))
}

func gitOut(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func dirtyInScope() (string, error) {
	return gitOut("status", "--porcelain", "--untracked-files=no", "--", "*.go", "*.ts", "*.vue")
}

func runScan(minChars int, force bool) error {
	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the repo root")
	}
	if d, err := dirtyInScope(); err != nil {
		return err
	} else if d != "" && !force {
		return fmt.Errorf("in-scope files are dirty; commit or stash first:\n%s", d)
	}
	files, err := trackedFiles()
	if err != nil {
		return err
	}
	cs, us, err := extractAll(files, minChars, 2000)
	if err != nil {
		return err
	}
	base, _ := gitOut("rev-parse", "HEAD")
	meta := Meta{Base: base, MinChars: minChars, MaxChars: 2000}
	if old, err := loadMeta(); err == nil {
		meta.CreatedAt = old.CreatedAt
	} else {
		meta.CreatedAt = time.Now().UTC()
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(wpath("meta.json"), append(raw, '\n'), 0o644); err != nil {
		return err
	}
	if err := writeAll("manifest.jsonl", cs); err != nil {
		return err
	}
	if err := writeAll("units.jsonl", us); err != nil {
		return err
	}
	var over, skipped []Comment
	for _, c := range cs {
		switch c.Status {
		case stOverMax:
			over = append(over, c)
		case stDirective, stLicense, stNotWholeLine, stCRLF:
			skipped = append(skipped, c)
		}
	}
	if err := writeAll("over-limit.jsonl", over); err != nil {
		return err
	}
	if err := writeAll("skipped.jsonl", skipped); err != nil {
		return err
	}
	scanReport(cs, us)
	fmt.Printf("manifest: %d comments, %d eligible units, base %s\n", len(cs), len(us), strconv.Quote(base))
	return nil
}

// sortedKeys is a small helper for stable report output.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
