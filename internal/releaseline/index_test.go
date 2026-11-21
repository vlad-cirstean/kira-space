package releaseline

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/next_step_index.json")

var (
	zero  = "sha256:" + strings.Repeat("0", 64)
	dmgAs = func(size int64, digest string) []Asset {
		return []Asset{{Name: asset, State: "uploaded", Size: size, Digest: digest}}
	}
)

func TestFormatIndex(t *testing.T) {
	good := func(tag string) Release { return Release{Tag: tag, Assets: dmgAs(10, zero)} }
	draft := good("v0.3.2")
	draft.Draft = true
	pre := good("v0.3.3")
	pre.Prerelease = true
	noAsset := Release{Tag: "v0.3.4"}
	empty := Release{Tag: "v0.3.5", Assets: dmgAs(0, zero)}
	suffix := good("v0.3.6-rc.1")
	noDigest := Release{Tag: "v0.3.7", Assets: dmgAs(10, "")}

	tests := []struct {
		name    string
		rels    []Release
		publish string
		want    string // tags in output order
		wantErr string
	}{
		{name: "filters and sorts", rels: []Release{good("v0.3.10"), draft, pre, noAsset, empty, suffix, good("v0.3.9")}, want: "v0.3.9 v0.3.10"},
		{name: "publish draft included", rels: []Release{good("v0.3.1"), draft}, publish: "v0.3.2", want: "v0.3.1 v0.3.2"},
		{name: "draft without publish excluded", rels: []Release{good("v0.3.1"), draft}, want: "v0.3.1"},
		{name: "publish prerelease draft rejected", rels: []Release{good("v0.3.1"), func() Release { r := draft; r.Prerelease = true; return r }()}, publish: "v0.3.2", wantErr: "not found"},
		{name: "missing digest", rels: []Release{good("v0.3.1"), noDigest}, wantErr: "v0.3.7"},
		{name: "duplicate", rels: []Release{good("v0.3.1"), good("v0.3.1")}, wantErr: "duplicate"},
		{name: "unknown publish", rels: []Release{good("v0.3.1")}, publish: "v0.3.9", wantErr: "not found"},
		{name: "empty", rels: []Release{pre}, wantErr: "no eligible"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatIndex(tc.rels, asset, tc.publish)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var tags []string
			for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n")[1:] {
				tags = append(tags, strings.Fields(l)[0])
			}
			if strings.Join(tags, " ") != tc.want {
				t.Fatalf("tags = %v, want %s", tags, tc.want)
			}
			if !strings.HasPrefix(got, IndexHeader+"\n") || !strings.HasSuffix(got, "\n") {
				t.Fatalf("bad framing: %q", got)
			}
		})
	}
}

func TestParseIndex(t *testing.T) {
	line := "v0.3.3 app.dmg 10 " + zero
	h := IndexHeader + "\n"
	rejects := []struct{ name, in string }{
		{"no header", line + "\n"},
		{"other header", "# kira-release-index 2\n" + line + "\n"},
		{"empty", ""},
		{"header only", h},
		{"three fields", h + "v0.3.3 app.dmg 10\n"},
		{"five fields", h + line + " x\n"},
		{"double space", h + "v0.3.3  app.dmg 10 " + zero + "\n"},
		{"suffix tag", h + "v0.3.3-rc.1 app.dmg 10 " + zero + "\n"},
		{"no v tag", h + "0.3.3 app.dmg 10 " + zero + "\n"},
		{"zero size", h + "v0.3.3 app.dmg 0 " + zero + "\n"},
		{"leading zero size", h + "v0.3.3 app.dmg 010 " + zero + "\n"},
		{"signed size", h + "v0.3.3 app.dmg +10 " + zero + "\n"},
		{"overflow size", h + "v0.3.3 app.dmg 99999999999999999999 " + zero + "\n"},
		{"short digest", h + "v0.3.3 app.dmg 10 sha256:00\n"},
		{"upper digest", h + "v0.3.3 app.dmg 10 sha256:" + strings.Repeat("A", 64) + "\n"},
		{"other algo", h + "v0.3.3 app.dmg 10 sha512:" + strings.Repeat("0", 64) + "\n"},
		{"duplicate", h + line + "\n" + line + "\n"},
		{"blank line", h + line + "\n\n"},
		{"too big", h + strings.Repeat("#", maxIndexBytes) + "\n"},
	}
	for _, tc := range rejects {
		t.Run("reject "+tc.name, func(t *testing.T) {
			if _, err := ParseIndex(strings.NewReader(tc.in)); err == nil {
				t.Fatal("want error")
			}
		})
	}

	t.Run("round trip", func(t *testing.T) {
		rels := []Release{{Tag: "v0.3.10", Assets: dmgAs(12, zero)}, {Tag: "v0.3.9", Assets: dmgAs(11, zero)}}
		text, err := FormatIndex(rels, asset, "")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseIndex(strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Tag != "v0.3.9" || got[1].Assets[0].Size != 12 || !HasAsset(got[1], asset) {
			t.Fatalf("got %+v", got)
		}
	})
}

type indexCase struct {
	Name    string `json:"name"`
	Current string `json:"current"`
	Index   string `json:"index"`
	Target  string `json:"target"`
}

// TestIndexGolden pushes every next_step.json case through FormatIndex and ParseIndex, checks
// NextStep still gives the case target, and pins the text in testdata/next_step_index.json.
// scripts/check-install-step.ts replays that golden through install.sh's awk index_next_step and
// compares the JXA apiToIndex fallback against its index text.
func TestIndexGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/next_step.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name     string    `json:"name"`
		Current  string    `json:"current"`
		Releases []Release `json:"releases"`
		Target   string    `json:"target"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	var golden []indexCase
	for _, tc := range cases {
		// The table repeats a tag on purpose for NextStep; an index never holds one.
		seen := map[string]bool{}
		var rels []Release
		for _, r := range tc.Releases {
			if seen[r.Tag] {
				continue
			}
			seen[r.Tag] = true
			r.Assets = append([]Asset(nil), r.Assets...)
			for i := range r.Assets {
				r.Assets[i].Digest = zero
			}
			rels = append(rels, r)
		}
		text, err := FormatIndex(rels, asset, "")
		if err != nil {
			if tc.Target != "" {
				t.Fatalf("%s: %v", tc.Name, err)
			}
			golden = append(golden, indexCase{Name: tc.Name, Current: tc.Current})
			continue
		}
		parsed, err := ParseIndex(strings.NewReader(text))
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if got := NextStep(tc.Current, parsed, asset).Target; got != tc.Target {
			t.Fatalf("%s: target %q, want %q", tc.Name, got, tc.Target)
		}
		golden = append(golden, indexCase{tc.Name, tc.Current, text, tc.Target})
	}
	out, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	const path = "testdata/next_step_index.json"
	if *update {
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(out) {
		t.Fatalf("%s is stale; run go test ./internal/releaseline -run IndexGolden -update", path)
	}
}
