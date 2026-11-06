package model

import "testing"

// TestEncodeDecodePathRoundTrip covers the path codec's escaping rules: a segment is
// `<kind>:<encodeURIComponent(name)>` joined by '/', so a name containing '/' or ':' — both of
// which are the codec's own delimiters — must survive, as must non-ASCII text through the
// hand-rolled byte-wise encodeURIComponent/decodeURIComponent pair in uriescape.go.
func TestEncodeDecodePathRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		segments []PathSegment
	}{
		{"name with slash", []PathSegment{{Kind: "table", Name: "order/items"}}},
		{"name with colon", []PathSegment{{Kind: "table", Name: "a:b"}}},
		{"non-ascii name", []PathSegment{{Kind: "table", Name: "pässwörd"}}},
		{"empty", []PathSegment{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodePath(tt.segments)
			decoded, err := DecodePath("conn1", encoded)
			if err != nil {
				t.Fatalf("DecodePath(%q): %v", encoded, err)
			}
			if decoded.ConnectionID != "conn1" {
				t.Errorf("ConnectionID = %q, want conn1", decoded.ConnectionID)
			}
			if len(decoded.Segments) != len(tt.segments) {
				t.Fatalf("got %d segments, want %d", len(decoded.Segments), len(tt.segments))
			}
			for i, s := range tt.segments {
				if decoded.Segments[i] != s {
					t.Errorf("segment[%d] = %+v, want %+v", i, decoded.Segments[i], s)
				}
			}
		})
	}
}
