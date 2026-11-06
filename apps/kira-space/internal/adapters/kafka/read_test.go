package kafka

import "testing"

// TestAdvanceWindows pins the end-of-log clamp and its refinement:
// the arithmetic that decides, from one poll round's own observations, whether a partition's
// remaining [Next, End) gap is provably nothing but non-data offsets. This is the one function in
// the package reachable without a broker, and its history includes two real regressions (hasMore
// stuck true forever, and a partition silently clamped mid-data because a round capped by
// franz-go's own per-partition byte limit looked identical to "the round wasn't page-capped"), so
// it gets a dedicated Go unit test rather than relying on the acceptance suite's single
// transactional-gap scenario alone.
func TestAdvanceWindows(t *testing.T) {
	cases := []struct {
		name    string
		windows []partitionWindow
		touched map[int32]int64
		records map[int32]int
		want    []partitionWindow
	}{
		{
			name:    "every window already drained is left alone",
			windows: []partitionWindow{{Partition: 0, Next: 5, End: 5}},
			touched: map[int32]int64{0: 5},
			records: map[int32]int{},
			want:    []partitionWindow{{Partition: 0, Next: 5, End: 5}},
		},
		{
			name:    "F2: a round that delivered records for this partition proves nothing and is never clamped, even at the watermark",
			windows: []partitionWindow{{Partition: 0, Next: 1, End: 3}},
			touched: map[int32]int64{0: 3},
			records: map[int32]int{0: 2},
			want:    []partitionWindow{{Partition: 0, Next: 1, End: 3}},
		},
		{
			name:    "an untouched partition this round is left alone",
			windows: []partitionWindow{{Partition: 0, Next: 1, End: 3}, {Partition: 1, Next: 0, End: 2}},
			touched: map[int32]int64{0: 3},
			records: map[int32]int{},
			want:    []partitionWindow{{Partition: 0, Next: 3, End: 3}, {Partition: 1, Next: 0, End: 2}},
		},
		{
			name:    "the transaction commit-marker gap: next is one behind end, watermark already reached, zero records delivered this round -- clamp",
			windows: []partitionWindow{{Partition: 0, Next: 1, End: 2}},
			touched: map[int32]int64{0: 2},
			records: map[int32]int{},
			want:    []partitionWindow{{Partition: 0, Next: 2, End: 2}},
		},
		{
			name:    "watermark exactly one below end: genuinely not yet caught up, no clamp",
			windows: []partitionWindow{{Partition: 0, Next: 1, End: 3}},
			touched: map[int32]int64{0: 2},
			records: map[int32]int{},
			want:    []partitionWindow{{Partition: 0, Next: 1, End: 3}},
		},
		{
			name:    "an empty poll with no watermark evidence at all does not clamp (indistinguishable from a slow broker)",
			windows: []partitionWindow{{Partition: 0, Next: 1, End: 3}},
			touched: map[int32]int64{},
			records: map[int32]int{},
			want:    []partitionWindow{{Partition: 0, Next: 1, End: 3}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := append([]partitionWindow(nil), c.windows...)
			advanceWindows(got, c.touched, c.records)
			if len(got) != len(c.want) {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("window %d: got %+v, want %+v", i, got[i], c.want[i])
				}
			}
		})
	}
}
