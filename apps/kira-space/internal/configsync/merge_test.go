package configsync

import (
	"slices"
	"testing"
)

// TestDecide guards the three-way table, including the deactivate interplay and a local unit of the
// same name that was never synced.
func TestDecide(t *testing.T) {
	has := func(v string) side { return side{val: v, ok: true} }
	none := side{}
	cases := []struct {
		name               string
		ours, base, theirs side
		deactivated        bool
		want               verdict
	}{
		{"untouched", has("a"), has("a"), has("a"), false, verdictNone},
		{"shared changed", has("a"), has("a"), has("b"), false, verdictTake},
		{"shared added", none, none, has("b"), false, verdictTake},
		{"shared removed", has("a"), has("a"), none, false, verdictTake},
		{"local changed only", has("x"), has("a"), has("a"), false, verdictNone},
		{"both changed to same", has("b"), has("a"), has("b"), false, verdictNone},
		{"both changed differently", has("x"), has("a"), has("b"), false, verdictConflict},
		{"local changed, shared removed", has("x"), has("a"), none, false, verdictConflict},
		{"local removed, shared changed", none, has("a"), has("b"), false, verdictConflict},
		{"never synced, same text", has("a"), none, has("a"), false, verdictNone},
		{"never synced, differs", has("x"), none, has("a"), false, verdictConflict},
		{"never synced, local absent", none, none, has("a"), false, verdictTake},
		{"deactivated, applied earlier, untouched", has("a"), has("a"), has("a"), true, verdictTake},
		{"deactivated, applied earlier, edited", has("x"), has("a"), has("a"), true, verdictConflict},
		{"deactivated, never applied", none, none, has("a"), true, verdictNone},
		{"deactivated, never synced local file", has("x"), none, has("a"), true, verdictNone},
	}
	for _, c := range cases {
		if got := decide(c.ours, c.base, effective(c.theirs, c.deactivated)); got != c.want {
			t.Errorf("%s: decide = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestPlanEntries guards the entry unit: each entry decides on its own, deactivated and blocked
// entries leave the local ones alone, and ids with no change are not listed.
func TestPlanEntries(t *testing.T) {
	ours := map[string]string{"env/A": `"1"`, "mcp_servers/x": `{"url":"u"}`, "mcp_servers/mine": `{"url":"m"}`}
	base := map[string]string{"env/A": `"1"`, "mcp_servers/x": `{"url":"u"}`}
	theirs := map[string]string{"env/A": `"2"`, "mcp_servers/x": `{"url":"u"}`, "mcp_servers/y": `{"url":"y"}`, "hooks/h": `{"event":"turn_end"}`}
	got := planEntries(ours, base, theirs, map[string]bool{"hooks/h": true})
	var ids []string
	for _, e := range got {
		ids = append(ids, e.id)
	}
	slices.Sort(ids)
	if want := []string{"env/A", "mcp_servers/y"}; !slices.Equal(ids, want) {
		t.Fatalf("changed ids = %v, want %v", ids, want)
	}
	for _, e := range got {
		if e.verdict != verdictTake {
			t.Errorf("%s verdict = %d, want take", e.id, e.verdict)
		}
	}
}
