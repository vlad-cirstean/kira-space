package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kirathecat/kira-studio/internal/releaseline"
)

const testAsset = "a.dmg"

var nextID int64

func rel(tag string) releaseline.Release {
	nextID += 2
	return releaseline.Release{ID: nextID, Tag: tag, Assets: []releaseline.Asset{{ID: nextID - 1, Name: testAsset, State: "uploaded", Size: 5}}}
}

func rels(tags ...string) []releaseline.Release {
	var out []releaseline.Release
	for _, t := range tags {
		out = append(out, rel(t))
	}
	return out
}

func edit(r releaseline.Release, f func(*releaseline.Release)) releaseline.Release {
	f(&r)
	return r
}

// outcome reduces a plan to sorted kept tags and sorted deleted tags.
func outcome(t *testing.T, rs []releaseline.Release, latest string) (kept, deleted []string, err error) {
	t.Helper()
	ds, dels, err := plan(rs, latest, testAsset)
	if err != nil {
		return nil, nil, err
	}
	for _, d := range ds {
		if d.Action == "keep" {
			kept = append(kept, d.Tag)
		}
	}
	for _, x := range dels {
		deleted = append(deleted, x.Tag)
	}
	slices.Sort(kept)
	slices.Sort(deleted)
	return kept, deleted, nil
}

func TestPlan(t *testing.T) {
	live := rels("v0.3.15", "v0.3.14", "v0.3.13", "v0.3.12", "v0.3.11", "v0.3.10", "v0.3.9", "v0.3.6", "v0.3.5", "v0.3.4", "v0.3.3")
	tests := []struct {
		name         string
		rels         []releaseline.Release
		latest       string
		kept, delete []string
		errHas       string
	}{
		{"live shaped list", live, "v0.3.15",
			[]string{"v0.3.11", "v0.3.12", "v0.3.13", "v0.3.14", "v0.3.15", "v0.3.3"},
			[]string{"v0.3.10", "v0.3.4", "v0.3.5", "v0.3.6", "v0.3.9"}, ""},
		{"two past minors keep .0, first and last",
			rels("v0.5.0", "v0.4.3", "v0.4.2", "v0.4.1", "v0.4.0", "v0.3.9", "v0.3.8", "v0.3.7", "v0.3.0", "v0.2.4", "v0.2.3", "v0.2.2", "v0.2.1", "v0.2.0"), "v0.5.0",
			[]string{"v0.2.0", "v0.2.4", "v0.3.0", "v0.3.9", "v0.4.0", "v0.4.3", "v0.5.0"},
			[]string{"v0.2.1", "v0.2.2", "v0.2.3", "v0.3.7", "v0.3.8", "v0.4.1", "v0.4.2"}, ""},
		{"missing .0 keeps lowest with assets",
			rels("v0.5.0", "v0.4.4", "v0.4.3", "v0.4.2", "v0.4.1"), "v0.5.0",
			[]string{"v0.4.1", "v0.4.4", "v0.5.0"}, []string{"v0.4.2", "v0.4.3"}, ""},
		{"release without assets is deleted unless kept, never first or last",
			append(rels("v0.4.0", "v0.3.5", "v0.3.4", "v0.3.3"), releaseline.Release{ID: 9001, Tag: "v0.3.2"}, releaseline.Release{ID: 9002, Tag: "v0.3.6"}), "v0.4.0",
			[]string{"v0.3.3", "v0.3.5", "v0.4.0"}, []string{"v0.3.2", "v0.3.4", "v0.3.6"}, ""},
		{"asset-less .0 still kept",
			append(rels("v0.5.0", "v0.4.3", "v0.4.2", "v0.4.1"), releaseline.Release{ID: 9003, Tag: "v0.4.0"}), "v0.5.0",
			[]string{"v0.4.0", "v0.4.1", "v0.4.3", "v0.5.0"}, []string{"v0.4.2"}, ""},
		{"release without the updater asset uploaded is ignored for first and last",
			append(rels("v0.4.0", "v0.3.5", "v0.3.4", "v0.3.3"),
				edit(rel("v0.3.6"), func(r *releaseline.Release) { r.Assets[0].Name = "sums.txt" }),
				edit(rel("v0.3.2"), func(r *releaseline.Release) { r.Assets[0].State = "new" })), "v0.4.0",
			[]string{"v0.3.3", "v0.3.5", "v0.4.0"}, []string{"v0.3.2", "v0.3.4", "v0.3.6"}, ""},
		{"draft and prerelease are untouched",
			append(rels("v0.4.0", "v0.3.3", "v0.3.2", "v0.3.1"),
				edit(rel("v0.3.9"), func(r *releaseline.Release) { r.Draft = true }),
				edit(rel("v0.4.1-rc.1"), func(r *releaseline.Release) { r.Prerelease = true }),
				rel("nightly")), "v0.4.0",
			[]string{"v0.3.1", "v0.3.3", "v0.4.0"}, []string{"v0.3.2"}, ""},
		{"fewer than 5 in the current minor", rels("v0.4.2", "v0.4.1", "v0.3.7", "v0.3.6", "v0.3.5"), "v0.4.2",
			[]string{"v0.3.5", "v0.3.7", "v0.4.1", "v0.4.2"}, []string{"v0.3.6"}, ""},
		{"latest lags the highest release", rels("v0.4.1", "v0.4.0", "v0.3.9", "v0.3.8"), "v0.3.9",
			[]string{"v0.3.8", "v0.3.9", "v0.4.0", "v0.4.1"}, nil, ""},
		{"latest not a candidate", rels("v0.4.0", "v0.3.9"), "v0.5.0", nil, nil, "not a candidate"},
		{"latest is a prerelease", []releaseline.Release{edit(rel("v0.4.0"), func(r *releaseline.Release) { r.Prerelease = true })}, "v0.4.0", nil, nil, "no candidate"},
		{"empty listing", nil, "v0.4.0", nil, nil, "no candidate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kept, deleted, err := outcome(t, tc.rels, tc.latest)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(kept, tc.kept) || !reflect.DeepEqual(deleted, tc.delete) {
				t.Fatalf("kept %v deleted %v\nwant kept %v deleted %v", kept, deleted, tc.kept, tc.delete)
			}
		})
	}
}

// A rerun after the deletions applied deletes nothing new.
func TestPlanIsIdempotent(t *testing.T) {
	rs := rels("v0.4.0", "v0.3.9", "v0.3.8", "v0.3.7", "v0.3.6", "v0.3.5", "v0.3.4", "v0.3.3")
	kept, _, _ := outcome(t, rs, "v0.4.0")
	_, dels, err := plan(rs, "v0.4.0", testAsset)
	if err != nil || len(dels) == 0 {
		t.Fatalf("first plan: %v, %d deletes", err, len(dels))
	}
	ids := map[int64]bool{}
	for _, r := range rs {
		ids[r.ID] = true
	}
	gone := map[int64]bool{}
	for _, d := range dels {
		if !ids[d.ID] {
			t.Fatalf("delete id %d is not a release id", d.ID)
		}
		gone[d.ID] = true
	}
	rs = slices.DeleteFunc(rs, func(r releaseline.Release) bool { return gone[r.ID] })
	keptAgain, again, err := outcome(t, rs, "v0.4.0")
	if err != nil || len(again) != 0 {
		t.Fatalf("rerun: %v, %d deletes, want none", err, len(again))
	}
	if !reflect.DeepEqual(kept, keptAgain) {
		t.Fatalf("rerun kept %v, want %v", keptAgain, kept)
	}
}
