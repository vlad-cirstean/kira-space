package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	locatorRe = regexp.MustCompile(`^(?::\d+(?:-\d+)?|(?:\s*(?:§\s*\d[\d.]*[a-z]?|\bD\d+\b|\bsection\s+\d[\d.]*|\bAppendix\s+[A-Z]\b))+)`)
	fileIDRe  = regexp.MustCompile(`^[PCMG]\d+(?:\.\d+)?[a-z]*`)
)

func refPattern(commits map[string]string) *regexp.Regexp {
	alt := make([]string, 0, len(commits))
	for ch := range commits {
		alt = append(alt, regexp.QuoteMeta(ch))
	}
	return regexp.MustCompile(`docs/(` + strings.Join(alt, "|") + `)/([A-Za-z0-9_.\-/]*)`)
}

func operative(path string) bool {
	return strings.HasPrefix(path, "scripts/") || strings.HasPrefix(path, ".githooks/") ||
		path == "docs/DEV_ENVIRONMENT.md" || path == "CLAUDE.md"
}

type repointStats struct{ refs, files int }

// refTarget is the replacement for one matched reference, or "" when the shape is not handled. loc says
// whether trailing locators (section, decision, line numbers) belong to the reference and are dropped.
func refTarget(ch, tail string) (rep string, loc bool) {
	switch {
	case tail == "SPEC.md" || tail == "README.md":
		return "docs/HISTORY.md (" + ch + ")", true
	case strings.HasPrefix(tail, "plans/") && strings.HasSuffix(tail, ".md") && !strings.Contains(tail[6:], "/"):
		if id := fileIDRe.FindString(tail[6:]); id != "" {
			return "docs/HISTORY.md (" + ch + " " + id + ")", true
		}
	case tail == "" || tail == "plans" || tail == "plans/":
		return "docs/HISTORY.md (" + ch + ")", false
	}
	return "", false
}

// repointText rewrites the doc references in text; commits maps chapter to its 12-char source commit.
func repointText(path, text string, commits map[string]string) (string, int, []string) {
	var out strings.Builder
	var unmatched []string
	n, last := 0, 0
	op := operative(path)
	md := strings.HasSuffix(path, ".md")
	for _, m := range refPattern(commits).FindAllStringSubmatchIndex(text, -1) {
		ch, tail, end := text[m[2]:m[3]], text[m[4]:m[5]], m[1]
		if strings.HasSuffix(tail, ".") {
			tail = strings.TrimSuffix(tail, ".")
			end--
		}
		rep, loc := refTarget(ch, tail)
		if rep == "" || (end < len(text) && strings.ContainsAny(text[end:end+1], "*{[")) {
			unmatched = append(unmatched, "docs/"+ch+"/"+tail)
			continue
		}
		if op {
			if !strings.HasSuffix(tail, ".md") {
				unmatched = append(unmatched, "docs/"+ch+"/"+tail)
				continue
			}
			rep, loc = "git show "+commits[ch]+":docs/"+ch+"/"+tail, false
			if md && !(m[0] > 0 && text[m[0]-1] == '`') {
				rep = "`" + rep + "`"
			}
		}
		if loc {
			end += len(locatorRe.FindString(text[end:]))
		}
		out.WriteString(text[last:m[0]] + rep)
		last = end
		n++
	}
	out.WriteString(text[last:])
	return out.String(), n, unmatched
}

func skipRepoint(path string) bool {
	return strings.HasPrefix(path, "docs/v") || path == "docs/HISTORY.md" || strings.HasPrefix(path, "docs/history/") ||
		strings.HasPrefix(path, ".github/workflows/") ||
		strings.HasPrefix(path, "scripts/docshistory/") || strings.HasPrefix(path, "apps/kira-space/tests/claude/testdata/") ||
		strings.HasPrefix(path, "internal/llmprompts/testdata/")
}

func cmdRepoint(runs string, dry bool, chapters []string) error {
	commits := map[string]string{}
	for _, ch := range chapters {
		recs, err := loadRecords(filepath.Join(runs, ch+".jsonl"))
		if err != nil {
			return err
		}
		for _, r := range recs {
			commits[ch] = r.SourceCommit[:12]
			break
		}
		if commits[ch] == "" {
			return fmt.Errorf("%s: no runs recorded, cannot pin a source commit", ch)
		}
	}
	list, err := git("ls-files")
	if err != nil {
		return err
	}
	ref := refPattern(commits)
	var st repointStats
	var unmatched []string
	for _, f := range strings.Split(strings.TrimSpace(list), "\n") {
		if f == "" || skipRepoint(f) {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil || bytes.IndexByte(raw, 0) >= 0 {
			continue
		}
		text := string(raw)
		if !ref.MatchString(text) {
			continue
		}
		next, n, um := repointText(f, text, commits)
		for _, u := range um {
			unmatched = append(unmatched, f+": "+u)
		}
		if n == 0 {
			continue
		}
		st.refs += n
		st.files++
		if !dry {
			if err := os.WriteFile(f, []byte(next), 0o644); err != nil {
				return err
			}
		}
	}
	fmt.Printf("repointed %d references in %d files (dry=%v)\n", st.refs, st.files, dry)
	for _, u := range unmatched {
		fmt.Println("unmatched:", u)
	}
	return nil
}
