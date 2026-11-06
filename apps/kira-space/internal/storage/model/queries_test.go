package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// TestSortSpecAcceptsAnEmptyTermList is the accept half of SortSpec's hand-written codec: a
// structured spec with a present-but-empty terms key must survive a round trip, which is what
// makes it distinguishable from the absent-terms case TestSortSpecUnmarshalRejectsInvalid pins.
func TestSortSpecAcceptsAnEmptyTermList(t *testing.T) {
	spec := model.SortSpec{Kind: "structured", Terms: []model.SortTerm{}}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got model.SortSpec
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if diff := cmp.Diff(spec, got); diff != "" {
		t.Errorf("round trip mismatch (-want +got):\n%s", diff)
	}
}

// TestSortSpecUnmarshalRejectsInvalid is the reject half, and the reason the decoder cannot
// unmarshal into the struct: a missing `terms`/`text` key and an empty one both land as a nil Go
// slice, so only probing the raw JSON can tell "structured with no terms key" (invalid) from
// "structured with an empty term list" (valid). A bare null must be rejected too.
func TestSortSpecUnmarshalRejectsInvalid(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"unknown kind", `{"kind":"banana"}`},
		{"structured missing terms", `{"kind":"structured"}`},
		{"text missing text", `{"kind":"text"}`},
		{"text over 4096 chars", `{"kind":"text","text":"` + strings.Repeat("a", 4097) + `"}`},
		{"null", `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got model.SortSpec
			// A JSON null unmarshals into the zero-value probe struct (kind ""), which correctly
			// falls through UnmarshalJSON's "unknown kind" branch — matching the TS build's own
			// discriminated union, which also rejects a bare null.
			if err := json.Unmarshal([]byte(tt.json), &got); err == nil {
				t.Fatalf("Unmarshal(%s) = nil error, want an error", tt.json)
			}
		})
	}
}
