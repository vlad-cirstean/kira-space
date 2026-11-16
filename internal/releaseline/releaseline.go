// Package releaseline holds the one definition of "which release comes next" shared by the in-app
// updater (internal/appupdate) and the release-asset retention tool (scripts/releaseretention).
// Leaf package: stdlib and x/mod/semver only.
//
// A line is a major.minor pair (v0.3). Lines order by semver. Pre-1.0 needs no special case:
// 0.3.14 is line v0.3, and v0.9 is followed by v1.0.
package releaseline

import (
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// Asset is the subset of a GitHub release asset this package reads.
type Asset struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Size  int64  `json:"size"`
}

// Release is the subset of a GitHub release object this package reads.
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Version returns the canonical form (vX.Y.Z) of tag, and false when tag is not a plain
// release version: no v prefix, shorthand, prerelease or build suffix.
func Version(tag string) (string, bool) {
	if !semver.IsValid(tag) || semver.Canonical(tag) != tag {
		return "", false
	}
	if semver.Prerelease(tag) != "" || semver.Build(tag) != "" {
		return "", false
	}
	return tag, true
}

// HasAsset reports whether r carries a fully uploaded, non-empty asset called name.
func HasAsset(r Release, name string) bool {
	for _, a := range r.Assets {
		if a.Name == name && a.State == "uploaded" && a.Size > 0 {
			return true
		}
	}
	return false
}

// Line is one major.minor line with its versions, ascending.
type Line struct {
	Key      string
	Versions []string
}

// Lines groups the releases that pass eligible (and have a plain version tag) by line. Lines and
// versions within them are ascending and deduplicated.
func Lines(rels []Release, eligible func(Release) bool) []Line {
	byKey := map[string][]string{}
	for _, r := range rels {
		v, ok := Version(r.Tag)
		if !ok || (eligible != nil && !eligible(r)) {
			continue
		}
		k := semver.MajorMinor(v)
		if !slices.Contains(byKey[k], v) {
			byKey[k] = append(byKey[k], v)
		}
	}
	lines := make([]Line, 0, len(byKey))
	for k, vs := range byKey {
		slices.SortFunc(vs, semver.Compare)
		lines = append(lines, Line{Key: k, Versions: vs})
	}
	slices.SortFunc(lines, func(a, b Line) int { return semver.Compare(a.Key, b.Key) })
	return lines
}

// Step is the answer for one running version. Versions carry no v prefix.
type Step struct {
	Target  string // next version to install; "" when none
	Newest  string // highest eligible release; "" when none
	Stepped bool   // Target is not Newest
}

// NextStep picks the update for a running version: the last patch of its own line when that is
// newer, else the lowest release of the next line. Empty lines are passed over. The target is
// always strictly newer than current, so a downgrade is never offered.
func NextStep(current string, rels []Release, asset string) Step {
	cur := "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(cur) {
		return Step{}
	}
	lines := Lines(rels, func(r Release) bool {
		return !r.Draft && !r.Prerelease && HasAsset(r, asset)
	})
	if len(lines) == 0 {
		return Step{}
	}
	lastLine := lines[len(lines)-1]
	newest := lastLine.Versions[len(lastLine.Versions)-1]

	curLine := semver.MajorMinor(cur)
	target := ""
	for _, l := range lines {
		switch c := semver.Compare(l.Key, curLine); {
		case c == 0:
			if top := l.Versions[len(l.Versions)-1]; semver.Compare(top, cur) > 0 {
				target = top
			}
		case c > 0:
			if target == "" {
				target = l.Versions[0]
			}
		}
		if target != "" {
			break
		}
	}
	step := Step{Newest: strings.TrimPrefix(newest, "v")}
	if target != "" {
		step.Target = strings.TrimPrefix(target, "v")
		step.Stepped = target != newest
	}
	return step
}
