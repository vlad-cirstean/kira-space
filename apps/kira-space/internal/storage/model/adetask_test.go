package model

import (
	"encoding/json"
	"slices"
	"testing"
)

// Guards review rounds stored before comment ids became strings: their spec_json holds numbers.
func TestAdeReviewSpecReadsLegacyNumericCommentIDs(t *testing.T) {
	for in, want := range map[string][]string{
		`{"commentIds":[1,22]}`:           {"1", "22"},
		`{"commentIds":["a","b"]}`:        {"a", "b"},
		`{"commentIds":[]}`:               {},
		`{"commentIds":["0190-aa", "7"]}`: {"0190-aa", "7"},
	} {
		var spec AdeReviewSpec
		if err := json.Unmarshal([]byte(in), &spec); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if !slices.Equal(spec.CommentIDs, want) {
			t.Fatalf("%s: CommentIDs = %v, want %v", in, spec.CommentIDs, want)
		}
	}
}
