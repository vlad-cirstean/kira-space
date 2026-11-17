package main

import (
	"errors"
	"fmt"
	"slices"

	"golang.org/x/mod/semver"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

const keepRecent = 5

// Decision is what retention does with one release.
type Decision struct {
	Tag     string
	Action  string   // keep, skip, none, delete
	Reasons []string // why kept or skipped
}

// Delete is one asset to remove.
type Delete struct {
	ID        int64
	Tag, Name string
	Size      int64
}

// plan decides which release assets to delete. Candidates are published, non-prerelease releases
// with a plain vX.Y.Z tag; everything else is skipped and never touched. A candidate keeps every
// asset when it is a minor's .0, the first or last of its line (the next-step targets of
// releaseline), among the 5 highest of the current line, or releases/latest. Any other candidate
// loses all assets. First and last of a line count only releases the updater would pick
// (releaseline.Eligible for asset). Errors leave nothing to delete.
func plan(rels []releaseline.Release, latest, asset string) ([]Decision, []Delete, error) {
	var decisions []Decision
	var cands []releaseline.Release
	seen := map[string]bool{}
	for _, r := range rels {
		switch {
		case seen[r.Tag]:
			decisions = append(decisions, Decision{r.Tag, "skip", []string{"duplicate"}})
			continue
		case r.Draft:
			decisions = append(decisions, Decision{r.Tag, "skip", []string{"draft"}})
		case r.Prerelease:
			decisions = append(decisions, Decision{r.Tag, "skip", []string{"prerelease"}})
		default:
			if _, ok := releaseline.Version(r.Tag); !ok {
				decisions = append(decisions, Decision{r.Tag, "skip", []string{"not a vX.Y.Z tag"}})
			} else {
				cands = append(cands, r)
			}
		}
		seen[r.Tag] = true
	}
	if len(cands) == 0 {
		return nil, nil, errors.New("no candidate releases")
	}
	slices.SortFunc(cands, func(a, b releaseline.Release) int { return semver.Compare(b.Tag, a.Tag) })
	if !slices.ContainsFunc(cands, func(r releaseline.Release) bool { return r.Tag == latest }) {
		return nil, nil, fmt.Errorf("latest release %q is not a candidate", latest)
	}

	reasons := map[string][]string{}
	add := func(tag, why string) { reasons[tag] = append(reasons[tag], why) }

	for _, r := range cands {
		if semver.MajorMinor(r.Tag)+".0" == r.Tag {
			add(r.Tag, "x.y.0")
		}
	}
	for _, l := range releaseline.Lines(cands, func(r releaseline.Release) bool { return releaseline.Eligible(r, asset) }) {
		add(l.Versions[0], "first of minor")
		add(l.Versions[len(l.Versions)-1], "last patch of minor")
	}
	current := semver.MajorMinor(cands[0].Tag)
	n := 0
	for _, r := range cands {
		if semver.MajorMinor(r.Tag) == current && n < keepRecent {
			add(r.Tag, "current minor, last 5")
			n++
		}
	}
	add(latest, "releases/latest")

	var deletes []Delete
	for _, r := range cands {
		switch {
		case len(r.Assets) == 0:
			decisions = append(decisions, Decision{r.Tag, "none", []string{"no assets"}})
		case len(reasons[r.Tag]) > 0:
			decisions = append(decisions, Decision{r.Tag, "keep", reasons[r.Tag]})
		default:
			decisions = append(decisions, Decision{r.Tag, "delete", nil})
			for _, a := range r.Assets {
				deletes = append(deletes, Delete{ID: a.ID, Tag: r.Tag, Name: a.Name, Size: a.Size})
			}
		}
	}
	for _, d := range deletes {
		if d.Tag == latest {
			return nil, nil, fmt.Errorf("plan deletes an asset of the latest release %s", latest)
		}
	}
	return decisions, deletes, nil
}
