// Command docshistory re-renders folded chapter blocks in docs/history/.
//
//	go run ./scripts/docshistory units|render|verify|repoint [flags] <chapter...>
//	go run ./scripts/docshistory doc units|keep|render|verify|show [flags] <standing doc>
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const usage = `usage: docshistory <units|render|verify|repoint> [flags] <chapter...>
  units    print the unit list and sizes (no tokens)
  render   rewrite the chapters' existing blocks in docs/history/ (a chapter with no block is refused)
  verify   write a sample report to check decisions survived
  repoint  rewrite docs/<chapter>/ references in the rest of the repo
  doc      the same for a standing doc: docshistory doc <units|keep|render|verify|show> [flags] <docs/ARCHITECTURE.md|docs/PACKAGING.md>`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if os.Args[1] == "doc" {
		if err := standingMain(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "docshistory:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "docshistory:", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if err := os.Chdir(strings.TrimSpace(top)); err != nil {
		return err
	}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	runs := fs.String("runs", "docs/history-runs", "runs directory")
	history := fs.String("history", "docs/history", "history directory")
	only := fs.String("only", "", "comma-separated unit slugs")
	sampleN := fs.Int("sample", 12, "verify sample size")
	out := fs.String("out", "-", "verify report path, - for stdout")
	dry := fs.Bool("dry-run", false, "repoint: report without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	chapters := fs.Args()
	if len(chapters) == 0 {
		return fmt.Errorf("name at least one chapter")
	}
	for _, ch := range chapters {
		if err := checkChapter(ch); err != nil {
			return err
		}
	}
	onlySet := map[string]bool{}
	for _, s := range strings.Split(*only, ",") {
		if s = strings.TrimSpace(s); s != "" {
			onlySet[s] = true
		}
	}
	chapters = sortChapters(chapters, false)
	switch cmd {
	case "units":
		return cmdUnits(chapters, onlySet)
	case "render":
		return cmdRender(*runs, *history, chapters)
	case "verify":
		return cmdVerify(*runs, *out, *sampleN, chapters)
	case "repoint":
		return cmdRepoint(*runs, *dry, chapters)
	}
	return fmt.Errorf("unknown command %q\n%s", cmd, usage)
}

func cmdUnits(chapters []string, only map[string]bool) error {
	var nUnits, nBytes int
	for _, ch := range chapters {
		commit, err := recordedCommit(ch)
		if err != nil {
			return err
		}
		p, err := loadChapter(ch, commit)
		if err != nil {
			return err
		}
		n, size := 0, 0
		for _, u := range buildUnits(p) {
			if len(only) > 0 && !only[u.Slug] {
				continue
			}
			fmt.Printf("%s\t%d\n", u.Slug, len([]rune(u.Text)))
			if !u.Notes {
				n++
			}
			size += len(u.Text)
		}
		fmt.Printf("# %s: %d phases, %d bytes, commit %s\n", ch, n, size, commit[:12])
		nUnits += n
		nBytes += size
	}
	fmt.Printf("# total %d phases, %d bytes\n", nUnits, nBytes)
	return nil
}

func recordedCommit(ch string) (string, error) {
	if err := checkClean(ch); err != nil {
		return "", err
	}
	return chapterCommit(ch)
}
