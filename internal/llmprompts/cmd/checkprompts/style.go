package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
)

const budgetFile = "internal/llmprompts/testdata/budget.tsv"

var (
	protectedRe = regexp.MustCompile("(?s)\\{\\{.*?\\}\\}|`[^`]*`|\"[^\"\\n]*\"")
	fillerRe    = regexp.MustCompile(`(?i)\b(please|just|simply|basically|actually|really|very|of course|certainly|kindly|in order to|make sure|note that|it is important|you should|feel free)\b|→`)
)

// styleProblems flags filler outside template actions, backticks and double-quoted spans
// (docs/WORKING_AGREEMENT.md Communication style).
func styleProblems() []string {
	var out []string
	for _, in := range llmprompts.All() {
		text := protectedRe.ReplaceAllString(in.Default, " ")
		for _, m := range fillerRe.FindAllString(text, -1) {
			out = append(out, fmt.Sprintf("%s: filler %q (docs/WORKING_AGREEMENT.md Communication style)", in.ID, strings.ToLower(m)))
		}
	}
	return out
}

// budgetProblems compares each entry's rendered example size (runes) with its line in budget.tsv.
func budgetProblems(root string) []string {
	raw, err := os.ReadFile(filepath.Join(root, budgetFile))
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", budgetFile, err)}
	}
	max := map[string]int{}
	var out []string
	for i, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, n, ok := strings.Cut(line, "\t")
		v, err := strconv.Atoi(n)
		if !ok || err != nil {
			out = append(out, fmt.Sprintf("%s:%d: want <id><TAB><max runes>", budgetFile, i+1))
			continue
		}
		max[id] = v
	}
	known := map[string]bool{}
	for _, in := range llmprompts.All() {
		id := string(in.ID)
		known[id] = true
		text, err := llmprompts.RenderExample(in.ID)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		limit, ok := max[id]
		switch n := utf8.RuneCountInString(text); {
		case !ok:
			out = append(out, fmt.Sprintf("%s: no size budget; write it terse and add a line to %s", id, budgetFile))
		case n > limit:
			out = append(out, fmt.Sprintf("%s: %d characters, budget %d; tighten it (docs/WORKING_AGREEMENT.md Communication style) or raise the line in %s", id, n, limit, budgetFile))
		}
	}
	for id := range max {
		if !known[id] {
			out = append(out, fmt.Sprintf("%s: budget line for no registry entry; remove it from %s", id, budgetFile))
		}
	}
	return out
}
