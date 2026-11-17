package releaseline

import (
	"encoding/json"
	"os"
	"testing"
)

const asset = "app.dmg"

// testdata/next_step.json is the one table of stepping cases. scripts/install.sh carries a JXA twin
// of NextStep for old apps; scripts/check-install-step.ts replays this same file through it
// (verify-packaging S16). Add a case here and both implementations are held to it.
func TestNextStep(t *testing.T) {
	data, err := os.ReadFile("testdata/next_step.json")
	if err != nil {
		t.Fatal(err)
	}
	var tests []struct {
		Name     string    `json:"name"`
		Current  string    `json:"current"`
		Releases []Release `json:"releases"`
		Target   string    `json:"target"`
		Newest   string    `json:"newest"`
		Stepped  bool      `json:"stepped"`
	}
	if err := json.Unmarshal(data, &tests); err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.Name, func(t *testing.T) {
			got := NextStep(tc.Current, tc.Releases, asset)
			want := Step{Target: tc.Target, Newest: tc.Newest, Stepped: tc.Stepped}
			if got != want {
				t.Fatalf("NextStep(%q) = %+v, want %+v", tc.Current, got, want)
			}
		})
	}
}
