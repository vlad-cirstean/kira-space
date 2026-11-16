package releaseline

import "testing"

const asset = "app.dmg"

func rel(tag string) Release {
	return Release{Tag: tag, Assets: []Asset{{Name: asset, State: "uploaded", Size: 10}}}
}

func rels(tags ...string) []Release {
	out := make([]Release, 0, len(tags))
	for _, t := range tags {
		out = append(out, rel(t))
	}
	return out
}

func with(r Release, f func(*Release)) Release { f(&r); return r }

func TestNextStep(t *testing.T) {
	tests := []struct {
		name           string
		current        string
		rels           []Release
		target, newest string
		stepped        bool
	}{
		{"last patch of own line first", "0.3.9", rels("v0.3.9", "v0.3.14", "v0.3.10", "v0.4.0"), "0.3.14", "0.4.0", true},
		{"last patch steps to next .0", "0.3.14", rels("v0.3.14", "v0.4.0", "v0.4.1", "v0.5.0"), "0.4.0", "0.5.0", true},
		{"next line missing .0", "0.2.5", rels("v0.2.5", "v0.3.3", "v0.3.4"), "0.3.3", "0.3.4", true},
		{"next .0 draft", "0.3.14", append(rels("v0.3.14", "v0.4.1"), with(rel("v0.4.0"), func(r *Release) { r.Draft = true })), "0.4.1", "0.4.1", false},
		{"next .0 prerelease", "0.3.14", append(rels("v0.3.14", "v0.4.1"), with(rel("v0.4.0"), func(r *Release) { r.Prerelease = true })), "0.4.1", "0.4.1", false},
		{"next .0 no asset", "0.3.14", append(rels("v0.3.14", "v0.4.1"), Release{Tag: "v0.4.0"}), "0.4.1", "0.4.1", false},
		{"next .0 asset not uploaded", "0.3.14", append(rels("v0.3.14", "v0.4.1"), with(rel("v0.4.0"), func(r *Release) { r.Assets[0].State = "starter" })), "0.4.1", "0.4.1", false},
		{"next .0 asset empty", "0.3.14", append(rels("v0.3.14", "v0.4.1"), with(rel("v0.4.0"), func(r *Release) { r.Assets = []Asset{{Name: asset, State: "uploaded"}} })), "0.4.1", "0.4.1", false},
		{"empty minor between lines", "0.3.14", rels("v0.3.14", "v0.5.0", "v0.5.2"), "0.5.0", "0.5.2", true},
		{"major bump", "0.9.4", rels("v0.9.4", "v1.0.0", "v1.0.1"), "1.0.0", "1.0.1", true},
		{"own line top lacks asset", "0.3.9", append(rels("v0.3.9", "v0.3.12"), Release{Tag: "v0.3.13"}), "0.3.12", "0.3.12", false},
		{"own line nothing newer with asset", "0.3.12", append(rels("v0.3.12", "v0.4.0"), Release{Tag: "v0.3.13"}), "0.4.0", "0.4.0", false},
		{"current unpublished", "0.3.7", rels("v0.3.6", "v0.3.14"), "0.3.14", "0.3.14", false},
		{"current prerelease", "0.4.0-rc.1", rels("v0.3.14", "v0.4.0", "v0.5.0"), "0.4.0", "0.5.0", true},
		{"current newer than all", "0.9.0", rels("v0.3.14", "v0.4.0"), "", "0.4.0", false},
		{"current is newest", "0.4.0", rels("v0.3.14", "v0.4.0"), "", "0.4.0", false},
		{"older line no releases", "0.2.30", rels("v0.3.3", "v0.3.14"), "0.3.3", "0.3.14", true},
		{"suffix, malformed and duplicate tags ignored", "0.5.0",
			append(rels("v0.5.0", "v0.6.0-rc.1", "v0.5.1+meta", "0.5.2", "v0.5", "vx", "v0.5.3", "v0.5.3"), rel("v0.5.3")), "0.5.3", "0.5.3", false},
		{"no releases", "0.3.0", nil, "", "", false},
		{"invalid current", "banana", rels("v0.3.0"), "", "", false},
		{"target is newest", "0.3.1", rels("v0.3.1", "v0.3.2"), "0.3.2", "0.3.2", false},
		{"v prefix on current", "v0.3.9", rels("v0.3.14"), "0.3.14", "0.3.14", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NextStep(tc.current, tc.rels, asset)
			want := Step{Target: tc.target, Newest: tc.newest, Stepped: tc.stepped}
			if got != want {
				t.Fatalf("NextStep(%q) = %+v, want %+v", tc.current, got, want)
			}
		})
	}
}
