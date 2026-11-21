// Command releaseretention plans which releases to delete whole (docs/PACKAGING.md §7). It only
// plans: the workflow applies the TSV through releases/{id}. Git tags, drafts, prereleases and
// releases/latest are never touched.
//
//	go run ./scripts/releaseretention -releases releases.json -latest v0.3.15 [-deletes deletes.tsv]
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

func main() {
	releases := flag.String("releases", "", "file with the releases listing (`gh api --paginate --slurp` output or a flat array)")
	latest := flag.String("latest", "", "tag releases/latest returns")
	asset := flag.String("asset", "kira-space-macos-arm64.dmg", "asset the updater installs; releases without it uploaded are not line members")
	deletes := flag.String("deletes", "", "write the release deletions here as TSV: release id, tag")
	flag.Parse()
	if err := run(*releases, *latest, *asset, *deletes); err != nil {
		fmt.Fprintln(os.Stderr, "releaseretention:", err)
		os.Exit(1)
	}
}

func run(releasesPath, latest, asset, deletesPath string) error {
	if releasesPath == "" || latest == "" {
		return fmt.Errorf("-releases and -latest are required")
	}
	data, err := os.ReadFile(releasesPath)
	if err != nil {
		return err
	}
	rels, err := releaseline.DecodeReleases(data)
	if err != nil {
		return err
	}
	decisions, dels, err := plan(rels, latest, asset)
	if err != nil {
		return err
	}
	for _, d := range decisions {
		if d.Action == "delete" {
			for _, x := range dels {
				if x.Tag == d.Tag {
					fmt.Printf("delete %s (release id %d, %d assets, %d MB); tag kept\n", x.Tag, x.ID, x.Assets, x.Size/1_000_000)
				}
			}
			continue
		}
		fmt.Printf("%s %s: %s\n", d.Action, d.Tag, strings.Join(d.Reasons, "; "))
	}
	var assets int
	var size int64
	for _, x := range dels {
		assets += x.Assets
		size += x.Size
	}
	fmt.Printf("total: %d releases, %d to delete (%d assets, %d MB); tags kept\n", len(decisions), len(dels), assets, size/1_000_000)
	if deletesPath == "" {
		return nil
	}
	var b strings.Builder
	for _, x := range dels {
		fmt.Fprintf(&b, "%d\t%s\n", x.ID, x.Tag)
	}
	return os.WriteFile(deletesPath, []byte(b.String()), 0o644)
}
