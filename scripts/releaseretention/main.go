// Command releaseretention plans which release assets to delete (docs/PACKAGING.md §7). It only
// plans: the workflow applies the TSV with gh. Tags, release pages and releases/latest are never
// touched; only assets of releases that are no longer an updater target.
//
//	go run ./scripts/releaseretention -releases releases.json -latest v0.3.14 [-deletes deletes.tsv]
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

func main() {
	releases := flag.String("releases", "", "file with the releases listing (`gh api --paginate --slurp` output or a flat array)")
	latest := flag.String("latest", "", "tag releases/latest returns")
	deletes := flag.String("deletes", "", "write the asset deletions here as TSV: id, tag, name")
	flag.Parse()
	if err := run(*releases, *latest, *deletes); err != nil {
		fmt.Fprintln(os.Stderr, "releaseretention:", err)
		os.Exit(1)
	}
}

func run(releasesPath, latest, deletesPath string) error {
	if releasesPath == "" || latest == "" {
		return fmt.Errorf("-releases and -latest are required")
	}
	data, err := os.ReadFile(releasesPath)
	if err != nil {
		return err
	}
	rels, err := parseReleases(data)
	if err != nil {
		return err
	}
	decisions, dels, err := plan(rels, latest)
	if err != nil {
		return err
	}
	for _, d := range decisions {
		if d.Action == "delete" {
			for _, x := range dels {
				if x.Tag == d.Tag {
					fmt.Printf("delete %s asset %s (id %d, %d MB)\n", x.Tag, x.Name, x.ID, x.Size/1_000_000)
				}
			}
			continue
		}
		fmt.Printf("%s %s: %s\n", d.Action, d.Tag, strings.Join(d.Reasons, "; "))
	}
	fmt.Printf("total: %d releases, %d assets to delete\n", len(decisions), len(dels))
	if deletesPath == "" {
		return nil
	}
	var b strings.Builder
	for _, x := range dels {
		fmt.Fprintf(&b, "%d\t%s\t%s\n", x.ID, x.Tag, x.Name)
	}
	return os.WriteFile(deletesPath, []byte(b.String()), 0o644)
}

// parseReleases accepts an array of pages (gh --slurp) or a flat array of releases, or a mix.
func parseReleases(data []byte) ([]releaseline.Release, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	var out []releaseline.Release
	for _, it := range items {
		if bytes.HasPrefix(bytes.TrimSpace(it), []byte("[")) {
			var page []releaseline.Release
			if err := json.Unmarshal(it, &page); err != nil {
				return nil, fmt.Errorf("decode releases page: %w", err)
			}
			out = append(out, page...)
			continue
		}
		var r releaseline.Release
		if err := json.Unmarshal(it, &r); err != nil {
			return nil, fmt.Errorf("decode release: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}
