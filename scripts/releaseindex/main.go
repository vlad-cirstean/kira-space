// Command releaseindex builds the release index (kira-space-releases.txt) that scripts/install.sh
// and the app read instead of the GitHub REST API (docs/PACKAGING.md §7). Reads the slurped
// releases listing on stdin, writes the index on stdout.
//
//	gh api --paginate --slurp "repos/$GH_REPO/releases?per_page=100" |
//	  go run ./scripts/releaseindex -asset kira-space-macos-arm64.dmg -publish v0.3.22
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

func main() {
	asset := flag.String("asset", "", "asset the updater installs (required)")
	publish := flag.String("publish", "", "tag of the draft being published; listed although still a draft")
	flag.Parse()
	if err := run(*asset, *publish); err != nil {
		fmt.Fprintln(os.Stderr, "releaseindex:", err)
		os.Exit(1)
	}
}

func run(asset, publish string) error {
	if asset == "" {
		return fmt.Errorf("-asset is required")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	rels, err := releaseline.DecodeReleases(data)
	if err != nil {
		return err
	}
	out, err := releaseline.FormatIndex(rels, asset, publish)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(out)
	return err
}
