// Command promptsizes prints the rendered default size of every registry entry: id, characters and
// estimated tokens (ceil(runes/3), the repo estimate). Run from the repo root.
//
//	go run ./internal/llmprompts/cmd/promptsizes [-prefix headless.] [-snapshot f.json]
//	[-compare f.json]
//
// -snapshot writes {id: {default, chars, tokens}}. -compare prints before/after sizes per id
// against a snapshot, then checks that no protected token (actions, numbers, quoted spans,
// identifiers, field names, negations) was lost; exit 1 on an unexplained loss.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/renameio/v2"

	"github.com/kirathecat/kira-studio/internal/llmprompts"
	"github.com/kirathecat/kira-studio/internal/textloss"
)

type snap struct {
	Default string `json:"default"`
	Chars   int    `json:"chars"`
	Tokens  int    `json:"tokens"`
}

func tokens(n int) int { return (n + 2) / 3 }

func main() {
	prefix := flag.String("prefix", "", "only entries whose id starts with this")
	snapshot := flag.String("snapshot", "", "write a JSON snapshot of every selected entry to this file")
	compare := flag.String("compare", "", "compare against this snapshot")
	allowed := flag.String("allowed", "internal/llmprompts/testdata/P289-runs/allowed-losses.tsv", "allowed protected-token losses (id, token, why)")
	flag.Parse()

	cur := map[string]snap{}
	var ids []string
	var chars, toks int
	for _, in := range llmprompts.All() {
		if !strings.HasPrefix(string(in.ID), *prefix) {
			continue
		}
		text, err := llmprompts.RenderExample(in.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "promptsizes: %s: %v\n", in.ID, err)
			os.Exit(1)
		}
		n := utf8.RuneCountInString(text)
		cur[string(in.ID)] = snap{Default: in.Default, Chars: n, Tokens: tokens(n)}
		ids = append(ids, string(in.ID))
		chars += n
		toks += tokens(n)
		if *compare == "" {
			fmt.Printf("%s\t%d\t%d\n", in.ID, n, tokens(n))
		}
	}
	if *compare == "" {
		fmt.Printf("total\t%d\t%d\n", chars, toks)
	}
	if *snapshot != "" {
		b, err := json.MarshalIndent(cur, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := renameio.WriteFile(*snapshot, append(b, '\n'), 0o644); err != nil {
			fatal(err)
		}
	}
	if *compare != "" {
		os.Exit(compareTo(*compare, *allowed, ids, cur))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "promptsizes:", err)
	os.Exit(2)
}

func compareTo(file, allowedFile string, ids []string, cur map[string]snap) int {
	raw, err := os.ReadFile(file)
	if err != nil {
		fatal(err)
	}
	var before map[string]snap
	if err := json.Unmarshal(raw, &before); err != nil {
		fatal(err)
	}
	allow := readAllowed(allowedFile)
	fields := fieldsByEntry()
	var bc, bt, ac, at, changed int
	var problems []string
	fmt.Println("id\tbefore chars\tafter chars\tbefore tok\tafter tok\tchange")
	sort.Strings(ids)
	for _, id := range ids {
		a := cur[id]
		b, ok := before[id]
		if !ok {
			fmt.Printf("%s\t-\t%d\t-\t%d\tnew\n", id, a.Chars, a.Tokens)
			continue
		}
		bc, bt, ac, at = bc+b.Chars, bt+b.Tokens, ac+a.Chars, at+a.Tokens
		if a.Default == b.Default {
			continue
		}
		changed++
		fmt.Printf("%s\t%d\t%d\t%d\t%d\t%+.1f%%\n", id, b.Chars, a.Chars, b.Tokens, a.Tokens, pct(b.Chars, a.Chars))
		for _, l := range textloss.Lost(b.Default, a.Default, fields[id]) {
			if !allow[id+"\t"+l] {
				problems = append(problems, fmt.Sprintf("%s: lost %s", id, l))
			}
		}
	}
	fmt.Printf("total\t%d\t%d\t%d\t%d\t%+.1f%%\t(%d entries changed)\n", bc, ac, bt, at, pct(bc, ac), changed)
	if len(problems) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(problems, "\n"))
		fmt.Fprintf(os.Stderr, "promptsizes: %d unexplained protected-token loss(es); restore or list in the allowed file\n", len(problems))
		return 1
	}
	return 0
}

func pct(before, after int) float64 {
	if before == 0 {
		return 0
	}
	return float64(after-before) * 100 / float64(before)
}

func readAllowed(file string) map[string]bool {
	out := map[string]bool{}
	raw, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		p := strings.Split(line, "\t")
		if len(p) >= 2 && !strings.HasPrefix(line, "#") {
			out[p[0]+"\t"+p[1]] = true
		}
	}
	return out
}

// fieldsByEntry maps an entry id to the argument names of its tool: the entry's own children for a
// tool, its siblings for an argument.
func fieldsByEntry() map[string][]string {
	byParent := map[llmprompts.ID][]string{}
	for _, in := range llmprompts.All() {
		if in.Parent != "" && in.Field != "" {
			byParent[in.Parent] = append(byParent[in.Parent], in.Field)
		}
	}
	out := map[string][]string{}
	for _, in := range llmprompts.All() {
		parent := in.ID
		if in.Parent != "" {
			parent = in.Parent
		}
		out[string(in.ID)] = byParent[parent]
	}
	return out
}
