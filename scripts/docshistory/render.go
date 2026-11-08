package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type histBlock struct{ file, body string }

var chapterBlockRe = regexp.MustCompile(`(?s)<!-- chapter (\S+) -->\n(.*?)<!-- /chapter (\S+) -->\n`)

// readBlocks maps each chapter in dir/*.md to its file and body.
func readBlocks(dir string) (map[string]histBlock, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	blocks := map[string]histBlock{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		for _, m := range chapterBlockRe.FindAllStringSubmatch(string(raw), -1) {
			if m[1] != m[3] {
				return nil, fmt.Errorf("%s: chapter %s closed as %s", f, m[1], m[3])
			}
			if prev, ok := blocks[m[1]]; ok {
				return nil, fmt.Errorf("chapter %s in both %s and %s", m[1], prev.file, f)
			}
			blocks[m[1]] = histBlock{file: f, body: m[2]}
		}
	}
	return blocks, nil
}

// spliceBlock replaces chapter ch's body in path; every other byte stays.
func spliceBlock(path, ch, body string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	q := regexp.QuoteMeta(ch)
	re := regexp.MustCompile(`(?s)<!-- chapter ` + q + ` -->\n(.*?)<!-- /chapter ` + q + ` -->\n`)
	m := re.FindStringSubmatchIndex(text)
	if m == nil {
		return fmt.Errorf("%s: no block for %s", path, ch)
	}
	return os.WriteFile(path, []byte(text[:m[2]]+body+text[m[3]:]), 0o644)
}

func requireBlocks(dir string, chapters []string) (map[string]histBlock, error) {
	blocks, err := readBlocks(dir)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, ch := range chapters {
		if _, ok := blocks[ch]; !ok {
			errs = append(errs, fmt.Errorf("%s: no block in %s; a new chapter is closed by hand (CLAUDE.md \"Closing a chapter\")", ch, dir))
		}
	}
	return blocks, errors.Join(errs...)
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func recLines(u Unit, r Record) []string {
	if r.State != stateOK {
		return fallbackLines(u, r)
	}
	var out []string
	for _, l := range r.Lines {
		if l = oneLine(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func phaseHeading(ch string, u Unit) string {
	head := fmt.Sprintf("### %s %s", ch, u.ID)
	if t := clip(oneLine(strings.ReplaceAll(u.Title, "**", "")), 120); t != "" {
		head += " " + t
	}
	if u.Status != "" {
		head += " (" + oneLine(u.Status) + ")"
	}
	return head
}

func renderChapter(ch string, p *Parsed, recs map[string]Record, commit string) string {
	units := buildUnits(p)
	var b strings.Builder
	var pending []string
	nPhases := 0
	for _, u := range units {
		if !u.Notes {
			nPhases++
			if _, ok := recs[u.Slug]; !ok {
				pending = append(pending, u.ID)
			}
		}
	}
	fmt.Fprintf(&b, "## %s\nSource: docs/%s/ at %s. %d phases.\n", ch, ch, commit[:12], nPhases)
	for _, u := range units {
		if r, ok := recs[u.Slug]; ok && u.Notes {
			for _, l := range recLines(u, r) {
				b.WriteString(l + "\n")
			}
		}
	}
	if len(pending) > 0 {
		b.WriteString("Not distilled yet: " + strings.Join(pending, ", ") + ".\n")
	}
	for _, u := range units {
		r, ok := recs[u.Slug]
		if u.Notes || !ok {
			continue
		}
		b.WriteString("\n" + phaseHeading(ch, u) + "\n")
		for _, l := range recLines(u, r) {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

func fallbackLines(u Unit, r Record) []string {
	var out []string
	if u.Phase != nil {
		row := u.Phase.Title
		if len(row) < 60 && len(u.Phase.Cells) > 0 {
			row += " " + u.Phase.Cells[0]
		}
		if s := firstSentence(strings.ReplaceAll(row, "**", ""), 180); s != "" {
			out = append(out, s)
		}
	}
	for _, l := range markerLines(u.Text) {
		out = append(out, clip(oneLine(l), 200))
	}
	return append(out, "(not distilled: "+r.Reason+")")
}

// cmdRender re-renders the chapters' existing blocks in dir and prints sizes.
func cmdRender(runs, dir string, chapters []string) error {
	blocks, err := requireBlocks(dir, chapters)
	if err != nil {
		return err
	}
	for _, ch := range chapters {
		recs, err := loadRecords(filepath.Join(runs, ch+".jsonl"))
		if err != nil {
			return err
		}
		if len(recs) == 0 {
			continue
		}
		commit := ""
		for _, r := range recs {
			commit = r.SourceCommit
			break
		}
		p, err := loadChapter(ch, commit)
		if err != nil {
			return err
		}
		body := renderChapter(ch, p, recs, commit)
		if err := spliceBlock(blocks[ch].file, ch, body); err != nil {
			return err
		}
		fmt.Printf("%s %s %d bytes, %d lines\n", ch, blocks[ch].file, len(body), strings.Count(body, "\n"))
	}
	return nil
}
